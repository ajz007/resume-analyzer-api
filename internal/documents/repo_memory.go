package documents

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryRepo is an in-memory implementation of DocumentsRepo.
type MemoryRepo struct {
	mu   sync.RWMutex
	data map[string][]Document // userId -> documents
}

// NewMemoryRepo constructs a MemoryRepo.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		data: make(map[string][]Document),
	}
}

// Create stores/overwrites the current document for a user.
func (r *MemoryRepo) Create(ctx context.Context, doc Document) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[doc.UserID] = append(r.data[doc.UserID], doc)
	return nil
}

// GetCurrentByUser returns the current document for a user.
func (r *MemoryRepo) GetCurrentByUser(ctx context.Context, userId string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	docs, ok := r.data[userId]
	if !ok || len(docs) == 0 {
		return Document{}, ErrNotFound
	}
	return docs[len(docs)-1], nil
}

// GetByID returns a document by ID for a user.
func (r *MemoryRepo) GetByID(ctx context.Context, userId, documentID string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	docs := r.data[userId]
	for i := range docs {
		if docs[i].ID == documentID {
			return docs[i], nil
		}
	}
	return Document{}, ErrNotFound
}

// UpdateExtraction stores the extracted text metadata for a document.
func (r *MemoryRepo) UpdateExtraction(ctx context.Context, userId, documentID, extractedKey string, extractedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	docs := r.data[userId]
	for i := range docs {
		if docs[i].ID == documentID {
			if docs[i].ExtractedTextKey == "" {
				docs[i].ExtractedTextKey = extractedKey
				docs[i].ExtractedAt = &extractedAt
				r.data[userId] = docs
			}
			return nil
		}
	}
	return ErrNotFound
}

// ListByUser returns documents for a user, newest first, honoring limit/offset.
func (r *MemoryRepo) ListByUser(ctx context.Context, userId string, limit, offset int) ([]Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}

	r.mu.RLock()
	userDocs := r.data[userId]
	r.mu.RUnlock()

	if len(userDocs) == 0 || offset >= len(userDocs) {
		return []Document{}, nil
	}

	// Copy and sort newest-first by CreatedAt.
	docs := make([]Document, len(userDocs))
	copy(docs, userDocs)
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].CreatedAt.After(docs[j].CreatedAt)
	})

	end := len(docs)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}

	return docs[offset:end], nil
}
