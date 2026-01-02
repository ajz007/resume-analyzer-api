package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
)

const requestIDKey = "requestId"

// RequestID attaches a request ID to context and response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			id = generateRequestID()
		}
		c.Set(requestIDKey, id)
		c.Writer.Header().Set("X-Request-Id", id)
		c.Next()
	}
}

// RequestIDFromContext fetches the request ID stored by RequestID middleware.
func RequestIDFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	val, _ := c.Get(requestIDKey)
	if id, ok := val.(string); ok {
		return id
	}
	return ""
}

func generateRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
