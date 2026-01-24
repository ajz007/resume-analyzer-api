package analyses

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/documents"
	"resume-backend/internal/llm"
	"resume-backend/internal/shared/server/middleware"
	local "resume-backend/internal/shared/storage/object/local"
)

type staticLLM struct {
	response []byte
}

func (s staticLLM) AnalyzeResume(ctx context.Context, input llm.AnalyzeInput) (json.RawMessage, error) {
	_ = ctx
	_ = input
	return json.RawMessage(s.response), nil
}

func TestAnalysisResultPassthroughV2_1(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fixture := loadFixture(t, "testdata/v1_good.json")
	router, analysisRepo := setupAnalysisRouterWithLLM(t, fixture)

	analysisID := startAnalysis(t, router)
	waitForStatus(t, analysisRepo, analysisID, StatusCompleted)

	resp := getAnalysis(t, router, analysisID)
	if resp.Status != StatusCompleted {
		t.Fatalf("expected status completed, got %q", resp.Status)
	}

	var expected map[string]any
	if err := json.Unmarshal(fixture, &expected); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	summaryRaw, ok := resp.Result["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary in response")
	}
	expectedSummary := expected["summary"].(map[string]any)
	if summaryRaw["overallAssessment"] != expectedSummary["overallAssessment"] {
		t.Fatalf("summary.overallAssessment mismatch")
	}

	atsRaw, ok := resp.Result["ats"].(map[string]any)
	if !ok {
		t.Fatalf("expected ats in response")
	}
	expectedATS := expected["ats"].(map[string]any)
	if atsRaw["score"] != expectedATS["score"] {
		t.Fatalf("ats.score mismatch")
	}
}

func TestAnalysisSortsSetLikeLists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fixture := loadFixture(t, "testdata/v1_shuffled.json")
	router, analysisRepo := setupAnalysisRouterWithLLM(t, fixture)

	analysisID := startAnalysis(t, router)
	waitForStatus(t, analysisRepo, analysisID, StatusCompleted)

	resp := getAnalysis(t, router, analysisID)
	if resp.Status != StatusCompleted {
		t.Fatalf("expected status completed, got %q", resp.Status)
	}

	atsRaw, ok := resp.Result["ats"].(map[string]any)
	if !ok {
		t.Fatalf("expected ats to be object")
	}
	missingRaw, ok := atsRaw["missingKeywords"].(map[string]any)
	if !ok {
		t.Fatalf("expected ats.missingKeywords to be object")
	}
	assertStringSlice(t, missingRaw, "fromJobDescription", []string{"kubernetes", "observability", "typescript"})
	assertStringSlice(t, atsRaw, "formattingIssues", []string{"date formatting", "inconsistent bullets"})
	assertStringSlice(t, resp.Result, "missingInformation", []string{"Certifications", "Portfolio"})

	actionPlanRaw, ok := resp.Result["actionPlan"].(map[string]any)
	if !ok {
		t.Fatalf("expected actionPlan to be object")
	}
	assertStringSlice(t, actionPlanRaw, "quickWins", []string{"Add metrics", "Tighten summary"})
	assertStringSlice(t, actionPlanRaw, "mediumEffort", []string{"Reorder skills", "Rewrite bullet 2"})
	assertStringSlice(t, actionPlanRaw, "deepFixes", []string{"Refactor experience section", "Rewrite resume header"})
}

type analysisResponse struct {
	ID     string         `json:"id"`
	Status string         `json:"status"`
	Result map[string]any `json:"result"`
}

func setupAnalysisRouterWithLLM(t *testing.T, fixture []byte) (*gin.Engine, *MemoryRepo) {
	t.Helper()
	docRepo := documents.NewMemoryRepo()
	analysisRepo := NewMemoryRepo()
	storeDir := t.TempDir()
	store := local.New(storeDir)

	userID := "guest:test-guest"
	extractedKey, _, _, err := store.Save(context.Background(), userID, "extracted.txt", bytes.NewReader([]byte("resume text")))
	if err != nil {
		t.Fatalf("save extracted text: %v", err)
	}
	doc := documents.Document{
		ID:               "doc-" + userID,
		UserID:           userID,
		FileName:         "resume.txt",
		MimeType:         "text/plain",
		SizeBytes:        123,
		StorageKey:       "original",
		ExtractedTextKey: extractedKey,
		CreatedAt:        time.Now().UTC(),
	}
	if err := docRepo.Create(context.Background(), doc); err != nil {
		t.Fatalf("create document: %v", err)
	}

	svc := &Service{
		Repo:    analysisRepo,
		DocRepo: docRepo,
		Store:   store,
		LLM:     staticLLM{response: fixture},
	}
	handler := NewHandler(svc, docRepo)

	router := gin.New()
	router.Use(middleware.Auth("dev"))
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api)

	return router, analysisRepo
}

func startAnalysis(t *testing.T, router *gin.Engine) string {
	t.Helper()
	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 300),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-guest:test-guest/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Guest-Id", "test-guest")
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
	return created.AnalysisID
}

func getAnalysis(t *testing.T, router *gin.Engine, analysisID string) analysisResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+analysisID, nil)
	req.Header.Set("X-Guest-Id", "test-guest")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	var out analysisResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func waitForStatus(t *testing.T, repo *MemoryRepo, analysisID, status string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		analysis, err := repo.GetByID(context.Background(), analysisID)
		if err == nil && analysis.Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	analysis, _ := repo.GetByID(context.Background(), analysisID)
	t.Fatalf("analysis did not reach status %q, got %q", status, analysis.Status)
}

func assertStringSlice(t *testing.T, container map[string]any, key string, expected []string) {
	t.Helper()
	raw, ok := container[key]
	if !ok {
		t.Fatalf("missing %s", key)
	}
	list, ok := raw.([]any)
	if !ok {
		t.Fatalf("%s not a list", key)
	}
	got := make([]string, 0, len(list))
	for _, item := range list {
		str, ok := item.(string)
		if !ok {
			t.Fatalf("%s contains non-string", key)
		}
		got = append(got, str)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("%s mismatch: got %v want %v", key, got, expected)
	}
}
