// resourcecontroller implements all controllers for resources
package resourcescontroller

import (
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror-api/internal/customvalidators"
	"github.com/NorskHelsenett/ror-api/internal/models/responses"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/rorresourceowner"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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

		ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "v2.resourcescontroller.ExistsResources")
		defer span.End()
		span.SetAttributes(attribute.String("resource.uid", c.Param("uid")))

		if c.Param("uid") == "" {
			span.SetStatus(codes.Error, "missing uid")
			c.JSON(http.StatusBadRequest, "empty uid")
			return
		}

		resources := resourcesv2service.GetResourceByUID(ctx, c.Param("uid"))
		if resources == nil {
			span.SetStatus(codes.Error, "resource not found")
			c.Status(http.StatusNotFound)
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
		// Access: Read
		accessModel := aclservice.CheckAccessByRorOwnerref(ctx, resource.GetRorMeta().Ownerref)
		if !accessModel.Read {
			span.SetStatus(codes.Error, "access denied")
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		if resources.Len() == 1 {
			span.SetStatus(codes.Ok, "")
			c.Status(http.StatusNoContent)
			return
		} else {
			span.SetStatus(codes.Error, "resource not found")
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

		ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "v2.resourcescontroller.GetResourceHashList")
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
			span.SetStatus(codes.Error, "access denied")
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		hashList, err := resourcesv2service.ResourceGetHashlist(ctx, resourceOwner)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to get hash list")
			rlog.Error("Error getting resource hash list:", err)
			c.JSON(http.StatusInternalServerError, responses.Cluster{Status: http.StatusInternalServerError, Message: "error", Data: map[string]interface{}{"data": err.Error()}})
			return
		}

		span.SetStatus(codes.Ok, "")
		c.JSON(http.StatusOK, hashList)
	}
}
