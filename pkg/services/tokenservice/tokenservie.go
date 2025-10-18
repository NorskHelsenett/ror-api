package tokenservice

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"strings"
	"time"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

var (
	oidcProviderURL string = "https://auth.sky.nhn.no/dex"
	oidcClientId    string = "clusterauth"
	privateKey      string = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDKpv/w83xrjIfx
yqJWcvqxKn6auwr6Dh/3Td/yYiDdv6QUHnFTm6yx6UOxaSxvu0E0QBQ7uuOn1qmi
PrgaMAg48FWVq+NulfdCjnWxyS15ZjG9eGnoh2TICzWkf3s/12PRZn4E6W5WkAu5
wqUlmKU4nXD+S1mg8CXeUH4ULl7tO+FrkVUaDoCe/IeiSCFAw3o21z4vUUBtQwKT
I/+8w9LmwwrXveFkGK/552DuamSy/XhOHUCx+5U+8UBaZfp0LwpFOZY8iT2aHCL6
A30eROt+KGeWnrDPNQ4mk/8ymyhSsewysOtlh/ej3RfaIQ7rBNh6kH+Cz+5fVp5Z
aqKydrbfAgMBAAECggEADiwt4/uJCrFX+yR91mN/K4roDe3GH64VovD2PLzUjk/b
2ULtNHjDnTFgLxUP2LsiwcLu6+Pl6Y9hxmk7NeNcNZDsLIFu3jw+22scJMaH145x
dfKyHyxfKlmCV9Jttx/tO0J18woiP4uwstBR/W0NbQYfes8Nz7SnRMLrWu8/j92L
a8y+55eWmyRW6TsHt9ApEPNscIBMQc8900aRvyWMQtXn6gprx2PeIAMl174J6GOl
bd3DsKzuYW+Eozrb8lfS/fIBXO+7S1qYF7QallG19JqfpIWrZYtNSphYK/QRBgjO
htb5JQhlhFA5h9DwkPYjkckGIszW99d4aDf+hy9b1QKBgQDqDecdHBfwY3h15W0r
ehV7fP/nbBvAt448DhJwZxPtFTRn1o9WT5N/C0qln5ztgiQqmFaWHEeODR72zE0A
hgO3aulphoOMqRm6RrpSXtD2fsCmrz1B3UnYIidqgzy1ACNyIUEGjY5TBqo3FVG6
2px5PRyqAvVuAwcsy8bF8mBlIwKBgQDdp1fj3VlANFRnF1CHNZndxhDa37/Mrc6N
eswbndeinMsjCqBbPVRfqS6fg7YP77AW2j32weOe6w1A3CBKk5s7nUTEY/eQKH+1
guCLMEMMnEWDWCQQ6RAScJw4t2H4eCr8KhjEIfa7WdlVGE9F6h/FkLbHmwQZDEpj
B1HyPHoZFQKBgQDOqYX3GxY8KOh1WSXi7MJJLl8a7Uc4DBtoBZjcbPeYME/8m+Qm
ds8qr0KzKVM8F9xtS+OwWboIwKcljdEz9CEV9C2zApXnPmy8ILVmA9iIvfTHeRYi
sQ0B7W5WSxjwTPX/UUOEULtprgnf51AqJ9tf5ckIiOJCyCOutyOFJvVcdwKBgQCy
SOoO5HnnhK/nA//H4btTgP8JrjNuBNdBQWZvSDSsHYXfN6rn+JqnH0PbFmwYwWhX
2U9B7Y6Swum0I9rtYXDZMJShiu8Tyx999jl6e2VS/VeEYB8SYwSEcIOXsxlga/fX
QF0PVWpKI+kF4znQOJM5rD74qp1PMG2c3cRyHWbwSQKBgDJCtwsXXwjY6+IHUFCM
1d12Apw3XhCTHAt4uJjksWYI+FJh87eb8CY9Ts1LhFlJOyeYoO80s+uqXSoyrN15
TUzfH7D7o0LPoQG8fsHliMZyaJxbqV5aB+2ViJTcLFB31TXv2Jq6MCYSsAfWpEIs
9ExB6AlRxuOa/pYc3MtDS/Mf
-----END PRIVATE KEY-----`
)

// ExchangeToken exchanges a token for a new resigned token
// 1 . Verifies the provided token
// 2. Extracts user information from the token
// 3. (Optional) Checks if the user has admin privileges if admin is true
// 4. Generates and returns a new token for the specified clusterID
func ExchangeToken(ctx context.Context, clusterID string, token string, admin bool) (string, error) {

	provider, err := oidc.NewProvider(ctx, oidcProviderURL)
	if err != nil {
		return "", err
	}
	idTokenVerifier := provider.Verifier(&oidc.Config{
		ClientID: oidcClientId,
	})

	// Parse and verify ID Token payload.
	idToken, err := idTokenVerifier.Verify(ctx, token)
	if err != nil {
		return "", err
	}

	// Extract custom user.
	user := identitymodels.User{Groups: []string{"NotAuthorized"}}
	if err := idToken.Claims(&user); err != nil {
		return "", err
	}

	groupsWithDomain, err := ExtractGroups(&user)
	if err != nil {
		return "", err
	}

	user.Groups = groupsWithDomain

	if admin {
		user.Groups = append(user.Groups, "cluster-admin@ror.io")
	}

	newtoken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":              user.Email,
		"iss":              "https://auth.ror.nhn.no",
		"email":            user.Email,
		"groups":           user.Groups,
		"nbf":              time.Now().Add(-1 * time.Minute).Unix(),
		"iat":              time.Now().Unix(),
		"exp":              time.Now().Add(time.Hour).Unix(),
		"aud":              oidcClientId,
		"clusterID":        clusterID,
		"providerISS":      user.Issuer,
		"providerAudience": user.Audience,
	})
	newtoken.Header["kid"] = "ror-api-key-1"
	rsaPriv, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKey))
	if err != nil {
		return "", err
	}
	signed, err := newtoken.SignedString(rsaPriv)
	if err != nil {
		return "", err
	}

	return signed, nil
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

func GetJwks() (jwk.Set, error) {

	rsaPriv, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKey))
	if err != nil {
		panic(err)
	}

	pubKey := rsaPriv.Public().(*rsa.PublicKey)

	jwkKey, err := jwk.FromRaw(pubKey)
	if err != nil {
		return nil, err
	}
	jwkKey.Set(jwk.KeyIDKey, "ror-api-key-1")
	jwkKey.Set(jwk.AlgorithmKey, "RS256")
	jwkKey.Set(jwk.KeyUsageKey, "sig")

	set := jwk.NewSet()
	set.AddKey(jwkKey)

	return set, nil
}
