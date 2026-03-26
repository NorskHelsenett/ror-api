package resourcescontroller

import (
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror-api/internal/models/responses"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

// Patch a resource by uid. Only the fields present in the request body are
// updated; all other fields in the stored document are preserved.
//
//	@Summary	Patch resource by uid
//	@Schemes
//	@Description	Partially update a resource
//	@Tags			resources
//	@Accept			application/json
//	@Produce		application/json
//	@Param			uid		path		string	true	"UID"
//	@Param			patch	body		object	true	"Partial resource fields to update"
//	@Success		200		{object}	rorresources.ResourceUpdateResults
//	@Failure		400		{object}	responses.Cluster
//	@Failure		403		{string}	Forbidden
//	@Failure		401		{object}	rorerror.ErrorData
//	@Failure		404		{string}	Not	Found
//	@Failure		500		{string}	Failure	message
//	@Router			/v2/resources/uid/{uid} [patch]
//	@Security		ApiKey || AccessToken
func PatchResource() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		ctx, span := rortracer.StartSpan(ctx, "v2.resourcescontroller.PatchResource")
		defer span.End()

		uid := c.Param("uid")
		span.SetAttributes(attribute.String("resource.uid", uid))

		if uid == "" {
			rortracer.SpanErrorf(span, "missing uid")
			c.JSON(http.StatusBadRequest, responses.Cluster{
				Status:  http.StatusBadRequest,
				Message: "error",
				Data:    map[string]interface{}{"data": "uid is required"},
			})
			return
		}

		var partial rorresources.Resource
		if err := c.BindJSON(&partial); err != nil {
			rortracer.SpanError(span, err, "failed to bind JSON")
			c.JSON(http.StatusBadRequest, responses.Cluster{
				Status:  http.StatusBadRequest,
				Message: "error",
				Data:    map[string]interface{}{"data": err.Error()},
			})
			return
		}

		span.AddEvent("request validated")

		result := resourcesv2service.PatchResource(ctx, uid, &partial)

		status := http.StatusOK
		for _, r := range result.Results {
			status = r.Status
			break
		}

		rortracer.SpanOk(span)
		c.JSON(status, result)
	}
}
