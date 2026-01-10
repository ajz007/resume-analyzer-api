package analyses

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"resume-backend/internal/usage"
)

const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
)

// Service contains business logic for analyses.
type Service struct {
	Repo     Repo
	Usage    *usage.Service
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

	// Simulate work
	time.Sleep(time.Second + 500*time.Millisecond)

	result := map[string]any{
		"analysisId":        analysisID,
		"createdAt":         time.Now().UTC().Format(time.RFC3339),
		"matchScore":        72,
		"missingKeywords":   []string{},
		"weakKeywords":      []string{},
		"atsChecks":         []map[string]any{},
		"bulletSuggestions": []map[string]any{},
		"summary":           "Stub analysis complete",
		"nextSteps":         []string{},
	}
	_ = s.Repo.UpdateStatus(context.Background(), analysisID, StatusCompleted, result)
}
