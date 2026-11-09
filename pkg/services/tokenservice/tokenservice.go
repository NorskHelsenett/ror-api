package tokenservice

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"
	"github.com/NorskHelsenett/ror/pkg/helpers/fouramhelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper"
	rorerror "github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v4/jwk"
)

const INTERNAL_DOMAIN = "ror.io"

var (
	adminTokenDuration = 1 * time.Hour

	// Validator and Signer are set during initialization via SetManager.
	validator oidchelper.TokenValidator
	signer    oidchelper.TokenSigner
)

// SetManager configures the token service with an oidchelper.Manager.
func SetManager(m *oidchelper.Manager) {
	validator = m
	signer = m
}

// ExchangeToken exchanges a token for a new resigned token.
// 1. Verifies the provided token via the multi-issuer validator
// 2. Extracts user information from the token
// 3. Binds issuer/subject to the authenticated user and checks both credentials' access
// 4. Generates and returns a new token for the specified clusterID
func ExchangeToken(ctx context.Context, clusterID string, token string, admin bool) (string, error) {
	caller, err := rorcontext.GetIdentityFromRorContext(ctx)
	if err != nil {
		return "", rorerror.NewRorError(http.StatusUnauthorized, "Missing authenticated user", err)
	}
	if caller.Type != identitymodels.IdentityTypeUser || caller.Auth.AuthProvider != identitymodels.IdentityProviderOidc {
		return "", rorerror.NewRorError(http.StatusForbidden, "Token exchange requires an OIDC user")
	}
	if caller.GetToken() == "" {
		return "", rorerror.NewRorError(http.StatusUnauthorized, "Missing authenticated bearer token")
	}
	if validator == nil {
		return "", fmt.Errorf("token validator is not initialized")
	}
	callerClaims, err := validator.ValidateToken(ctx, caller.GetToken())
	if err != nil {
		return "", rorerror.NewRorError(http.StatusUnauthorized, "Invalid bearer token", err)
	}
	claims, err := validator.ValidateToken(ctx, token)
	if err != nil {
		return "", rorerror.NewRorError(http.StatusUnauthorized, "Invalid token to exchange", err)
	}
	callerSubject, err := validatedSubject(caller.GetToken())
	if err != nil {
		return "", rorerror.NewRorError(http.StatusUnauthorized, "Invalid bearer subject", err)
	}
	subject, err := validatedSubject(token)
	if err != nil {
		return "", rorerror.NewRorError(http.StatusUnauthorized, "Invalid exchange subject", err)
	}
	if callerClaims.Issuer == "" || callerClaims.Issuer != claims.Issuer || callerSubject != subject {
		return "", rorerror.NewRorError(http.StatusForbidden, "Token issuer and subject must match the authenticated user")
	}

	groupsWithDomain, err := oidchelper.ExtractGroups(claims.Email, claims.Groups)
	if err != nil {
		return "", rorerror.NewRorError(http.StatusUnauthorized, "Invalid exchange identity", err)
	}
	freshIdentity, err := identitymodels.NewUserIdentity(identitymodels.AuthInfo{
		AuthProvider:   identitymodels.IdentityProviderOidc,
		AuthProviderID: claims.Email,
		ExpirationTime: claims.ExpirationTime,
	}, claims.Email, claims.Name, groupsWithDomain, map[string]string{
		"iss": claims.Issuer, "sub": subject, "aud": claims.Audience,
		"email_verified": strconv.FormatBool(claims.EmailVerified),
	})
	if err != nil {
		return "", rorerror.NewRorError(http.StatusUnauthorized, "Invalid exchange identity", err)
	}
	if claims.ACLGrantsPresent {
		freshIdentity.SetACLGrants(claims.ACLGrants)
	}
	freshIdentity.SetToken(token)
	freshContext := context.WithValue(ctx, identitymodels.ContexIdentity, freshIdentity)

	requiredAccess := aclmodels.CapKubernetes.WithVerb(aclmodels.VerbLogon)
	if admin {
		configured, err := aclservice.HasConfiguredAccess(ctx, aclscope.ScopeCluster, aclscope.Subject(clusterID), aclmodels.AccessKubernetesAdmin)
		if err != nil {
			return "", fmt.Errorf("check configured admin access: %w", err)
		}
		if configured {
			requiredAccess = aclmodels.AccessKubernetesAdmin
		}
	}
	for _, authorizationContext := range []context.Context{ctx, freshContext} {
		allowed, err := aclservice.HasAccess(authorizationContext, aclscope.ScopeCluster, aclscope.Subject(clusterID), requiredAccess)
		if err != nil {
			return "", fmt.Errorf("check token exchange access: %w", err)
		}
		if !allowed {
			return "", rorerror.NewRorError(http.StatusForbidden, "No access to login to cluster")
		}
	}

	groupsWithDomain, err = aclservice.GetGroupsInUse(freshContext, groupsWithDomain)
	if err != nil {
		return "", err
	}

	// Filter out groups with internal domain
	filtered := groupsWithDomain[:0]
	for _, g := range groupsWithDomain {
		if !strings.HasSuffix(g, "@"+INTERNAL_DOMAIN) {
			filtered = append(filtered, g)
		}
	}
	groupsWithDomain = filtered

	exp := fouramhelper.FourAm()

	if admin {
		exp = time.Now().Add(adminTokenDuration)
		groupsWithDomain = append(groupsWithDomain, "cluster-admin@"+INTERNAL_DOMAIN)
	}

	mapClaims := jwt.MapClaims{
		"sub":              claims.Email,
		"email":            claims.Email,
		"groups":           groupsWithDomain,
		"nbf":              time.Now().Add(-1 * time.Minute).Unix(),
		"iat":              time.Now().Unix(),
		"exp":              exp.Unix(),
		"aud":              claims.Audience,
		"clusterID":        clusterID,
		"providerISS":      claims.Issuer,
		"providerAudience": claims.Audience,
	}

	return signer.SignMapClaims(mapClaims)
}

// validatedSubject reads the original sub only after ValidateToken has verified
// the same token. The shared validator currently normalizes Subject to email.
func validatedSubject(token string) (string, error) {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return "", err
	}
	subject, err := claims.GetSubject()
	if err != nil {
		return "", err
	}
	if subject == "" {
		return "", fmt.Errorf("missing OIDC subject")
	}
	return subject, nil
}

// GetJwks publishes the API signing keys for token and ACL snapshot verification.
// All retained keys are included so verification survives signing-key rotation.
func GetJwks() (jwk.Set, error) {
	if signer == nil {
		return nil, fmt.Errorf("token signer is not initialized")
	}
	keys, err := signer.GetJWKS()
	if err != nil {
		return nil, fmt.Errorf("get API signing keys: %w", err)
	}
	if keys == nil || keys.Len() == 0 {
		return nil, fmt.Errorf("no API signing keys available")
	}
	publicKeys, err := jwk.PublicSetOf(keys)
	if err != nil {
		return nil, fmt.Errorf("extract public API signing keys: %w", err)
	}
	return publicKeys, nil
}
