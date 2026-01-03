package usage

import "context"

type store interface {
	Get(ctx context.Context, userID string) (Usage, error)
	EnsurePeriod(ctx context.Context, userID string) (Usage, error)
	Consume(ctx context.Context, userID string, n int) (Usage, error)
	Reset(ctx context.Context, userID string) (Usage, error)
}

// Service manages usage data via an underlying store.
type Service struct {
	store store
}

// NewService constructs a Service with in-memory store.
func NewService() *Service {
	return &Service{store: newMemoryStore()}
}

// NewPostgresService constructs a Service backed by Postgres.
func NewPostgresService(pgStore store) *Service {
	return &Service{store: pgStore}
}

// Get returns the current usage for a user, initializing defaults if absent.
func (s *Service) Get(ctx context.Context, userID string) (Usage, error) {
	return s.store.Get(ctx, userID)
}

// EnsurePeriod resets usage if the period has expired.
func (s *Service) EnsurePeriod(ctx context.Context, userID string) (Usage, error) {
	return s.store.EnsurePeriod(ctx, userID)
}

// CanConsume reports whether the user can consume n units.
func (s *Service) CanConsume(ctx context.Context, userID string, n int) (bool, Usage, error) {
	u, err := s.store.EnsurePeriod(ctx, userID)
	if err != nil {
		return false, Usage{}, err
	}
	if n <= 0 {
		return true, u, nil
	}
	if u.Used+n > u.Limit {
		return false, u, nil
	}
	return true, u, nil
}

// Consume increments usage by n if within limit.
func (s *Service) Consume(ctx context.Context, userID string, n int) (Usage, error) {
	return s.store.Consume(ctx, userID, n)
}

// Reset sets usage to zero and resets the window.
func (s *Service) Reset(ctx context.Context, userID string) (Usage, error) {
	return s.store.Reset(ctx, userID)
}
