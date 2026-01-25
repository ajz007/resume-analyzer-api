package users

import (
	"context"
	"errors"
	"strings"
)

type Service struct {
	Repo Repo
}

func NewService(repo Repo) *Service {
	return &Service{Repo: repo}
}

// UpsertFromAuth persists the user identity from OAuth to stabilize history and usage ownership.
func (s *Service) UpsertFromAuth(ctx context.Context, user User) error {
	if s == nil || s.Repo == nil {
		return errors.New("users service not configured")
	}
	if strings.TrimSpace(user.ID) == "" || strings.TrimSpace(user.Email) == "" {
		return errors.New("user id and email are required")
	}
	return s.Repo.Upsert(ctx, user)
}

func (s *Service) GetByID(ctx context.Context, userID string) (User, error) {
	if s == nil || s.Repo == nil {
		return User{}, errors.New("users service not configured")
	}
	if strings.TrimSpace(userID) == "" {
		return User{}, errors.New("user id is required")
	}
	return s.Repo.GetByID(ctx, userID)
}
