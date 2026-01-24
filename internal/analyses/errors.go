package analyses

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrRetryRequired = errors.New("retry required")
)

const (
	ErrorCodeValidation        = "VALIDATION_ERROR"
	ErrorCodeLLMTimeout        = "LLM_TIMEOUT"
	ErrorCodeLLMSchemaMismatch = "LLM_SCHEMA_MISMATCH"
	ErrorCodeStorage           = "STORAGE_ERROR"
	ErrorCodeInternal          = "INTERNAL_ERROR"
)
