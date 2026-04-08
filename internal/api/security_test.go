package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---- messageHash validation ----

func TestIsValidMessageHash_Valid(t *testing.T) {
	cases := []string{
		"0x" + strings.Repeat("a", 64),
		"0x" + strings.Repeat("0", 64),
		"0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		"0xABCDEF1234567890ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890",
	}
	for _, h := range cases {
		if !isValidMessageHash(h) {
			t.Errorf("expected valid: %s", h)
		}
	}
}

func TestIsValidMessageHash_Invalid(t *testing.T) {
	cases := []string{
		"",
		"0x",
		"0x" + strings.Repeat("a", 63),  // too short
		"0x" + strings.Repeat("a", 65),  // too long
		strings.Repeat("a", 66),          // missing 0x prefix
		"0x" + strings.Repeat("g", 64),  // invalid hex char
		"../../etc/passwd",
		"0x1234/../admin",
	}
	for _, h := range cases {
		if isValidMessageHash(h) {
			t.Errorf("expected invalid: %s", h)
		}
	}
}

// ---- CCTP attestation handler rejects bad messageHash ----

func TestCCTPAttestationHandler_RejectsBadHash(t *testing.T) {
	r := gin.New()
	r.GET("/cctp/:messageHash", CCTPAttestationHandler("https://example.com"))

	// Note: path-traversal hashes (../../) are already blocked by Gin's router
	// path normalization (returns 404 before handler runs). We test format validation here.
	bad := []string{
		"notahash",
		"0x123",                          // too short
		"0x" + strings.Repeat("g", 64),  // invalid hex char
		"0x" + strings.Repeat("a", 63),  // 63 chars, not 64
		"0x" + strings.Repeat("a", 65),  // 65 chars, not 64
	}
	for _, h := range bad {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/cctp/"+h, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("hash %q: expected 400, got %d", h, w.Code)
		}
	}
}

func TestCCTPAttestationHandler_AcceptsValidHash(t *testing.T) {
	// Valid hash should pass validation (will fail at network level with a fake URL,
	// but we get 502 not 400 — confirming validation passed)
	r := gin.New()
	r.GET("/cctp/:messageHash", CCTPAttestationHandler("http://127.0.0.1:19999")) // no server here

	validHash := "0x" + strings.Repeat("ab", 32)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cctp/"+validHash, nil)
	r.ServeHTTP(w, req)
	// 502 = passed validation, failed network. NOT 400.
	if w.Code == http.StatusBadRequest {
		t.Fatalf("valid hash was incorrectly rejected with 400")
	}
}

// ---- ListOperationsHandler rejects missing wallet ----

func TestListOperationsHandler_RequiresWallet(t *testing.T) {
	r := gin.New()
	r.GET("/operations", ListOperationsHandler(nil)) // nil store → 503, but wallet check fires first

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/operations", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing wallet, got %d", w.Code)
	}
}

func TestListOperationsHandler_WithWalletAndNoStore_Returns503(t *testing.T) {
	r := gin.New()
	r.GET("/operations", ListOperationsHandler(nil))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/operations?wallet=0xabc", nil)
	r.ServeHTTP(w, req)
	// nil store → 503 (database not configured), which means wallet check passed
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (no DB), got %d", w.Code)
	}
}

// ---- Body size limit ----

func TestBodySizeLimit(t *testing.T) {
	r := gin.New()
	// Apply body limit middleware
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		c.Next()
	})
	r.POST("/quote", func(c *gin.Context) {
		// Try to read body — this triggers the size limit
		var v map[string]any
		if err := c.ShouldBindJSON(&v); err != nil {
			if strings.Contains(err.Error(), "too large") || strings.Contains(err.Error(), "request body") {
				c.Status(http.StatusRequestEntityTooLarge)
				return
			}
		}
		c.Status(http.StatusOK)
	})

	// 2MB body — should exceed 1MB limit
	body := strings.Repeat("a", 2<<20)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/quote", strings.NewReader(`{"data":"`+body+`"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Fatal("expected non-200 for oversized body")
	}
}
