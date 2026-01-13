package apikeyscontroller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	apikeysservice "github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysService"
	"github.com/NorskHelsenett/ror-api/internal/customvalidators"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

func init() {
	rlog.Debug("init apikeys controller")
	validate = validator.New()
	customvalidators.Setup(validate)
}

// TODO: Describe
//
//	@Summary	Get apikeys by filter
//	@Schemes
//	@Description	Get apikeys by filter
//	@Tags			api keys
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200					{object}	apicontracts.PaginatedResult[apicontracts.ApiKey]
//	@Failure		403					{object}	rorerror.ErrorData
//	@Failure		401					{object}	rorerror.ErrorData
//	@Failure		500					{object}	rorerror.ErrorData
//	@Router			/v1/apikeys/filter	[post]
//	@Param			filter				body	apicontracts.Filter	true	"Filter"
//	@Security		ApiKey || AccessToken
func GetByFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		var filter apicontracts.Filter

		//validate the request body
		if err := c.BindJSON(&filter); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Missing parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if validationErr := validate.Struct(&filter); validationErr != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "validation of required fields failed", validationErr)
			rerr.GinLogErrorAbort(c)
			return
		}

		// importing apicontracts for swagger
		var _ apicontracts.PaginatedResult[apicontracts.Cluster]

		paginatedResult, err := apikeysservice.GetByFilter(ctx, &filter)
		if err != nil {
			rlog.Errorc(ctx, "could not apicontracts", err)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		if paginatedResult == nil {
			empty := apicontracts.PaginatedResult[apicontracts.ApiKey]{}
			c.JSON(http.StatusOK, empty)
			return
		}

		c.JSON(http.StatusOK, paginatedResult)
	}
}

// TODO: Describe
func CreateForAgent() gin.HandlerFunc {
	return func(c *gin.Context) {
		// ROR context is not available for anonymous alloed endpoint, using regular go context for this endpoint
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var input apicontracts.AgentApiKeyModel
		if err := c.BindJSON(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Required fields are missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if err := validate.Struct(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not validate project object", err)
			rerr.GinLogErrorAbort(c)
			return
		}
		rlog.Infof("Creating api key for agent %s", input.Identifier)
		apikeyText, err := apikeysservice.CreateForAgent(ctx, &input)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Unable to create api key", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.Data(http.StatusOK, "text/plain", []byte(apikeyText))
	}
}

// TODO: Describe
//
//	@Summary	Delete api key
//	@Schemes
//	@Description	Delete a api key by id
//	@Tags			api keys
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200						{bool}		bool
//	@Failure		403						{object}	rorerror.ErrorData
//	@Failure		401						{object}	rorerror.ErrorData
//	@Failure		500						{object}	rorerror.ErrorData
//	@Router			/v1/apikeys/{apikeyId}	[delete]
//	@Param			apikeyId				path	string	true	"apikeyId"
//	@Security		ApiKey || AccessToken
func Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.GetIdentityFromRorContext(ctx)

		apikeyId := c.Param("id")
		if apikeyId == "" || len(apikeyId) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid id", fmt.Errorf("id is zero length"))
			rerr.GinLogErrorAbort(c)
			return
		}

		// Access check
		// Scope: ror
		// Subject: apikey
		// Access: delete
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)

		//TODO: Investegate
		//accessObject := aclservice.CheckAccessByContextScopeSubject(ctx, aclmodels.Acl2ScopeRor, aclmodels.Acl2Subject(identity.GetId()))
		if !accessObject.Delete {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		result, err := apikeysservice.Delete(ctx, apikeyId, &identity)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not delete object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// @Summary	Create api key
// @Schemes
// @Description	Create a api key
// @Tags			apikeys
// @Accept			application/json
// @Produce		application/json
// @Success		200					{string}	api	key
// @Failure		403					{object}	rorerror.ErrorData
// @Failure		401					{object}	rorerror.ErrorData
// @Failure		500					{object}	rorerror.ErrorData
// @Router			/v1/apikeys/apikeys	[post]
// @Param			project				body	apicontracts.ApiKey	true	"Api key"
// @Security		ApiKey || AccessToken
func CreateApikey() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.GetIdentityFromRorContext(ctx)

		// Access check
		// Scope: ror
		// Subject: apikey
		// Access: create
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var input apicontracts.ApiKey
		if err := c.BindJSON(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Required fields are missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if err := validate.Struct(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not validate project object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if input.Type != apicontracts.ApiKeyTypeService {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid api key type, only service api keys are supported")
			rerr.GinLogErrorAbort(c)
			return
		}

		apikeyText, err := apikeysservice.Create(ctx, &input, &identity)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Unable to create api key, perhaps it already exist?", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, apikeyText)
	}
}
