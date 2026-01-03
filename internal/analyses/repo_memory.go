package analyses

import (
	"context"
	"sort"
	"sync"
)

// MemoryRepo stores analyses in memory and is safe for concurrent use.
type MemoryRepo struct {
	mu     sync.RWMutex
	byID   map[string]Analysis
	byUser map[string][]Analysis
}

// NewMemoryRepo constructs a MemoryRepo.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		byID:   make(map[string]Analysis),
		byUser: make(map[string][]Analysis),
	}
}

// Create stores the analysis.
func (r *MemoryRepo) Create(ctx context.Context, analysis Analysis) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[analysis.ID] = analysis
	r.byUser[analysis.UserID] = append(r.byUser[analysis.UserID], analysis)
	return nil
}

// GetByID returns an analysis by its ID.
func (r *MemoryRepo) GetByID(ctx context.Context, analysisID string) (Analysis, error) {
	if err := ctx.Err(); err != nil {
		return Analysis{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	analysis, ok := r.byID[analysisID]
	if !ok {
		return Analysis{}, ErrNotFound
	}
	return analysis, nil
}

// UpdateStatus updates the status and result for an existing analysis.
func (r *MemoryRepo) UpdateStatus(ctx context.Context, analysisID, status string, result map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	analysis, ok := r.byID[analysisID]
	if !ok {
		return ErrNotFound
	}
	analysis.Status = status
	if result != nil {
		analysis.Result = result
	}
	r.byID[analysisID] = analysis

	// update in user slice
	userAnalyses := r.byUser[analysis.UserID]
	for i := range userAnalyses {
		if userAnalyses[i].ID == analysisID {
			userAnalyses[i] = analysis
			break
		}
	}
	r.byUser[analysis.UserID] = userAnalyses

	return nil
}

// ListByUser returns analyses for a user, newest first, with limit/offset.
func (r *MemoryRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]Analysis, error) {
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
	userAnalyses := r.byUser[userID]
	r.mu.RUnlock()

	if len(userAnalyses) == 0 || offset >= len(userAnalyses) {
		return []Analysis{}, nil
	}

	analyses := make([]Analysis, len(userAnalyses))
	copy(analyses, userAnalyses)
	sort.Slice(analyses, func(i, j int) bool {
		return analyses[i].CreatedAt.After(analyses[j].CreatedAt)
	})

	end := len(analyses)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return analyses[offset:end], nil
}
