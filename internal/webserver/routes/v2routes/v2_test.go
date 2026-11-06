package v2routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware"
	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclgrants"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/gin-gonic/gin"
)

type snapshotTestAuth struct {
	identity identitymodels.Identity
}

type emptySnapshotGrants struct{}

func (emptySnapshotGrants) GetGrants(context.Context, []string) (aclgrants.Grants, error) {
	return aclgrants.Grants{}, nil
}

func (snapshotTestAuth) IsOfType(c *gin.Context) bool {
	return c.GetHeader("X-API-KEY") == "test-only"
}

func (p snapshotTestAuth) Authenticate(c *gin.Context, _ context.Context) {
	c.Set("identity", p.identity)
}

func TestSnapshotRoute(t *testing.T) {
	aclservice.SetResolver(acl.NewResolverWithGrantSource(emptySnapshotGrants{}))
	t.Cleanup(func() { aclservice.SetResolver(nil) })
	oldProviders := authmiddleware.AuthProviders
	oldTimeout := rorconfig.GetString(rorconfig.HTTP_TIMEOUT)
	bounds := []string{"ACL_SNAPSHOT_MAX_PAYLOAD_BYTES", "ACL_SNAPSHOT_MAX_ENVELOPE_BYTES",
		"ACL_SNAPSHOT_MAX_ROWS", "ACL_SNAPSHOT_MAX_ACCESS_ENTRIES", "ACL_SNAPSHOT_MAX_FIELD_BYTES", "ACL_SNAPSHOT_MAX_HEADER_BYTES"}
	old := map[string]string{}
	for _, key := range bounds {
		old[key] = rorconfig.GetString(key)
		rorconfig.Set(key, "")
	}
	t.Cleanup(func() {
		authmiddleware.AuthProviders = oldProviders
		rorconfig.Set(rorconfig.HTTP_TIMEOUT, oldTimeout)
		for key, value := range old {
			rorconfig.Set(key, value)
		}
	})
	identity, err := identitymodels.NewServiceIdentity(
		identitymodels.AuthInfo{AuthProvider: identitymodels.IdentityProviderApiKey}, "reader")
	if err != nil {
		t.Fatal(err)
	}
	authmiddleware.AuthProviders = []authmiddleware.GinAuthProvider{snapshotTestAuth{identity}}
	rorconfig.Set(rorconfig.HTTP_TIMEOUT, "1s")
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
		{http.MethodGet, true, 403},
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
	oldRows := rorconfig.GetString("ACL_SNAPSHOT_MAX_ROWS")
	rorconfig.Set("ACL_SNAPSHOT_MAX_ROWS", "invalid")
	t.Cleanup(func() { rorconfig.Set("ACL_SNAPSHOT_MAX_ROWS", oldRows) })
	if err := SetupRoutes(gin.New()); err == nil {
		t.Fatal("partially configured snapshot endpoint did not fail route setup")
	}
}
