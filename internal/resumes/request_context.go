package resumes

import "context"

type requestIDContextKey struct{}

type requestUserIDContextKey struct{}

type generationJobIDContextKey struct{}

func withRequestLogContext(ctx context.Context, requestID, userID string) context.Context {
	ctx = context.WithValue(ctx, requestIDContextKey{}, requestID)
	return context.WithValue(ctx, requestUserIDContextKey{}, userID)
}

// WithRequestLogContext attaches request and user identifiers for resume generation telemetry.
func WithRequestLogContext(ctx context.Context, requestID, userID string) context.Context {
	return withRequestLogContext(ctx, requestID, userID)
}

func withGenerationJobID(ctx context.Context, generationJobID string) context.Context {
	return context.WithValue(ctx, generationJobIDContextKey{}, generationJobID)
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func requestUserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	userID, _ := ctx.Value(requestUserIDContextKey{}).(string)
	return userID
}

func generationJobIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	generationJobID, _ := ctx.Value(generationJobIDContextKey{}).(string)
	return generationJobID
}
