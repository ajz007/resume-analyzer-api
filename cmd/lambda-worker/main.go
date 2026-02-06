package main

// Build the Lambda handler binary:
//   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap ./cmd/lambda-worker

import (
	"context"
	"log"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"resume-backend/internal/bootstrap"
	"resume-backend/internal/shared/config"
	"resume-backend/internal/workerproc"
)

var (
	initOnce sync.Once
	initErr  error
	app      *bootstrap.App
)

func initApp() {
	cfg := config.Load()
	built, err := bootstrap.Build(cfg)
	if err != nil {
		initErr = err
		return
	}
	app = built
}

func handler(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	initOnce.Do(initApp)
	if initErr != nil {
		log.Printf("bootstrap error: %v", initErr)
		failures := make([]events.SQSBatchItemFailure, 0, len(event.Records))
		for _, record := range event.Records {
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
		}
		return events.SQSEventResponse{BatchItemFailures: failures}, initErr
	}

	failures := make([]events.SQSBatchItemFailure, 0)
	for _, record := range event.Records {
		if err := workerproc.HandleMessage(ctx, app, record.Body); err != nil {
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
		}
	}

	return events.SQSEventResponse{BatchItemFailures: failures}, nil
}

func main() {
	lambda.Start(handler)
}
