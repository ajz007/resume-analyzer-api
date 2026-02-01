package main

import (
	"context"
	"log"
	"os"
	"strings"

	"resume-backend/internal/queue"
	"resume-backend/internal/shared/config"
	"resume-backend/internal/shared/server"
)

func main() {
	cfg := config.Load()
	var jobQueue queue.Client
	if strings.TrimSpace(os.Getenv("RA_SQS_QUEUE_URL")) != "" {
		sqsClient, err := queue.NewSQSClient(context.Background())
		if err != nil {
			log.Fatalf("failed to initialize sqs queue client: %v", err)
		}
		jobQueue = sqsClient
	}
	r := server.NewRouter(cfg, jobQueue)

	addr := server.Addr(cfg.Port)
	log.Printf("Starting API server on %s", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
