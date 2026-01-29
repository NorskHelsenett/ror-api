package userscontroller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysservice"
	"github.com/NorskHelsenett/ror-api/internal/customvalidators"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

func init() {
	rlog.Debug("init user controller")
	validate = validator.New()
	customvalidators.Setup(validate)
}

// @Summary	Get user
// @Schemes
// @Description	Get user details
// @Tags			users
// @Accept			application/json
// @Produce		application/json
// @Success		200	{object}	apicontracts.User
// @Failure		403	{string}	Forbidden
// @Failure		400	{object}	rorerror.ErrorData
// @Failure		401	{string}	Unauthorized
// @Failure		500	{string}	Failure	message
// @Router			/v1/users/self [get]
// @Security		ApiKey || AccessToken
func GetUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, _ := gincontext.GetRorContextFromGinContext(c)

		identity := rorcontext.GetIdentityFromRorContext(ctx)
		if !identity.IsUser() {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid identity")
			rerr.GinLogErrorAbort(c)
			return
		}

		if identity.User == nil {
			c.JSON(http.StatusForbidden, nil)
			return
		}

		result := apicontracts.User{
			Name:   identity.User.Name,
			Email:  identity.User.Email,
			Groups: identity.User.Groups,
		}

		c.JSON(http.StatusOK, result)
	}
}

// @Summary	Get apikeys by filter
// @Schemes
// @Description	Get apikeys by filter
// @Tags			users
// @Accept			application/json
// @Produce		application/json
// @Success		200								{object}	apicontracts.PaginatedResult[apicontracts.ApiKey]
// @Failure		403								{object}	rorerror.ErrorData
// @Failure		400								{object}	rorerror.ErrorData
// @Failure		401								{object}	rorerror.ErrorData
// @Failure		500								{object}	rorerror.ErrorData
// @Router			/v1/users/self/apikeys/filter	[post]
// @Param			filter							body	apicontracts.Filter	true	"Filter"
// @Security		ApiKey || AccessToken
func GetApiKeysByFilter() gin.HandlerFunc {
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
			rlog.Errorc(ctx, "failed to validate required fields", validationErr)
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, validationErr.Error())
			rerr.GinLogErrorAbort(c)
			return
		}

		identity := rorcontext.GetIdentityFromRorContext(ctx)
		if !identity.IsUser() {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid identity")
			rerr.GinLogErrorAbort(c)
			return
		}

		// importing apicontracts for swagger
		var _ apicontracts.PaginatedResult[apicontracts.Cluster]
		filter.Filters = append(filter.Filters, apicontracts.FilterMetadata{
			Field:     "identifier",
			MatchMode: apicontracts.MatchModeEquals,
			Value:     identity.User.Email,
		})
		paginatedResult, err := apikeysservice.GetByFilter(ctx, &filter)
		if err != nil {
			rlog.Errorc(ctx, "could not get apikeys", err)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		if paginatedResult == nil {
			empty := apicontracts.PaginatedResult[apicontracts.Cluster]{}
			c.JSON(http.StatusOK, empty)
			return
		}

		c.JSON(http.StatusOK, paginatedResult)
	}
}

// @Summary	Create api key
// @Schemes
// @Description	Create a api key
// @Tags			users
// @Accept			application/json
// @Produce		application/json
// @Success		200						{string}	api	key
// @Failure		403						{object}	rorerror.ErrorData
// @Failure		400						{object}	rorerror.ErrorData
// @Failure		401						{object}	rorerror.ErrorData
// @Failure		500						{object}	rorerror.ErrorData
// @Router			/v1/users/self/apikeys	[post]
// @Param			project					body	apicontracts.ApiKey	true	"Api key"
// @Security		ApiKey || AccessToken
func CreateApikey() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.GetIdentityFromRorContext(ctx)

		if !identity.IsUser() {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid identity")
			rerr.GinLogErrorAbort(c)
			return
		}

		var input apicontracts.ApiKey
		if err := c.BindJSON(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Required fields are missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		// Ensure that you cant create a api key for another user
		input.Identifier = identity.GetId()

		if err := validate.Struct(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not validate project object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		apikeyText, err := apikeysservice.Create(ctx, &input, &identity)
		if err != nil {
			if strings.Contains(err.Error(), "too many apikeys") {
				rerr := rorginerror.NewRorGinError(http.StatusForbidden, "Too many apikeys, limit of 100 reached.", err)
				rerr.GinLogErrorAbort(c)
				return
			}
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Unable to create api key, perhaps it already exist?", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, apikeyText)
	}
}

// @Summary	Delete api key for user
// @Schemes
// @Description	Delete a api key by id for user
// @Tags			user
// @Accept			application/json
// @Produce		application/json
// @Success		200									{bool}		bool
// @Failure		403									{object}	rorerror.ErrorData
// @Failure		400									{object}	rorerror.ErrorData
// @Failure		401									{object}	rorerror.ErrorData
// @Failure		500									{object}	rorerror.ErrorData
// @Router			/v1/users/self/apikeys/{apikeyId}	[delete]
// @Param			apikeyId							path	string	true	"apikeyId"
// @Security		ApiKey || AccessToken
func DeleteApiKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		apikeyId := c.Param("id")
		if apikeyId == "" || len(apikeyId) == 0 {
			rlog.Errorc(ctx, "invalid id", fmt.Errorf("id is zero length"))
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		identity := rorcontext.GetIdentityFromRorContext(ctx)

		if !identity.IsUser() {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid identity")
			rerr.GinLogErrorAbort(c)
			return
		}

		// todo fix delete for user
		result, err := apikeysservice.DeleteForUser(ctx, apikeyId, &identity)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not delete object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}
