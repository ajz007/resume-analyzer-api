package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"resume-backend/internal/queue"
)

type fakeSQS struct {
	deleted []string
}

func (f *fakeSQS) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	_ = ctx
	_ = params
	_ = optFns
	return &sqs.ReceiveMessageOutput{}, nil
}

func (f *fakeSQS) DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	_ = ctx
	_ = optFns
	f.deleted = append(f.deleted, aws.ToString(params.ReceiptHandle))
	return &sqs.DeleteMessageOutput{}, nil
}

type fakeProcessor struct {
	err error
}

func (f fakeProcessor) ProcessAnalysis(ctx context.Context, analysisID string) error {
	_ = ctx
	_ = analysisID
	return f.err
}

func TestWorkerDeletesMessageOnSuccess(t *testing.T) {
	client := &fakeSQS{}
	svc := fakeProcessor{}
	msgBody, _ := queue.EncodeMessage(queue.Message{AnalysisID: "analysis-1", RequestID: "req-1"})
	msg := sqstypes.Message{
		MessageId:     aws.String("m1"),
		ReceiptHandle: aws.String("r1"),
		Body:          aws.String(string(msgBody)),
		Attributes:    map[string]string{"ApproximateReceiveCount": "1"},
	}

	handleMessage(context.Background(), client, "queue", svc, msg)

	if len(client.deleted) != 1 {
		t.Fatalf("expected delete, got %d", len(client.deleted))
	}
}

func TestWorkerDoesNotDeleteOnFailure(t *testing.T) {
	client := &fakeSQS{}
	svc := fakeProcessor{err: errors.New("boom")}
	msgBody, _ := queue.EncodeMessage(queue.Message{AnalysisID: "analysis-2", RequestID: "req-2"})
	msg := sqstypes.Message{
		MessageId:     aws.String("m2"),
		ReceiptHandle: aws.String("r2"),
		Body:          aws.String(string(msgBody)),
	}

	handleMessage(context.Background(), client, "queue", svc, msg)

	if len(client.deleted) != 0 {
		t.Fatalf("expected no delete, got %d", len(client.deleted))
	}
}

func TestWorkerDeletesOnInvalidJSON(t *testing.T) {
	client := &fakeSQS{}
	svc := fakeProcessor{}
	msg := sqstypes.Message{
		MessageId:     aws.String("m3"),
		ReceiptHandle: aws.String("r3"),
		Body:          aws.String("{bad-json"),
	}

	handleMessage(context.Background(), client, "queue", svc, msg)

	if len(client.deleted) != 1 {
		t.Fatalf("expected delete, got %d", len(client.deleted))
	}
}
