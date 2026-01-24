//go:build phase2
// +build phase2

package applies_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/analyses"
	"resume-backend/internal/applies"
	"resume-backend/internal/documents"
	"resume-backend/internal/generatedresumes"
	"resume-backend/internal/shared/storage/object/local"
)

type mockLLM struct {
	response string
}

func (m mockLLM) Complete(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return m.response, nil
}

func TestApplyHandlersHappyPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	root := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	userID := "user-123"
	docID := "doc-123"
	analysisID := "analysis-123"

	store := local.New(t.TempDir())
	docRepo := documents.NewMemoryRepo()
	analysisRepo := analyses.NewMemoryRepo()
	genRepo := generatedresumes.NewMemoryRepo()

	extractedKey, _, _, err := store.Save(context.Background(), userID, "resume.txt", bytes.NewReader([]byte("sample resume text")))
	if err != nil {
		t.Fatalf("save extracted text: %v", err)
	}

	doc := documents.Document{
		ID:               docID,
		UserID:           userID,
		FileName:         "resume.txt",
		MimeType:         "text/plain",
		SizeBytes:        123,
		StorageKey:       "original",
		ExtractedTextKey: extractedKey,
		CreatedAt:        time.Now().UTC(),
	}
	if err := docRepo.Create(context.Background(), doc); err != nil {
		t.Fatalf("create doc: %v", err)
	}

	analysis := analyses.Analysis{
		ID:         analysisID,
		DocumentID: docID,
		UserID:     userID,
		Status:     analyses.StatusCompleted,
		Result: map[string]any{
			"summary": map[string]any{
				"overallAssessment": "ok",
			},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	llmResponse := `{
  "header": {
    "name": "Test User",
    "title": "Engineer",
    "email": "[email]",
    "phone": "",
    "location": "",
    "links": [],
    "nationality": "",
    "maritalStatus": ""
  },
  "summary": ["Summary line"],
  "skills": {
    "languages": [],
    "frameworks": [],
    "databases": [],
    "cloudDevOps": [],
    "observability": [],
    "tools": []
  },
  "experience": [
    {
      "id": "1",
      "company": "Acme",
      "role": "Engineer",
      "location": "",
      "start": "",
      "end": "",
      "highlights": ["Did work"]
    }
  ],
  "projects": [],
  "education": [],
  "achievements": [],
  "certifications": []
}`

	svc := &applies.Service{
		AnalysisRepo:  analysisRepo,
		DocumentsRepo: docRepo,
		GeneratedRepo: genRepo,
		Store:         store,
		LLM:           mockLLM{response: llmResponse},
	}
	handler := applies.NewHandler(svc, genRepo, store)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("isGuest", false)
		c.Next()
	})
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyses/"+analysisID+"/apply", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.Code)
	}

	var created applies.GeneratedResumeResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode apply response: %v", err)
	}
	if created.GeneratedResumeID == "" {
		t.Fatalf("expected generatedResumeId, got empty")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/generated-resumes", nil)
	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listResp.Code)
	}
	var list []applies.GeneratedResumeResponse
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 generated resume, got %d", len(list))
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/generated-resumes/"+created.GeneratedResumeID, nil)
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", getResp.Code)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/v1/generated-resumes/"+created.GeneratedResumeID+"/download", nil)
	downloadResp := httptest.NewRecorder()
	router.ServeHTTP(downloadResp, downloadReq)
	if downloadResp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", downloadResp.Code)
	}
	if downloadResp.Body.Len() == 0 {
		t.Fatalf("expected download body")
	}
}
