package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// Logging emits a structured log per request.
func Logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		reqID := RequestIDFromContext(c)

		log.Printf("req_id=%s method=%s path=%s status=%d latency=%s", reqID, c.Request.Method, c.Request.URL.Path, status, latency)
	}
}
