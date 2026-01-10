package llm

import (
	"context"
	"encoding/json"
	"errors"
)

// Client abstracts LLM providers for resume analysis.
type Client interface {
	AnalyzeResume(ctx context.Context, input AnalyzeInput) (json.RawMessage, error)
}

// AnalyzeInput captures the inputs needed for resume analysis.
type AnalyzeInput struct {
	ResumeText     string
	JobDescription string
	PromptVersion  string
	TargetRole     string
}

type fixJSONKey struct{}

// WithFixJSON returns a context signaling a fix-JSON retry with the given raw output.
func WithFixJSON(ctx context.Context, raw string) context.Context {
	return context.WithValue(ctx, fixJSONKey{}, raw)
}

// FixJSONFromContext returns the raw JSON to repair, if any.
func FixJSONFromContext(ctx context.Context) (string, bool) {
	val := ctx.Value(fixJSONKey{})
	raw, ok := val.(string)
	return raw, ok
}

// ErrNotImplemented is returned by the placeholder client.
var ErrNotImplemented = errors.New("LLM not implemented")

// PlaceholderClient is a stub implementation until provider wiring is added.
type PlaceholderClient struct{}

// AnalyzeResume returns ErrNotImplemented.
func (PlaceholderClient) AnalyzeResume(ctx context.Context, input AnalyzeInput) (json.RawMessage, error) {
	_ = ctx
	_ = input
	return nil, ErrNotImplemented
}
