package generatedresumes

import "errors"

var (
	// ErrNotFound indicates an entity was not found.
	ErrNotFound = errors.New("not found")

	// ErrInvalidInput indicates validation or bad input.
	ErrInvalidInput = errors.New("invalid input")

	// ErrForbidden indicates access is not allowed.
	ErrForbidden = errors.New("forbidden")
)
