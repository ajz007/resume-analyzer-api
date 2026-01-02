package middleware

import "github.com/gin-gonic/gin"

const userIDKey = "userId"

// Auth populates a user ID from header or defaults to "demo".
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-Id")
		if userID == "" {
			userID = "demo"
		}
		c.Set(userIDKey, userID)
		c.Next()
	}
}

// UserIDFromContext fetches the user ID set by the auth middleware.
func UserIDFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	val, _ := c.Get(userIDKey)
	if id, ok := val.(string); ok {
		return id
	}
	return ""
}
