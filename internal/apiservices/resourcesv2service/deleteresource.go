package resourcesv2service

import (
	"context"
	"fmt"

	"github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
	"go.opentelemetry.io/otel/attribute"
)

func DeleteResource(ctx context.Context, resource *rorresources.Resource) error {
	ctx, span := rortracer.StartSpan(ctx, "v2.resourcesv2service.DeleteResource")
	defer span.End()
	span.SetAttributes(attribute.String("resource.uid", resource.GetUID()))

	// Access check
	// Scope: input.Owner.Scope
	// Subject: input.Owner.Subject
	// Access: delete

	accessAllowed, accessErr := aclservice.HasAccess(ctx, resource.GetRorMeta().Ownerref.Scope, resource.GetRorMeta().Ownerref.Subject, aclmodels.CapRor.WithVerb(aclmodels.VerbUpdate))
	if accessErr != nil || !accessAllowed {
		err := fmt.Errorf("403: No access to uid %s", resource.GetUID())
		rortracer.SpanError(span, err, "access denied")
		return err
	}

	// Protected kinds (e.g. Config) require an additional per-kind write capability.
	protectedAllowed, protectedErr := hasProtectedKindWriteAccess(ctx, resource.GetKind(), resource.GetRorMeta().Ownerref)
	if protectedErr != nil || !protectedAllowed {
		err := fmt.Errorf("403: No access to delete protected kind %s uid %s", resource.GetKind(), resource.GetUID())
		rortracer.SpanError(span, err, "access denied for protected kind")
		return err
	}

	//cache := GetResourceCache()
	//cache.Remove(ctx, resource.GetUID())
	databaseHelpers := NewResourceMongoDB(mongodb.GetMongodbConnection())
	err := sendToMessageBus(ctx, resource, rortypes.K8sActionDelete)
	if err != nil {
		rortracer.SpanError(span, err, "unable to send delete action on rabbit queue")
		rlog.Errorc(ctx, "unable to send delete action on rabbit queue", err)
	}
	delErr := databaseHelpers.Del(ctx, resource)
	if delErr != nil {
		rortracer.SpanError(span, delErr, "failed to delete resource")
		return delErr
	}
	rortracer.SpanOk(span)
	return nil
}
