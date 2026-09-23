package oauthmiddleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper/oidctest"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupMiddleware(t *testing.T) (OauthMiddlewareInterface, *oidctest.TestIssuer, func()) {
	t.Helper()

	issuer, err := oidctest.NewTestIssuer()
	if err != nil {
		t.Fatalf("could not create test issuer: %v", err)
	}

	cfg := issuer.Config(oidctest.DefaultTestClientID)
	mw, err := NewOauthMiddlewareFromConfig(cfg)
	if err != nil {
		issuer.Close()
		t.Fatalf("could not create middleware: %v", err)
	}

	return mw, issuer, issuer.Close
}

func signToken(t *testing.T, issuer *oidctest.TestIssuer, claims oidchelper.TokenClaims) string {
	t.Helper()
	return oidctest.MustSignToken(t, issuer, claims)
}

// doRequest runs a request through the middleware and returns the recorder plus
// the identity the middleware placed in the context. The identity is read from
// the context rather than the response body: it is internal state, not a
// serialised contract.
func doRequest(mw OauthMiddlewareInterface, authHeader string) (*httptest.ResponseRecorder, *identitymodels.Identity) {
	w := httptest.NewRecorder()
	c, engine := gin.CreateTestContext(w)

	var captured *identitymodels.Identity

	engine.Use(func(ctx *gin.Context) {
		mw.Authenticate(ctx, ctx.Request.Context())
	})
	engine.GET("/test", func(ctx *gin.Context) {
		value, exists := ctx.Get("identity")
		if !exists {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "no identity"})
			return
		}
		identity, ok := value.(identitymodels.Identity)
		if !ok {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "unexpected identity type"})
			return
		}
		captured = &identity
		ctx.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	c.Request = req

	engine.ServeHTTP(w, req)
	return w, captured
}

func TestIsOfType_BearerToken(t *testing.T) {
	mw, _, cleanup := setupMiddleware(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer some-token")

	if !mw.IsOfType(c) {
		t.Error("expected IsOfType to return true for Bearer token")
	}
}

func TestIsOfType_NoBearer(t *testing.T) {
	mw, _, cleanup := setupMiddleware(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	if mw.IsOfType(c) {
		t.Error("expected IsOfType to return false for Basic auth")
	}
}

func TestIsOfType_NoHeader(t *testing.T) {
	mw, _, cleanup := setupMiddleware(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	if mw.IsOfType(c) {
		t.Error("expected IsOfType to return false for missing header")
	}
}

func TestAuthenticate_ValidToken(t *testing.T) {
	mw, issuer, cleanup := setupMiddleware(t)
	defer cleanup()

	claims := oidctest.DefaultUserClaims("alice@example.com", "admins", "devs")
	token := signToken(t, issuer, claims)

	w, identity := doRequest(mw, "Bearer "+token)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if identity == nil {
		t.Fatal("middleware did not set an identity")
	}

	email, err := identity.GetEmail()
	if err != nil {
		t.Fatalf("could not resolve email: %v", err)
	}
	if email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", email)
	}
	if identity.Type != identitymodels.IdentityTypeUser {
		t.Errorf("expected identity type user, got %s", identity.Type)
	}
	if identity.Auth.AuthProvider != identitymodels.IdentityProviderOidc {
		t.Errorf("expected auth provider oidc, got %s", identity.Auth.AuthProvider)
	}

	// Groups should have domain appended
	expectedGroups := []string{"admins@example.com", "devs@example.com"}
	groups, err := identity.GetGroups()
	if err != nil {
		t.Fatalf("could not resolve groups: %v", err)
	}
	if len(groups) != len(expectedGroups) {
		t.Fatalf("expected %d groups, got %d: %v", len(expectedGroups), len(groups), groups)
	}
	for i, g := range expectedGroups {
		if groups[i] != g {
			t.Errorf("expected group %s, got %s", g, groups[i])
		}
	}
}

func TestAuthenticate_NoAuthHeader(t *testing.T) {
	mw, _, cleanup := setupMiddleware(t)
	defer cleanup()

	w, _ := doRequest(mw, "")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthenticate_NonBearerAuth(t *testing.T) {
	mw, _, cleanup := setupMiddleware(t)
	defer cleanup()

	w, _ := doRequest(mw, "Basic dXNlcjpwYXNz")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	mw, _, cleanup := setupMiddleware(t)
	defer cleanup()

	w, _ := doRequest(mw, "Bearer not-a-valid-jwt")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthenticate_ExpiredToken(t *testing.T) {
	mw, issuer, cleanup := setupMiddleware(t)
	defer cleanup()

	claims := oidctest.DefaultUserClaims("alice@example.com", "admins")
	claims.ExpirationTime = time.Now().Add(-1 * time.Hour)
	token := signToken(t, issuer, claims)

	w, _ := doRequest(mw, "Bearer "+token)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", w.Code)
	}
}

func TestAuthenticate_NoGroups(t *testing.T) {
	mw, issuer, cleanup := setupMiddleware(t)
	defer cleanup()

	claims := oidctest.DefaultUserClaims("alice@example.com")
	token := signToken(t, issuer, claims)

	w, _ := doRequest(mw, "Bearer "+token)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for token without groups, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthenticate_WrongAudience(t *testing.T) {
	mw, issuer, cleanup := setupMiddleware(t)
	defer cleanup()

	claims := oidctest.DefaultUserClaims("alice@example.com", "admins")
	claims.Audience = "wrong-client-id"
	token := signToken(t, issuer, claims)

	w, _ := doRequest(mw, "Bearer "+token)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong audience, got %d", w.Code)
	}
}

func TestAuthenticate_IdentityFieldsPopulated(t *testing.T) {
	mw, issuer, cleanup := setupMiddleware(t)
	defer cleanup()

	claims := oidctest.DefaultUserClaims("bob@corp.io", "engineering")
	claims.Name = "Bob Builder"
	claims.EmailVerified = true
	token := signToken(t, issuer, claims)

	w, identity := doRequest(mw, "Bearer "+token)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if identity == nil {
		t.Fatal("middleware did not set an identity")
	}

	name, err := identity.GetName()
	if err != nil {
		t.Fatalf("could not resolve name: %v", err)
	}
	if name != "Bob Builder" {
		t.Errorf("expected name 'Bob Builder', got %q", name)
	}
	// Claims without a dedicated field are captured verbatim.
	if verified, ok := identity.GetClaim("email_verified"); !ok || verified != "true" {
		t.Errorf("expected email_verified claim true, got %q (present: %t)", verified, ok)
	}
	if iss, ok := identity.GetClaim("iss"); !ok || iss != issuer.IssuerURL {
		t.Errorf("expected issuer %q, got %q", issuer.IssuerURL, iss)
	}
	if aud, ok := identity.GetClaim("aud"); !ok || aud != oidctest.DefaultTestClientID {
		t.Errorf("expected audience %q, got %q", oidctest.DefaultTestClientID, aud)
	}
	if identity.Auth.AuthProviderID != "bob@corp.io" {
		t.Errorf("expected auth provider ID bob@corp.io, got %s", identity.Auth.AuthProviderID)
	}
}
