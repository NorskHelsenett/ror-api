package handlerv2selfcontroller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The /v2/self response is a published API contract (apicontractsv2self.SelfData
// + identitymodels.AuthInfo). These tests pin the exact JSON for every identity
// type so a refactor of the identity model cannot change the wire format.

var selfTestExpiry = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

func selfAuthInfo(provider identitymodels.IdentityProvider, id string) identitymodels.AuthInfo {
	return identitymodels.AuthInfo{
		AuthProvider:   provider,
		AuthProviderID: id,
		ExpirationTime: selfTestExpiry,
	}
}

// callSelf runs the GetSelf handler with the given identity and returns the raw body.
func callSelf(t *testing.T, identity identitymodels.Identity) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(recorder)
	engine.Use(func(c *gin.Context) { c.Set("identity", identity) })
	engine.GET("/v2/self", GetSelf())

	req := httptest.NewRequest(http.MethodGet, "/v2/self", nil)
	engine.ServeHTTP(recorder, req)
	return recorder.Code, recorder.Body.String()
}

func TestGetSelf_UserResponseShape(t *testing.T) {
	identity := identitymodels.Identity{
		Auth: selfAuthInfo(identitymodels.IdentityProviderOidc, "alice@example.com"),
		Type: identitymodels.IdentityTypeUser,
		User: &identitymodels.User{
			Name:   "Alice Example",
			Email:  "alice@example.com",
			Groups: []string{"team-blue@example.com", "admins@example.com"},
		},
	}

	code, body := callSelf(t, identity)
	require.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, `{
		"auth": {
			"authProvider": "OIDC",
			"authProviderId": "alice@example.com",
			"expirationTime": "2030-01-01T00:00:00Z"
		},
		"type": "User",
		"user": {
			"name": "Alice Example",
			"email": "alice@example.com",
			"groups": ["team-blue@example.com", "admins@example.com"]
		}
	}`, body)
}

func TestGetSelf_ClusterResponseShape(t *testing.T) {
	identity := identitymodels.Identity{
		Auth: selfAuthInfo(identitymodels.IdentityProviderApiKey, "apikey-1"),
		Type: identitymodels.IdentityTypeCluster,
		ClusterIdentity: &identitymodels.ServiceIdentity{
			Id:  "prod-cluster-1",
			Uid: "6f1c2b7e-0000-4000-8000-000000000001",
		},
	}

	code, body := callSelf(t, identity)
	require.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, `{
		"auth": {
			"authProvider": "APIKEY",
			"authProviderId": "apikey-1",
			"expirationTime": "2030-01-01T00:00:00Z"
		},
		"type": "Cluster",
		"user": {
			"name": "prod-cluster-1",
			"uid": "6f1c2b7e-0000-4000-8000-000000000001"
		}
	}`, body)
}

func TestGetSelf_ServiceResponseShape(t *testing.T) {
	identity := identitymodels.Identity{
		Auth: selfAuthInfo(identitymodels.IdentityProviderApiKey, "apikey-2"),
		Type: identitymodels.IdentityTypeService,
		ServiceIdentity: &identitymodels.ServiceIdentity{
			Id: "service-vulnerability-scanner",
		},
	}

	code, body := callSelf(t, identity)
	require.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, `{
		"auth": {
			"authProvider": "APIKEY",
			"authProviderId": "apikey-2",
			"expirationTime": "2030-01-01T00:00:00Z"
		},
		"type": "Service",
		"user": {
			"name": "service-vulnerability-scanner"
		}
	}`, body)
}

// The omitempty tags on SelfUser must keep absent fields out of the payload:
// a cluster response carries no email/groups, a service response no uid.
func TestGetSelf_OmitsEmptyFields(t *testing.T) {
	identity := identitymodels.Identity{
		Auth: selfAuthInfo(identitymodels.IdentityProviderApiKey, "apikey-2"),
		Type: identitymodels.IdentityTypeService,
		ServiceIdentity: &identitymodels.ServiceIdentity{
			Id: "service-vulnerability-scanner",
		},
	}

	_, body := callSelf(t, identity)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &payload))
	user, ok := payload["user"].(map[string]any)
	require.True(t, ok, "user object missing: %s", body)

	assert.NotContains(t, user, "uid")
	assert.NotContains(t, user, "email")
	assert.NotContains(t, user, "groups")
	assert.ElementsMatch(t, []string{"auth", "type", "user"}, keysOf(payload))
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
