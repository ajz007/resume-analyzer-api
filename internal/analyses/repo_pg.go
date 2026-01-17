package analyses

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// PGRepo implements Repo using Postgres.
type PGRepo struct {
	DB *sql.DB
}

// Create inserts a new analysis.
func (r *PGRepo) Create(ctx context.Context, analysis Analysis) error {
	const query = `
INSERT INTO analyses (id, document_id, user_id, status, result, job_description, prompt_version, provider, model, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := r.DB.ExecContext(ctx, query, analysis.ID, analysis.DocumentID, analysis.UserID, analysis.Status, nil, analysis.JobDescription, analysis.PromptVersion, analysis.Provider, analysis.Model, analysis.CreatedAt)
	return err
}

// GetByID returns an analysis by ID.
func (r *PGRepo) GetByID(ctx context.Context, analysisID string) (Analysis, error) {
	const query = `
SELECT id, document_id, user_id, status, result, job_description, prompt_version, provider, model,
       error_message, started_at, completed_at, created_at, updated_at
FROM analyses
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1`
	var a Analysis
	var result sql.NullString
	var jobDescription sql.NullString
	var promptVersion sql.NullString
	var provider sql.NullString
	var model sql.NullString
	var errorMessage sql.NullString
	var startedAt sql.NullTime
	var completedAt sql.NullTime
	err := r.DB.QueryRowContext(ctx, query, analysisID).Scan(
		&a.ID, &a.DocumentID, &a.UserID, &a.Status, &result, &jobDescription, &promptVersion, &provider, &model,
		&errorMessage, &startedAt, &completedAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Analysis{}, ErrNotFound
		}
		return Analysis{}, err
	}
	if result.Valid {
		a.Result = map[string]any{}
		if err := json.Unmarshal([]byte(result.String), &a.Result); err == nil {
			// keep result parsed
		}
	}
	if jobDescription.Valid {
		a.JobDescription = jobDescription.String
	}
	if promptVersion.Valid {
		a.PromptVersion = promptVersion.String
	}
	if provider.Valid {
		a.Provider = provider.String
	}
	if model.Valid {
		a.Model = model.String
	}
	if errorMessage.Valid {
		a.ErrorMessage = &errorMessage.String
	}
	if startedAt.Valid {
		a.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		a.CompletedAt = &completedAt.Time
	}
	return a, nil
}

// UpdateStatus updates status/result for an analysis.
func (r *PGRepo) UpdateStatus(ctx context.Context, analysisID, status string, result map[string]any) error {
	return r.UpdateStatusResultAndError(ctx, analysisID, status, result, nil, nil, nil)
}

// UpdateStatusResultAndError updates status/result/error fields and timestamps.
func (r *PGRepo) UpdateStatusResultAndError(ctx context.Context, analysisID, status string, result map[string]any, errorMessage *string, startedAt *time.Time, completedAt *time.Time) error {
	const query = `
UPDATE analyses
SET status = $1,
    result = COALESCE($2::jsonb, result),
    error_message = COALESCE($3::text, error_message),
    started_at = CASE
        WHEN $4::timestamptz IS NOT NULL THEN $4::timestamptz
        WHEN $1 = 'processing' AND started_at IS NULL THEN now()
        ELSE started_at
    END,
    completed_at = CASE
        WHEN $5::timestamptz IS NOT NULL THEN $5::timestamptz
        WHEN ($1 = 'completed' OR $1 = 'failed') AND completed_at IS NULL THEN now()
        ELSE completed_at
    END,
    updated_at = now()
WHERE id = $6::uuid`

	var payload any
	var err error
	if result != nil {
		payload, err = json.Marshal(result)
		if err != nil {
			return err
		}
	}

	res, err := r.DB.ExecContext(ctx, query, status, payload, errorMessage, startedAt, completedAt, analysisID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListByUser lists analyses for a user ordered newest-first.
func (r *PGRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]Analysis, error) {
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
SELECT id, document_id, user_id, status, result, job_description, prompt_version, provider, model,
       error_message, started_at, completed_at, created_at, updated_at
FROM analyses
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`

	rows, err := r.DB.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Analysis
	for rows.Next() {
		var a Analysis
		var result sql.NullString
		var jobDescription sql.NullString
		var promptVersion sql.NullString
		var provider sql.NullString
		var model sql.NullString
		var errorMessage sql.NullString
		var startedAt sql.NullTime
		var completedAt sql.NullTime
		if err := rows.Scan(
			&a.ID, &a.DocumentID, &a.UserID, &a.Status, &result, &jobDescription, &promptVersion, &provider, &model,
			&errorMessage, &startedAt, &completedAt, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if result.Valid {
			a.Result = map[string]any{}
			if err := json.Unmarshal([]byte(result.String), &a.Result); err != nil {
				// ignore parse errors, keep nil
			}
		}
		if jobDescription.Valid {
			a.JobDescription = jobDescription.String
		}
		if promptVersion.Valid {
			a.PromptVersion = promptVersion.String
		}
		if provider.Valid {
			a.Provider = provider.String
		}
		if model.Valid {
			a.Model = model.String
		}
		if errorMessage.Valid {
			a.ErrorMessage = &errorMessage.String
		}
		if startedAt.Valid {
			a.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			a.CompletedAt = &completedAt.Time
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

var _ Repo = (*PGRepo)(nil)
