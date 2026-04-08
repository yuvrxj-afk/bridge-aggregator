package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bridge-aggregator/internal/middleware"

	"github.com/gin-gonic/gin"
)

func newAPIKeyRouter(key string) *gin.Engine {
	r := gin.New()
	r.POST("/protected", middleware.RequireAPIKey(key), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestAPIKey_RejectsWhenKeyConfiguredAndMissing(t *testing.T) {
	r := newAPIKeyRouter("secret-key-123")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPIKey_RejectsWrongKey(t *testing.T) {
	r := newAPIKeyRouter("secret-key-123")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPIKey_AcceptsCorrectKey(t *testing.T) {
	r := newAPIKeyRouter("secret-key-123")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	req.Header.Set("X-API-Key", "secret-key-123")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAPIKey_NoOpWhenKeyEmpty(t *testing.T) {
	// When API_KEY is not configured, middleware passes through (dev mode)
	r := newAPIKeyRouter("")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (no-op), got %d", w.Code)
	}
}
