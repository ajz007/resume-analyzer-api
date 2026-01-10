package documents

import (
	"context"
	"time"
)

// DocumentsRepo defines persistence operations for documents.
type DocumentsRepo interface {
	Create(ctx context.Context, doc Document) error
	GetCurrentByUser(ctx context.Context, userId string) (Document, error)
	ListByUser(ctx context.Context, userId string, limit, offset int) ([]Document, error)
	GetByID(ctx context.Context, userId, documentID string) (Document, error)
	UpdateExtraction(ctx context.Context, userId, documentID, extractedKey string, extractedAt time.Time) error
}
