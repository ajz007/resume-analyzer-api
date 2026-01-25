package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/analyses"
	"resume-backend/internal/documents"
)

func TestClaimGuestMigratesData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	docRepo := documents.NewMemoryRepo()
	analysisRepo := analyses.NewMemoryRepo()
	svc := NewService(docRepo, analysisRepo)
	handler := NewHandler(svc)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "user-1")
		c.Set("isGuest", false)
		c.Next()
	})
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api)

	guestID := "11111111-1111-1111-1111-111111111111"
	guestUserID := "guest:" + guestID

	doc := documents.Document{
		ID:        "doc-1",
		UserID:    guestUserID,
		FileName:  "resume.pdf",
		MimeType:  "application/pdf",
		SizeBytes: 123,
		CreatedAt: time.Now().UTC(),
	}
	if err := docRepo.Create(context.Background(), doc); err != nil {
		t.Fatalf("create document: %v", err)
	}
	analysis := analyses.Analysis{
		ID:         "analysis-1",
		DocumentID: doc.ID,
		UserID:     guestUserID,
		Status:     analyses.StatusCompleted,
		CreatedAt:  time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/claim-guest", nil)
	req.Header.Set("X-Guest-Id", guestID)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}

	docs, err := docRepo.ListByUser(context.Background(), "user-1", 10, 0)
	if err != nil {
		t.Fatalf("list docs: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 migrated doc, got %d", len(docs))
	}

	analysesList, err := analysisRepo.ListByUser(context.Background(), "user-1", 10, 0)
	if err != nil {
		t.Fatalf("list analyses: %v", err)
	}
	if len(analysesList) != 1 {
		t.Fatalf("expected 1 migrated analysis, got %d", len(analysesList))
	}
}

func TestClaimGuestIdempotentAndIsolated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	docRepo := documents.NewMemoryRepo()
	analysisRepo := analyses.NewMemoryRepo()
	svc := NewService(docRepo, analysisRepo)
	handler := NewHandler(svc)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "user-1")
		c.Set("isGuest", false)
		c.Next()
	})
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api)

	guestID := "22222222-2222-2222-2222-222222222222"
	guestUserID := "guest:" + guestID

	doc := documents.Document{
		ID:        "doc-2",
		UserID:    guestUserID,
		FileName:  "resume.pdf",
		MimeType:  "application/pdf",
		SizeBytes: 123,
		CreatedAt: time.Now().UTC(),
	}
	if err := docRepo.Create(context.Background(), doc); err != nil {
		t.Fatalf("create document: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/claim-guest", nil)
	req.Header.Set("X-Guest-Id", guestID)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/account/claim-guest", nil)
	req2.Header.Set("X-Guest-Id", guestID)
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusOK {
		t.Fatalf("expected status 200 on idempotent call, got %d", resp2.Code)
	}

	docs, err := docRepo.ListByUser(context.Background(), "user-2", 10, 0)
	if err != nil {
		t.Fatalf("list docs: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("expected no docs for other user, got %d", len(docs))
	}
}
