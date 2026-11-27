package resourcesv2service

import (
	"context"
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
	"go.opentelemetry.io/otel/attribute"
)

func NewOrUpdateResource(ctx context.Context, resource *rorresources.Resource) rorresources.ResourceUpdateResults {
	ctx, span := rortracer.StartSpan(ctx, "v2.resourcesv2service.NewOrUpdateResource")
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
	accessAllowed, accessErr := aclservice.HasAccess(ctx, ownerref.Scope, ownerref.Subject, aclmodels.CapRor.WithVerb(aclmodels.VerbCreate))
	if accessErr != nil || !accessAllowed {
		_ = rortracer.SpanErrorf(span, "access denied")
		return rorresources.ResourceUpdateResults{
			Results: map[string]rorresources.ResourceUpdateResult{
				resource.GetUID(): {
					Status:  http.StatusForbidden,
					Message: "403: No access",
				},
			},
		}
	}

	// Protected kinds (e.g. Config) require an additional per-kind write capability.
	protectedAllowed, protectedErr := hasProtectedKindWriteAccess(ctx, resource.GetKind(), ownerref)
	if protectedErr != nil || !protectedAllowed {
		_ = rortracer.SpanErrorf(span, "access denied for protected kind %s", resource.GetKind())
		return rorresources.ResourceUpdateResults{
			Results: map[string]rorresources.ResourceUpdateResult{
				resource.GetUID(): {
					Status:  http.StatusForbidden,
					Message: "403: No access",
				},
			},
		}
	}

	// Normalize ownerref before persistence: translate legacy scope/subject
	// to Kind/UID format so old agents don't re-insert legacy values.
	normalizeOwnerref(ctx, resource)

	err := resource.ApplyInputFilter()
	if err != nil {
		_ = rortracer.SpanError(span, err, "could not apply input filter")
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
		rortracer.SpanError(span, err, "failed to set resource")
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
		rortracer.SpanError(span, err, "failed to send message to bus")
	}

	//rlog.Debug("Resource created", rlog.Any("resource", resource.GetAPIVersion()), rlog.Any("kind", resource.GetKind()), rlog.Any("name", resource.GetName()))
	rortracer.SpanOk(span)
	return rorresources.ResourceUpdateResults{
		Results: map[string]rorresources.ResourceUpdateResult{
			resource.GetUID(): {
				Status:  http.StatusAccepted,
				Message: "202: Resource created",
			},
		},
	}
}
