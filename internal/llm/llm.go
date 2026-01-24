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
type extraSystemKey struct{}
type promptHashKey struct{}

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

// WithExtraSystemMessage returns a context that adds an extra system message to the prompt.
func WithExtraSystemMessage(ctx context.Context, message string) context.Context {
	return context.WithValue(ctx, extraSystemKey{}, message)
}

// ExtraSystemMessageFromContext returns the extra system message, if any.
func ExtraSystemMessageFromContext(ctx context.Context) (string, bool) {
	val := ctx.Value(extraSystemKey{})
	msg, ok := val.(string)
	return msg, ok
}

// WithPromptHashCapture attaches a sink for the prompt hash computed by the LLM client.
func WithPromptHashCapture(ctx context.Context, out *string) context.Context {
	return context.WithValue(ctx, promptHashKey{}, out)
}

// PromptHashSinkFromContext returns the prompt hash sink, if any.
func PromptHashSinkFromContext(ctx context.Context) (*string, bool) {
	val := ctx.Value(promptHashKey{})
	ptr, ok := val.(*string)
	return ptr, ok
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
