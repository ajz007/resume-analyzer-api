package uploads

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/shared/telemetry"
	"resume-backend/internal/shared/util"
)

const (
	maxUploadBytes       = 5 << 20
	presignExpires       = 15 * time.Minute
	defaultRegion        = "us-east-1"
	defaultUploadsPrefix = "documents/"
)

var allowedContentTypes = map[string]struct{}{
	"application/pdf":    {},
	"application/msword": {},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {},
}

type Handler struct {
	presign *s3.PresignClient
	bucket  string
	prefix  string
}

func NewHandlerFromEnv(ctx context.Context) (*Handler, error) {
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = defaultRegion
	}
	bucket := strings.TrimSpace(os.Getenv("UPLOADS_S3_BUCKET"))
	if bucket == "" {
		return nil, errConfig("UPLOADS_S3_BUCKET is required")
	}
	prefix := strings.TrimSpace(os.Getenv("UPLOADS_S3_PREFIX"))
	if prefix == "" {
		prefix = defaultUploadsPrefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, errConfig("failed to load aws config")
	}

	client := s3.NewFromConfig(cfg)
	return &Handler{
		presign: s3.NewPresignClient(client),
		bucket:  bucket,
		prefix:  prefix,
	}, nil
}

type presignRequest struct {
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	MimeType    string `json:"mimeType"`
	SizeBytes   int64  `json:"sizeBytes"`
}

type presignResponse struct {
	UploadURL        string `json:"uploadUrl"`
	S3Key            string `json:"s3Key"`
	ExpiresInSeconds int64  `json:"expiresInSeconds"`
}

func RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/uploads/presign", presign)
}

func presign(c *gin.Context) {
	var req presignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid request body", nil)
		return
	}

	req.FileName = strings.TrimSpace(req.FileName)
	req.ContentType = strings.TrimSpace(req.ContentType)

	if req.FileName == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "fileName is required", nil)
		return
	}
	if _, ok := allowedContentTypes[req.ContentType]; !ok {
		respond.Error(c, http.StatusBadRequest, "validation_error", "contentType is not allowed", nil)
		return
	}
	if req.SizeBytes <= 0 || req.SizeBytes > maxUploadBytes {
		respond.Error(c, http.StatusBadRequest, "validation_error", "sizeBytes exceeds limit", nil)
		return
	}

	handler, err := NewHandlerFromEnv(c.Request.Context())
	if err != nil {
		var cfgErr errConfig
		if errors.As(err, &cfgErr) {
			respond.Error(c, http.StatusInternalServerError, "internal_error", "uploads not configured", nil)
			return
		}
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to initialize uploader", nil)
		return
	}

	userID := middleware.UserIDFromContext(c)
	docID := uuid.NewString()
	fileID := uuid.NewString()

	sanitized, err := util.SanitizeFileName(req.FileName)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid fileName", nil)
		return
	}

	key := path.Join(handler.prefix, userID, docID, fileID+"-"+sanitized)

	expires := presignExpires
	input := presignInput(handler.bucket, key)
	out, err := handler.presign.PresignPutObject(c.Request.Context(), input, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		telemetry.Error("uploads.presign.failed", map[string]any{
			"err":         err.Error(),
			"bucket":      handler.bucket,
			"key":         key,
			"contentType": req.ContentType,
			"sizeBytes":   req.SizeBytes,
			"request_id":  c.GetString("requestId"),
		})
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to generate upload url", nil)
		return
	}

	respond.JSON(c, http.StatusOK, presignResponse{
		UploadURL:        out.URL,
		S3Key:            key,
		ExpiresInSeconds: int64(expires.Seconds()),
	})
}

func presignInput(bucket, key string) *s3.PutObjectInput {
	return &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
}

type errConfig string

func (e errConfig) Error() string { return string(e) }
