package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSOptionsPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS([]string{"http://localhost:5173"}))
	router.OPTIONS("/api/v1/documents/:id/analyze", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/documents/123/analyze", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.Code)
	}
	assertCORSHeaders(t, resp)
}

func TestCORSHeadersOnPost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS([]string{"http://localhost:5173"}))
	router.POST("/api/v1/documents/:id/analyze", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/123/analyze", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	assertCORSHeaders(t, resp)
}

func assertCORSHeaders(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected Allow-Origin http://localhost:5173, got %q", got)
	}
	if got := resp.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatalf("expected Allow-Methods header")
	}
	if got := resp.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatalf("expected Allow-Headers header")
	}
	if got := resp.Header().Get("Access-Control-Max-Age"); got != "600" {
		t.Fatalf("expected Max-Age 600, got %q", got)
	}
}
