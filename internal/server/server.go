package server

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/config"
	"resume-backend/internal/services/health"
)

// NewEngine builds the gin engine with routes registered.
func NewEngine(cfg config.Config, healthSvc *health.Service) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())

	registerRoutes(engine, healthSvc)
	return engine
}

// Addr returns a normalized listen address for the given port.
func Addr(port string) string {
	if port == "" {
		return ":8080"
	}
	if port[0] == ':' {
		return port
	}
	return fmt.Sprintf(":%s", port)
}
