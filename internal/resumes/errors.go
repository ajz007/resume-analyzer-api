package resumes

import (
	"errors"

	modelv1 "resume-backend/resume/modelv1"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
	ErrForbidden    = errors.New("forbidden")
)

type ValidationError struct {
	Errors []modelv1.ValidationError
}

func (e ValidationError) Error() string {
	return "resume validation failed"
}
