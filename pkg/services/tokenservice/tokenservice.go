package tokenservice

import (
	"context"
	"strings"
	"time"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror/pkg/helpers/fouramhelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
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
// 3. (Optional) Checks if the user has admin privileges if admin is true
// 4. Generates and returns a new token for the specified clusterID
func ExchangeToken(ctx context.Context, clusterID string, token string, admin bool) (string, error) {
	claims, err := validator.ValidateToken(ctx, token)
	if err != nil {
		return "", err
	}

	groupsWithDomain, err := oidchelper.ExtractGroups(claims.Email, claims.Groups)
	if err != nil {
		return "", err
	}

	groupsWithDomain = aclservice.FilterGroupsInUse(ctx, groupsWithDomain)

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

// GetJwks returns the JSON Web Key Set (JWKS) containing the public keys.
func GetJwks() (jwk.Set, error) {
	return signer.GetJWKS()
}
