package oauthmiddleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
)

type OauthMiddlewareInterface interface {
	Authenticate(c *gin.Context)
	IsOfType(c *gin.Context) bool
}

type OauthMiddleware struct {
	providers map[string]OauthProviderInterface
}

func (d *OauthMiddleware) GetProviderByURL(url string) (OauthProviderInterface, bool) {
	provider, exists := d.providers[url]
	if !exists {
		return nil, false
	}
	return provider, true
}

func (d *OauthMiddleware) AddProvider(name string, provider OauthProviderInterface) {
	if d.providers == nil {
		d.providers = make(map[string]OauthProviderInterface)
	}
	d.providers[name] = provider
}

func (d *OauthMiddleware) IsOfType(c *gin.Context) bool {
	authorization := c.Request.Header.Get("Authorization")
	return strings.HasPrefix(authorization, "Bearer ")
}

func (d *OauthMiddleware) Authenticate(c *gin.Context) {
	auth := c.Request.Header.Get("Authorization")
	if auth == "" {
		rerr := rorginerror.NewRorGinError(http.StatusUnauthorized, "No Authorization header provided ")
		rerr.GinLogErrorAbort(c)
		return
	}

	identity, rerr := d.getIdentityFromToken(c.Request.Context(), auth)

	if rerr != nil {
		rerr.GinLogErrorAbort(c)
		return
	}

	token, _ := extractTokenFromAuthorizationHeader(auth)
	identity.SetToken(token)
	c.Set("identity", *identity)
}

func NewOauthMiddleware(opts ...OauthProvidersOption) OauthMiddlewareInterface {
	providers := &OauthMiddleware{}

	for _, opt := range opts {
		opt.apply(providers)
	}

	return providers
}

func NewDefaultOauthMiddleware(opts ...OauthProvidersOption) OauthMiddlewareInterface {
	opts = append(opts, OptionDefaultProvider())
	return NewOauthMiddleware(opts...)
}

func (d *OauthMiddleware) getIdentityFromToken(c context.Context, auth string) (*identitymodels.Identity, rorginerror.RorGinError) {
	var err error

	// Extract token from Authorization header
	token, rerr := extractTokenFromAuthorizationHeader(auth)
	if rerr != nil {
		return nil, rerr
	}

	// Extract unverified claims to determine issuer and client ID
	unverifiedClaims, rerr := extractUnverifiedClaims(token)
	if rerr != nil {
		return nil, rerr
	}

	// Get the appropriate OIDC provider based on the issuer
	oauthProvider, exists := d.GetProviderByURL(unverifiedClaims.Issuer)

	if !exists {
		return nil, rorginerror.NewRorGinError(http.StatusUnauthorized, fmt.Sprintf("No OIDC provider found for issuer: %s", unverifiedClaims.Issuer))
	}

	provider := oauthProvider.GetProvider()
	clientIDs := oauthProvider.GetIssuers()

	// Match audiences against configured client IDs
	clientID, matched := unverifiedClaims.MatchAudience(clientIDs...)
	if !matched {
		return nil, rorginerror.NewRorGinError(http.StatusUnauthorized, "Token audience does not match any configured client IDs.")
	}

	// Create ID token verifier
	idTokenVerifier := provider.Verifier(&oidc.Config{
		ClientID:                   clientID,
		SkipIssuerCheck:            oauthProvider.IsSkipverify(),
		InsecureSkipSignatureCheck: oauthProvider.IsSkipverify(),
	})

	// Verify token
	idToken, verifyErr := idTokenVerifier.Verify(c, token)
	if verifyErr != nil {
		return nil, rorginerror.NewRorGinError(http.StatusUnauthorized, "Could not verify token", verifyErr)
	}

	// Extract user from claims.
	user := identitymodels.User{Groups: []string{"NotAuthorized"}}
	if err := idToken.Claims(&user); err != nil {
		return nil, rorginerror.NewRorGinError(http.StatusUnauthorized, "Not authorized")
	}

	// Extract groups and append domain
	user.Groups, err = ExtractGroups(&user)
	if err != nil || len(user.Groups) == 0 {
		return nil, rorginerror.NewRorGinError(http.StatusUnauthorized, "Not authorized, missing groups")
	}

	// extract expiration time from token
	exptime := time.Unix(int64(user.ExpirationTime), 0)

	// return identity
	return &identitymodels.Identity{
		Auth: identitymodels.AuthInfo{
			AuthProvider:   identitymodels.IdentityProviderOidc,
			AuthProviderID: user.Email,
			ExpirationTime: exptime,
		},
		Type: identitymodels.IdentityTypeUser,
		User: &user,
	}, nil

}
