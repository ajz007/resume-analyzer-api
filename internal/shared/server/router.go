package server

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/analyses"
	googleauth "resume-backend/internal/auth"
	"resume-backend/internal/documents"
	"resume-backend/internal/shared/config"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/shared/storage/db"
	"resume-backend/internal/shared/storage/object"
	localstore "resume-backend/internal/shared/storage/object/local"
	s3store "resume-backend/internal/shared/storage/object/s3"
	"resume-backend/internal/usage"
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
		middleware.Auth(cfg.Env),
	)

	// Dependencies
	var store object.ObjectStore
	switch cfg.ObjectStoreType {
	case "s3":
		if cfg.AWSRegion == "" || cfg.S3Bucket == "" {
			log.Fatal("OBJECT_STORE=s3 requires AWS_REGION and S3_BUCKET")
		}
		s3Store, err := s3store.New(context.Background(), cfg.AWSRegion, cfg.S3Bucket, cfg.S3Prefix, cfg.SSEKMSKeyID)
		if err != nil {
			log.Fatalf("failed to initialize s3 object store: %v", err)
		}
		store = s3Store
	default:
		store = localstore.New(cfg.LocalStoreDir)
	}
	var sqlDB *sql.DB
	if cfg.DatabaseURL != "" {
		dbConn, err := db.Connect(context.Background(), cfg.DatabaseURL)
		if err != nil {
			log.Printf("failed to connect database, falling back to memory: %v", err)
		} else {
			if err := db.RunMigrations(context.Background(), dbConn); err != nil {
				log.Printf("failed to run migrations, falling back to memory: %v", err)
				dbConn = nil
			}
		}
		sqlDB = dbConn
	}

	var docRepo documents.DocumentsRepo
	if sqlDB != nil {
		docRepo = &documents.PGRepo{DB: sqlDB}
	} else {
		docRepo = documents.NewMemoryRepo()
	}
	docSvc := &documents.Service{Store: store, Repo: docRepo}
	docHandler := documents.NewHandler(docSvc)
	var usageSvc *usage.Service
	if sqlDB != nil {
		usageSvc = usage.NewPostgresService(usage.NewPGStore(sqlDB))
	} else {
		usageSvc = usage.NewService()
	}
	usageHandler := usage.NewHandler(usageSvc)
	var analysisRepo analyses.Repo
	if sqlDB != nil {
		analysisRepo = &analyses.PGRepo{DB: sqlDB}
	} else {
		analysisRepo = analyses.NewMemoryRepo()
	}
	analysisSvc := &analyses.Service{
		Repo:     analysisRepo,
		Usage:    usageSvc,
		Provider: cfg.LLMProvider,
		Model:    cfg.LLMModel,
	}
	analysisHandler := analyses.NewHandler(analysisSvc, docRepo)
	googleAuthSvc := googleauth.NewGoogleService(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL, cfg.UIRedirectURL)

	api := r.Group("/api/v1")
	api.GET("/health", func(c *gin.Context) {
		respond.JSON(c, http.StatusOK, gin.H{"ok": true})
	})
	googleAuthSvc.RegisterRoutes(api)
	registerMeRoutes(api)
	docHandler.RegisterRoutes(api)
	analysisHandler.RegisterRoutes(api)
	usageHandler.RegisterRoutes(api)
	if cfg.Env == "dev" {
		dev := api.Group("/dev")
		usageHandler.RegisterDevRoutes(dev)
	}

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
