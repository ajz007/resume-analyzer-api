package respond

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// JSON writes a JSON response with the given status.
func JSON(c *gin.Context, status int, payload interface{}) {
	c.JSON(status, payload)
}

// OK writes a 200 OK JSON response.
func OK(c *gin.Context, payload interface{}) {
	JSON(c, http.StatusOK, payload)
}
