package analyses

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"resume-backend/internal/documents"
	"resume-backend/internal/llm"
	"resume-backend/internal/shared/storage/object/local"
)

type staticLLMResponse struct {
	resp string
}

func (s staticLLMResponse) AnalyzeResume(ctx context.Context, input llm.AnalyzeInput) (json.RawMessage, error) {
	_ = ctx
	_ = input
	return json.RawMessage(s.resp), nil
}

func setupServiceWithDoc(t *testing.T, llmClient llm.Client) (*Service, *MemoryRepo, *documents.MemoryRepo, string) {
	t.Helper()
	store := local.New(t.TempDir())
	docRepo := documents.NewMemoryRepo()
	analysisRepo := NewMemoryRepo()

	userID := "user-1"
	extractedKey, _, _, err := store.Save(context.Background(), userID, "resume.txt", bytes.NewReader([]byte("resume text")))
	if err != nil {
		t.Fatalf("save extracted text: %v", err)
	}

	doc := documents.Document{
		ID:               "doc-1",
		UserID:           userID,
		FileName:         "resume.txt",
		MimeType:         "text/plain",
		SizeBytes:        10,
		StorageKey:       "original",
		ExtractedTextKey: extractedKey,
		CreatedAt:        time.Now().UTC(),
	}
	if err := docRepo.Create(context.Background(), doc); err != nil {
		t.Fatalf("create doc: %v", err)
	}

	svc := &Service{
		Repo:    analysisRepo,
		DocRepo: docRepo,
		Store:   store,
		LLM:     llmClient,
	}

	return svc, analysisRepo, docRepo, doc.ID
}

func TestAnalysisRawStoredOnParseFailure(t *testing.T) {
	svc, repo, _, docID := setupServiceWithDoc(t, staticLLMResponse{resp: "{not-json"})

	analysis := Analysis{
		ID:             "analysis-raw-fail",
		DocumentID:     docID,
		UserID:         "user-1",
		JobDescription: "jd",
		PromptVersion:  "v1",
		Status:         StatusQueued,
		CreatedAt:      time.Now().UTC(),
	}
	if err := repo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	svc.completeAsync(analysis.ID)

	got, err := repo.GetByID(context.Background(), analysis.ID)
	if err != nil {
		t.Fatalf("get analysis: %v", err)
	}
	if got.AnalysisRaw == nil {
		t.Fatalf("expected analysis raw to be stored on parse failure")
	}
	rawMap, ok := got.AnalysisRaw.(map[string]any)
	if !ok || rawMap["rawText"] == "" {
		t.Fatalf("expected analysis_raw rawText to be set, got %#v", got.AnalysisRaw)
	}
}

func TestAnalysisCompletedTimestampSetOnSuccess(t *testing.T) {
	validV1 := `{
  "summary": {"overallAssessment": "ok", "strengths": [], "weaknesses": []},
  "ats": {"score": 80, "missingKeywords": [], "formattingIssues": []},
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins": [], "mediumEffort": [], "deepFixes": []}
}`
	svc, repo, _, docID := setupServiceWithDoc(t, staticLLMResponse{resp: validV1})

	analysis := Analysis{
		ID:             "analysis-success",
		DocumentID:     docID,
		UserID:         "user-1",
		JobDescription: "jd",
		PromptVersion:  "v1",
		Status:         StatusQueued,
		CreatedAt:      time.Now().UTC(),
	}
	if err := repo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	svc.completeAsync(analysis.ID)

	got, err := repo.GetByID(context.Background(), analysis.ID)
	if err != nil {
		t.Fatalf("get analysis: %v", err)
	}
	if got.AnalysisCompletedAt == nil {
		t.Fatalf("expected analysis_completed_at to be set on success")
	}
	if got.Status != StatusCompleted {
		t.Fatalf("expected status completed, got %s", got.Status)
	}
}
