// resourcecontroller implements all controllers for resources
package resourcescontroller

import (
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror-api/internal/customvalidators"
	"github.com/NorskHelsenett/ror-api/internal/models/responses"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/rorresourceowner"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.opentelemetry.io/otel/attribute"
)

var (
	validate *validator.Validate
)

// Init is called to initialize the resources controller
func init() {
	rlog.Debug("init resources controller")
	validate = validator.New()
	customvalidators.Setup(validate)
}

// Check if a cluster resource of given uid exists.
//
//	@Summary	Check cluster resource by uid
//	@Schemes
//	@Description	Get a list of cluster resources
//	@Tags			resources
//	@Accept			application/json
//	@Produce		application/json
//	@Param			ownerScope		query	aclmodels.Acl2Scope	true	"The kind of the owner, currently only support 'Cluster'"
//	@Param			ownerSubject	query	string				true	"The name og the owner"
//	@Param			uid				path	string				true	"UID"
//	@Success		204
//	@Failure		404	{string}	NotFound
//	@Failure		401	{object}	rorerror.ErrorData
//	@Failure		500	{string}	Failure	message
//	@Router			/v2/resources/uid/{uid} [head]
//	@Security		ApiKey || AccessToken
func ExistsResources() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		ctx, span := rortracer.StartSpan(ctx, "v2.resourcescontroller.ExistsResources")
		defer span.End()
		span.SetAttributes(attribute.String("resource.uid", c.Param("uid")))

		if c.Param("uid") == "" {
			rortracer.SpanErrorf(span, "missing uid")
			c.JSON(http.StatusBadRequest, "empty uid")
			return
		}

		resources := resourcesv2service.GetResourceByUID(ctx, c.Param("uid"))
		if resources == nil {
			rortracer.SpanErrorf(span, "resource not found")
			c.Status(http.StatusNotFound)
			return
		}

		// Validate that the correct uid is provided
		if len(resources.Resources) != 1 {
			rortracer.SpanErrorf(span, "unexpected number of resources")
			c.JSON(http.StatusNotImplemented, "501: Wrong number of resources found")
			return
		}

		resource := resources.Resources[0]

		if c.Param("uid") != resource.GetUID() {
			rortracer.SpanErrorf(span, "uid mismatch")
			c.JSON(http.StatusBadRequest, "400: Wrong resource found")
			return
		}

		// Access check
		// Scope: input.Owner.Scope
		// Subject: input.Owner.Subject
		// Access: Read
		accessModel := aclservice.CheckAccessByRorOwnerref(ctx, resource.GetRorMeta().Ownerref)
		if !accessModel.Read {
			rortracer.SpanErrorf(span, "access denied")
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		if resources.Len() == 1 {
			rortracer.SpanOk(span)
			c.Status(http.StatusNoContent)
			return
		} else {
			rortracer.SpanErrorf(span, "resource not found")
			c.Status(http.StatusNotFound)
			return
		}
	}
}

// Get a list of hashes of saved resources for given cluster.
// Parameter clusterid must match authorized clusterid
//
//	@Summary	Get resource hash list
//	@Schemes
//	@Description	Get a resource list
//	@Tags			resources
//	@Accept			application/json
//	@Produce		application/json
//	@Param			ownerScope		query		aclmodels.Acl2Scope	true	"The kind of the owner, currently only support 'Cluster'"
//	@Param			ownerSubject	query		string				true	"The name og the owner"
//	@Success		200				{array}		apiresourcecontracts.HashList
//	@Failure		403				{string}	Forbidden
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{string}	Failure	message
//	@Router			/v2/resources/hashes [get]
//	@Security		ApiKey || AccessToken
func GetResourceHashList() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		ctx, span := rortracer.StartSpan(ctx, "v2.resourcescontroller.GetResourceHashList")
		defer span.End()
		span.SetAttributes(
			attribute.String("owner.scope", c.Query("ownerScope")),
			attribute.String("owner.subject", c.Query("ownerSubject")),
		)

		resourceOwner := rorresourceowner.RorResourceOwnerReference{
			Scope:   aclmodels.Acl2Scope(c.Query("ownerScope")),
			Subject: aclmodels.Acl2Subject(c.Query("ownerSubject")),
		}

		// Access check
		// Scope: c.Query("ownerScope")
		// Subject: c.Query("ownerSubject")
		// Access: update
		accessObject := aclservice.CheckAccessByRorOwnerref(ctx, resourceOwner)
		if !accessObject.Update {
			rortracer.SpanErrorf(span, "access denied")
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		hashList, err := resourcesv2service.ResourceGetHashlist(ctx, resourceOwner)
		if err != nil {
			rortracer.SpanError(span, err, "failed to get hash list")
			rlog.Error("Error getting resource hash list:", err)
			c.JSON(http.StatusInternalServerError, responses.Cluster{Status: http.StatusInternalServerError, Message: "error", Data: map[string]interface{}{"data": err.Error()}})
			return
		}

		rortracer.SpanOk(span)
		c.JSON(http.StatusOK, hashList)
	}
}
