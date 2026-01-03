package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/shared/telemetry"
)

// Recovery recovers from panics and returns a standardized error response.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID := RequestIDFromContext(c)
				telemetry.Error("panic", map[string]any{
					"request_id": reqID,
					"error":      rec,
					"stack":      string(debug.Stack()),
					"path":       c.Request.URL.Path,
					"method":     c.Request.Method,
				})
				respond.Error(c, http.StatusInternalServerError, "internal", "Unexpected server error", nil)
				c.Abort()
			}
		}()
		c.Next()
	}
}
