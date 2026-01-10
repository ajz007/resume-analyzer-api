package analyses

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/documents"
	"resume-backend/internal/shared/server/middleware"
	local "resume-backend/internal/shared/storage/object/local"
)

func TestStartAnalysisDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, userID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", nil)
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", resp.Code)
	}

	var created struct {
		AnalysisID string `json:"analysisId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.AnalysisID == "" {
		t.Fatalf("expected analysisId, got empty")
	}

	analysis, err := analysisRepo.GetByID(context.Background(), created.AnalysisID)
	if err != nil {
		t.Fatalf("get analysis: %v", err)
	}
	if analysis.JobDescription != "" {
		t.Fatalf("expected empty jobDescription, got %q", analysis.JobDescription)
	}
	if analysis.PromptVersion != "v2_1" {
		t.Fatalf("expected promptVersion v2_1, got %q", analysis.PromptVersion)
	}
}

func TestStartAnalysisWithBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, userID)

	payload := map[string]string{
		"jobDescription": "hello",
		"promptVersion":  "v2",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", resp.Code)
	}

	var created struct {
		AnalysisID string `json:"analysisId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.AnalysisID == "" {
		t.Fatalf("expected analysisId, got empty")
	}

	analysis, err := analysisRepo.GetByID(context.Background(), created.AnalysisID)
	if err != nil {
		t.Fatalf("get analysis: %v", err)
	}
	if analysis.JobDescription != "hello" {
		t.Fatalf("expected jobDescription hello, got %q", analysis.JobDescription)
	}
	if analysis.PromptVersion != "v2" {
		t.Fatalf("expected promptVersion v2, got %q", analysis.PromptVersion)
	}
}

func TestStartAnalysisRejectsLongJobDescription(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, _ := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, userID)

	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 50001),
		"promptVersion":  "v1",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.Code)
	}
}

func setupAnalysisRouter(t *testing.T) (*gin.Engine, *documents.MemoryRepo, *MemoryRepo) {
	t.Helper()
	docRepo := documents.NewMemoryRepo()
	analysisRepo := NewMemoryRepo()
	storeDir := t.TempDir()
	store := local.New(storeDir)
	svc := &Service{Repo: analysisRepo, DocRepo: docRepo, Store: store}
	handler := NewHandler(svc, docRepo)

	router := gin.New()
	router.Use(middleware.Auth("dev"))
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api)

	return router, docRepo, analysisRepo
}

func seedDocument(t *testing.T, repo *documents.MemoryRepo, userID string) string {
	t.Helper()

	doc := documents.Document{
		ID:         "doc-" + userID,
		UserID:     userID,
		FileName:   "resume.pdf",
		MimeType:   "application/pdf",
		SizeBytes:  123,
		StorageKey: "test-key",
		CreatedAt:  time.Now().UTC(),
	}
	if err := repo.Create(context.Background(), doc); err != nil {
		t.Fatalf("create document: %v", err)
	}
	return doc.ID
}

func addGuestHeader(req *http.Request) {
	req.Header.Set("X-Guest-Id", "test-guest")
}
