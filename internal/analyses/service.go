package analyses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"resume-backend/internal/documents"
	"resume-backend/internal/extract"
	"resume-backend/internal/llm"
	"resume-backend/internal/shared/storage/object"
	"resume-backend/internal/usage"
)

const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// Service contains business logic for analyses.
type Service struct {
	Repo     Repo
	Usage    *usage.Service
	DocRepo  documents.DocumentsRepo
	Store    object.ObjectStore
	LLM      llm.Client
	Provider string
	Model    string
}

// Create enqueues a new analysis and kicks off asynchronous completion.
func (s *Service) Create(ctx context.Context, documentID, userID, jobDescription, promptVersion string) (Analysis, error) {
	if documentID == "" || userID == "" {
		return Analysis{}, errors.New("documentID and userID are required")
	}
	if promptVersion == "" {
		promptVersion = "v1"
	}

	if s.Usage != nil {
		ok, _, err := s.Usage.CanConsume(ctx, userID, 1)
		if err != nil {
			return Analysis{}, err
		}
		if !ok {
			return Analysis{}, usage.ErrLimitReached
		}
	}

	analysis := Analysis{
		ID:             uuid.NewString(),
		DocumentID:     documentID,
		UserID:         userID,
		JobDescription: jobDescription,
		PromptVersion:  promptVersion,
		Provider:       normalizeProvider(s.Provider),
		Model:          s.Model,
		Status:         StatusQueued,
		CreatedAt:      time.Now().UTC(),
	}

	if err := s.Repo.Create(ctx, analysis); err != nil {
		return Analysis{}, err
	}

	if s.Usage != nil {
		if _, err := s.Usage.Consume(ctx, userID, 1); err != nil {
			return Analysis{}, err
		}
	}

	go s.completeAsync(analysis.ID)

	return analysis, nil
}

// Get returns an analysis by ID.
func (s *Service) Get(ctx context.Context, analysisID string) (Analysis, error) {
	if analysisID == "" {
		return Analysis{}, errors.New("analysisID is required")
	}
	return s.Repo.GetByID(ctx, analysisID)
}

// List returns analyses for a user ordered newest-first.
func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]Analysis, error) {
	if userID == "" {
		return nil, errors.New("userID is required")
	}
	return s.Repo.ListByUser(ctx, userID, limit, offset)
}

func normalizeProvider(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return "openai"
	}
	return provider
}

func (s *Service) completeAsync(analysisID string) {
	_ = s.Repo.UpdateStatus(context.Background(), analysisID, StatusProcessing, nil)

	analysis, err := s.Repo.GetByID(context.Background(), analysisID)
	if err != nil {
		s.failAnalysis(analysisID, fmt.Errorf("analysis lookup: %w", err))
		return
	}
	if s.DocRepo == nil || s.Store == nil {
		s.failAnalysis(analysisID, errors.New("missing document store dependencies"))
		return
	}
	if s.LLM == nil {
		s.failAnalysis(analysisID, errors.New("missing llm client"))
		return
	}

	doc, err := s.DocRepo.GetByID(context.Background(), analysis.UserID, analysis.DocumentID)
	if err != nil {
		s.failAnalysis(analysisID, fmt.Errorf("document lookup id=%s mime=%s: %w", analysis.DocumentID, doc.MimeType, err))
		return
	}

	extractedKey := doc.ExtractedTextKey
	if extractedKey == "" {
		if _, err := extract.ExtractText(context.Background(), s.Store, doc.StorageKey, doc.MimeType); err != nil {
			s.failAnalysis(analysisID, fmt.Errorf("document %s mime %s: %w", doc.ID, doc.MimeType, err))
			return
		}
		extractedKey = doc.StorageKey + ".extracted.txt"
		if err := s.DocRepo.UpdateExtraction(context.Background(), doc.UserID, doc.ID, extractedKey, time.Now().UTC()); err != nil {
			s.failAnalysis(analysisID, fmt.Errorf("document %s mime %s: update extraction: %w", doc.ID, doc.MimeType, err))
			return
		}
	}

	extracted, err := loadText(context.Background(), s.Store, extractedKey)
	if err != nil {
		s.failAnalysis(analysisID, fmt.Errorf("document %s mime %s: load extracted text: %w", doc.ID, doc.MimeType, err))
		return
	}

	raw, err := s.LLM.AnalyzeResume(context.Background(), llm.AnalyzeInput{
		ResumeText:     extracted,
		JobDescription: analysis.JobDescription,
		PromptVersion:  analysis.PromptVersion,
		TargetRole:     "",
	})
	if err != nil {
		s.failAnalysis(analysisID, fmt.Errorf("llm analyze: %w", err))
		return
	}

	var parsed AnalysisResultV1
	if err := json.Unmarshal(raw, &parsed); err != nil {
		rawRetry, retryErr := s.LLM.AnalyzeResume(llm.WithFixJSON(context.Background(), string(raw)), llm.AnalyzeInput{
			ResumeText:     extracted,
			JobDescription: analysis.JobDescription,
			PromptVersion:  analysis.PromptVersion,
			TargetRole:     "",
		})
		if retryErr != nil {
			s.failAnalysis(analysisID, fmt.Errorf("llm analyze retry: %w", retryErr))
			return
		}
		if err := json.Unmarshal(rawRetry, &parsed); err != nil {
			s.failAnalysis(analysisID, fmt.Errorf("llm output invalid: %w", err))
			return
		}
		raw = rawRetry
	}

	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		s.failAnalysis(analysisID, fmt.Errorf("llm output parse: %w", err))
		return
	}

	// Simulate work
	time.Sleep(time.Second + 500*time.Millisecond)

	_ = s.Repo.UpdateStatus(context.Background(), analysisID, StatusCompleted, result)
}

func (s *Service) failAnalysis(analysisID string, err error) {
	msg := sanitizeError(err)
	_ = s.Repo.UpdateStatus(context.Background(), analysisID, StatusFailed, map[string]any{
		"error": msg,
	})
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ReplaceAll(err.Error(), "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.TrimSpace(msg)
	const maxLen = 500
	if len(msg) > maxLen {
		msg = msg[:maxLen]
	}
	return msg
}

func loadText(ctx context.Context, store object.ObjectStore, key string) (string, error) {
	body, err := store.Open(ctx, key)
	if err != nil {
		return "", err
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
