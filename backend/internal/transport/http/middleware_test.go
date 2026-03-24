package httptransport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type fakeHTTPRuntimeConfigProvider struct {
	config HTTPRuntimeConfig
}

func (f fakeHTTPRuntimeConfigProvider) CurrentHTTPRuntimeConfig() HTTPRuntimeConfig {
	return f.config
}

func TestCORSMiddleware_HandlesPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(CORSMiddleware())
	engine.POST("/api/v1/auth/nonce", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/nonce", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type")

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("unexpected allow origin: %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatalf("expected allow methods header")
	}
}

func TestTraceMiddleware_RejectsMissingTraceHeaderWhenRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(TraceMiddleware(fakeHTTPRuntimeConfigProvider{config: HTTPRuntimeConfig{TraceHeaderRequired: true}}))
	engine.POST("/api/v1/orders", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestAuthMiddleware_RejectsRevokedSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(AuthMiddleware(fakeAccessVerifier{err: errorsx.ErrUnauthorized}))
	engine.GET("/api/v1/account/summary", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/summary", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}
