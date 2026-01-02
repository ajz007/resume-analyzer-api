package documents

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	"resume-backend/internal/shared/storage/object"
)

// Service contains business logic for documents.
type Service struct {
	Store object.ObjectStore
	Repo  DocumentsRepo
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

	doc := Document{
		ID:         uuid.NewString(),
		UserID:     userId,
		FileName:   fileName,
		MimeType:   mimeType,
		SizeBytes:  size,
		StorageKey: storageKey,
		CreatedAt:  time.Now().UTC(),
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
