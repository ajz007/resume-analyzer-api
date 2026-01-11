package usage

import (
	"context"
	"sync"
	"time"
)

type memoryStore struct {
	mu               sync.RWMutex
	data             map[string]Usage
	applyRuns        map[string]ApplyRun
	documentVersions map[string]DocumentVersion
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		data:             make(map[string]Usage),
		applyRuns:        make(map[string]ApplyRun),
		documentVersions: make(map[string]DocumentVersion),
	}
}

func (s *memoryStore) Get(ctx context.Context, userID string) (Usage, error) {
	if err := ctx.Err(); err != nil {
		return Usage{}, err
	}
	s.mu.RLock()
	u, ok := s.data[userID]
	s.mu.RUnlock()
	if ok {
		return u, nil
	}
	return s.ensure(ctx, userID)
}

func (s *memoryStore) EnsurePeriod(ctx context.Context, userID string) (Usage, error) {
	return s.ensure(ctx, userID)
}

func (s *memoryStore) ensure(ctx context.Context, userID string) (Usage, error) {
	if err := ctx.Err(); err != nil {
		return Usage{}, err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.data[userID]
	if !ok {
		u = defaultUsage()
	}
	if now.After(u.ResetsAt) || now.Equal(u.ResetsAt) {
		u.Used = 0
		u.ResetsAt = now.Add(7 * 24 * time.Hour)
	}
	s.data[userID] = u
	return u, nil
}

func (s *memoryStore) Consume(ctx context.Context, userID string, n int) (Usage, error) {
	if n <= 0 {
		return s.ensure(ctx, userID)
	}
	if err := ctx.Err(); err != nil {
		return Usage{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	u, ok := s.data[userID]
	if !ok {
		u = defaultUsage()
	}
	if now.After(u.ResetsAt) || now.Equal(u.ResetsAt) {
		u.Used = 0
		u.ResetsAt = now.Add(7 * 24 * time.Hour)
	}
	if u.Used+n > u.Limit {
		return Usage{}, ErrLimitReached
	}
	u.Used += n
	s.data[userID] = u
	return u, nil
}

func (s *memoryStore) Reset(ctx context.Context, userID string) (Usage, error) {
	if err := ctx.Err(); err != nil {
		return Usage{}, err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.data[userID]
	if !ok {
		u = defaultUsage()
	}
	u.Used = 0
	u.ResetsAt = now.Add(7 * 24 * time.Hour)
	s.data[userID] = u
	return u, nil
}

func (s *memoryStore) CreateApplyRun(ctx context.Context, run ApplyRun) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyRuns[run.ID] = run
	return nil
}

func (s *memoryStore) GetApplyRun(ctx context.Context, userID, runID string) (ApplyRun, error) {
	if err := ctx.Err(); err != nil {
		return ApplyRun{}, err
	}
	s.mu.RLock()
	run, ok := s.applyRuns[runID]
	s.mu.RUnlock()
	if !ok || run.UserID != userID {
		return ApplyRun{}, ErrApplyRunNotFound
	}
	return run, nil
}

func (s *memoryStore) UpdateApplyRun(ctx context.Context, update ApplyRunUpdate) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.applyRuns[update.ID]
	if !ok || run.UserID != update.UserID {
		return ErrApplyRunNotFound
	}
	run.Status = update.Status
	run.AutoFixesCount = update.AutoFixesCount
	run.SafeRewritesCount = update.SafeRewritesCount
	run.BlockedRewritesCount = update.BlockedRewritesCount
	run.NeedsInputCount = update.NeedsInputCount
	run.PlaceholdersRemaining = update.PlaceholdersRemaining
	run.DocumentVersionID = update.DocumentVersionID
	s.applyRuns[update.ID] = run
	return nil
}

func (s *memoryStore) CreateDocumentVersion(ctx context.Context, version DocumentVersion) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documentVersions[version.ID] = version
	return nil
}
