package users

import (
	"context"
	"sync"
	"time"
)

type MemoryRepo struct {
	mu    sync.RWMutex
	users map[string]User
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{users: make(map[string]User)}
}

func (r *MemoryRepo) Upsert(ctx context.Context, user User) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.users[user.ID]
	now := time.Now().UTC()
	if !ok {
		user.CreatedAt = now
	} else {
		user.CreatedAt = existing.CreatedAt
	}
	user.UpdatedAt = now
	r.users[user.ID] = user
	return nil
}

func (r *MemoryRepo) GetByID(ctx context.Context, userID string) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	user, ok := r.users[userID]
	if !ok {
		return User{}, ErrNotFound
	}
	return user, nil
}
