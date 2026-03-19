package resourcescontroller

import (
	"net/http"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror-api/internal/models/responses"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Delete a cluster resource of given group/version/kind by uid.
//
//	@Summary	Delete a resource by uid
//	@Schemes
//	@Description	Delete a resource
//	@Tags			resources
//	@Accept			application/json
//	@Produce		application/json
//	@Param			uid	path		string	true	"UID"
//	@Success		200	{object}	rorresources.ResourceUpdateResults
//	@Failure		403	{string}	Forbidden
//	@Failure		401	{object}	rorerror.ErrorData
//	@Failure		500	{string}	Failure	message
//	@Router			/v2/resources/uid/{uid} [delete]
//	@Security		ApiKey || AccessToken
func DeleteResource() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		ctx, span := rortracer.StartSpan(ctx, "v2.resourcescontroller.DeleteResource")
		defer span.End()
		span.SetAttributes(attribute.String("resource.uid", c.Param("uid")))

		resources := resourcesv2service.GetResourceByUID(ctx, c.Param("uid"))

		if resources == nil {
			span.SetStatus(codes.Error, "resource not found")
			c.JSON(http.StatusNotFound, "404: Resource not found")
			return
		}

		// Validate that the correct uid is provided
		if len(resources.Resources) != 1 {
			span.SetStatus(codes.Error, "unexpected number of resources")
			c.JSON(http.StatusNotImplemented, "501: Wrong number of resources found")
			return
		}

		resource := resources.Resources[0]

		if c.Param("uid") != resource.GetUID() {
			span.SetStatus(codes.Error, "uid mismatch")
			c.JSON(http.StatusBadRequest, "400: Wrong resource found")
			return
		}

		// Access check
		// Scope: input.Owner.Scope
		// Subject: input.Owner.Subject
		// Access: update
		accessModel := aclservice.CheckAccessByRorOwnerref(ctx, resource.GetRorMeta().Ownerref)
		if !accessModel.Update {
			span.SetStatus(codes.Error, "access denied")
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		err := resourcesv2service.DeleteResource(ctx, resource)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "delete failed")
			c.JSON(
				http.StatusInternalServerError,
				responses.Cluster{
					Status:  http.StatusInternalServerError,
					Message: "error",
					Data:    map[string]interface{}{"data": err.Error()},
				})
			return
		}

		res := rorresources.ResourceUpdateResults{
			Results: map[string]rorresources.ResourceUpdateResult{
				resource.GetUID(): {
					Status:  http.StatusAccepted,
					Message: "202: Resource deleted",
				},
			},
		}

		span.SetStatus(codes.Ok, "")
		c.JSON(http.StatusOK, res)
	}
}
