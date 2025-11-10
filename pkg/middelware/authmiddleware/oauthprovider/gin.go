package oauthprovider

import (
	"net/http"

	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	"github.com/gin-gonic/gin"
)

func OauthGinMiddleware(c *gin.Context) {
	auth := c.Request.Header.Get("Authorization")
	if auth == "" {
		rerr := rorerror.NewRorError(http.StatusUnauthorized, "No Authorization header provided ")
		rerr.GinLogErrorAbort(c)
		return
	}

	identity, rerr := getIdentityFromToken(c.Request.Context(), auth)

	if rerr != nil {
		rerr.GinLogErrorAbort(c)
		return
	}

	c.Set("user", identity.User)
	c.Set("identity", identity)
}
