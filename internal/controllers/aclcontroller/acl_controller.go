// The acl controller package provides controller functions for the /acl endpoints in the api V1.
package aclcontroller

import (
	"net/http"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/internal/customvalidators"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/context/gincontext"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	"github.com/NorskHelsenett/ror/pkg/messagebuscontracts"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

func init() {
	rlog.Debug("init cluster controller")
	validate = validator.New()
	customvalidators.Setup(validate)
}

// GetScopes provides a array of aclmodels.Acl2Scope
//
//	@Summary	Get acl scopes
//	@Schemes
//	@Description	Get acl scopes
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200				{array}		aclmodels.Acl2Scope
//	@Failure		401				{object}	rorerror.ErrorData
//	@Router			/v1/acl/scopes	[get]
//	@Security		ApiKey || AccessToken
func GetScopes() gin.HandlerFunc {
	return func(c *gin.Context) {
		results := aclmodels.GetScopes()
		c.JSON(http.StatusOK, results)
	}
}

// TODO: Describe
//
//	@Summary	Check acl
//	@Schemes
//	@Description	Check acl by scope, subject and access method
//	@Tags			acl
//	@Success		200
//	@Failure		403
//	@Failure		400
//	@Failure		401
//	@Param			scope								path		string	false	"Scope"
//	@Param			subject								path		string	false	"Subject"
//	@Param			access								path		string	false	"read,write,update or delete"
//	@Router			/v1/acl/{scope}/{subject}/{access}	[head]
//	@Security		ApiKey || AccessToken
func CheckAcl() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		scope := c.Param("scope")
		if scope == "" || len(scope) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid scope")
			rerr.GinLogErrorAbort(c)
			return
		}

		subject := c.Param("subject")
		if subject == "" || len(subject) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid subject")
			rerr.GinLogErrorAbort(c)
			return
		}

		access := c.Param("access")
		if access == "" || len(access) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		// Check access
		// Scope: c.Param("scope")
		// Subject: c.Param("subject")
		// Access: c.Param("access")
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(scope, subject)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		switch access {
		case "read":
			if accessObject.Read {
				c.Status(http.StatusOK)
				return
			}
		case "create":
			if accessObject.Create {
				c.Status(http.StatusOK)
				return
			}
		case "update":
			if accessObject.Update {
				c.Status(http.StatusOK)
				return
			}
		case "delete":
			if accessObject.Delete {
				c.Status(http.StatusOK)
				return
			}
		case "owner":
			if accessObject.Owner {
				c.Status(http.StatusOK)
				return
			}
		default:
			c.Status(http.StatusForbidden)
			return
		}

		c.Status(http.StatusForbidden)
	}
}

// TODO: Describe
//
//	@Summary	Get acl by id
//	@Schemes
//	@Description	Get acl by id
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200				{object}	apicontracts.PaginatedResult[aclmodels.AclV2ListItem]
//	@Failure		403				{object}	rorerror.ErrorData
//	@Failure		400				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{object}	rorerror.ErrorData
//	@Router			/v1/acl/{aclId}	[get]
//	@Param			id				path	string	true	"id"
//	@Security		ApiKey || AccessToken
func GetById() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Check access
		// Scope: Ror
		// Subject: Acl
		// Access: Read
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectAcl))
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Read {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		aclId := c.Param("id")
		if aclId == "" || len(aclId) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		var _ aclmodels.AclV2ListItem
		object, err := aclservice.GetById(ctx, aclId)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not get object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, object)
	}
}

// TODO: Describe
//
//	@Summary	Get acl by filter
//	@Schemes
//	@Description	Get acl by filter
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200				{object}	apicontracts.PaginatedResult[aclmodels.AclV2ListItem]
//	@Failure		403				{object}	rorerror.ErrorData
//	@Failure		400				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{object}	rorerror.ErrorData
//	@Router			/v1/acl/filter	[post]
//	@Param			filter			body	apicontracts.Filter	true	"Filter"
//	@Security		ApiKey || AccessToken
func GetByFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		var filter apicontracts.Filter

		// Check access
		// Scope: Ror
		// Subject: Acl
		// Access: Read
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectAcl))
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)

		if !accessObject.Read {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		//validate the request body
		if err := c.BindJSON(&filter); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Missing parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if validationErr := validate.Struct(&filter); validationErr != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, validationErr.Error())
			rerr.GinLogErrorAbort(c)
			return
		}

		// importing apicontracts for swagger
		var _ apicontracts.PaginatedResult[aclmodels.AclV2ListItem]
		paginatedResult, err := aclservice.GetByFilter(ctx, &filter)
		if err != nil {
			rlog.Errorc(ctx, err.Error(), err)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		if paginatedResult == nil {
			empty := apicontracts.PaginatedResult[aclmodels.AclV2ListItem]{}
			c.JSON(http.StatusOK, empty)
			return
		}

		c.JSON(http.StatusOK, paginatedResult)
	}
}

// TODO: Describe
//
//	@Summary	Create acl
//	@Schemes
//	@Description	Create acl
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200		{object}	aclmodels.AclV2ListItem
//	@Failure		403		{object}	rorerror.ErrorData
//	@Failure		400		{object}	rorerror.ErrorData
//	@Failure		401		{object}	rorerror.ErrorData
//	@Failure		500		{object}	rorerror.ErrorData
//	@Router			/v1/acl	[post]
//	@Param			acl		body	aclmodels.AclV2ListItem	true	"Acl"
//	@Security		ApiKey || AccessToken
func Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.GetIdentityFromRorContext(ctx)

		// Check access
		// Scope: Ror
		// Subject: Acl
		// Access: Create
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectAcl))
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var aclModel aclmodels.AclV2ListItem
		if err := c.BindJSON(&aclModel); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Required fields are missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if err := validate.Struct(&aclModel); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not validate object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		created, err := aclservice.Create(ctx, &aclModel, &identity)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Unable to create", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		_ = apiconnections.RabbitMQConnection.SendMessage(ctx, messagebuscontracts.AclUpdateEvent{Action: "Create"}, messagebuscontracts.Route_Acl_Update, nil)
		c.JSON(http.StatusOK, created)
	}
}

// TODO: Describe
//
//	@Summary	Update acl
//	@Schemes
//	@Description	Update acl
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200				{object}	aclmodels.AclV2ListItem
//	@Failure		403				{object}	rorerror.ErrorData
//	@Failure		400				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{object}	rorerror.ErrorData
//	@Router			/v1/acl/{aclId}	[put]
//	@Param			aclId			path	string					true	"aclId"
//	@Param			acl				body	aclmodels.AclV2ListItem	true	"Acl"
//	@Security		ApiKey || AccessToken
func Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.GetIdentityFromRorContext(ctx)

		// Check access
		// Scope: Ror
		// Subject: Acl
		// Access: Update
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectAcl))
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Update {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		aclId := c.Param("id")
		if aclId == "" || len(aclId) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		var aclModel aclmodels.AclV2ListItem
		if err := c.BindJSON(&aclModel); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Required fields are missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if err := validate.Struct(&aclModel); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not validate object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		created, err := aclservice.Update(ctx, aclId, &aclModel, &identity)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Unable to update", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		payload := messagebuscontracts.AclUpdateEvent{Action: "Update"}
		_ = apiconnections.RabbitMQConnection.SendMessage(ctx, payload, messagebuscontracts.Route_Acl_Update, nil)

		c.JSON(http.StatusOK, created)

	}
}

// TODO: Describe
//
//	@Summary		Delete acl
//	@Description	Delete a acl by id
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200				{bool}		bool
//	@Failure		403				{object}	rorerror.ErrorData
//	@Failure		400				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{object}	rorerror.ErrorData
//	@Router			/v1/acl/{aclId}	[delete]
//	@Param			aclId			path	string	true	"aclId"
//	@Security		ApiKey || AccessToken
func Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.GetIdentityFromRorContext(ctx)
		aclId := c.Param("id")
		if aclId == "" || len(aclId) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		// Check access
		// Scope: Ror
		// Subject: Acl
		// Access: Delete
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectAcl))
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Delete {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		result, _, err := aclservice.Delete(ctx, aclId, &identity)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not delete object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		payload := messagebuscontracts.AclUpdateEvent{Action: "Delete"}
		_ = apiconnections.RabbitMQConnection.SendMessage(ctx, payload, messagebuscontracts.Route_Acl_Update, nil)

		c.JSON(http.StatusOK, result)
	}
}

// TODO: Describe
//
//	@Summary	Migrate acl
//	@Schemes
//	@Description	Migrate acl
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Param			id	path		string	true	"id"
//	@Success		200	{string}	Status
//	@Failure		403	{string}	Forbidden
//	@Failure		401	{object}	rorerror.ErrorData
//	@Failure		500	{string}	Failure	message
//	@Router			/v1/acl/migrate [get]
//	@Security		ApiKey || AccessToken
func MigrateAcls() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Check access
		// Scope: Ror
		// Subject: Acl
		// Access: Update
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectAcl))
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Update {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		err := aclservice.MigrateAcl1toAcl2(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "message": "error migrating ACL"})
			return
		}

		payload := messagebuscontracts.AclUpdateEvent{Action: "Migrate"}
		_ = apiconnections.RabbitMQConnection.SendMessage(ctx, payload, messagebuscontracts.Route_Acl_Update, nil)

		c.JSON(http.StatusOK, map[string]interface{}{"success": true, "message": "ACL migrated"})
	}
}
