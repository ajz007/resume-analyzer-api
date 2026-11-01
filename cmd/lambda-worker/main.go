package main

// Build the Lambda handler binary:
//   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap ./cmd/lambda-worker

import (
	"context"
	"encoding/json"
	"errors"
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

func handleSQS(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
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

// decodeInvocation rejects mixed/unknown event shapes before initializing services.
func decodeInvocation(raw json.RawMessage) (bool, events.SQSEvent, error) {
	var envelope struct {
		Type    string          `json:"type"`
		Records json.RawMessage `json:"Records"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return false, events.SQSEvent{}, err
	}
	if envelope.Type == "analysis_recovery" {
		if len(envelope.Records) > 0 {
			return false, events.SQSEvent{}, errors.New("mixed recovery and SQS event")
		}
		return true, events.SQSEvent{}, nil
	}
	if envelope.Type != "" || len(envelope.Records) == 0 {
		return false, events.SQSEvent{}, errors.New("unsupported worker event")
	}
	var event events.SQSEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return false, event, err
	}
	return false, event, nil
}
func handler(ctx context.Context, raw json.RawMessage) (events.SQSEventResponse, error) {
	recovery, event, err := decodeInvocation(raw)
	if err != nil {
		return events.SQSEventResponse{}, err
	}
	if !recovery {
		return handleSQS(ctx, event)
	}
	initOnce.Do(initApp)
	if initErr != nil {
		return events.SQSEventResponse{}, initErr
	}
	if app == nil || app.AnalysesService == nil {
		return events.SQSEventResponse{}, errors.New("analysis recovery not configured")
	}
	checked, err := app.AnalysesService.RecoverBatch(ctx)
	log.Printf("analysis recovery checked=%d success=%t", checked, err == nil)
	return events.SQSEventResponse{}, err
}
