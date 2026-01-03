package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/telemetry"
)

// Logging emits a structured log per request.
func Logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.EqualFold(c.Request.Method, "OPTIONS") {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		reqID := RequestIDFromContext(c)

		userID, _ := c.Get(userIDKey)
		isGuest, _ := c.Get("isGuest")

		telemetry.Info("request.complete", map[string]any{
			"request_id":  reqID,
			"method":      c.Request.Method,
			"path":        c.Request.URL.Path,
			"status":      status,
			"duration_ms": float64(latency.Microseconds()) / 1000.0,
			"user_id":     userID,
			"is_guest":    isGuest,
			"client_ip":   c.ClientIP(),
			"user_agent":  c.Request.UserAgent(),
		})
	}
}
