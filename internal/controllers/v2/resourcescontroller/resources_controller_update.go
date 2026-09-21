package resourcescontroller

import (
	"errors"
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror-api/internal/helpers/responsehelper"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

// Update a cluster resource of given group/version/kind/uid.
//
//	@Summary	Update resource by uid
//	@Schemes
//	@Description	Update a resource
//	@Tags			resources
//	@Accept			application/json
//	@Produce		application/json
//	@Param			uid			path		string					true	"UID"
//	@Param			rorresource	body		rorresources.Resource	true	"Resource"
//	@Success		200			{object}	rorresources.ResourceUpdateResults
//	@Failure		403			{string}	Forbidden
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		500			{string}	Failure	message
//	@Router			/v2/resources/uid/{uid} [put]
//	@Security		ApiKey || AccessToken
func UpdateResource() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		ctx, span := rortracer.StartSpan(ctx, "v2.resourcescontroller.UpdateResource")
		defer span.End()

		uid := c.Param("uid")
		span.SetAttributes(attribute.String("resource.uid", uid))
		if uid == "" {
			rortracer.SpanErrorf(span, "missing uid")
			responsehelper.ErrorResponse(c, http.StatusBadRequest, errors.New("uid is required"))
			return
		}

		var input rorresources.Resource
		if err := c.BindJSON(&input); err != nil {
			_ = rortracer.SpanError(span, err, "failed to bind JSON")
			rlog.Error("error binding json", err)
			responsehelper.ErrorResponse(c, http.StatusBadRequest, err)
			return
		}
		if validationErr := validate.Struct(&input); validationErr != nil {
			_ = rortracer.SpanError(span, validationErr, "validation failed")
			rlog.Error("validation failed", validationErr)
			responsehelper.ErrorResponse(c, http.StatusBadRequest, validationErr)
			return
		}
		span.AddEvent("request validated")

		// Restore typed resource and common methods lost in transit (json).
		resource, err := rorresources.NewResourceFromStruct(input)
		switch {
		case err == nil:
			// No error, continue processing
		case errors.Is(err, rorresources.ErrUnknownResourceKind):
			_ = rortracer.SpanError(span, err, "unknown resource kind")
			responsehelper.ErrorResponse(c, http.StatusBadRequest, err)
			return
		default:
			_ = rortracer.SpanError(span, err, "failed to create resource from struct")
			responsehelper.ErrorResponse(c, http.StatusInternalServerError, err)
			return
		}

		// PUT targets the single resource identified by the path uid.
		if resource.GetUID() != uid {
			rortracer.SpanErrorf(span, "uid mismatch")
			responsehelper.ErrorResponse(c, http.StatusBadRequest, errors.New("resource uid must match path uid"))
			return
		}

		result := resourcesv2service.NewOrUpdateResource(ctx, resource)

		status := http.StatusOK
		if r, ok := result.Results[uid]; ok {
			status = r.Status
		}

		span.AddEvent("resource updated")
		rortracer.SpanOk(span)
		c.JSON(status, result)
	}
}
