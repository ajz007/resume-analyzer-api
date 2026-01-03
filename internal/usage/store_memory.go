package usage

import (
	"context"
	"sync"
	"time"
)

type memoryStore struct {
	mu   sync.RWMutex
	data map[string]Usage
}

func newMemoryStore() *memoryStore {
	return &memoryStore{data: make(map[string]Usage)}
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
