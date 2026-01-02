package middleware

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/server/respond"
)

// Recovery recovers from panics and returns a standardized error response.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID := RequestIDFromContext(c)
				log.Printf("panic req_id=%s err=%v", reqID, rec)
				respond.Error(c, http.StatusInternalServerError, "internal_error", "internal server error", nil)
				c.Abort()
			}
		}()
		c.Next()
	}
}
