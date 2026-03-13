package resourcesv2service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror-api/internal/apiconnections"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/apiresourcecontracts"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/messagebuscontracts"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/rorresourceowner"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	slowQueryDuration = 500 * time.Millisecond
	getTimeout        = 5000 * time.Millisecond
	setTimeout        = 10000 * time.Millisecond
)

func HandleResourceUpdate(ctx context.Context, resource *rorresources.Resource) rorresources.ResourceUpdateResults {
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "v2.resourcesv2service.HandleResourceUpdate")
	defer span.End()
	span.SetAttributes(
		attribute.String("resource.uid", resource.GetUID()),
		attribute.String("resource.action", string(resource.GetRorMeta().Action)),
	)

	switch resource.GetRorMeta().Action {
	case rortypes.K8sActionAdd:
		return NewOrUpdateResource(ctx, resource)
	case rortypes.K8sActionUpdate:
		return NewOrUpdateResource(ctx, resource)
	case rortypes.K8sActionDelete:
		err := DeleteResource(ctx, resource)
		if err != nil {
			rlog.Error("Could not delete resource", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "could not delete resource")
			return rorresources.ResourceUpdateResults{
				Results: map[string]rorresources.ResourceUpdateResult{
					resource.GetUID(): {
						Status:  http.StatusInternalServerError,
						Message: "500: Could not delete resource",
					},
				},
			}
		}
		return rorresources.ResourceUpdateResults{
			Results: map[string]rorresources.ResourceUpdateResult{
				resource.GetUID(): {
					Status:  http.StatusAccepted,
					Message: "202: Resource deleted",
				},
			},
		}
	default:
		span.SetStatus(codes.Error, "unknown action")
		return rorresources.ResourceUpdateResults{
			Results: map[string]rorresources.ResourceUpdateResult{
				resource.GetUID(): {
					Status:  http.StatusBadRequest,
					Message: "400: Unknown action",
				},
			},
		}
	}
}

func NewOrUpdateResource(ctx context.Context, resource *rorresources.Resource) rorresources.ResourceUpdateResults {
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "v2.resourcesv2service.NewOrUpdateResource")
	defer span.End()
	span.SetAttributes(
		attribute.String("resource.uid", resource.GetUID()),
		attribute.String("resource.apiversion", resource.GetAPIVersion()),
		attribute.String("resource.kind", resource.GetKind()),
	)

	ownerref := resource.GetRorMeta().Ownerref

	// Access check
	// Scope: input.Owner.Scope
	// Subject: input.Owner.Subject
	// Access: create
	accessObject := aclservice.CheckAccessByRorOwnerref(ctx, ownerref)
	if !accessObject.Create {
		span.SetStatus(codes.Error, "access denied")
		return rorresources.ResourceUpdateResults{
			Results: map[string]rorresources.ResourceUpdateResult{
				resource.GetUID(): {
					Status:  http.StatusForbidden,
					Message: "403: No access",
				},
			},
		}
	}

	err := resource.ApplyInputFilter()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not apply input filter")
		return rorresources.ResourceUpdateResults{
			Results: map[string]rorresources.ResourceUpdateResult{
				resource.GetUID(): {
					Status:  http.StatusBadRequest,
					Message: "400: Could not apply filter to resource",
				},
			},
		}
	}
	//cache := GetResourceCache()
	//cache.Set(ctx, resource)

	mongoCtx, cancel := context.WithTimeout(ctx, setTimeout)
	defer cancel()

	databaseHelpers := NewResourceMongoDB(mongodb.GetMongodbConnection())
	err = databaseHelpers.Set(mongoCtx, resource)
	if err != nil {
		rlog.Errorc(ctx, "Failed to set resource", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to set resource")
		return rorresources.ResourceUpdateResults{
			Results: map[string]rorresources.ResourceUpdateResult{
				resource.GetUID(): {
					Status:  http.StatusInternalServerError,
					Message: "500: Could not create resource",
				},
			},
		}
	}

	if err := sendToMessageBus(ctx, resource, resource.RorMeta.Action); err != nil {
		rlog.Errorc(ctx, "Failed to send message to bus", err)
		span.RecordError(err)
	}

	//rlog.Debug("Resource created", rlog.Any("resource", resource.GetAPIVersion()), rlog.Any("kind", resource.GetKind()), rlog.Any("name", resource.GetName()))
	span.SetStatus(codes.Ok, "")
	return rorresources.ResourceUpdateResults{
		Results: map[string]rorresources.ResourceUpdateResult{
			resource.GetUID(): {
				Status:  http.StatusAccepted,
				Message: "202: Resource created",
			},
		},
	}
}

func GetResourceByUID(ctx context.Context, uid string) *rorresources.ResourceSet {
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "v2.resourcesv2service.GetResourceByUID")
	defer span.End()
	span.SetAttributes(attribute.String("resource.uid", uid))

	var returnrs *rorresources.ResourceSet
	//cache := GetResourceCache()
	//resource := cache.Get(ctx, uid)
	// if resource != nil {
	// 	returnrs = rorresources.NewResourceSet()
	// 	returnrs.Resources = append(returnrs.Resources, resource)
	// 	rlog.Debug("Resource found in cache", rlog.String("uid", uid), rlog.Any("duration", time.Since(start)))
	// } else {
	databaseHelpers := NewResourceMongoDB(mongodb.GetMongodbConnection())
	mongoCtx, cancel := context.WithTimeout(ctx, getTimeout)
	defer cancel()
	var err error
	query := rorresources.NewResourceQuery().WithUID(uid)
	queryStart := time.Now()
	returnrs, err = databaseHelpers.Get(mongoCtx, query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not get resource by uid")
		rlog.Error("Could not get resource by uid", err, rlog.String("uid", uid), rlog.Any("error", err))
		return nil
	}
	if returnrs == nil {
		return nil
	}
	duration := time.Since(queryStart)
	if duration > slowQueryDuration {
		rlog.Warn("Slow query detected in GetResourceByUID", rlog.String("uid", uid), rlog.Any("duration", duration))
	}
	//cache.Set(ctx, returnrs.Resources[0])

	// }

	// Access check
	// Scope: input.Owner.Scope
	// Subject: input.Owner.Subject
	// Access: read
	for _, resource := range returnrs.Resources {
		accessModel := aclservice.CheckAccessByRorOwnerref(ctx, resource.GetRorMeta().Ownerref)
		if !accessModel.Read {
			span.SetStatus(codes.Error, "access denied")
			return nil
		}
	}

	span.SetStatus(codes.Ok, "")
	span.SetAttributes(attribute.Int("resources.count", len(returnrs.Resources)))
	return returnrs
}

func DeleteResource(ctx context.Context, resource *rorresources.Resource) error {
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "v2.resourcesv2service.DeleteResource")
	defer span.End()
	span.SetAttributes(attribute.String("resource.uid", resource.GetUID()))

	// Access check
	// Scope: input.Owner.Scope
	// Subject: input.Owner.Subject
	// Access: delete

	accessModel := aclservice.CheckAccessByRorOwnerref(ctx, resource.GetRorMeta().Ownerref)
	if !accessModel.Update {
		err := fmt.Errorf("403: No access to uid %s", resource.GetUID())
		span.RecordError(err)
		span.SetStatus(codes.Error, "access denied")
		return err
	}

	//cache := GetResourceCache()
	//cache.Remove(ctx, resource.GetUID())
	databaseHelpers := NewResourceMongoDB(mongodb.GetMongodbConnection())
	err := sendToMessageBus(ctx, resource, rortypes.K8sActionDelete)
	if err != nil {
		span.RecordError(err)
		rlog.Errorc(ctx, "unable to send delete action on rabbit queue", err)
	}
	delErr := databaseHelpers.Del(ctx, resource)
	if delErr != nil {
		span.RecordError(delErr)
		span.SetStatus(codes.Error, "failed to delete resource")
		return delErr
	}
	span.SetStatus(codes.Ok, "")
	return nil
}

func GetResourceByQuery(ctx context.Context, query *rorresources.ResourceQuery) (*rorresources.ResourceSet, error) {
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "v2.resourcesv2service.GetResourceByQuery")
	defer span.End()

	databaseHelpers := NewResourceMongoDB(mongodb.GetMongodbConnection())
	mongoCtx, cancel := context.WithTimeout(ctx, getTimeout)
	defer cancel()
	queryStart := time.Now()
	rs, err := databaseHelpers.Get(mongoCtx, query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not get resource by query")
		rlog.Error("Could not get resource by query", err, rlog.Any("error", err))
		return nil, fmt.Errorf("could not get resource by query: %w", err)
	}
	if elapsed := time.Since(queryStart); elapsed > slowQueryDuration {
		rlog.Warn("Slow query detected in GetResourceByQuery", rlog.Any("duration", elapsed))
	}
	if rs == nil {
		return nil, nil
	}

	// Access check
	// Scope: input.Owner.Scope
	// Subject: input.Owner.Subject
	// Access: read

	returnrs := rorresources.NewResourceSet()
	var checkedOwnerRef = make(map[string]int, 0)
	for _, resource := range rs.Resources {
		if checked, ok := checkedOwnerRef[resource.GetRorMeta().Ownerref.String()]; ok {
			if checked == 1 {
				returnrs.Add(resource)
			}
			continue
		}
		accessModel := aclservice.CheckAccessByRorOwnerref(ctx, resource.GetRorMeta().Ownerref)
		if accessModel.Read {
			checkedOwnerRef[resource.GetRorMeta().Ownerref.String()] = 1
			returnrs.Add(resource)
			continue
		} else {
			checkedOwnerRef[resource.GetRorMeta().Ownerref.String()] = -1
		}
	}
	span.SetStatus(codes.Ok, "")
	span.SetAttributes(attribute.Int("resources.count", len(returnrs.Resources)))
	return returnrs, nil
}

func sendToMessageBus(ctx context.Context, resource *rorresources.Resource, action rortypes.ResourceAction) error {
	b, err := json.Marshal(resource)
	if err != nil {
		return errors.New("could not cast resource to byte[]")
	}

	payload := apiresourcecontracts.ResourceModel[apiresourcecontracts.ResourceNamespace]{}
	payload.ApiVersion = resource.GetAPIVersion()
	payload.Kind = resource.GetKind()
	payload.Uid = resource.GetUID()
	payload.Hash = resource.GetRorHash()
	payload.Internal = resource.GetRorMeta().Internal
	payload.Owner.Scope = resource.GetRorMeta().Ownerref.Scope
	payload.Owner.Subject = string(resource.GetRorMeta().Ownerref.Subject)
	payload.Version = apiresourcecontracts.ResourceVersionV2
	err = json.Unmarshal(b, &payload)
	if err != nil {
		rlog.Error("Could not convert to json", err)
		return errors.New("could not cast resource to ResourceNamespace")
	}

	switch action {
	case rortypes.K8sActionAdd:
		_ = apiconnections.RabbitMQConnection.SendMessage(ctx,
			payload,
			messagebuscontracts.Route_ResourceCreated,
			map[string]interface{}{"apiVersion": payload.ApiVersion, "kind": payload.Kind})
	case rortypes.K8sActionUpdate:
		_ = apiconnections.RabbitMQConnection.SendMessage(ctx,
			payload,
			messagebuscontracts.Route_ResourceUpdated,
			map[string]interface{}{"apiVersion": payload.ApiVersion, "kind": payload.Kind})
	case rortypes.K8sActionDelete:
		_ = apiconnections.RabbitMQConnection.SendMessage(ctx,
			payload,
			messagebuscontracts.Route_ResourceDeleted,
			map[string]interface{}{"apiVersion": payload.ApiVersion, "kind": payload.Kind})
	}
	return nil
}

func ResourceGetHashlist(ctx context.Context, owner rorresourceowner.RorResourceOwnerReference) (apiresourcecontracts.HashList, error) {
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "v2.resourcesv2service.ResourceGetHashlist")
	defer span.End()

	query := rorresources.ResourceQuery{
		OwnerRefs: []rorresourceowner.RorResourceOwnerReference{owner},
		Limit:     -1,
	}
	mongoCtx, cancel := context.WithTimeout(ctx, getTimeout)
	defer cancel()
	databaseHelpers := NewResourceMongoDB(mongodb.GetMongodbConnection())
	result, err := databaseHelpers.GetHashlistByQuery(mongoCtx, &query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get hashlist")
		return result, err
	}
	span.SetStatus(codes.Ok, "")
	return result, nil
}
