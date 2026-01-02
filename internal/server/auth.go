package server

import (
	"context"

	"github.com/gin-gonic/gin"
)

type ctxKey string

const userIDKey ctxKey = "user_id"

// authMiddleware ensures a user ID is present in the request context.
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-Id")
		if userID == "" {
			userID = "demo"
		}

		// Store in gin context for handlers that access it directly.
		c.Set(string(userIDKey), userID)

		// Store in request context for downstream services.
		ctx := context.WithValue(c.Request.Context(), userIDKey, userID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// UserIDFromContext fetches the user ID from a context populated by authMiddleware.
func UserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}
