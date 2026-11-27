// Package resourcesv2service implements the v2 resource service, which is responsible for managing resources in the ROR system.
package resourcesv2service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror-api/internal/apiconnections"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/apiresourcecontracts"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"
	"github.com/NorskHelsenett/ror/pkg/messagebuscontracts"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/rorresourceowner"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rordefs"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"go.opentelemetry.io/otel/attribute"
)

var (
	slowQueryDuration                    = 1000 * time.Millisecond
	getTimeout                           = 10000 * time.Millisecond
	setTimeout                           = 10000 * time.Millisecond
	newResourceDB      resourceDBFactory = NewResourceMongoDB
	getMongoConnection                   = mongodb.GetMongodbConnection
)

type resourceDBFactory func(*mongodb.MongodbCon) ResourceDBProvider

// hasProtectedKindWriteAccess enforces the per-kind protected capability for
// write operations (create/update/delete). Kinds without a ProtectedBy
// capability are unrestricted here; protected kinds additionally require the
// caller to hold the capability's write verb, independent of ownerref access.
// Every identity resolves through the same grant check — a cluster is denied
// because its ACL grant lacks the write capability, not because of its type.
func hasProtectedKindWriteAccess(ctx context.Context, kind string, ownerref rorresourceowner.RorResourceOwnerReference) (bool, error) {
	capability := rordefs.Resourcedefs.ProtectedByKind(kind)
	if capability == "" {
		return true, nil
	}
	return aclservice.HasAccess(ctx, ownerref.Scope, ownerref.Subject, capability.WithVerb(aclmodels.VerbWrite))
}

func HandleResourceUpdate(ctx context.Context, resource *rorresources.Resource) rorresources.ResourceUpdateResults {
	ctx, span := rortracer.StartSpan(ctx, "v2.resourcesv2service.HandleResourceUpdate")
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
			_ = rortracer.SpanError(span, err, "could not delete resource")
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
		_ = rortracer.SpanErrorf(span, "unknown action")
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

func GetResourceByUID(ctx context.Context, uid string) (*rorresources.ResourceSet, error) {
	ctx, span := rortracer.StartSpan(ctx, "v2.resourcesv2service.GetResourceByUID")
	defer span.End()
	span.SetAttributes(attribute.String("resource.uid", uid))

	query := rorresources.NewResourceQuery().WithUID(uid)
	return GetResourceByQuery(ctx, query)
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
			map[string]any{"apiVersion": payload.ApiVersion, "kind": payload.Kind})
	case rortypes.K8sActionUpdate:
		_ = apiconnections.RabbitMQConnection.SendMessage(ctx,
			payload,
			messagebuscontracts.Route_ResourceUpdated,
			map[string]any{"apiVersion": payload.ApiVersion, "kind": payload.Kind})
	case rortypes.K8sActionDelete:
		_ = apiconnections.RabbitMQConnection.SendMessage(ctx,
			payload,
			messagebuscontracts.Route_ResourceDeleted,
			map[string]any{"apiVersion": payload.ApiVersion, "kind": payload.Kind})
	}
	return nil
}

func ResourceGetHashlist(ctx context.Context, owner rorresourceowner.RorResourceOwnerReference) (apiresourcecontracts.HashList, error) {
	ctx, span := rortracer.StartSpan(ctx, "v2.resourcesv2service.ResourceGetHashlist")
	defer span.End()

	// Normalize the ownerref: translate legacy scope and clusterid→UID
	owner.Scope = owner.Scope.ToKind()
	if owner.Scope == aclscope.ScopeCluster.ToKind() {
		owner.Subject = clusterSubjectToUID(rorcontext.MustGetIdentityFromRorContext(ctx), owner.Subject)
	}

	query := rorresources.ResourceQuery{
		OwnerRefs: []rorresourceowner.RorResourceOwnerReference{owner},
		Limit:     -1,
	}
	mongoCtx, cancel := context.WithTimeout(ctx, getTimeout)
	defer cancel()
	databaseHelpers := NewResourceMongoDB(mongodb.GetMongodbConnection())
	result, err := databaseHelpers.GetHashlistByQuery(mongoCtx, &query)
	if err != nil {
		rortracer.SpanError(span, err, "failed to get hashlist")
		return result, err
	}
	rortracer.SpanOk(span)
	return result, nil
}

// normalizeOwnerref translates legacy ownerref values to the Kind/UID format
// before persisting. This ensures old agents (sending scope="cluster" and
// subject=clusterid) don't re-insert legacy values into the migrated database.
// TODO: Tracing should be added to check for legacy ownerrefs so we can correct any agents still sending them.
func normalizeOwnerref(ctx context.Context, resource *rorresources.Resource) {
	ownerref := &resource.RorMeta.Ownerref

	// Translate legacy scope (e.g. "cluster" → "KubernetesCluster")
	ownerref.Scope = ownerref.Scope.ToKind()

	// For cluster-scoped resources, translate clusterid subject to UID
	if ownerref.Scope == aclscope.ScopeCluster.ToKind() {
		ownerref.Subject = clusterSubjectToUID(rorcontext.MustGetIdentityFromRorContext(ctx), ownerref.Subject)
	}
}

// clusterSubjectToUID rewrites a caller's own cluster id to its uid, the form
// ownerrefs are stored in. Any other subject is returned unchanged.
func clusterSubjectToUID(identity identitymodels.Identity, subject aclscope.Subject) aclscope.Subject {
	if !identity.IsCluster() {
		return subject
	}
	clusterID, err := identity.GetName()
	if err != nil {
		return subject
	}
	uid, err := identity.GetSubject()
	if err != nil || string(subject) != clusterID {
		return subject
	}
	return aclscope.Subject(uid)
}
