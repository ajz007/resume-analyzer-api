package analyses

import "context"

type requestIDKey struct{}

func withRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil || requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// WithRequestID attaches a request ID to the context for logging.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return withRequestID(ctx, requestID)
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

func backgroundWithRequestID(ctx context.Context) context.Context {
	requestID := requestIDFromContext(ctx)
	if requestID == "" {
		return context.Background()
	}
	return withRequestID(context.Background(), requestID)
}
