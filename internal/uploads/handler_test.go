package uploads

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestPresignSignedHeadersExcludeContentLength(t *testing.T) {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	}
	client := s3.NewFromConfig(cfg)
	presigner := s3.NewPresignClient(client)

	input := presignInput("bucket", "documents/user/doc/file.pdf")
	out, err := presigner.PresignPutObject(context.Background(), input)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}

	parsed, err := url.Parse(out.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	signed := parsed.Query().Get("X-Amz-SignedHeaders")
	if signed == "" {
		t.Fatalf("expected X-Amz-SignedHeaders")
	}
	if strings.Contains(signed, "content-length") {
		t.Fatalf("unexpected content-length in signed headers: %s", signed)
	}
	if !strings.Contains(signed, "host") {
		t.Fatalf("expected host in signed headers: %s", signed)
	}
}
