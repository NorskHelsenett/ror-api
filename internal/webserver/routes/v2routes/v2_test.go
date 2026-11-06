package v2routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/gin-gonic/gin"
)

type snapshotTestAuth struct {
	identity identitymodels.Identity
}

func (snapshotTestAuth) IsOfType(c *gin.Context) bool {
	return c.GetHeader("X-API-KEY") == "test-only"
}

func (p snapshotTestAuth) Authenticate(c *gin.Context, _ context.Context) {
	c.Set("identity", p.identity)
}

func TestSnapshotRoute(t *testing.T) {
	oldProviders := authmiddleware.AuthProviders
	oldTimeout := rorconfig.GetString(rorconfig.HTTP_TIMEOUT)
	oldReader := rorconfig.GetString("ACL_SNAPSHOT_READER_SERVICE_ID")
	t.Cleanup(func() {
		authmiddleware.AuthProviders = oldProviders
		rorconfig.Set(rorconfig.HTTP_TIMEOUT, oldTimeout)
		rorconfig.Set("ACL_SNAPSHOT_READER_SERVICE_ID", oldReader)
	})
	identity, err := identitymodels.NewServiceIdentity(
		identitymodels.AuthInfo{AuthProvider: identitymodels.IdentityProviderApiKey}, "reader")
	if err != nil {
		t.Fatal(err)
	}
	authmiddleware.AuthProviders = []authmiddleware.GinAuthProvider{snapshotTestAuth{identity}}
	rorconfig.Set(rorconfig.HTTP_TIMEOUT, "1s")
	rorconfig.Set("ACL_SNAPSHOT_READER_SERVICE_ID", "")
	router := gin.New()
	if err := SetupRoutes(router); err != nil {
		t.Fatal(err)
	}
	registered := false
	for _, route := range router.Routes() {
		if route.Path == "/v2/acl/snapshot" {
			registered = true
			if route.Method != http.MethodGet {
				t.Fatalf("snapshot write route registered: %s", route.Method)
			}
		}
	}
	if !registered {
		t.Fatal("snapshot route missing")
	}
	for _, tt := range []struct {
		method string
		auth   bool
		status int
	}{
		{http.MethodGet, false, 401},
		{http.MethodGet, true, 503},
		{http.MethodPost, true, 404},
	} {
		req := httptest.NewRequest(tt.method, "/v2/acl/snapshot", nil)
		if tt.auth {
			req.Header.Set("X-API-KEY", "test-only")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != tt.status {
			t.Fatalf("%s authenticated=%v: got %d: %s", tt.method, tt.auth, response.Code, response.Body)
		}
	}
	rorconfig.Set("ACL_SNAPSHOT_READER_SERVICE_ID", "reader")
	oldRows := rorconfig.GetString("ACL_SNAPSHOT_MAX_ROWS")
	rorconfig.Set("ACL_SNAPSHOT_MAX_ROWS", "invalid")
	t.Cleanup(func() { rorconfig.Set("ACL_SNAPSHOT_MAX_ROWS", oldRows) })
	if err := SetupRoutes(gin.New()); err == nil {
		t.Fatal("partially configured snapshot endpoint did not fail route setup")
	}
}
