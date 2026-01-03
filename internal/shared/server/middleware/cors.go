package middleware

import (
	"strings"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS sets CORS headers and handles preflight requests.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	origins := make(map[string]struct{})
	for _, o := range allowedOrigins {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			origins[trimmed] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := origins[origin]; ok {
				h := c.Writer.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Vary", "Origin")
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Guest-Id, X-User-Id, X-Request-Id")
				h.Set("Access-Control-Expose-Headers", "X-Request-Id")
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}

		c.Next()
	}
}
