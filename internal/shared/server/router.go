package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/analyses"
	"resume-backend/internal/applies"
	googleauth "resume-backend/internal/auth"
	"resume-backend/internal/documents"
	"resume-backend/internal/generatedresumes"
	"resume-backend/internal/llm"
	openai "resume-backend/internal/llm/openai"
	"resume-backend/internal/shared/config"
	"resume-backend/internal/shared/metrics"
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

	r.GET("/metrics", metrics.Handler())

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
	var analysisRepo analyses.Repo
	if sqlDB != nil {
		analysisRepo = &analyses.PGRepo{DB: sqlDB}
	} else {
		analysisRepo = analyses.NewMemoryRepo()
	}
	var generatedResumeRepo generatedresumes.Repo
	if sqlDB != nil {
		generatedResumeRepo = &generatedresumes.PGRepo{DB: sqlDB}
	} else {
		generatedResumeRepo = generatedresumes.NewMemoryRepo()
	}
	llmClient := llm.Client(llm.PlaceholderClient{})
	if cfg.LLMProvider == "openai" {
		openaiClient, err := openai.NewClient(os.Getenv("OPENAI_API_KEY"), cfg.LLMModel)
		if err != nil {
			log.Fatalf("failed to initialize openai client: %v", err)
		}
		llmClient = openaiClient
	}
	applyLLMClient := applies.LLMClient(promptPlaceholder{})
	if cfg.LLMProvider == "openai" {
		promptClient, err := openai.NewPromptClient(os.Getenv("OPENAI_API_KEY"), cfg.LLMModel)
		if err != nil {
			log.Fatalf("failed to initialize openai prompt client: %v", err)
		}
		applyLLMClient = promptClient
	}

	analysisSvc := &analyses.Service{
		Repo:            analysisRepo,
		Usage:           usageSvc,
		DocRepo:         docRepo,
		Store:           store,
		LLM:             llmClient,
		Provider:        cfg.LLMProvider,
		Model:           cfg.LLMModel,
		AnalysisVersion: cfg.AnalysisVersion,
	}
	analysisHandler := analyses.NewHandler(analysisSvc, docRepo)
	generatedResumeSvc := &generatedresumes.Service{
		Repo:         generatedResumeRepo,
		AnalysisRepo: analysisAdapter{repo: analysisRepo},
		DocRepo:      docRepo,
		Store:        store,
	}
	usageHandler := usage.NewHandler(usageSvc, analysisAdapter{repo: analysisRepo}, docRepo, store, generatedResumeSvc)
	applySvc := &applies.Service{
		AnalysisRepo:  analysisRepo,
		DocumentsRepo: docRepo,
		GeneratedRepo: generatedResumeRepo,
		Store:         store,
		LLM:           applyLLMClient,
	}
	applyHandler := applies.NewHandler(applySvc, generatedResumeRepo, store)
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
	applyHandler.RegisterRoutes(api)
	if cfg.Env == "dev" {
		dev := api.Group("/dev")
		usageHandler.RegisterDevRoutes(dev)
	}

	return r
}

type analysisAdapter struct {
	repo analyses.Repo
}

func (a analysisAdapter) GetByID(ctx context.Context, analysisID string) (usage.AnalysisRecord, error) {
	analysis, err := a.repo.GetByID(ctx, analysisID)
	if err != nil {
		if errors.Is(err, analyses.ErrNotFound) {
			return usage.AnalysisRecord{}, usage.ErrAnalysisNotFound
		}
		return usage.AnalysisRecord{}, err
	}
	return usage.AnalysisRecord{
		ID:         analysis.ID,
		UserID:     analysis.UserID,
		DocumentID: analysis.DocumentID,
		Status:     analysis.Status,
		Result:     analysis.Result,
	}, nil
}

func (a analysisAdapter) GetAnalysisByID(ctx context.Context, analysisID string) (generatedresumes.AnalysisRecord, error) {
	analysis, err := a.repo.GetByID(ctx, analysisID)
	if err != nil {
		if errors.Is(err, analyses.ErrNotFound) {
			return generatedresumes.AnalysisRecord{}, generatedresumes.ErrNotFound
		}
		return generatedresumes.AnalysisRecord{}, err
	}
	return generatedresumes.AnalysisRecord{
		ID:         analysis.ID,
		UserID:     analysis.UserID,
		DocumentID: analysis.DocumentID,
		Status:     analysis.Status,
		Result:     analysis.Result,
	}, nil
}

type promptPlaceholder struct{}

func (promptPlaceholder) Complete(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return "", errors.New("llm prompt client not configured")
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
