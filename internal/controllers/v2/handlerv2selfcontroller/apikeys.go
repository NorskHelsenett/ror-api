package handlerv2selfcontroller

import (
	"fmt"
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysservice"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apicontractsv2self"

	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/gin-gonic/gin"
)

// @Summary	Create or renew api key
// @Schemes
// @Description	Create or renew an api key
// @Tags			self
// @Accept			application/json
// @Produce		application/json
// @Success		200					{object}	apicontractsv2self.CreateOrRenewApikeyResponse
// @Failure		403					{object}	rorerror.ErrorData
// @Failure		400					{object}	rorerror.ErrorData
// @Failure		401					{object}	rorerror.ErrorData
// @Failure		500					{object}	rorerror.ErrorData
// @Router			/v2/self/apikeys	[post]
// @Param			apikey				body	apicontractsv2self.CreateOrRenewApikeyRequest	true	"Api key"
// @Security		ApiKey || AccessToken
func CreateOrRenewApikey() gin.HandlerFunc {
	return func(c *gin.Context) {
		var input apicontractsv2self.CreateOrRenewApikeyRequest
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.GetIdentityFromRorContext(ctx)
		if identity.Auth.AuthProvider == identitymodels.IdentityProviderApiKey {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "cannot create apikey with apikey")
			rerr.GinLogErrorAbort(c)
		}

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

		apikeyresponse, err := apikeysservice.CreateOrRenew(ctx, &input)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Unable to create api key, perhaps it already exist?", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, apikeyresponse)
	}
}

// @Summary	Delete api key
// @Schemes
// @Description	Delete an api key by id for user
// @Tags			self
// @Accept			application/json
// @Produce		application/json
// @Success		200							{boolean}	bool
// @Failure		403							{object}	rorerror.ErrorData
// @Failure		400							{object}	rorerror.ErrorData
// @Failure		401							{object}	rorerror.ErrorData
// @Failure		500							{object}	rorerror.ErrorData
// @Router			/v2/self/apikeys/{apikeyId}	[delete]
// @Param			apikeyId					path	string	true	"apikeyId"
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
