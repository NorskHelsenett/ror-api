package authmiddleware

import (
	"context"
	"net/http"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
	"github.com/gin-gonic/gin"
)

var (
	AuthProviders      []GinAuthProvider
	TraceAuthProviders = false
)

type GinAuthProvider interface {
	IsOfType(c *gin.Context) bool
	Authenticate(c *gin.Context, ctx context.Context)
}

func AuthenticationMiddleware(c *gin.Context) {
	ctx := c.Request.Context()
	ctx, span := rortracer.StartSpan(ctx, "AuthenticationMiddleware")
	defer span.End()
	if TraceAuthProviders {
		ctx = rortracer.SuppressTracing(ctx)
	}
	for _, provider := range AuthProviders {
		if provider.IsOfType(c) {
			provider.Authenticate(c, ctx)
			rortracer.SpanOk(span)
			span.End()
			c.Next()
			return
		}
	}
	rerr := rorginerror.NewRorGinError(http.StatusUnauthorized, "Authorization provider not supported")
	rortracer.SpanError(span, rerr, "Autentications failed")
	rerr.GinLogErrorAbort(c)
}

func RegisterAuthProvider(provider GinAuthProvider) {
	if provider == nil {
		return
	}
	AuthProviders = append(AuthProviders, provider)
}
