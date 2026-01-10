package analyses

import (
	"context"
	"time"
)

// Repo defines persistence operations for analyses.
type Repo interface {
	Create(ctx context.Context, analysis Analysis) error
	GetByID(ctx context.Context, analysisID string) (Analysis, error)
	UpdateStatus(ctx context.Context, analysisID, status string, result map[string]any) error
	UpdateStatusResultAndError(ctx context.Context, analysisID, status string, result map[string]any, errorMessage *string, startedAt *time.Time, completedAt *time.Time) error
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]Analysis, error)
}
