package analyses

import "context"

// Repo defines persistence operations for analyses.
type Repo interface {
	Create(ctx context.Context, analysis Analysis) error
	GetByID(ctx context.Context, analysisID string) (Analysis, error)
	UpdateStatus(ctx context.Context, analysisID, status string, result map[string]any) error
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]Analysis, error)
}
