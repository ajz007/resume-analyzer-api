package queue

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

const sqsRegion = "us-east-1"

// SQSClient sends queue messages to AWS SQS.
type SQSClient struct {
	client   *sqs.Client
	queueURL string
}

// NewSQSClient constructs an SQS-backed queue client.
func NewSQSClient(ctx context.Context) (*SQSClient, error) {
	queueURL := strings.TrimSpace(os.Getenv("RA_SQS_QUEUE_URL"))
	if queueURL == "" {
		return nil, fmt.Errorf("RA_SQS_QUEUE_URL is required")
	}

	_ = strings.TrimSpace(os.Getenv("AWS_REGION"))

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(sqsRegion))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &SQSClient{
		client:   sqs.NewFromConfig(cfg),
		queueURL: queueURL,
	}, nil
}

// Send delivers a message to the configured SQS queue.
func (s *SQSClient) Send(ctx context.Context, msg Message) error {
	payload, err := EncodeMessage(msg)
	if err != nil {
		return fmt.Errorf("encode sqs message: %w", err)
	}

	_, err = s.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(s.queueURL),
		MessageBody: aws.String(string(payload)),
	})
	if err != nil {
		return fmt.Errorf("sqs send message: %w", err)
	}
	return nil
}

var _ Client = (*SQSClient)(nil)
