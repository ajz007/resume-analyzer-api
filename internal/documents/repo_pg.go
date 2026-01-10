package documents

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// PGRepo implements DocumentsRepo using Postgres.
type PGRepo struct {
	DB *sql.DB
}

// Create inserts a new document.
func (r *PGRepo) Create(ctx context.Context, doc Document) error {
	const query = `
INSERT INTO documents (id, user_id, file_name, mime_type, size_bytes, storage_key, checksum, created_at)
VALUES ($1, $2, $3, $4, $5, $6, NULL, $7)`
	_, err := r.DB.ExecContext(ctx, query, doc.ID, doc.UserID, doc.FileName, doc.MimeType, doc.SizeBytes, doc.StorageKey, doc.CreatedAt)
	return err
}

// GetCurrentByUser returns the latest document for a user.
func (r *PGRepo) GetCurrentByUser(ctx context.Context, userId string) (Document, error) {
	const query = `
SELECT id, user_id, file_name, mime_type, size_bytes, storage_key, extracted_text_key, extracted_at, created_at
FROM documents
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT 1`
	var doc Document
	var extractedKey sql.NullString
	var extractedAt sql.NullTime
	err := r.DB.QueryRowContext(ctx, query, userId).Scan(
		&doc.ID, &doc.UserID, &doc.FileName, &doc.MimeType, &doc.SizeBytes, &doc.StorageKey, &extractedKey, &extractedAt, &doc.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Document{}, ErrNotFound
		}
		return Document{}, err
	}
	if extractedKey.Valid {
		doc.ExtractedTextKey = extractedKey.String
	}
	if extractedAt.Valid {
		doc.ExtractedAt = &extractedAt.Time
	}
	return doc, nil
}

// GetByID fetches a document by ID for a user.
func (r *PGRepo) GetByID(ctx context.Context, userId, documentID string) (Document, error) {
	const query = `
SELECT id, user_id, file_name, mime_type, size_bytes, storage_key, extracted_text_key, extracted_at, created_at
FROM documents
WHERE user_id = $1 AND id = $2 AND deleted_at IS NULL
LIMIT 1`
	var doc Document
	var extractedKey sql.NullString
	var extractedAt sql.NullTime
	err := r.DB.QueryRowContext(ctx, query, userId, documentID).Scan(
		&doc.ID, &doc.UserID, &doc.FileName, &doc.MimeType, &doc.SizeBytes, &doc.StorageKey, &extractedKey, &extractedAt, &doc.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Document{}, ErrNotFound
		}
		return Document{}, err
	}
	if extractedKey.Valid {
		doc.ExtractedTextKey = extractedKey.String
	}
	if extractedAt.Valid {
		doc.ExtractedAt = &extractedAt.Time
	}
	return doc, nil
}

// ListByUser lists documents ordered newest-first.
func (r *PGRepo) ListByUser(ctx context.Context, userId string, limit, offset int) ([]Document, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	const query = `
SELECT id, user_id, file_name, mime_type, size_bytes, storage_key, extracted_text_key, extracted_at, created_at
FROM documents
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`

	rows, err := r.DB.QueryContext(ctx, query, userId, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Document
	for rows.Next() {
		var doc Document
		var extractedKey sql.NullString
		var extractedAt sql.NullTime
		if err := rows.Scan(&doc.ID, &doc.UserID, &doc.FileName, &doc.MimeType, &doc.SizeBytes, &doc.StorageKey, &extractedKey, &extractedAt, &doc.CreatedAt); err != nil {
			return nil, err
		}
		if extractedKey.Valid {
			doc.ExtractedTextKey = extractedKey.String
		}
		if extractedAt.Valid {
			doc.ExtractedAt = &extractedAt.Time
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

// UpdateExtraction stores the extracted text metadata for a document.
func (r *PGRepo) UpdateExtraction(ctx context.Context, userId, documentID, extractedKey string, extractedAt time.Time) error {
	const query = `
UPDATE documents
SET extracted_text_key = $1, extracted_at = $2
WHERE user_id = $3 AND id = $4 AND extracted_text_key IS NULL`
	_, err := r.DB.ExecContext(ctx, query, extractedKey, extractedAt, userId, documentID)
	return err
}

var _ DocumentsRepo = (*PGRepo)(nil)
