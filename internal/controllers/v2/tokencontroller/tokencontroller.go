package tokencontroller

import (
	"net/http"

	aclrepository "github.com/NorskHelsenett/ror-api/internal/acl/repositories"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror-api/pkg/services/tokenservice"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type ExchangeTokenRequest struct {
	ClusterID string `json:"clusterId" validate:"required"`
	Admin     bool   `json:"admin,omitempty"`
	Token     string `json:"token" validate:"required"`
}

var (
	validate *validator.Validate
)

// @Summary	Excahnges a token for a new resigned token
// @Schemes
// @Description	Create a api key
// @Tags			users
// @Accept			application/json
// @Produce		application/json
// @Success		200					{object}	string
// @Failure		403					{object}	rorerror.ErrorData
// @Failure		400					{object}	rorerror.ErrorData
// @Failure		401					{object}	rorerror.ErrorData
// @Failure		500					{object}	rorerror.ErrorData
// @Router			/v2/token/exchange	[post]
// @Param			token				body	ExchangeTokenRequest	true	"token to exchange"
// @Security		ApiKey || AccessToken
func ExchangeToken() gin.HandlerFunc {
	return func(c *gin.Context) {

		var input ExchangeTokenRequest
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		if err := c.BindJSON(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Required fields are missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		// Access check
		// Scope: cluster
		// Subject: clusterId
		// Access: kubernetes.logon
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, input.ClusterID)
		access := aclrepository.CheckAcl2ByCluster(ctx, accessQuery)
		var hasAccess = false
		for _, acl := range access {
			if acl.Kubernetes.Logon {
				hasAccess = true
			}
		}

		if !hasAccess {
			rerr := rorginerror.NewRorGinError(http.StatusForbidden, "No access to login to cluster")
			rerr.GinLogErrorAbort(c)
			return
		}

		validate = validator.New()
		if err := validate.Struct(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not validate token exchange object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		newToken, err := tokenservice.ExchangeToken(ctx, input.ClusterID, input.Token, input.Admin)

		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Unable to exchange token", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, newToken)
	}
}

// @Summary	Get JWKS
// @Schemes
// @Description	Get JWKS for token verification
// @Tags			users
// @Accept			application/json
// @Produce		application/json
// @Success		200					{object}	interface{}
// @Failure		500					{object}	rorerror.ErrorData
// @Router			/v2/token/jwks		[get]
// @Security		ApiKey || AccessToken
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
