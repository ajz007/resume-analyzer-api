package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
)

// registerMeRoutes attaches the /me endpoint.
func registerMeRoutes(rg *gin.RouterGroup) {
	rg.GET("/me", meHandler)
}

func meHandler(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	if userID == "" {
		respond.Error(c, http.StatusUnauthorized, "unauthorized", "missing or invalid token", nil)
		return
	}

	response := gin.H{
		"userId": userID,
	}
	if email := middleware.UserEmailFromContext(c); email != "" {
		response["email"] = email
	}
	if name := middleware.UserNameFromContext(c); name != "" {
		response["name"] = name
	}
	if picture := middleware.UserPictureFromContext(c); picture != "" {
		response["picture"] = picture
	}

	respond.JSON(c, http.StatusOK, response)
}
