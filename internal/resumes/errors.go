package resumes

import (
	"errors"

	modelv1 "resume-backend/resume/modelv1"
)

var (
	ErrNotFound              = errors.New("not found")
	ErrAlreadyExists         = errors.New("already exists")
	ErrInvalidInput          = errors.New("invalid input")
	ErrForbidden             = errors.New("forbidden")
	ErrInvalidLLMOutput      = errors.New("invalid llm output")
	ErrGenerationTimeout     = errors.New("resume generation timeout")
	ErrJobQueueNotConfigured = errors.New("job queue not configured")
)

type ValidationError struct {
	Errors []modelv1.ValidationError
}

func (e ValidationError) Error() string {
	return "resume validation failed"
}
