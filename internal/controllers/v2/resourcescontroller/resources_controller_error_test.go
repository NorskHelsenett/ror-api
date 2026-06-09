package resourcescontroller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupRouter(handler gin.HandlerFunc, method string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("identity", identitymodels.Identity{
			Type: identitymodels.IdentityTypeCluster,
			ClusterIdentity: &identitymodels.ServiceIdentity{
				Id: "test-cluster",
			},
			ServiceIdentity: &identitymodels.ServiceIdentity{},
		})
		c.Next()
	})
	router.Handle(method, "/v2/resources/uid/:uid", handler)
	return router
}

func TestExistsResources_ReturnsInternalServerErrorOnGetResourceByUIDError(t *testing.T) {
	orig := getResourceByUID
	getResourceByUID = func(ctx context.Context, uid string) (*rorresources.ResourceSet, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { getResourceByUID = orig })

	router := setupRouter(ExistsResources(), http.MethodHead)
	req := httptest.NewRequest(http.MethodHead, "/v2/resources/uid/test-uid", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestGetResource_ReturnsInternalServerErrorOnGetResourceByUIDError(t *testing.T) {
	orig := getResourceByUID
	getResourceByUID = func(ctx context.Context, uid string) (*rorresources.ResourceSet, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { getResourceByUID = orig })

	router := setupRouter(GetResource(), http.MethodGet)
	req := httptest.NewRequest(http.MethodGet, "/v2/resources/uid/test-uid", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestGetResource_ReturnsNotFoundWhenGetResourceByUIDReturnsNilWithoutError(t *testing.T) {
	orig := getResourceByUID
	getResourceByUID = func(ctx context.Context, uid string) (*rorresources.ResourceSet, error) {
		return nil, nil
	}
	t.Cleanup(func() { getResourceByUID = orig })

	router := setupRouter(GetResource(), http.MethodGet)
	req := httptest.NewRequest(http.MethodGet, "/v2/resources/uid/test-uid", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteResource_ReturnsInternalServerErrorOnGetResourceByUIDError(t *testing.T) {
	orig := getResourceByUID
	getResourceByUID = func(ctx context.Context, uid string) (*rorresources.ResourceSet, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { getResourceByUID = orig })

	router := setupRouter(DeleteResource(), http.MethodDelete)
	req := httptest.NewRequest(http.MethodDelete, "/v2/resources/uid/test-uid", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
