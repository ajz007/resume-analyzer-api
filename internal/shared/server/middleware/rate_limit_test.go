package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimitPollingHigherThanDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(func() time.Time { return now })

	groupFor := func(c *gin.Context) string {
		if c.Request.Method == http.MethodGet && c.FullPath() == "/api/v1/analyses/:id" {
			return "POLLING"
		}
		return "DEFAULT"
	}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", "guest:test-guest")
		c.Next()
	})
	r.Use(RateLimit(RateLimitConfig{
		DefaultGroup: "DEFAULT",
		GroupFor:     groupFor,
		Limiter:      limiter,
		Rules: map[string]RateLimitRule{
			"DEFAULT": {Rate: 1, Burst: 2},
			"POLLING": {Rate: 5, Burst: 10},
		},
	}))

	r.GET("/api/v1/analyses/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.POST("/api/v1/documents", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/analyses/analysis-1", nil)
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("polling request %d expected 200, got %d", i+1, resp.Code)
		}
	}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", nil)
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("default request %d expected 200, got %d", i+1, resp.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("default request 3 expected 429, got %d", resp.Code)
	}
}

func TestRateLimit429IncludesRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(func() time.Time { return now })

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", "guest:test-guest")
		c.Next()
	})
	r.Use(RateLimit(RateLimitConfig{
		DefaultGroup: "DEFAULT",
		GroupFor: func(c *gin.Context) string {
			return "DEFAULT"
		},
		Limiter: limiter,
		Rules: map[string]RateLimitRule{
			"DEFAULT": {Rate: 1, Burst: 1},
		},
	}))
	r.GET("/api/v1/limited", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/limited", nil)
	resp1 := httptest.NewRecorder()
	r.ServeHTTP(resp1, req1)
	if resp1.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", resp1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/limited", nil)
	resp2 := httptest.NewRecorder()
	r.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp2.Code)
	}
	if resp2.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header")
	}

	var payload map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["error"] != "rate_limited" {
		t.Fatalf("expected error=rate_limited")
	}
	if _, ok := payload["retryAfterMs"]; !ok {
		t.Fatalf("expected retryAfterMs in response")
	}
}
