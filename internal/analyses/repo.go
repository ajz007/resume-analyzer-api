package analyses

import (
	"context"
	"time"
)

// Repo defines persistence operations for analyses.
type Repo interface {
	Create(ctx context.Context, analysis Analysis) error
	GetOrCreateForDocument(ctx context.Context, analysis Analysis, allowRetry bool, allowCreate func() error) (Analysis, bool, error)
	GetByID(ctx context.Context, analysisID string) (Analysis, error)
	GetActiveShareByAnalysisOwner(ctx context.Context, analysisID string, ownerUserID *string, ownerGuestID *string) (AnalysisShare, error)
	GetShareByTokenHash(ctx context.Context, tokenHash string) (AnalysisShare, error)
	CreateShare(ctx context.Context, share AnalysisShare) error
	RevokeShare(ctx context.Context, shareID string, revokedAt time.Time) error
	UpdateStatus(ctx context.Context, analysisID, status string, result map[string]any) error
	UpdateStatusResultAndError(ctx context.Context, analysisID, status string, result map[string]any, errorCode *string, errorMessage *string, errorRetryable *bool, startedAt *time.Time, completedAt *time.Time) error
	UpdateAnalysisRaw(ctx context.Context, analysisID string, raw any) error
	UpdateAnalysisResult(ctx context.Context, analysisID string, result map[string]any, completedAt *time.Time) error
	UpdatePromptMetadata(ctx context.Context, analysisID, analysisVersion, promptHash string) error
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]Analysis, error)
}
