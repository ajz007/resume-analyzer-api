package users

import "context"

var ErrNotFound = errNotFound{}

type errNotFound struct{}

func (errNotFound) Error() string { return "user not found" }

type Repo interface {
	Upsert(ctx context.Context, user User) error
	GetByID(ctx context.Context, userID string) (User, error)
}
