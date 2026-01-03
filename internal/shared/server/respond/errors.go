package respond

import (
	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/telemetry"
)

// ErrorBody defines the standardized error object.
type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// ErrorResponse wraps the error body.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// Error sends a standardized error response.
func Error(c *gin.Context, status int, code, message string, details interface{}) {
	fields := map[string]any{
		"status":     status,
		"code":       code,
		"message":    message,
		"path":       c.Request.URL.Path,
		"method":     c.Request.Method,
		"request_id": c.GetString("requestId"),
	}
	if userID := c.GetString("userId"); userID != "" {
		fields["user_id"] = userID
	}
	if isGuest, ok := c.Get("isGuest"); ok {
		fields["is_guest"] = isGuest
	}
	telemetry.Error("http.error", fields)

	c.AbortWithStatusJSON(status, ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
