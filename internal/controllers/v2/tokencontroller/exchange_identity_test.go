package tokencontroller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NorskHelsenett/ror-api/pkg/services/tokenservice"
	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper/oidctest"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/gin-gonic/gin"
)

// Identity binding must fail before ACL lookups or token signing.
func TestExchangeRejectsUnboundCredentials(t *testing.T) {
	manager, issuer, cleanup := oidctest.NewTestManager(t, "https://api.example.test")
	defer cleanup()
	tokenservice.SetManager(manager)
	t.Cleanup(func() { tokenservice.SetManager(oidchelper.NewManagerWithValidator("", jwksStorage{}, nil)) })
	otherIssuer, err := oidctest.NewTestIssuer()
	if err != nil {
		t.Fatal(err)
	}
	defer otherIssuer.Close()
	if err := manager.Validator.AddIssuer(otherIssuer.Config(oidctest.DefaultTestClientID)); err != nil {
		t.Fatal(err)
	}
	adminClaims := oidctest.DefaultUserClaims("admin@example.test", "admins")
	adminClaims.Subject = "stable-subject"
	adminToken := oidctest.MustSignToken(t, issuer, adminClaims)
	readerToken := oidctest.MustSignToken(t, issuer, oidctest.DefaultUserClaims("reader@example.test", "readers"))
	changedSubject := adminClaims
	changedSubject.Subject = "different-subject"
	sameEmailToken := oidctest.MustSignToken(t, issuer, changedSubject)
	otherIssuerToken := oidctest.MustSignToken(t, otherIssuer, adminClaims)
	for _, tc := range []struct {
		name      string
		bearer    string
		bodyToken string
		admin     bool
		status    int
	}{
		{"admin identity switch", adminToken, readerToken, true, http.StatusForbidden},
		{"normal identity switch", adminToken, readerToken, false, http.StatusForbidden},
		{"same email different subject", adminToken, sameEmailToken, true, http.StatusForbidden},
		{"same subject different issuer", adminToken, otherIssuerToken, true, http.StatusForbidden},
		{"invalid body token", adminToken, "invalid", true, http.StatusUnauthorized},
		{"missing bearer", "", readerToken, true, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity, err := identitymodels.NewUserIdentity(identitymodels.AuthInfo{
				AuthProvider: identitymodels.IdentityProviderOidc,
			}, "admin@example.test", "Stored Admin", []string{"admins@example.test"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			identity.SetToken(tc.bearer)
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("identity", identity) })
			router.POST("/v2/token/exchange", ExchangeToken())
			body, err := json.Marshal(ExchangeTokenRequest{
				ClusterID: "11111111-1111-4111-8111-111111111111",
				Token:     tc.bodyToken, Admin: tc.admin,
			})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/v2/token/exchange", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("unbound exchange returned %d, want %d before ACL/signing: %s", response.Code, tc.status, response.Body)
			}
			var signedToken string
			if json.Unmarshal(response.Body.Bytes(), &signedToken) == nil && signedToken != "" {
				t.Fatal("denied exchange returned a token")
			}
		})
	}
}
