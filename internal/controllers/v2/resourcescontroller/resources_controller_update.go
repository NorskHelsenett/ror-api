package resourcescontroller

import (
	"net/http"

	resourcesservice "github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesService"
	"github.com/NorskHelsenett/ror-api/internal/models/responses"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror/pkg/apicontracts/apiresourcecontracts"
	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Update a cluster resource of given group/version/kind/uid.
//
//	@Summary	Update resource by uid
//	@Schemes
//	@Description	Update a resource
//	@Tags			resources
//	@Accept			application/json
//	@Produce		application/json
//	@Param			uid				path		string									true	"UID"
//	@Param			resourcereport	body		apiresourcecontracts.ResourceUpdateModel	true	"ResourceUpdate"
//	@Success		201				{string}	Created
//	@Failure		403				{string}	Forbidden
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{string}	Failure	message
//	@Router			/v2/resources/uid/{uid} [put]
//	@Security		ApiKey || AccessToken
func UpdateResource() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		ctx, span := rortracer.StartSpan(ctx, "v2.resourcescontroller.UpdateResource")
		defer span.End()
		span.SetAttributes(attribute.String("resource.uid", c.Param("uid")))

		var input apiresourcecontracts.ResourceUpdateModel

		//validate the request body
		if err := c.BindJSON(&input); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to bind JSON")
			c.JSON(http.StatusBadRequest, responses.Cluster{Status: http.StatusBadRequest, Message: "error", Data: map[string]interface{}{"data": err.Error()}})
			return
		}
		//use the validator library to validate required fields
		if validationErr := validate.Struct(&input); validationErr != nil {
			span.RecordError(validationErr)
			span.SetStatus(codes.Error, "validation failed")
			c.JSON(http.StatusBadRequest, responses.Cluster{Status: http.StatusBadRequest, Message: "error", Data: map[string]interface{}{"data": validationErr.Error()}})
			return
		}

		// Validate that the correct uid is provided
		if input.Uid != c.Param("uid") {
			span.SetStatus(codes.Error, "uid mismatch")
			c.JSON(http.StatusNotImplemented, "501: Wrong uid")
			return
		}
		span.AddEvent("request validated")

		scope := aclmodels.Acl2Scope(input.Owner.Scope)
		subject := input.Owner.Subject

		if subject == "" || scope == "" {
			span.SetStatus(codes.Error, "missing owner scope or subject")
			c.JSON(http.StatusBadRequest, responses.Cluster{Status: http.StatusBadRequest, Message: "error", Data: map[string]interface{}{"data": "owner scope and subject must be set"}})
			return
		}
		// Access check
		// Scope: input.Owner.Scope
		// Subject: input.Owner.Subject
		// Access: update
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(scope, subject)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Update {
			span.SetStatus(codes.Error, "access denied")
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}
		span.AddEvent("access checked")

		err := resourcesservice.ResourceNewCreateService(ctx, input)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "service failed")
			c.JSON(http.StatusInternalServerError, responses.Cluster{Status: http.StatusInternalServerError, Message: "error", Data: map[string]interface{}{"data": err.Error()}})
			return
		}

		span.AddEvent("resource updated")
		span.SetStatus(codes.Ok, "")
		c.JSON(http.StatusCreated, nil)
	}
}
