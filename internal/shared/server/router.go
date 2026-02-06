package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/account"
	"resume-backend/internal/analyses"
	"resume-backend/internal/applies"
	googleauth "resume-backend/internal/auth"
	"resume-backend/internal/documents"
	"resume-backend/internal/shared/config"
	"resume-backend/internal/shared/metrics"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/uploads"
	"resume-backend/internal/usage"
	"resume-backend/internal/users"
)

// RouterDeps contains prebuilt dependencies for router wiring.
type RouterDeps struct {
	Config          config.Config
	AccountHandler  *account.Handler
	AnalysisHandler *analyses.Handler
	ApplyHandler    *applies.Handler
	DocumentHandler *documents.Handler
	UsageHandler    *usage.Handler
	UserHandler     *users.Handler
	GoogleAuth      *googleauth.GoogleService
}

// NewRouter constructs the Gin engine with middleware and routes registered.
func NewRouter(deps RouterDeps) *gin.Engine {
	cfg := deps.Config
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(
		middleware.RequestID(),
		middleware.Logging(),
		middleware.Recovery(),
		middleware.CORS(cfg.CORSAllowOrigin),
		middleware.Auth(cfg.Env),
		middleware.RateLimit(middleware.RateLimitConfig{
			DefaultGroup: "DEFAULT",
			GroupFor:     rateLimitGroupFor,
			Rules: map[string]middleware.RateLimitRule{
				"DEFAULT": {Rate: 4, Burst: 8},
				"POLLING": {Rate: 12, Burst: 24},
			},
		}),
	)

	r.GET("/metrics", metrics.Handler())

	api := r.Group("/api/v1")
	api.GET("/health", func(c *gin.Context) {
		respond.JSON(c, http.StatusOK, gin.H{"ok": true})
	})
	deps.GoogleAuth.RegisterRoutes(api)
	uploads.RegisterRoutes(api)
	deps.DocumentHandler.RegisterRoutes(api)
	deps.AccountHandler.RegisterRoutes(api)
	deps.AnalysisHandler.RegisterRoutes(api)
	deps.UserHandler.RegisterRoutes(api)
	deps.UsageHandler.RegisterRoutes(api)
	deps.ApplyHandler.RegisterRoutes(api)
	if cfg.Env == "dev" {
		dev := api.Group("/dev")
		deps.UsageHandler.RegisterDevRoutes(dev)
	}

	return r
}

func rateLimitGroupFor(c *gin.Context) string {
	if c == nil {
		return "DEFAULT"
	}
	if strings.ToUpper(c.Request.Method) != http.MethodGet {
		return "DEFAULT"
	}
	switch c.FullPath() {
	case "/api/v1/analyses/:id",
		"/api/v1/documents/:id",
		"/api/v1/documents/:id/status":
		return "POLLING"
	default:
		return "DEFAULT"
	}
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
