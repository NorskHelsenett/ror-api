package tokencontroller

import (
	"errors"
	"net/http"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror-api/pkg/services/tokenservice"
	rorerror "github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type ExchangeTokenRequest struct {
	ClusterID string `json:"clusterId" validate:"required"`
	Admin     bool   `json:"admin,omitempty"`
	Token     string `json:"token" validate:"required"`
}

var (
	validate = validator.New()
)

// @Summary	Exchange token for a new resigned token
// @Schemes
// @Description	Exchange a token for a new resigned token to access a cluster
// @Tags			token
// @Accept			application/json
// @Produce		application/json
// @Success		200					{object}	string
// @Failure		403					{object}	rorerror.ErrorData
// @Failure		400					{object}	rorerror.ErrorData
// @Failure		401					{object}	rorerror.ErrorData
// @Failure		500					{object}	rorerror.ErrorData
// @Router			/v2/token/exchange	[post]
// @Param			token				body	ExchangeTokenRequest	true	"token to exchange"
// @Security		AccessToken
func ExchangeToken() gin.HandlerFunc {
	return func(c *gin.Context) {

		var input ExchangeTokenRequest
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()
		if ctx == nil {
			return
		}

		if err := c.BindJSON(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Required fields are missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if err := validate.Struct(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not validate token exchange object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		newToken, err := tokenservice.ExchangeToken(ctx, input.ClusterID, input.Token, input.Admin)

		if err != nil {
			var apiError rorerror.RorError
			if errors.As(err, &apiError) {
				rorginerror.NewRorGinError(apiError.GetStatusCode(), apiError.GetMessage(), apiError.GetErrors()...).GinLogErrorAbort(c)
				return
			}
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Unable to exchange token", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, newToken)
	}
}

// @Summary	Get JWKS
// @Schemes
// @Description	Get public ror-api signing keys for token and ACL snapshot verification
// @Tags			token
// @Accept			application/json
// @Produce		application/json
// @Success		200					{object}	interface{}
// @Failure		500					{object}	rorerror.ErrorData
// @Router			/v2/token/jwks		[get]
func GetJwks() gin.HandlerFunc {
	return func(c *gin.Context) {
		jwks, err := tokenservice.GetJwks()
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Unable to get JWKS", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, jwks)
	}
}
