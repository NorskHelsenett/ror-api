package oauthprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
)

type OauthProvider struct{}

func (d *OauthProvider) IsOfType(c *gin.Context) bool {
	authorization := c.Request.Header.Get("Authorization")
	return strings.HasPrefix(authorization, "Bearer ")
}

func (d *OauthProvider) Authenticate(c *gin.Context) {
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

	c.Set("identity", *identity)
}

func NewOauthProvider() *OauthProvider {
	return &OauthProvider{}
}

func getIdentityFromToken(c context.Context, auth string) (*identitymodels.Identity, rorerror.RorError) {

	skipVerificationCheck := rorconfig.GetBool(rorconfig.OIDC_SKIP_ISSUER_VERIFY)

	oicdProvider := rorconfig.GetString(rorconfig.OIDC_PROVIDER)

	var provider *oidc.Provider
	var err error

	if !skipVerificationCheck {
		provider, err = oidc.NewProvider(c, oicdProvider)
	} else {
		issuerURL := oicdProvider
		ctx := oidc.InsecureIssuerURLContext(c, issuerURL)
		// Provider will be discovered with the discoveryBaseURL, but use issuerURL
		// for future issuer validation.
		provider, err = oidc.NewProvider(ctx, oicdProvider)
	}

	if err != nil {
		return nil, rorerror.NewRorError(http.StatusBadRequest, fmt.Sprintf("Could not get provider, %s", oicdProvider), err)
	}

	token := strings.TrimPrefix(auth, "Bearer ")
	if token == auth {
		return nil, rorerror.NewRorError(http.StatusUnauthorized, "Could not find bearer token in Authorization header")
	}

	clientIDs := []string{
		rorconfig.GetString(rorconfig.OIDC_CLIENT_ID),
		rorconfig.GetString(rorconfig.OIDC_DEVICE_CLIENT_ID),
	}

	var idToken *oidc.IDToken
	var verifyErr error

	// Try each client ID until one works
	for _, clientID := range clientIDs {
		if clientID == "" {
			continue
		}

		idTokenVerifier := provider.Verifier(&oidc.Config{
			ClientID:                   clientID,
			SkipIssuerCheck:            skipVerificationCheck,
			InsecureSkipSignatureCheck: skipVerificationCheck,
		})

		idToken, verifyErr = idTokenVerifier.Verify(c, token)
		if verifyErr == nil {
			break // Successfully verified with this client ID
		}
	}

	if verifyErr != nil {
		return nil, rorerror.NewRorError(http.StatusUnauthorized, "Could not verify token with any client ID.")
	}

	// Extract custom user.
	user := identitymodels.User{Groups: []string{"NotAuthorized"}}
	if err := idToken.Claims(&user); err != nil {
		return nil, rorerror.NewRorError(http.StatusUnauthorized, "Not authorized")
	}

	groupsWithDomain, err := ExtractGroups(&user)
	if err != nil || len(groupsWithDomain) == 0 {
		return nil, rorerror.NewRorError(http.StatusUnauthorized, "Not authorized, missing groups")
	}

	user.Groups = groupsWithDomain

	exptime := time.Unix(int64(user.ExpirationTime), 0)

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

// Function extracts groups from user object
func ExtractGroups(user *identitymodels.User) ([]string, error) {
	if user == nil {
		msg := "user is nil"
		rlog.Debug(msg)
		return make([]string, 0), errors.New(msg)
	}

	emailArray := strings.Split(user.Email, "@")
	if len(emailArray) > 2 {
		msg := "could not extract domain from email"
		rlog.Debug(msg)
		return make([]string, 0), errors.New(msg)
	}

	domain := emailArray[1]
	groups := make([]string, 0)
	for i := 0; i < len(user.Groups); i++ {
		g := fmt.Sprintf("%s@%s", user.Groups[i], domain)
		groups = append(groups, g)
	}

	return groups, nil
}
