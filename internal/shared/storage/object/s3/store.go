package s3

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"resume-backend/internal/shared/storage/object"
	"resume-backend/internal/shared/util"
)

// Store implements ObjectStore using Amazon S3.
type Store struct {
	client   *s3.Client
	bucket   string
	prefix   string
	kmsKeyID string
}

// New creates a new S3-backed object store.
func New(ctx context.Context, region, bucket, prefix, kmsKeyID string) (object.ObjectStore, error) {
	if bucket == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &Store{
		client:   s3.NewFromConfig(cfg),
		bucket:   bucket,
		prefix:   normalizePrefix(prefix),
		kmsKeyID: strings.TrimSpace(kmsKeyID),
	}, nil
}

// Save uploads the reader contents to S3 under the user's namespace.
func (s *Store) Save(ctx context.Context, userId string, fileName string, r io.Reader) (string, int64, string, error) {
	sanitizedName, err := util.SanitizeFileName(fileName)
	if err != nil {
		return "", 0, "", fmt.Errorf("sanitize file name: %w", err)
	}

	storageUserKey := util.HashUserKey(userId)

	if err := ctx.Err(); err != nil {
		return "", 0, "", err
	}

	prefix := randomID()
	finalName := fmt.Sprintf("%s_%s", prefix, sanitizedName)

	storageKey := path.Join(storageUserKey, finalName)
	objectKey := applyPrefix(s.prefix, storageKey)

	var sniff [512]byte
	n, readErr := io.ReadFull(r, sniff[:])
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return "", 0, "", fmt.Errorf("read sniff: %w", readErr)
	}

	mimeType := http.DetectContentType(sniff[:n])

	body := io.MultiReader(bytes.NewReader(sniff[:n]), r)
	counter := &countingReader{r: body}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		Body:        counter,
		ContentType: aws.String(mimeType),
	}
	if s.kmsKeyID != "" {
		input.ServerSideEncryption = s3types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = aws.String(s.kmsKeyID)
	} else {
		input.ServerSideEncryption = s3types.ServerSideEncryptionAes256
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return "", 0, "", fmt.Errorf("s3 put object bucket=%s key=%s: %w", s.bucket, objectKey, err)
	}

	return storageKey, counter.n, mimeType, nil
}

// Open downloads a stored object for reading.
func (s *Store) Open(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	objectKey := applyPrefix(s.prefix, storageKey)
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get object bucket=%s key=%s: %w", s.bucket, objectKey, err)
	}
	return out.Body, nil
}

// SaveWithKey uploads data to a specific storage key.
func (s *Store) SaveWithKey(ctx context.Context, storageKey string, contentType string, r io.Reader) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	objectKey := applyPrefix(s.prefix, storageKey)
	counter := &countingReader{r: r}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		Body:        counter,
		ContentType: aws.String(contentType),
	}
	if s.kmsKeyID != "" {
		input.ServerSideEncryption = s3types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = aws.String(s.kmsKeyID)
	} else {
		input.ServerSideEncryption = s3types.ServerSideEncryptionAes256
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return 0, fmt.Errorf("s3 put object bucket=%s key=%s: %w", s.bucket, objectKey, err)
	}
	return counter.n, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func normalizePrefix(prefix string) string {
	return strings.Trim(strings.TrimSpace(prefix), "/")
}

func applyPrefix(prefix, key string) string {
	cleanPrefix := strings.Trim(prefix, "/")
	cleanKey := strings.TrimLeft(key, "/")
	if cleanPrefix == "" {
		return cleanKey
	}
	if cleanKey == "" {
		return cleanPrefix
	}
	return cleanPrefix + "/" + cleanKey
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

var _ object.ObjectStore = (*Store)(nil)
