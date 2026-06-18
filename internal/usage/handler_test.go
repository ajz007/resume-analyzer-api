package usage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/auth"
	"resume-backend/internal/shared/server/middleware"
)

func TestGetUsageReturnsGuestMonthlyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newUsageRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil)
	req.Header.Set("X-Guest-Id", "guest-limit-test")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := int(body["limit"].(float64)); got != GuestMonthlyAnalysisLimit {
		t.Fatalf("expected guest limit %d, got %d", GuestMonthlyAnalysisLimit, got)
	}
	if got := int(body["remaining"].(float64)); got != GuestMonthlyAnalysisLimit {
		t.Fatalf("expected guest remaining %d, got %d", GuestMonthlyAnalysisLimit, got)
	}
	if got := body["isAuthenticated"].(bool); got {
		t.Fatalf("expected guest to be unauthenticated")
	}
	if got := body["userType"].(string); got != "guest" {
		t.Fatalf("expected userType guest, got %q", got)
	}
}

func TestGetUsageReturnsAuthenticatedFreeUserMonthlyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newUsageRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil)
	addUsageAuthHeader(t, req, "google:usage-user")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := int(body["limit"].(float64)); got != FreeUserMonthlyAnalysisLimit {
		t.Fatalf("expected authenticated limit %d, got %d", FreeUserMonthlyAnalysisLimit, got)
	}
	if got := int(body["remaining"].(float64)); got != FreeUserMonthlyAnalysisLimit {
		t.Fatalf("expected authenticated remaining %d, got %d", FreeUserMonthlyAnalysisLimit, got)
	}
	if got := body["isAuthenticated"].(bool); !got {
		t.Fatalf("expected authenticated user")
	}
	if got := body["userType"].(string); got != "authenticated" {
		t.Fatalf("expected userType authenticated, got %q", got)
	}
}

func newUsageRouter() *gin.Engine {
	svc := NewService()
	handler := NewHandler(svc, nil, nil, nil, nil)
	router := gin.New()
	router.Use(middleware.Auth("dev"))
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api)
	return router
}

func addUsageAuthHeader(t *testing.T, req *http.Request, userID string) {
	t.Helper()
	token, err := auth.SignJWT(auth.Claims{Sub: userID})
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
}
