package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/services/health"
)

func registerRoutes(r *gin.Engine, healthSvc *health.Service) {
	api := r.Group("/api/v1")
	api.Use(authMiddleware())

	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, healthSvc.Status())
	})
}
