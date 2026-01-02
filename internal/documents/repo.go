package documents

import "context"

// DocumentsRepo defines persistence operations for documents.
type DocumentsRepo interface {
	Create(ctx context.Context, doc Document) error
	GetCurrentByUser(ctx context.Context, userId string) (Document, error)
}
