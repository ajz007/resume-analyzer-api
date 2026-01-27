package analyses

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const maxS3DocBytes = 5 << 20

type s3DocClient struct {
	client *s3.Client
	bucket string
}

func newS3DocClient(ctx context.Context) (*s3DocClient, error) {
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = "us-east-1"
	}
	bucket := strings.TrimSpace(os.Getenv("UPLOADS_S3_BUCKET"))
	if bucket == "" {
		bucket = strings.TrimSpace(os.Getenv("S3_BUCKET"))
	}
	if bucket == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &s3DocClient{
		client: s3.NewFromConfig(cfg),
		bucket: bucket,
	}, nil
}

func (c *s3DocClient) GetObjectBytes(ctx context.Context, key string) ([]byte, error) {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get object key=%s: %w", key, err)
	}
	defer out.Body.Close()

	if out.ContentLength != 0 && out.ContentLength > maxS3DocBytes {
		return nil, fmt.Errorf("s3 object too large: %d bytes", out.ContentLength)
	}

	limited := io.LimitReader(out.Body, maxS3DocBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read s3 object key=%s: %w", key, err)
	}
	if int64(len(data)) > maxS3DocBytes {
		return nil, fmt.Errorf("s3 object too large: %d bytes", len(data))
	}
	return data, nil
}

func (c *s3DocClient) PutText(ctx context.Context, key string, text string) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        strings.NewReader(text),
		ContentType: aws.String("text/plain; charset=utf-8"),
	})
	if err != nil {
		return fmt.Errorf("s3 put object key=%s: %w", key, err)
	}
	return nil
}
