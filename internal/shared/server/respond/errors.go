package respond

import "github.com/gin-gonic/gin"

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
	c.AbortWithStatusJSON(status, ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
