package aclsnapshot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/gin-gonic/gin"
)

func TestHandlerRequestContext(t *testing.T) {
	cfg := testConfig()
	for _, tt := range []struct {
		name    string
		timeout any
		cancel  bool
		calls   int
	}{
		{"missing timeout", nil, false, 1},
		{"negative timeout", -time.Second, false, 0},
		{"invalid timeout type", "1s", false, 0},
		{"canceled request", time.Second, true, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			publisher := &fakePublisher{err: errors.New("stop after publishing")}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("identity", reader(t, cfg.ReaderServiceID, identitymodels.IdentityProviderApiKey))
				if tt.timeout != nil {
					c.Set("timeout", tt.timeout)
				}
			})
			router.GET("/v2/acl/snapshot", Handler(cfg, publisher, nil))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancel {
				cancel()
			}
			req := httptest.NewRequest(http.MethodGet, "/v2/acl/snapshot", nil).WithContext(ctx)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != 500 || publisher.calls != tt.calls {
				t.Fatalf("status %d, published %d times", response.Code, publisher.calls)
			}
			if tt.cancel && !errors.Is(publisher.ctx.Err(), context.Canceled) {
				t.Fatal("request cancellation was not propagated")
			}
		})
	}
}
