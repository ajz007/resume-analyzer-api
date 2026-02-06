package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/auth"
	"resume-backend/internal/shared/server/respond"
)

const (
	userIDKey      = "userId"
	userEmailKey   = "userEmail"
	userNameKey    = "userName"
	userPictureKey = "userPicture"
)

// Auth validates JWTs or guest headers and stores identity in context.
func Auth(env string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			return
		}

		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/v1/auth/google/") {
			c.Next()
			return
		}

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))

		if authHeader != "" {
			if !strings.HasPrefix(authHeader, "Bearer ") {
				respond.Error(c, http.StatusUnauthorized, "unauthorized", "missing or invalid token", nil)
				return
			}

			token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
			if token == "" {
				respond.Error(c, http.StatusUnauthorized, "unauthorized", "missing or invalid token", nil)
				return
			}

			claims, err := auth.VerifyJWT(token)
			if err != nil {
				respond.Error(c, http.StatusUnauthorized, "unauthorized", "missing or invalid token", nil)
				return
			}

			c.Set(userIDKey, claims.Sub)
			if claims.Email != "" {
				c.Set(userEmailKey, claims.Email)
			}
			if claims.Name != "" {
				c.Set(userNameKey, claims.Name)
			}
			if claims.Picture != "" {
				c.Set(userPictureKey, claims.Picture)
			}
			c.Set("isGuest", false)
			c.Next()
			return
		}

		guestID := strings.TrimSpace(c.GetHeader("X-Guest-Id"))
		if guestID == "" {
			respond.Error(c, http.StatusUnauthorized, "unauthorized", "Missing identity", nil)
			return
		}

		c.Set(userIDKey, "guest:"+guestID)
		c.Set("isGuest", true)
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

// UserEmailFromContext fetches the user email set by the auth middleware.
func UserEmailFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	val, _ := c.Get(userEmailKey)
	if email, ok := val.(string); ok {
		return email
	}
	return ""
}

// UserNameFromContext fetches the user name set by the auth middleware.
func UserNameFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	val, _ := c.Get(userNameKey)
	if name, ok := val.(string); ok {
		return name
	}
	return ""
}

// UserPictureFromContext fetches the user picture set by the auth middleware.
func UserPictureFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	val, _ := c.Get(userPictureKey)
	if picture, ok := val.(string); ok {
		return picture
	}
	return ""
}
