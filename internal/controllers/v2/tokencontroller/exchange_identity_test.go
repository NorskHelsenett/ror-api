package tokencontroller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/gin-gonic/gin"
)

// Identity binding must fail before ACL lookups or token signing. Deliberately
// leave those dependencies uninitialized so this test isolates the boundary.
func TestExchangeRejectsUnboundCredentials(t *testing.T) {
	for _, tc := range []struct {
		name      string
		bearer    string
		bodyToken string
		admin     bool
		status    int
	}{
		{"admin identity switch", "stored-admin-token", "fresh-reader-token", true, http.StatusForbidden},
		{"normal identity switch", "stored-admin-token", "fresh-reader-token", false, http.StatusForbidden},
		{"different credential for same authenticated user", "old-credential", "new-credential-same-email", true, http.StatusForbidden},
		{"missing bearer", "", "fresh-reader-token", true, http.StatusUnauthorized},
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
