package authmiddleware

import (
	"net/http"

	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror"
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
		}
	}
	rerr := rorerror.NewRorError(http.StatusUnauthorized, "Authorization provider not supported")
	rerr.GinLogErrorAbort(c)
}

func RegisterAuthProvider(provider GinAuthProvider) {
	AuthProviders = append(AuthProviders, provider)
}
