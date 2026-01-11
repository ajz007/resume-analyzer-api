package usage

import "context"

// AnalysisRecord contains the fields needed for apply flows.
type AnalysisRecord struct {
	ID         string
	UserID     string
	DocumentID string
	Status     string
	Result     map[string]any
}

// AnalysisRepo provides access to analysis records without importing analyses.
type AnalysisRepo interface {
	GetByID(ctx context.Context, analysisID string) (AnalysisRecord, error)
}
