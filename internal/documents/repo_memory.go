package documents

import (
	"context"
	"sync"
)

// MemoryRepo is an in-memory implementation of DocumentsRepo.
type MemoryRepo struct {
	mu   sync.RWMutex
	data map[string]Document // userId -> current document
}

// NewMemoryRepo constructs a MemoryRepo.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		data: make(map[string]Document),
	}
}

// Create stores/overwrites the current document for a user.
func (r *MemoryRepo) Create(ctx context.Context, doc Document) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[doc.UserID] = doc
	return nil
}

// GetCurrentByUser returns the current document for a user.
func (r *MemoryRepo) GetCurrentByUser(ctx context.Context, userId string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	doc, ok := r.data[userId]
	if !ok {
		return Document{}, ErrNotFound
	}
	return doc, nil
}
