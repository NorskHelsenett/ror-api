package authmiddleware

import (
	"net/http"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/gin-gonic/gin"
)

var AuthProviders []GinAuthProvider

type GinAuthProvider interface {
	IsOfType(c *gin.Context) bool
	Authenticate(c *gin.Context)
}

func AuthenticationMiddleware(c *gin.Context) {

	for _, provider := range AuthProviders {
		if provider.IsOfType(c) {
			provider.Authenticate(c)
			c.Next()
			return
		}
	}
	rerr := rorginerror.NewRorGinError(http.StatusUnauthorized, "Authorization provider not supported")
	rerr.GinLogErrorAbort(c)
}

func RegisterAuthProvider(provider GinAuthProvider) {
	if provider == nil {
		return
	}
	AuthProviders = append(AuthProviders, provider)
}
