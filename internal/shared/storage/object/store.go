package object

import (
	"context"
	"io"
)

// ObjectStore defines the contract for saving and retrieving binary objects.
type ObjectStore interface {
	Save(ctx context.Context, userId string, fileName string, r io.Reader) (storageKey string, sizeBytes int64, mimeType string, err error)
	Open(ctx context.Context, storageKey string) (io.ReadCloser, error)
}
