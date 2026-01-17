package generatedresumes

import (
	"context"
	"sort"
	"sync"
)

// MemoryRepo stores generated resumes in memory and is safe for concurrent use.
type MemoryRepo struct {
	mu     sync.RWMutex
	byID   map[string]GeneratedResume
	byUser map[string][]GeneratedResume
}

// NewMemoryRepo constructs a MemoryRepo.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		byID:   make(map[string]GeneratedResume),
		byUser: make(map[string][]GeneratedResume),
	}
}

// Create stores the generated resume.
func (r *MemoryRepo) Create(ctx context.Context, resume GeneratedResume) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[resume.ID] = resume
	r.byUser[resume.UserID] = append(r.byUser[resume.UserID], resume)
	return nil
}

// GetByID returns a generated resume by ID for a user.
func (r *MemoryRepo) GetByID(ctx context.Context, userID, generatedResumeID string) (GeneratedResume, error) {
	if err := ctx.Err(); err != nil {
		return GeneratedResume{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	resume, ok := r.byID[generatedResumeID]
	if !ok {
		return GeneratedResume{}, ErrNotFound
	}
	if resume.UserID != userID {
		return GeneratedResume{}, ErrForbidden
	}
	return resume, nil
}

// ListByUser returns generated resumes for a user, newest first, with limit/offset.
func (r *MemoryRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]GeneratedResume, error) {
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
	userResumes := r.byUser[userID]
	r.mu.RUnlock()

	if len(userResumes) == 0 || offset >= len(userResumes) {
		return []GeneratedResume{}, nil
	}

	resumes := make([]GeneratedResume, len(userResumes))
	copy(resumes, userResumes)
	sort.Slice(resumes, func(i, j int) bool {
		return resumes[i].CreatedAt.After(resumes[j].CreatedAt)
	})

	end := len(resumes)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return resumes[offset:end], nil
}
