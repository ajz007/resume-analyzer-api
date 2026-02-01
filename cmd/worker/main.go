package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"resume-backend/internal/analyses"
	"resume-backend/internal/documents"
	"resume-backend/internal/llm"
	openai "resume-backend/internal/llm/openai"
	"resume-backend/internal/queue"
	"resume-backend/internal/shared/config"
	"resume-backend/internal/shared/metrics"
	"resume-backend/internal/shared/storage/db"
	"resume-backend/internal/shared/storage/object"
	localstore "resume-backend/internal/shared/storage/object/local"
	s3store "resume-backend/internal/shared/storage/object/s3"
	"resume-backend/internal/shared/telemetry"
)

const (
	sqsRegion                 = "us-east-1"
	defaultVisibilitySeconds  = 1200
	defaultWorkerConcurrency  = 4
	defaultShutdownTimeoutSec = 30
)

func main() {
	cfg := config.Load()

	queueURL := strings.TrimSpace(os.Getenv("RA_SQS_QUEUE_URL"))
	if queueURL == "" {
		log.Fatal("RA_SQS_QUEUE_URL is required")
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	visibilitySeconds := envInt("RA_SQS_VISIBILITY_TIMEOUT_SECONDS", defaultVisibilitySeconds)
	concurrency := envInt("RA_WORKER_CONCURRENCY", defaultWorkerConcurrency)
	shutdownTimeout := time.Duration(envInt("RA_SHUTDOWN_TIMEOUT_SECONDS", defaultShutdownTimeoutSec)) * time.Second

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(sqsRegion))
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	var sqsClient sqsAPI = sqs.NewFromConfig(awsCfg)

	store, err := buildStore(cfg)
	if err != nil {
		log.Fatalf("build store: %v", err)
	}
	dbConn, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	if err := db.RunMigrations(ctx, dbConn); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	docRepo := &documents.PGRepo{DB: dbConn}
	analysisRepo := &analyses.PGRepo{DB: dbConn}
	llmClient, err := buildLLM(cfg)
	if err != nil {
		log.Fatalf("init llm: %v", err)
	}

	analysisSvc := &analyses.Service{
		Repo:            analysisRepo,
		DocRepo:         docRepo,
		Store:           store,
		LLM:             llmClient,
		Provider:        cfg.LLMProvider,
		Model:           cfg.LLMModel,
		AnalysisVersion: cfg.AnalysisVersion,
	}

	sem := make(chan struct{}, max(1, concurrency))
	var wg sync.WaitGroup

	log.Printf("worker started queue=%s concurrency=%d visibility=%ds", queueURL, concurrency, visibilitySeconds)

pollLoop:
	for {
		select {
		case <-ctx.Done():
			break pollLoop
		default:
		}

		resp, err := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
			VisibilityTimeout:   int32(visibilitySeconds),
			AttributeNames:      []sqstypes.QueueAttributeName{sqstypes.QueueAttributeName("ApproximateReceiveCount")},
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				break pollLoop
			}
			log.Printf("receive message: %v", err)
			continue
		}

		for _, msg := range resp.Messages {
			select {
			case <-ctx.Done():
				break pollLoop
			case sem <- struct{}{}:
			}
			metrics.IncAnalysisJobsReceived()
			wg.Add(1)
			go func(m sqstypes.Message) {
				defer wg.Done()
				defer func() { <-sem }()
				handleMessage(ctx, sqsClient, queueURL, analysisSvc, m)
			}(msg)
		}
	}

	log.Printf("shutdown requested, waiting up to %s for in-flight jobs", shutdownTimeout)
	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(shutdownTimeout):
		log.Printf("shutdown timeout reached; exiting with in-flight jobs")
	}
}

type sqsAPI interface {
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

type analysisProcessor interface {
	ProcessAnalysis(ctx context.Context, analysisID string) error
}

func handleMessage(ctx context.Context, client sqsAPI, queueURL string, svc analysisProcessor, msg sqstypes.Message) {
	body := aws.ToString(msg.Body)
	if strings.TrimSpace(body) == "" {
		fields := baseFields(msg, "", "")
		fields["body_len"] = 0
		telemetry.Error("worker.analysis.empty_body", fields)
		if deleteMessage(ctx, client, queueURL, msg, "", "") {
			metrics.IncAnalysisJobsDeletedUnrecoverable()
		}
		return
	}

	decoded, err := queue.DecodeMessage([]byte(body))
	if err != nil {
		fields := baseFields(msg, "", "")
		fields["body_len"] = len(body)
		fields["body_sha256"] = hashBody(body)
		fields["error"] = err.Error()
		telemetry.Error("worker.analysis.decode_failed", fields)
		if deleteMessage(ctx, client, queueURL, msg, "", "") {
			metrics.IncAnalysisJobsDeletedUnrecoverable()
		}
		return
	}

	if strings.TrimSpace(decoded.AnalysisID) == "" {
		fields := baseFields(msg, "", decoded.RequestID)
		fields["body_len"] = len(body)
		fields["body_sha256"] = hashBody(body)
		telemetry.Error("worker.analysis.missing_id", fields)
		if deleteMessage(ctx, client, queueURL, msg, "", decoded.RequestID) {
			metrics.IncAnalysisJobsDeletedUnrecoverable()
		}
		return
	}

	ctxWithRequest := analyses.WithRequestID(ctx, decoded.RequestID)
	telemetry.Info("worker.analysis.received", baseFields(msg, decoded.AnalysisID, decoded.RequestID))

	if err := svc.ProcessAnalysis(ctxWithRequest, decoded.AnalysisID); err != nil {
		fields := baseFields(msg, decoded.AnalysisID, decoded.RequestID)
		fields["error"] = err.Error()
		telemetry.Error("worker.analysis.failed", fields)
		metrics.IncAnalysisJobsFailed()
		return
	}

	if deleteMessage(ctx, client, queueURL, msg, decoded.AnalysisID, decoded.RequestID) {
		telemetry.Info("worker.analysis.completed", baseFields(msg, decoded.AnalysisID, decoded.RequestID))
		metrics.IncAnalysisJobsCompleted()
	}
}

func deleteMessage(ctx context.Context, client sqsAPI, queueURL string, msg sqstypes.Message, analysisID, requestID string) bool {
	receipt := aws.ToString(msg.ReceiptHandle)
	if receipt == "" {
		fields := baseFields(msg, analysisID, requestID)
		fields["error"] = "missing receipt handle"
		telemetry.Error("worker.analysis.delete_failed", fields)
		return false
	}
	if _, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: aws.String(receipt),
	}); err != nil {
		fields := baseFields(msg, analysisID, requestID)
		fields["error"] = err.Error()
		telemetry.Error("worker.analysis.delete_failed", fields)
		return false
	}
	return true
}

func baseFields(msg sqstypes.Message, analysisID, requestID string) map[string]any {
	fields := map[string]any{
		"analysis_id":    analysisID,
		"sqs_message_id": aws.ToString(msg.MessageId),
		"receive_count":  receiveCount(msg),
	}
	if strings.TrimSpace(requestID) != "" {
		fields["request_id"] = requestID
	}
	return fields
}

func receiveCount(msg sqstypes.Message) int {
	if msg.Attributes == nil {
		return 0
	}
	raw := msg.Attributes["ApproximateReceiveCount"]
	if raw == "" {
		return 0
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return parsed
}

func hashBody(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func buildStore(cfg config.Config) (object.ObjectStore, error) {
	switch cfg.ObjectStoreType {
	case "s3":
		if cfg.AWSRegion == "" || cfg.S3Bucket == "" {
			return nil, errors.New("OBJECT_STORE=s3 requires AWS_REGION and S3_BUCKET")
		}
		return s3store.New(context.Background(), cfg.AWSRegion, cfg.S3Bucket, cfg.S3Prefix, cfg.SSEKMSKeyID)
	default:
		return localstore.New(cfg.LocalStoreDir), nil
	}
}

func buildLLM(cfg config.Config) (llm.Client, error) {
	if cfg.LLMProvider == "openai" {
		return openai.NewClient(os.Getenv("OPENAI_API_KEY"), cfg.LLMModel)
	}
	return llm.PlaceholderClient{}, nil
}

func envInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return val
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
