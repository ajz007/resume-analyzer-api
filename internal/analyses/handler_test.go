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
	"resume-backend/internal/llm"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/storage/object"
	local "resume-backend/internal/shared/storage/object/local"
)

func TestStartAnalysisDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 300),
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
	if analysis.JobDescription == "" {
		t.Fatalf("expected jobDescription to be stored, got empty")
	}
	if analysis.PromptVersion != "v2_1" {
		t.Fatalf("expected promptVersion v2_1, got %q", analysis.PromptVersion)
	}
}

func TestStartAnalysisWithBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	jobDescription := strings.Repeat("a", 300)
	payload := map[string]string{
		"jobDescription": jobDescription,
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
	if analysis.JobDescription != jobDescription {
		t.Fatalf("expected jobDescription to match payload, got %q", analysis.JobDescription)
	}
	if analysis.PromptVersion != "v2" {
		t.Fatalf("expected promptVersion v2, got %q", analysis.PromptVersion)
	}
}

func TestStartAnalysisRejectsLongJobDescription(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, _, store := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

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

func TestStartAnalysisIdempotentDoublePostSingleJob(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 300),
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
	var first map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	firstID, _ := first["analysisId"].(string)
	if firstID == "" {
		t.Fatalf("expected analysisId in first response")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	addGuestHeader(req2)
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, req2)

	if resp2.Code != http.StatusAccepted && resp2.Code != http.StatusOK {
		t.Fatalf("expected status 202 or 200, got %d", resp2.Code)
	}
	var second map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	secondID, _ := second["analysisId"].(string)
	if secondID != firstID {
		t.Fatalf("expected same analysisId, got %q and %q", firstID, secondID)
	}

	analyses, err := analysisRepo.ListByUser(context.Background(), userID, 100, 0)
	if err != nil {
		t.Fatalf("list analyses: %v", err)
	}
	count := 0
	for _, a := range analyses {
		if a.DocumentID == documentID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 analysis for document, got %d", count)
	}
}

func TestStartAnalysisCompletedReturnsResult(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	result := map[string]any{"summary": "done"}
	analysis := Analysis{
		ID:         "analysis-completed",
		DocumentID: documentID,
		UserID:     userID,
		Status:     StatusCompleted,
		Result:     result,
		CreatedAt:  time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 300),
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

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	var decoded struct {
		AnalysisID string         `json:"analysisId"`
		Status     string         `json:"status"`
		Result     map[string]any `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if decoded.AnalysisID != analysis.ID {
		t.Fatalf("expected analysisId %q, got %q", analysis.ID, decoded.AnalysisID)
	}
	if decoded.Status != StatusCompleted {
		t.Fatalf("expected completed status, got %q", decoded.Status)
	}
	if decoded.Result == nil {
		t.Fatalf("expected result in response")
	}
}

func TestStartAnalysisFailedRequiresRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	msg := "boom"
	analysis := Analysis{
		ID:           "analysis-failed",
		DocumentID:   documentID,
		UserID:       userID,
		Status:       StatusFailed,
		ErrorMessage: &msg,
		CreatedAt:    time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 300),
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

	if resp.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", resp.Code)
	}
}

type stubLLM struct{}

func (stubLLM) AnalyzeResume(ctx context.Context, input llm.AnalyzeInput) (json.RawMessage, error) {
	_ = ctx
	_ = input
	return json.RawMessage(`{
  "summary": {"overallAssessment": "ok", "strengths": [], "weaknesses": []},
  "ats": {"score": 80, "missingKeywords": [], "formattingIssues": []},
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins": [], "mediumEffort": [], "deepFixes": []}
}`), nil
}

func setupAnalysisRouter(t *testing.T) (*gin.Engine, *documents.MemoryRepo, *MemoryRepo, object.ObjectStore) {
	t.Helper()
	docRepo := documents.NewMemoryRepo()
	analysisRepo := NewMemoryRepo()
	storeDir := t.TempDir()
	store := local.New(storeDir)
	svc := &Service{Repo: analysisRepo, DocRepo: docRepo, Store: store, LLM: stubLLM{}}
	handler := NewHandler(svc, docRepo)

	router := gin.New()
	router.Use(middleware.Auth("dev"))
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api)

	return router, docRepo, analysisRepo, store
}

func seedDocument(t *testing.T, repo *documents.MemoryRepo, store object.ObjectStore, userID string) string {
	t.Helper()

	extractedKey, _, _, err := store.Save(context.Background(), userID, "resume.txt", bytes.NewReader([]byte("resume text")))
	if err != nil {
		t.Fatalf("save extracted text: %v", err)
	}
	doc := documents.Document{
		ID:               "doc-" + userID,
		UserID:           userID,
		FileName:         "resume.pdf",
		MimeType:         "application/pdf",
		SizeBytes:        123,
		StorageKey:       "test-key",
		ExtractedTextKey: extractedKey,
		CreatedAt:        time.Now().UTC(),
	}
	if err := repo.Create(context.Background(), doc); err != nil {
		t.Fatalf("create document: %v", err)
	}
	return doc.ID
}

func addGuestHeader(req *http.Request) {
	req.Header.Set("X-Guest-Id", "test-guest")
}
