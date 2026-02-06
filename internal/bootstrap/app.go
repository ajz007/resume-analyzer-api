package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"

	"resume-backend/internal/account"
	"resume-backend/internal/analyses"
	"resume-backend/internal/applies"
	googleauth "resume-backend/internal/auth"
	"resume-backend/internal/documents"
	"resume-backend/internal/generatedresumes"
	"resume-backend/internal/llm"
	openai "resume-backend/internal/llm/openai"
	"resume-backend/internal/queue"
	"resume-backend/internal/shared/config"
	"resume-backend/internal/shared/server"
	"resume-backend/internal/shared/storage/db"
	"resume-backend/internal/shared/storage/object"
	localstore "resume-backend/internal/shared/storage/object/local"
	s3store "resume-backend/internal/shared/storage/object/s3"
	"resume-backend/internal/usage"
	"resume-backend/internal/users"
)

const (
	uploadsDefaultRegion = "us-east-1"
	uploadsDefaultPrefix = "documents/"
)

// App holds shared dependencies. Router is intentionally left nil for now.
type App struct {
	Config                  config.Config
	Router                  *gin.Engine
	DB                      *sql.DB
	Store                   object.ObjectStore
	Queue                   queue.Client
	UploadsPresign          *s3.PresignClient
	UploadsBucket           string
	UploadsPrefix           string
	DocumentsRepo           documents.DocumentsRepo
	AnalysesRepo            analyses.Repo
	GeneratedResumesRepo    generatedresumes.Repo
	UsersRepo               users.Repo
	DocumentsService        *documents.Service
	UsageService            *usage.Service
	AnalysesService         *analyses.Service
	AnalysisProcessor       AnalysisProcessor
	GeneratedResumesService *generatedresumes.Service
	ApplyService            *applies.Service
	AccountService          *account.Service
	UsersService            *users.Service
	DocumentsHandler        *documents.Handler
	AnalysisHandler         *analyses.Handler
	ApplyHandler            *applies.Handler
	AccountHandler          *account.Handler
	UsageHandler            *usage.Handler
	UsersHandler            *users.Handler
	GoogleAuth              *googleauth.GoogleService
	Services                map[string]any
}

// AnalysisProcessor allows callers to override analysis processing for tests.
type AnalysisProcessor interface {
	ProcessAnalysis(ctx context.Context, analysisID string) error
}

// Build prepares shared dependencies without wiring routes.
func Build(cfg config.Config) (*App, error) {
	if strings.TrimSpace(cfg.Env) == "" {
		cfg.Env = "dev"
	}
	if strings.TrimSpace(cfg.ObjectStoreType) == "" {
		cfg.ObjectStoreType = "local"
	}
	ctx := context.Background()

	sqlDB, err := buildDB(ctx, cfg)
	if err != nil {
		return nil, err
	}

	store, err := buildStore(ctx, cfg)
	if err != nil {
		return nil, err
	}

	queueClient, err := buildQueue(ctx)
	if err != nil {
		return nil, err
	}

	presign, bucket, prefix, err := buildUploadsPresign(ctx)
	if err != nil {
		return nil, err
	}

	app := &App{
		Config:         cfg,
		Router:         nil,
		DB:             sqlDB,
		Store:          store,
		Queue:          queueClient,
		UploadsPresign: presign,
		UploadsBucket:  bucket,
		UploadsPrefix:  prefix,
		Services:       map[string]any{},
	}

	if err := buildServices(app); err != nil {
		return nil, err
	}

	app.Router = server.NewRouter(server.RouterDeps{
		Config:          app.Config,
		AccountHandler:  app.AccountHandler,
		AnalysisHandler: app.AnalysisHandler,
		ApplyHandler:    app.ApplyHandler,
		DocumentHandler: app.DocumentsHandler,
		UsageHandler:    app.UsageHandler,
		UserHandler:     app.UsersHandler,
		GoogleAuth:      app.GoogleAuth,
	})

	return app, nil
}

func buildDB(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		if isDevLike(cfg.Env) {
			log.Printf("bootstrap: DATABASE_URL empty; using in-memory repositories")
			return nil, nil
		}
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	var (
		sqlDB *sql.DB
		err   error
	)
	if db.IsLambdaRuntime() {
		opts := db.OptionsFromEnv(db.DefaultLambdaOptions())
		sqlDB, err = db.GetSingleton(ctx, cfg.DatabaseURL, opts)
	} else {
		opts := db.OptionsFromEnv(db.DefaultServerOptions())
		sqlDB, err = db.Connect(ctx, cfg.DatabaseURL, opts)
	}
	if err != nil {
		if isDevLike(cfg.Env) {
			log.Printf("bootstrap: database connect failed; using in-memory repositories: %v", err)
			return nil, nil
		}
		return nil, err
	}

	return sqlDB, nil
}

func buildStore(ctx context.Context, cfg config.Config) (object.ObjectStore, error) {
	switch cfg.ObjectStoreType {
	case "s3":
		// if strings.TrimSpace(cfg.AWSRegion) == "" || strings.TrimSpace(cfg.S3Bucket) == "" {
		// 	return nil, fmt.Errorf("OBJECT_STORE=s3 requires AWS_REGION and S3_BUCKET")
		// }
		return s3store.New(ctx, cfg.AWSRegion, cfg.S3Bucket, cfg.S3Prefix, cfg.SSEKMSKeyID)
	default:
		return localstore.New(cfg.LocalStoreDir), nil
	}
}

func buildQueue(ctx context.Context) (queue.Client, error) {
	if strings.TrimSpace(os.Getenv("RA_SQS_QUEUE_URL")) == "" {
		return nil, nil
	}
	return queue.NewSQSClient(ctx)
}

func isDevLike(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "dev", "local":
		return true
	default:
		return false
	}
}

func buildUploadsPresign(ctx context.Context) (*s3.PresignClient, string, string, error) {
	bucket := strings.TrimSpace(os.Getenv("UPLOADS_S3_BUCKET"))
	if bucket == "" {
		return nil, "", "", nil
	}

	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = uploadsDefaultRegion
	}
	prefix := strings.TrimSpace(os.Getenv("UPLOADS_S3_PREFIX"))
	if prefix == "" {
		prefix = uploadsDefaultPrefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, "", "", fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg)
	return s3.NewPresignClient(client), bucket, prefix, nil
}

func buildServices(app *App) error {
	var docRepo documents.DocumentsRepo
	var analysisRepo analyses.Repo
	var generatedResumeRepo generatedresumes.Repo
	var userRepo users.Repo

	if app.DB != nil {
		docRepo = &documents.PGRepo{DB: app.DB}
		analysisRepo = &analyses.PGRepo{DB: app.DB}
		generatedResumeRepo = &generatedresumes.PGRepo{DB: app.DB}
		userRepo = &users.PGRepo{DB: app.DB}
	} else {
		docRepo = documents.NewMemoryRepo()
		analysisRepo = analyses.NewMemoryRepo()
		generatedResumeRepo = generatedresumes.NewMemoryRepo()
		userRepo = users.NewMemoryRepo()
	}

	docSvc := &documents.Service{
		Store:           app.Store,
		Repo:            docRepo,
		StorageProvider: app.Config.ObjectStoreType,
	}

	var usageSvc *usage.Service
	if app.DB != nil {
		usageSvc = usage.NewPostgresService(usage.NewPGStore(app.DB))
	} else {
		usageSvc = usage.NewService()
	}

	llmClient := llm.Client(llm.PlaceholderClient{})
	if app.Config.LLMProvider == "openai" {
		openaiClient, err := openai.NewClient(os.Getenv("OPENAI_API_KEY"), app.Config.LLMModel)
		if err != nil {
			return err
		}
		llmClient = openaiClient
	}

	applyLLMClient := applies.LLMClient(promptPlaceholder{})
	if app.Config.LLMProvider == "openai" {
		promptClient, err := openai.NewPromptClient(os.Getenv("OPENAI_API_KEY"), app.Config.LLMModel)
		if err != nil {
			return err
		}
		applyLLMClient = promptClient
	}

	analysisSvc := &analyses.Service{
		Repo:            analysisRepo,
		Usage:           usageSvc,
		DocRepo:         docRepo,
		Store:           app.Store,
		LLM:             llmClient,
		JobQueue:        app.Queue,
		Provider:        app.Config.LLMProvider,
		Model:           app.Config.LLMModel,
		AnalysisVersion: app.Config.AnalysisVersion,
	}

	analysisAdapter := analysisAdapter{repo: analysisRepo}
	generatedResumeSvc := &generatedresumes.Service{
		Repo:         generatedResumeRepo,
		AnalysisRepo: analysisAdapter,
		DocRepo:      docRepo,
		Store:        app.Store,
	}

	usageHandler := usage.NewHandler(usageSvc, analysisAdapter, docRepo, app.Store, generatedResumeSvc)
	applySvc := &applies.Service{
		AnalysisRepo:  analysisRepo,
		DocumentsRepo: docRepo,
		GeneratedRepo: generatedResumeRepo,
		Store:         app.Store,
		LLM:           applyLLMClient,
	}

	userSvc := users.NewService(userRepo)
	googleAuthSvc := googleauth.NewGoogleService(
		app.Config.GoogleClientID,
		app.Config.GoogleClientSecret,
		app.Config.GoogleRedirectURL,
		app.Config.UIRedirectURL,
		userSvc,
	)

	app.DocumentsRepo = docRepo
	app.AnalysesRepo = analysisRepo
	app.GeneratedResumesRepo = generatedResumeRepo
	app.UsersRepo = userRepo
	app.DocumentsService = docSvc
	app.UsageService = usageSvc
	app.AnalysesService = analysisSvc
	app.AnalysisProcessor = analysisSvc
	app.GeneratedResumesService = generatedResumeSvc
	app.ApplyService = applySvc
	app.AccountService = account.NewService(docRepo, analysisRepo)
	app.UsersService = userSvc
	app.DocumentsHandler = documents.NewHandler(docSvc)
	app.AnalysisHandler = analyses.NewHandler(analysisSvc, docRepo)
	app.ApplyHandler = applies.NewHandler(applySvc, generatedResumeRepo, app.Store)
	app.AccountHandler = account.NewHandler(app.AccountService)
	app.UsageHandler = usageHandler
	app.UsersHandler = users.NewHandler(userSvc)
	app.GoogleAuth = googleAuthSvc

	if app.DocumentsHandler == nil || app.AnalysisHandler == nil || app.UsageHandler == nil {
		return errors.New("failed to initialize handlers")
	}

	return nil
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
