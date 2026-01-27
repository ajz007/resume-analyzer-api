package documents

import (
	"context"
	"errors"
	"io"
	"log"
	"time"

	"github.com/google/uuid"

	"resume-backend/internal/shared/storage/object"
)

// Service contains business logic for documents.
type Service struct {
	Store           object.ObjectStore
	Repo            DocumentsRepo
	StorageProvider string
}

// Upload saves the file to object storage and records the document.
func (s *Service) Upload(ctx context.Context, userId, fileName string, r io.Reader) (Document, error) {
	if fileName == "" {
		return Document{}, ErrInvalidInput
	}

	storageKey, size, mimeType, err := s.Store.Save(ctx, userId, fileName, r)
	if err != nil {
		return Document{}, err
	}

	storageProvider := s.StorageProvider
	if storageProvider == "" {
		storageProvider = "local"
	}

	doc := Document{
		ID:               uuid.NewString(),
		UserID:           userId,
		FileName:         fileName,
		OriginalFilename: fileName,
		MimeType:         mimeType,
		ContentType:      mimeType,
		SizeBytes:        size,
		StorageProvider:  storageProvider,
		StorageKey:       storageKey,
		CreatedAt:        time.Now().UTC(),
	}

	log.Printf("Uploaded document %s for user %s: size=%d mime=%s", doc.ID, userId, size, mimeType)

	if err := s.Repo.Create(ctx, doc); err != nil {
		return Document{}, err
	}

	return doc, nil
}

// CreateFromS3 records a document that already exists in S3.
func (s *Service) CreateFromS3(ctx context.Context, userId, s3Key, originalFileName, contentType string, sizeBytes int64) (Document, error) {
	if userId == "" || s3Key == "" || originalFileName == "" || contentType == "" || sizeBytes <= 0 {
		return Document{}, ErrInvalidInput
	}

	doc := Document{
		ID:               uuid.NewString(),
		UserID:           userId,
		FileName:         originalFileName,
		OriginalFilename: originalFileName,
		MimeType:         contentType,
		ContentType:      contentType,
		SizeBytes:        sizeBytes,
		StorageProvider:  "s3",
		StorageKey:       s3Key,
		CreatedAt:        time.Now().UTC(),
	}

	if err := s.Repo.Create(ctx, doc); err != nil {
		return Document{}, err
	}

	return doc, nil
}

// Current returns the current document for a user.
func (s *Service) Current(ctx context.Context, userId string) (Document, error) {
	if userId == "" {
		return Document{}, errors.New("user id required")
	}
	return s.Repo.GetCurrentByUser(ctx, userId)
}

// List returns a user's documents ordered newest-first with limit/offset.
func (s *Service) List(ctx context.Context, userId string, limit, offset int) ([]Document, error) {
	if userId == "" {
		return nil, errors.New("user id required")
	}
	return s.Repo.ListByUser(ctx, userId, limit, offset)
}
