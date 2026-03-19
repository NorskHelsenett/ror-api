package authmiddleware

import (
	"context"
	"net/http"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/codes"
)

var AuthProviders []GinAuthProvider

type GinAuthProvider interface {
	IsOfType(c *gin.Context) bool
	Authenticate(c *gin.Context, ctx context.Context)
}

func AuthenticationMiddleware(c *gin.Context) {
	ctx := c.Request.Context()
	ctx, span := rortracer.StartSpan(ctx, "AuthenticationMiddleware")
	defer span.End()

	for _, provider := range AuthProviders {
		if provider.IsOfType(c) {
			provider.Authenticate(c, ctx)
			span.SetStatus(codes.Ok, "Authentication successful")
			span.End()
			c.Next()
			return
		}
	}
	rerr := rorginerror.NewRorGinError(http.StatusUnauthorized, "Authorization provider not supported")
	span.SetStatus(codes.Error, "Autentications failed")
	rerr.GinLogErrorAbort(c)
}

func RegisterAuthProvider(provider GinAuthProvider) {
	if provider == nil {
		return
	}
	AuthProviders = append(AuthProviders, provider)
}
