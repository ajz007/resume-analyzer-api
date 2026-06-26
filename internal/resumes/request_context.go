package resumes

import "context"

type requestIDContextKey struct{}

type requestUserIDContextKey struct{}

func withRequestLogContext(ctx context.Context, requestID, userID string) context.Context {
	ctx = context.WithValue(ctx, requestIDContextKey{}, requestID)
	return context.WithValue(ctx, requestUserIDContextKey{}, userID)
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
