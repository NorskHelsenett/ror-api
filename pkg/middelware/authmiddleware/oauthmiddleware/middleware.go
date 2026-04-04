package oauthmiddleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
	"github.com/gin-gonic/gin"
)

// OauthMiddlewareInterface is implemented by OauthMiddleware.
type OauthMiddlewareInterface interface {
	Authenticate(c *gin.Context, ctx context.Context)
	IsOfType(c *gin.Context) bool
}

// OauthMiddleware delegates token validation to oidchelper.MultiIssuerValidator.
type OauthMiddleware struct {
	validator *oidchelper.MultiIssuerValidator
}

// NewOauthMiddleware creates an OauthMiddleware backed by an existing MultiIssuerValidator.
func NewOauthMiddleware(validator *oidchelper.MultiIssuerValidator) OauthMiddlewareInterface {
	return &OauthMiddleware{validator: validator}
}

// NewOauthMiddlewareFromConfig creates an OauthMiddleware from issuer configs.
func NewOauthMiddlewareFromConfig(configs ...oidchelper.IssuerConfig) (OauthMiddlewareInterface, error) {
	v, err := oidchelper.NewMultiIssuerValidator(configs...)
	if err != nil {
		return nil, err
	}
	return &OauthMiddleware{validator: v}, nil
}

// NewDefaultOauthMiddleware creates an OauthMiddleware using environment configuration.
func NewDefaultOauthMiddleware() (OauthMiddlewareInterface, error) {
	configs, err := oidchelper.LoadFromEnv()
	if err != nil {
		return nil, err
	}
	return NewOauthMiddlewareFromConfig(configs...)
}

func (d *OauthMiddleware) IsOfType(c *gin.Context) bool {
	authorization := c.Request.Header.Get("Authorization")
	return strings.HasPrefix(authorization, "Bearer ")
}

func (d *OauthMiddleware) Authenticate(c *gin.Context, ctx context.Context) {
	ctx, span := rortracer.StartSpan(ctx, "OauthMiddleware.Authenticate")
	defer span.End()

	auth := c.Request.Header.Get("Authorization")
	if auth == "" {
		rerr := rorginerror.NewRorGinError(http.StatusUnauthorized, "No Authorization header provided")
		rortracer.SpanError(span, rerr, "No Authorization header provided")
		rerr.GinLogErrorAbort(c)
		return
	}

	token := strings.TrimPrefix(auth, "Bearer ")
	if token == auth {
		rerr := rorginerror.NewRorGinError(http.StatusUnauthorized, "Could not find bearer token in Authorization header")
		rortracer.SpanError(span, rerr, "Missing bearer token")
		rerr.GinLogErrorAbort(c)
		return
	}

	identity, rerr := d.getIdentityFromToken(c.Request.Context(), token)
	if rerr != nil {
		rortracer.SpanError(span, rerr, "Could not get identity from token")
		rerr.GinLogErrorAbort(c)
		return
	}

	identity.SetToken(token)
	c.Set("identity", *identity)
	rortracer.SpanOk(span)
}

func (d *OauthMiddleware) getIdentityFromToken(ctx context.Context, token string) (*identitymodels.Identity, rorginerror.RorGinError) {
	claims, err := d.validator.ValidateToken(ctx, token)
	if err != nil {
		return nil, rorginerror.NewRorGinError(http.StatusUnauthorized, err.Error())
	}

	// Append email domain to groups
	groups, err := oidchelper.ExtractGroups(claims.Email, claims.Groups)
	if err != nil || len(groups) == 0 {
		return nil, rorginerror.NewRorGinError(http.StatusUnauthorized, "Not authorized, missing groups")
	}

	user := &identitymodels.User{
		Email:           claims.Email,
		IsEmailVerified: claims.EmailVerified,
		Name:            claims.Name,
		Groups:          groups,
		Audience:        claims.Audience,
		Issuer:          claims.Issuer,
		ExpirationTime:  int(claims.ExpirationTime.Unix()),
	}

	return &identitymodels.Identity{
		Auth: identitymodels.AuthInfo{
			AuthProvider:   identitymodels.IdentityProviderOidc,
			AuthProviderID: claims.Email,
			ExpirationTime: claims.ExpirationTime,
		},
		Type: identitymodels.IdentityTypeUser,
		User: user,
	}, nil
}
