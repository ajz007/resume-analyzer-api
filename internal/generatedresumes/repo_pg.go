package generatedresumes

import (
	"context"
	"database/sql"
	"errors"
)

// PGRepo implements Repo using Postgres.
type PGRepo struct {
	DB *sql.DB
}

// Create inserts a generated resume.
func (r *PGRepo) Create(ctx context.Context, resume GeneratedResume) error {
	const query = `
INSERT INTO generated_resumes (
    id, user_id, document_id, analysis_id, template_id, storage_key, mime_type, size_bytes, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.DB.ExecContext(ctx, query,
		resume.ID,
		resume.UserID,
		resume.DocumentID,
		resume.AnalysisID,
		resume.TemplateID,
		resume.StorageKey,
		resume.MimeType,
		resume.SizeBytes,
		resume.CreatedAt,
	)
	return err
}

// GetByID returns a generated resume by ID for a user.
func (r *PGRepo) GetByID(ctx context.Context, userID, generatedResumeID string) (GeneratedResume, error) {
	const query = `
SELECT id, user_id, document_id, analysis_id, template_id, storage_key, mime_type, size_bytes, created_at
FROM generated_resumes
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1`
	var resume GeneratedResume
	err := r.DB.QueryRowContext(ctx, query, generatedResumeID).Scan(
		&resume.ID,
		&resume.UserID,
		&resume.DocumentID,
		&resume.AnalysisID,
		&resume.TemplateID,
		&resume.StorageKey,
		&resume.MimeType,
		&resume.SizeBytes,
		&resume.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GeneratedResume{}, ErrNotFound
		}
		return GeneratedResume{}, err
	}
	if resume.UserID != userID {
		return GeneratedResume{}, ErrForbidden
	}
	return resume, nil
}

// ListByUser lists generated resumes ordered newest-first.
func (r *PGRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]GeneratedResume, error) {
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
SELECT id, user_id, document_id, analysis_id, template_id, storage_key, mime_type, size_bytes, created_at
FROM generated_resumes
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`

	rows, err := r.DB.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GeneratedResume
	for rows.Next() {
		var resume GeneratedResume
		if err := rows.Scan(
			&resume.ID,
			&resume.UserID,
			&resume.DocumentID,
			&resume.AnalysisID,
			&resume.TemplateID,
			&resume.StorageKey,
			&resume.MimeType,
			&resume.SizeBytes,
			&resume.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, resume)
	}
	return out, rows.Err()
}

var _ Repo = (*PGRepo)(nil)
