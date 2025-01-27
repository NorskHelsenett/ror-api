package handlerv2selfcontroller

import (
	"fmt"
	"net/http"

	apikeysservice "github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysService"

	"github.com/NorskHelsenett/ror/pkg/context/gincontext"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apicontractsv2self"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror"

	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/gin-gonic/gin"
)

// @Summary	Create api key
// @Schemes
// @Description	Create a api key
// @Tags			users
// @Accept			application/json
// @Produce		application/json
// @Success		200					{object}	apicontractsv2self.CreateOrRenewApikeyResponse
// @Failure		403					{object}	rorerror.RorError
// @Failure		400					{object}	rorerror.RorError
// @Failure		401					{object}	rorerror.RorError
// @Failure		500					{object}	rorerror.RorError
// @Router			/v2/self/apikeys	[post]
// @Param			project				body	apicontractsv2self.CreateOrRenewApikeyRequest	true	"Api key"
// @Security		ApiKey || AccessToken
func CreateOrRenewApikey() gin.HandlerFunc {
	return func(c *gin.Context) {
		var input apicontractsv2self.CreateOrRenewApikeyRequest
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.GetIdentityFromRorContext(ctx)
		if identity.Auth.AuthProvider == identitymodels.IdentityProviderApiKey {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "cannot create apikey with apikey")
			rerr.GinLogErrorAbort(c)
		}

		if err := c.BindJSON(&input); err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Required fields are missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if err := validate.Struct(&input); err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Could not validate project object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		apikeyresponse, err := apikeysservice.CreateOrRenew(ctx, &input)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Unable to create api key, perhaps it already exist?", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, apikeyresponse)
	}
}

// @Summary	Delete api key for user
// @Schemes
// @Description	Delete a api key by id for user
// @Tags			user
// @Accept			application/json
// @Produce		application/json
// @Success		200							{bool}		bool
// @Failure		403							{object}	rorerror.RorError
// @Failure		400							{object}	rorerror.RorError
// @Failure		401							{object}	rorerror.RorError
// @Failure		500							{object}	rorerror.RorError
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
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		identity := rorcontext.GetIdentityFromRorContext(ctx)

		if !identity.IsUser() {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Invalid identity")
			rerr.GinLogErrorAbort(c)
			return
		}

		// todo fix delete for user
		result, err := apikeysservice.DeleteForUser(ctx, apikeyId, &identity)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Could not delete object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}
