package aimeter

import (
	"context"
	"sync/atomic"

	"github.com/google/uuid"
)

type scopeKey struct{}
type scope struct {
	ID, Kind, AnalysisID, ObjectID, Stage, Retry string
	Counter                                      *atomic.Int64
}

// WithOperation attaches internal identifiers only. No user-provided text belongs here.
func WithOperation(ctx context.Context, kind, analysisID, objectID string) context.Context {
	switch kind {
	case "analysis", "review_answer", "resume_generation", "resume_tailoring", "apply":
	default:
		kind = "unattributed"
	}
	return context.WithValue(ctx, scopeKey{}, scope{ID: uuid.NewString(), Kind: kind, AnalysisID: validID(analysisID), ObjectID: validID(objectID), Stage: "primary", Counter: &atomic.Int64{}})
}
func validID(id string) string {
	if _, err := uuid.Parse(id); err != nil {
		return ""
	}
	return id
}
func WithStage(ctx context.Context, stage string) context.Context {
	s, ok := ctx.Value(scopeKey{}).(scope)
	if !ok {
		return ctx
	}
	switch stage {
	case "primary", "json_repair", "validation_repair":
	default:
		stage = "primary"
	}
	s.Stage = stage
	return context.WithValue(ctx, scopeKey{}, s)
}
func WithRetry(ctx context.Context, retry string) context.Context {
	s, ok := ctx.Value(scopeKey{}).(scope)
	if !ok {
		return ctx
	}
	switch retry {
	case "transient", "parameter", "timeout":
	default:
		retry = ""
	}
	s.Retry = retry
	return context.WithValue(ctx, scopeKey{}, s)
}
func nextScope(ctx context.Context) (scope, int64) {
	s, ok := ctx.Value(scopeKey{}).(scope)
	if !ok {
		s = scope{ID: uuid.NewString(), Kind: "unattributed", Stage: "primary", Counter: &atomic.Int64{}}
	}
	return s, s.Counter.Add(1)
}

func EnsureOperation(ctx context.Context) context.Context {
	if _, ok := ctx.Value(scopeKey{}).(scope); ok {
		return ctx
	}
	return WithOperation(ctx, "unattributed", "", "")
}
