package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/documents"
	"resume-backend/internal/shared/config"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	localstore "resume-backend/internal/shared/storage/object/local"
)

// NewRouter constructs the Gin engine with middleware and routes registered.
func NewRouter(cfg config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(
		middleware.RequestID(),
		middleware.Logging(),
		middleware.Recovery(),
		middleware.CORS(cfg.CORSAllowOrigin),
		middleware.Auth(),
	)

	// Dependencies
	store := localstore.New(cfg.LocalStoreDir)
	repo := documents.NewMemoryRepo()
	docSvc := &documents.Service{Store: store, Repo: repo}
	docHandler := documents.NewHandler(docSvc)

	api := r.Group("/api/v1")
	api.GET("/health", func(c *gin.Context) {
		respond.JSON(c, http.StatusOK, gin.H{"ok": true})
	})
	docHandler.RegisterRoutes(api)

	return r
}

// Addr normalizes the listen address.
func Addr(port string) string {
	if port == "" {
		return ":8080"
	}
	if port[0] == ':' {
		return port
	}
	return ":" + port
}
