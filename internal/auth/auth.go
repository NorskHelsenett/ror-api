package auth

import (
	"net/http"
	"strings"

	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	"github.com/gin-gonic/gin"
)

func AuthenticationMiddleware(c *gin.Context) {
	xapikey := c.Request.Header.Get("X-API-KEY")
	if len(xapikey) > 0 {
		ApiKeyAuth(c)
		return
	}

	authorization := c.Request.Header.Get("Authorization")
	if strings.HasPrefix(authorization, "Bearer ") {
		DexMiddleware(c)
		return
	}

	rerr := rorerror.NewRorError(http.StatusUnauthorized, "Authorization provider not supported")
	rerr.GinLogErrorAbort(c)
}
