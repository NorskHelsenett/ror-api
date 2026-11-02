package rlogmiddleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLogMiddlewareWithoutUserAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LogMiddleware())
	r.GET("/fail", func(c *gin.Context) { c.Status(http.StatusBadRequest) })

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	req.Header.Del("User-Agent")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
