package rulesetscontroller

import (
	"net/http"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror-api/internal/apiservices/rulesetsService"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/messages"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
)

func init() {
	rlog.Debug("init rulesetsController controller")
}

// TODO: Describe function
//
//	@Summary	Get ruleset by cluster
//	@Schemes
//	@Description	Get ruleset by cluster
//	@Tags			rulesetsController
//	@Accept			application/json
//	@Produce		application/json
//	@Param			clusterId	path		string	true	"clusterId"
//	@Success		200			{object}	messages.RulesetModel
//	@Failure		403			{string}	Forbidden
//	@Failure		400			{object}	rorerror.ErrorData
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		500			{string}	Failure	message
//	@Router			/v1/rulesets/cluster/{clusterId} [get]
//	@Security		ApiKeyAuth
//	@Security		OAuth2Application[write, admin]
func GetByCluster() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		clusterId := c.Param("clusterId")

		if clusterId == "" || len(clusterId) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid cluster name")
			rerr.GinLogErrorAbort(c)
			return
		}

		// Access check
		// Scope: cluster
		// Subject: clusterId
		// Access: read
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, clusterId)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Read {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		ruleset, err := rulesetsService.FindCluster(ctx, clusterId)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not get ruleset", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, ruleset)
	}
}

// TODO: Describe function
//
//	@Summary	Get internal ruleset
//	@Schemes
//	@Description	Get the internal ruleset
//	@Tags			rulesetsController
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200	{object}	messages.RulesetModel
//	@Failure		403	{string}	Forbidden
//	@Failure		401	{object}	rorerror.ErrorData
//	@Failure		500	{string}	Failure	message
//	@Router			/v1/rulesets/internal [get]
//	@Security		ApiKeyAuth
//	@Security		OAuth2Application[write, admin]
func GetInternal() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Access check
		// Scope: ror
		// Subject: global
		// Access: read
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Read {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		ruleset, err := rulesetsService.FindInternal(ctx)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not get ruleset", err)
			rerr.GinLogErrorAbort(c)
		}

		c.JSON(http.StatusOK, ruleset)
	}
}

// TODO: Describe function
//
//	@Summary	Add a resource onto the ruleset
//	@Schemes
//	@Description	Append a resource onto the ruleset
//	@Tags			rulesetsController
//	@Accept			application/json
//	@Produce		application/json
//	@Param			rulesetId	path		string	true	"rulesetId"
//	@Success		200			{object}	messages.RulesetResourceModel
//	@Failure		403			{string}	Forbidden
//	@Failure		400			{object}	rorerror.ErrorData
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		500			{string}	Failure	message
//	@Router			/v1/rulesets/{rulesetId}/resources [post]
//	@Security		ApiKeyAuth
//	@Security		OAuth2Application[write, admin]
func AddResource() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		rulesetId := c.Param("rulesetId")
		var input messages.RulesetResourceInput

		if err := c.BindJSON(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid json", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		ruleset, err := rulesetsService.Find(ctx, rulesetId)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusNotFound, "could not find ruleset", err)
			rerr.GinLogErrorAbort(c)
		}
		var accessQuery aclmodels.AclV2QueryAccessScopeSubject
		if ruleset.Identity.Type == messages.RulesetIdentityTypeInternal {
			// Access check
			// Scope: ror
			// Subject: acl
			// Access: create
			// TODO: Check if this is correct
			accessQuery = aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectAcl)
		} else {
			// Access check
			// Scope: cluster
			// Subject: ruleset.Identity.Id
			// Access: create
			accessQuery = aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, ruleset.Identity.Id)
		}
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		resource, err := rulesetsService.AddResource(ctx, rulesetId, &input)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not add resource", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, resource)
	}
}

// TODO: Describe function
//
//	@Summary	Delete a resource
//	@Schemes
//	@Description	Delete a resource and all of its events.
//	@Tags			rulesetsController
//	@Accept			application/json
//	@Produce		application/json
//	@Param			rulesetId	path		string	true	"rulesetId"
//	@Param			resourceId	path		string	true	"resourceId"
//	@Success		200			{bool}		Deleted
//	@Failure		403			{string}	Forbidden
//	@Failure		400			{object}	rorerror.ErrorData
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		500			{string}	Failure	message
//	@Router			/v1/rulesets/{rulesetId}/resources/{resourceId} [delete]
//	@Security		ApiKeyAuth
//	@Security		OAuth2Application[write, admin]
func DeleteResource() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		rulesetId := c.Param("rulesetId")
		resourceId := c.Param("resourceId")

		ruleset, err := rulesetsService.Find(ctx, rulesetId)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusNotFound, "could not find ruleset", err)
			rerr.GinLogErrorAbort(c)
		}

		var accessQuery aclmodels.AclV2QueryAccessScopeSubject
		if ruleset.Identity.Type == messages.RulesetIdentityTypeInternal {
			// Access check
			// Scope: ror
			// Subject: acl
			// Access: delete
			// TODO: Check if this is correct
			accessQuery = aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectAcl)
		} else {
			// Access check
			// Scope: cluster
			// Subject: ruleset.Identity.Id
			// Access: delete
			accessQuery = aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, ruleset.Identity.Id)
		}
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Delete {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		if err := rulesetsService.DeleteResource(ctx, rulesetId, resourceId); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not delete resource", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, nil)
	}
}

// TODO: Describe function
//
//	@Summary	Add a resource rule
//	@Schemes
//	@Description	Add a resource rule
//	@Tags			rulesetsController
//	@Accept			application/json
//	@Produce		application/json
//	@Param			rulesetId	path		string	true	"rulesetId"
//	@Param			resourceId	path		string	true	"resourceId"
//	@Success		200			{object}	messages.RulesetRuleModel
//	@Failure		403			{string}	Forbidden
//	@Failure		400			{object}	rorerror.ErrorData
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		500			{string}	Failure	message
//	@Router			/v1/rulesets/{rulesetId}/resources/{resourceId}/rules [post]
//	@Security		ApiKeyAuth
//	@Security		OAuth2Application[write, admin]
func AddResourceRule() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		input := new(messages.RulesetRuleInput)
		if err := c.BindJSON(input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid json", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		rulesetId := c.Param("rulesetId")
		ruleset, err := rulesetsService.Find(ctx, rulesetId)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusNotFound, "could not find ruleset", err)
			rerr.GinLogErrorAbort(c)
		}

		var accessQuery aclmodels.AclV2QueryAccessScopeSubject
		if ruleset.Identity.Type == messages.RulesetIdentityTypeInternal {
			// Access check
			// Scope: ror
			// Subject: acl
			// Access: create
			// TODO: Check if this is correct
			accessQuery = aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectAcl)
		} else {
			// Access check
			// Scope: cluster
			// Subject: ruleset.Identity.Id
			// Access: create
			accessQuery = aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, ruleset.Identity.Id)
		}
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		resourceId := c.Param("resourceId")

		event, err := rulesetsService.AddResourceRule(ctx, rulesetId, resourceId, input)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not add resource rule", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, event)
	}
}

// TODO: Describe function
//
//	@Summary	Add a resource rule
//	@Schemes
//	@Description	Add a resource rule
//	@Tags			rulesetsController
//	@Accept			application/json
//	@Produce		application/json
//	@Param			rulesetId	path		string	true	"rulesetId"
//	@Param			resourceId	path		string	true	"resourceId"
//	@Param			ruleId		path		string	true	"ruleId"
//	@Success		200			{bool}		Deleted
//	@Failure		403			{string}	Forbidden
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		500			{string}	Failure	message
//	@Router			/v1/rulesets/{rulesetId}/resources/{resourceId}/rules/{ruleId} [post]
//	@Security		ApiKeyAuth
//	@Security		OAuth2Application[write, admin]
func DeleteResourceRule() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		rulesetId := c.Param("rulesetId")
		ruleset, err := rulesetsService.Find(ctx, rulesetId)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusNotFound, "could not find ruleset", err)
			rerr.GinLogErrorAbort(c)
		}

		var accessQuery aclmodels.AclV2QueryAccessScopeSubject
		if ruleset.Identity.Type == messages.RulesetIdentityTypeInternal {
			// Access check
			// Scope: ror
			// Subject: acl
			// Access: delete
			// TODO: Check if this is correct
			accessQuery = aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectAcl)
		} else {
			// Access check
			// Scope: cluster
			// Subject: ruleset.Identity.Id
			// Access: delete
			accessQuery = aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, ruleset.Identity.Id)
		}
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		resourceId := c.Param("resourceId")
		ruleId := c.Param("ruleId")

		if err := rulesetsService.DeleteResourceRule(ctx, rulesetId, resourceId, ruleId); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not delete resource rule", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, true)
	}
}

// only in development
func GetAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		rulesets, err := rulesetsService.FindAll(ctx)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not find rulesets", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, rulesets)
	}
}
