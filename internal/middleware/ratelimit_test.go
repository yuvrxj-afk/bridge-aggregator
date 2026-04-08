package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bridge-aggregator/internal/middleware"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRouter(rl *middleware.RateLimiter) *gin.Engine {
	r := gin.New()
	r.GET("/test", rl.Limit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	rl := middleware.NewRateLimiter(10, 5) // 10 rps, burst 5
	r := newRouter(rl)

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}
}

func TestRateLimit_BlocksOverLimit(t *testing.T) {
	// 1 rps, burst 1 — second request must be rejected
	rl := middleware.NewRateLimiter(1, 1)
	r := newRouter(rl)

	// First request should pass
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w.Code)
	}

	// Second request immediately after should be rejected
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.1:9999"
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", w2.Code)
	}
}

func TestRateLimit_IsolatesByIP(t *testing.T) {
	// 1 rps, burst 1 — each IP gets its own limiter
	rl := middleware.NewRateLimiter(1, 1)
	r := newRouter(rl)

	for _, ip := range []string{"10.0.0.1:1", "10.0.0.2:1", "10.0.0.3:1"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = ip
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}
}
