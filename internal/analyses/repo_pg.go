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

// GetOrCreateForDocument returns the latest analysis for a document or creates a new one.
func (r *PGRepo) GetOrCreateForDocument(ctx context.Context, analysis Analysis, allowRetry bool, allowCreate func() error) (Analysis, bool, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Analysis{}, false, err
	}
	defer tx.Rollback()

	// Serialize per-document to avoid duplicate analysis creation.
	if _, err := tx.ExecContext(ctx, `SELECT id FROM documents WHERE id = $1 AND user_id = $2 FOR UPDATE`, analysis.DocumentID, analysis.UserID); err != nil {
		return Analysis{}, false, err
	}

	latest, err := getLatestForDocument(ctx, tx, analysis.UserID, analysis.DocumentID)
	if err == nil {
		switch latest.Status {
		case StatusQueued, StatusProcessing:
			if err := tx.Commit(); err != nil {
				return Analysis{}, false, err
			}
			return latest, false, nil
		case StatusCompleted:
			if err := tx.Commit(); err != nil {
				return Analysis{}, false, err
			}
			return latest, false, nil
		case StatusFailed:
			if !allowRetry {
				if err := tx.Commit(); err != nil {
					return Analysis{}, false, err
				}
				return latest, false, ErrRetryRequired
			}
		}
	} else if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, ErrNotFound) {
		return Analysis{}, false, err
	}

	if allowCreate != nil {
		if err := allowCreate(); err != nil {
			return Analysis{}, false, err
		}
	}

	if err := createWithTx(ctx, tx, analysis); err != nil {
		return Analysis{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Analysis{}, false, err
	}
	return analysis, true, nil
}

// Create inserts a new analysis.
func (r *PGRepo) Create(ctx context.Context, analysis Analysis) error {
	const query = `
INSERT INTO analyses (
	id, document_id, user_id, status, result, analysis_raw, analysis_result, analysis_completed_at,
	job_description, prompt_version, analysis_version, prompt_hash, provider, model, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`
	rawPayload, err := marshalJSONB(analysis.AnalysisRaw)
	if err != nil {
		return err
	}
	resultPayload, err := marshalJSONB(analysis.Result)
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(ctx, query,
		analysis.ID,
		analysis.DocumentID,
		analysis.UserID,
		analysis.Status,
		nil,
		rawPayload,
		resultPayload,
		nil,
		analysis.JobDescription,
		analysis.PromptVersion,
		analysis.AnalysisVersion,
		analysis.PromptHash,
		analysis.Provider,
		analysis.Model,
		analysis.CreatedAt,
	)
	return err
}

// GetByID returns an analysis by ID.
func (r *PGRepo) GetByID(ctx context.Context, analysisID string) (Analysis, error) {
	const query = `
SELECT id, document_id, user_id, status, result, analysis_raw, analysis_result, analysis_completed_at,
       job_description, prompt_version, analysis_version, prompt_hash, provider, model,
       error_code, error_message, error_retryable, started_at, completed_at, created_at, updated_at
FROM analyses
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1`
	var a Analysis
	var result sql.NullString
	var analysisRaw sql.NullString
	var analysisResult sql.NullString
	var analysisCompletedAt sql.NullTime
	var jobDescription sql.NullString
	var promptVersion sql.NullString
	var analysisVersion sql.NullString
	var promptHash sql.NullString
	var provider sql.NullString
	var model sql.NullString
	var errorCode sql.NullString
	var errorMessage sql.NullString
	var errorRetryable sql.NullBool
	var startedAt sql.NullTime
	var completedAt sql.NullTime
	err := r.DB.QueryRowContext(ctx, query, analysisID).Scan(
		&a.ID,
		&a.DocumentID,
		&a.UserID,
		&a.Status,
		&result,
		&analysisRaw,
		&analysisResult,
		&analysisCompletedAt,
		&jobDescription,
		&promptVersion,
		&analysisVersion,
		&promptHash,
		&provider,
		&model,
		&errorCode,
		&errorMessage,
		&errorRetryable,
		&startedAt,
		&completedAt,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Analysis{}, ErrNotFound
		}
		return Analysis{}, err
	}
	if analysisRaw.Valid {
		if err := json.Unmarshal([]byte(analysisRaw.String), &a.AnalysisRaw); err != nil {
			// keep empty
		}
	}
	if analysisResult.Valid {
		a.Result = map[string]any{}
		if err := json.Unmarshal([]byte(analysisResult.String), &a.Result); err != nil {
			// keep empty
			a.Result = nil
		}
	} else if result.Valid {
		a.Result = map[string]any{}
		if err := json.Unmarshal([]byte(result.String), &a.Result); err != nil {
			a.Result = nil
		}
	}
	if jobDescription.Valid {
		a.JobDescription = jobDescription.String
	}
	if promptVersion.Valid {
		a.PromptVersion = promptVersion.String
	}
	if analysisVersion.Valid {
		a.AnalysisVersion = analysisVersion.String
	}
	if promptHash.Valid {
		a.PromptHash = promptHash.String
	}
	if analysisCompletedAt.Valid {
		a.AnalysisCompletedAt = &analysisCompletedAt.Time
	}
	if provider.Valid {
		a.Provider = provider.String
	}
	if model.Valid {
		a.Model = model.String
	}
	if errorCode.Valid {
		a.ErrorCode = errorCode.String
	}
	if errorMessage.Valid {
		a.ErrorMessage = &errorMessage.String
	}
	if errorRetryable.Valid {
		a.ErrorRetryable = errorRetryable.Bool
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
	return r.UpdateStatusResultAndError(ctx, analysisID, status, result, nil, nil, nil, nil, nil)
}

// UpdateStatusResultAndError updates status/result/error fields and timestamps.
func (r *PGRepo) UpdateStatusResultAndError(ctx context.Context, analysisID, status string, result map[string]any, errorCode *string, errorMessage *string, errorRetryable *bool, startedAt *time.Time, completedAt *time.Time) error {
	const query = `
UPDATE analyses
SET status = $1,
    result = COALESCE($2::jsonb, result),
    analysis_result = COALESCE($2::jsonb, analysis_result),
    error_code = COALESCE($3::text, error_code),
    error_message = COALESCE($4::text, error_message),
    error_retryable = CASE
        WHEN $5::boolean IS NOT NULL THEN $5::boolean
        ELSE error_retryable
    END,
    started_at = CASE
        WHEN $6::timestamptz IS NOT NULL THEN $6::timestamptz
        WHEN $1 = 'processing' AND started_at IS NULL THEN now()
        ELSE started_at
    END,
    completed_at = CASE
        WHEN $7::timestamptz IS NOT NULL THEN $7::timestamptz
        WHEN ($1 = 'completed' OR $1 = 'failed') AND completed_at IS NULL THEN now()
        ELSE completed_at
    END,
    updated_at = now()
WHERE id = $8::uuid`

	var payload any
	var err error
	if result != nil {
		payload, err = json.Marshal(result)
		if err != nil {
			return err
		}
	}

	res, err := r.DB.ExecContext(ctx, query, status, payload, errorCode, errorMessage, errorRetryable, startedAt, completedAt, analysisID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateAnalysisRaw updates analysis_raw.
func (r *PGRepo) UpdateAnalysisRaw(ctx context.Context, analysisID string, raw any) error {
	const query = `
UPDATE analyses
SET analysis_raw = $1::jsonb,
    updated_at = now()
WHERE id = $2::uuid`

	payload, err := marshalJSONB(raw)
	if err != nil {
		return err
	}
	res, err := r.DB.ExecContext(ctx, query, payload, analysisID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateAnalysisResult updates analysis_result and analysis_completed_at.
func (r *PGRepo) UpdateAnalysisResult(ctx context.Context, analysisID string, result map[string]any, completedAt *time.Time) error {
	const query = `
UPDATE analyses
SET analysis_result = $1::jsonb,
    analysis_completed_at = $2::timestamptz,
    status = 'completed',
    completed_at = COALESCE($2::timestamptz, completed_at),
    updated_at = now()
WHERE id = $3::uuid`

	payload, err := marshalJSONB(result)
	if err != nil {
		return err
	}
	res, err := r.DB.ExecContext(ctx, query, payload, completedAt, analysisID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdatePromptMetadata updates analysis_version and prompt_hash.
func (r *PGRepo) UpdatePromptMetadata(ctx context.Context, analysisID, analysisVersion, promptHash string) error {
	const query = `
UPDATE analyses
SET analysis_version = COALESCE(NULLIF($1::text, ''), analysis_version),
    prompt_hash = COALESCE(NULLIF($2::text, ''), prompt_hash),
    updated_at = now()
WHERE id = $3::uuid`

	res, err := r.DB.ExecContext(ctx, query, analysisVersion, promptHash, analysisID)
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
SELECT id, document_id, user_id, status, result, analysis_raw, analysis_result, analysis_completed_at,
       job_description, prompt_version, analysis_version, prompt_hash, provider, model,
       error_code, error_message, error_retryable, started_at, completed_at, created_at, updated_at
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
		var analysisRaw sql.NullString
		var analysisResult sql.NullString
		var analysisCompletedAt sql.NullTime
		var jobDescription sql.NullString
		var promptVersion sql.NullString
		var analysisVersion sql.NullString
		var promptHash sql.NullString
		var provider sql.NullString
		var model sql.NullString
		var errorCode sql.NullString
		var errorMessage sql.NullString
		var errorRetryable sql.NullBool
		var startedAt sql.NullTime
		var completedAt sql.NullTime
		if err := rows.Scan(
			&a.ID,
			&a.DocumentID,
			&a.UserID,
			&a.Status,
			&result,
			&analysisRaw,
			&analysisResult,
			&analysisCompletedAt,
			&jobDescription,
			&promptVersion,
			&analysisVersion,
			&promptHash,
			&provider,
			&model,
			&errorCode,
			&errorMessage,
			&errorRetryable,
			&startedAt,
			&completedAt,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if analysisRaw.Valid {
			if err := json.Unmarshal([]byte(analysisRaw.String), &a.AnalysisRaw); err != nil {
				// ignore parse errors, keep nil
			}
		}
		if analysisResult.Valid {
			a.Result = map[string]any{}
			if err := json.Unmarshal([]byte(analysisResult.String), &a.Result); err != nil {
				a.Result = nil
			}
		} else if result.Valid {
			a.Result = map[string]any{}
			if err := json.Unmarshal([]byte(result.String), &a.Result); err != nil {
				a.Result = nil
			}
		}
		if jobDescription.Valid {
			a.JobDescription = jobDescription.String
		}
		if promptVersion.Valid {
			a.PromptVersion = promptVersion.String
		}
		if analysisVersion.Valid {
			a.AnalysisVersion = analysisVersion.String
		}
		if promptHash.Valid {
			a.PromptHash = promptHash.String
		}
		if analysisCompletedAt.Valid {
			a.AnalysisCompletedAt = &analysisCompletedAt.Time
		}
		if provider.Valid {
			a.Provider = provider.String
		}
		if model.Valid {
			a.Model = model.String
		}
		if errorCode.Valid {
			a.ErrorCode = errorCode.String
		}
		if errorMessage.Valid {
			a.ErrorMessage = &errorMessage.String
		}
		if errorRetryable.Valid {
			a.ErrorRetryable = errorRetryable.Bool
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

func marshalJSONB(value any) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(value)
}

func createWithTx(ctx context.Context, tx *sql.Tx, analysis Analysis) error {
	const query = `
INSERT INTO analyses (
	id, document_id, user_id, status, result, analysis_raw, analysis_result, analysis_completed_at,
	job_description, prompt_version, analysis_version, prompt_hash, provider, model, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

	rawPayload, err := marshalJSONB(analysis.AnalysisRaw)
	if err != nil {
		return err
	}
	resultPayload, err := marshalJSONB(analysis.Result)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, query,
		analysis.ID,
		analysis.DocumentID,
		analysis.UserID,
		analysis.Status,
		nil,
		rawPayload,
		resultPayload,
		nil,
		analysis.JobDescription,
		analysis.PromptVersion,
		analysis.AnalysisVersion,
		analysis.PromptHash,
		analysis.Provider,
		analysis.Model,
		analysis.CreatedAt,
	)
	return err
}

func getLatestForDocument(ctx context.Context, q queryer, userID, documentID string) (Analysis, error) {
	const query = `
SELECT id, document_id, user_id, status, result, analysis_raw, analysis_result, analysis_completed_at,
       job_description, prompt_version, analysis_version, prompt_hash, provider, model,
       error_code, error_message, error_retryable, started_at, completed_at, created_at, updated_at
FROM analyses
WHERE document_id = $1 AND user_id = $2 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT 1`

	var a Analysis
	var result sql.NullString
	var analysisRaw sql.NullString
	var analysisResult sql.NullString
	var analysisCompletedAt sql.NullTime
	var jobDescription sql.NullString
	var promptVersion sql.NullString
	var analysisVersion sql.NullString
	var promptHash sql.NullString
	var provider sql.NullString
	var model sql.NullString
	var errorCode sql.NullString
	var errorMessage sql.NullString
	var errorRetryable sql.NullBool
	var startedAt sql.NullTime
	var completedAt sql.NullTime

	err := q.QueryRowContext(ctx, query, documentID, userID).Scan(
		&a.ID,
		&a.DocumentID,
		&a.UserID,
		&a.Status,
		&result,
		&analysisRaw,
		&analysisResult,
		&analysisCompletedAt,
		&jobDescription,
		&promptVersion,
		&analysisVersion,
		&promptHash,
		&provider,
		&model,
		&errorCode,
		&errorMessage,
		&errorRetryable,
		&startedAt,
		&completedAt,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Analysis{}, ErrNotFound
		}
		return Analysis{}, err
	}
	if analysisRaw.Valid {
		_ = json.Unmarshal([]byte(analysisRaw.String), &a.AnalysisRaw)
	}
	if analysisResult.Valid {
		a.Result = map[string]any{}
		if err := json.Unmarshal([]byte(analysisResult.String), &a.Result); err != nil {
			a.Result = nil
		}
	} else if result.Valid {
		a.Result = map[string]any{}
		if err := json.Unmarshal([]byte(result.String), &a.Result); err != nil {
			a.Result = nil
		}
	}
	if jobDescription.Valid {
		a.JobDescription = jobDescription.String
	}
	if promptVersion.Valid {
		a.PromptVersion = promptVersion.String
	}
	if analysisVersion.Valid {
		a.AnalysisVersion = analysisVersion.String
	}
	if promptHash.Valid {
		a.PromptHash = promptHash.String
	}
	if analysisCompletedAt.Valid {
		a.AnalysisCompletedAt = &analysisCompletedAt.Time
	}
	if provider.Valid {
		a.Provider = provider.String
	}
	if model.Valid {
		a.Model = model.String
	}
	if errorCode.Valid {
		a.ErrorCode = errorCode.String
	}
	if errorMessage.Valid {
		a.ErrorMessage = &errorMessage.String
	}
	if errorRetryable.Valid {
		a.ErrorRetryable = errorRetryable.Bool
	}
	if startedAt.Valid {
		a.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		a.CompletedAt = &completedAt.Time
	}
	return a, nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
