package analyses

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"resume-backend/internal/usage"
)

// AnalysisReserver admits an analysis in the same transaction as its creation.
type AnalysisReserver interface {
	ReserveAnalysisTx(context.Context, *sql.Tx, string, string) (usage.Usage, error)
}

// PGRepo implements Repo using Postgres.
type PGRepo struct {
	DB           *sql.DB
	Reservations AnalysisReserver
}

func (r *PGRepo) DurableJobs() bool { return r.Reservations != nil }
func (r *PGRepo) admitWithTx(ctx context.Context, tx *sql.Tx, a Analysis) error {
	if r.Reservations == nil {
		return nil
	}
	if _, err := r.Reservations.ReserveAnalysisTx(ctx, tx, a.UserID, a.ID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO analysis_dispatch(analysis_id,request_id,created_at) VALUES($1,$2,$3)`, a.ID, requestIDFromContext(ctx), a.CreatedAt)
	return err
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

	mode := analysis.Mode
	if mode == "" {
		mode = ModeJobMatch
	}
	latest, err := getLatestForDocument(ctx, tx, analysis.UserID, analysis.DocumentID, mode, analysis.JobDescription)
	if err == nil && !compatibleAnalysis(latest, analysis) {
		err = sql.ErrNoRows
	}
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
	if err := r.admitWithTx(ctx, tx, analysis); err != nil {
		return Analysis{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Analysis{}, false, err
	}
	return analysis, true, nil
}

// Create inserts a new analysis.
func (r *PGRepo) Create(ctx context.Context, analysis Analysis) error {
	if r.DurableJobs() {
		tx, err := r.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := createWithTx(ctx, tx, analysis); err != nil {
			return err
		}
		if err := r.admitWithTx(ctx, tx, analysis); err != nil {
			return err
		}
		return tx.Commit()
	}
	const query = `
INSERT INTO analyses (
	id, document_id, user_id, status, result, analysis_raw, analysis_result, analysis_completed_at,
	job_description, prompt_version, mode, analysis_version, prompt_hash, provider, model, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`
	rawPayload, err := marshalJSONB(analysis.AnalysisRaw)
	if err != nil {
		return err
	}
	resultPayload, err := marshalJSONB(analysis.Result)
	if err != nil {
		return err
	}
	mode := analysis.Mode
	if mode == "" {
		mode = ModeJobMatch
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
		mode,
		analysis.AnalysisVersion,
		analysis.PromptHash,
		analysis.Provider,
		analysis.Model,
		analysis.CreatedAt,
	)
	return err
}

// ClaimGuest reassigns analyses owned by a guest user to an authenticated user.
func (r *PGRepo) ClaimGuest(ctx context.Context, guestUserID, authedUserID string) (int, error) {
	const query = `
UPDATE analyses
SET user_id = $1
WHERE user_id = $2 AND deleted_at IS NULL`
	res, err := r.DB.ExecContext(ctx, query, authedUserID, guestUserID)
	if err != nil {
		return 0, err
	}
	updated, _ := res.RowsAffected()
	return int(updated), nil
}

// GetByID returns an analysis by ID.
func (r *PGRepo) GetByID(ctx context.Context, analysisID string) (Analysis, error) {
	const query = `
SELECT id, document_id, user_id, status, result, analysis_raw, analysis_result, analysis_completed_at,
       job_description, prompt_version, mode, analysis_version, prompt_hash, provider, model,
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
	var mode sql.NullString
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
		&mode,
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
	if mode.Valid {
		if parsed, err := ParseMode(mode.String); err == nil {
			a.Mode = parsed
		} else {
			a.Mode = ModeJobMatch
		}
	} else {
		a.Mode = ModeJobMatch
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

// GetActiveShareByAnalysisOwner returns the latest active share for an analysis/owner pair.
func (r *PGRepo) GetActiveShareByAnalysisOwner(ctx context.Context, analysisID string, ownerUserID *string, ownerGuestID *string) (AnalysisShare, error) {
	const query = `
SELECT id, analysis_id, owner_user_id, owner_guest_id, token_hash, token_ciphertext, created_at, revoked_at
FROM analysis_shares
WHERE analysis_id = $1
  AND owner_user_id IS NOT DISTINCT FROM $2
  AND owner_guest_id IS NOT DISTINCT FROM $3
  AND revoked_at IS NULL
ORDER BY created_at DESC
LIMIT 1`

	var share AnalysisShare
	var ownerUser sql.NullString
	var ownerGuest sql.NullString
	var revokedAt sql.NullTime
	err := r.DB.QueryRowContext(ctx, query, analysisID, ownerUserID, ownerGuestID).Scan(
		&share.ID,
		&share.AnalysisID,
		&ownerUser,
		&ownerGuest,
		&share.TokenHash,
		&share.TokenCipher,
		&share.CreatedAt,
		&revokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AnalysisShare{}, ErrShareNotFound
		}
		return AnalysisShare{}, err
	}
	if ownerUser.Valid {
		share.OwnerUserID = &ownerUser.String
	}
	if ownerGuest.Valid {
		share.OwnerGuestID = &ownerGuest.String
	}
	if revokedAt.Valid {
		share.RevokedAt = &revokedAt.Time
	}
	return share, nil
}

// GetShareByTokenHash returns a share by token hash.
func (r *PGRepo) GetShareByTokenHash(ctx context.Context, tokenHash string) (AnalysisShare, error) {
	const query = `
SELECT id, analysis_id, owner_user_id, owner_guest_id, token_hash, token_ciphertext, created_at, revoked_at
FROM analysis_shares
WHERE token_hash = $1
LIMIT 1`

	var share AnalysisShare
	var ownerUser sql.NullString
	var ownerGuest sql.NullString
	var revokedAt sql.NullTime
	err := r.DB.QueryRowContext(ctx, query, tokenHash).Scan(
		&share.ID,
		&share.AnalysisID,
		&ownerUser,
		&ownerGuest,
		&share.TokenHash,
		&share.TokenCipher,
		&share.CreatedAt,
		&revokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AnalysisShare{}, ErrShareNotFound
		}
		return AnalysisShare{}, err
	}
	if ownerUser.Valid {
		share.OwnerUserID = &ownerUser.String
	}
	if ownerGuest.Valid {
		share.OwnerGuestID = &ownerGuest.String
	}
	if revokedAt.Valid {
		share.RevokedAt = &revokedAt.Time
	}
	return share, nil
}

// CreateShare inserts a new share record.
func (r *PGRepo) CreateShare(ctx context.Context, share AnalysisShare) error {
	const query = `
INSERT INTO analysis_shares (
	id, analysis_id, owner_user_id, owner_guest_id, token_hash, token_ciphertext, created_at, revoked_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.DB.ExecContext(
		ctx,
		query,
		share.ID,
		share.AnalysisID,
		share.OwnerUserID,
		share.OwnerGuestID,
		share.TokenHash,
		share.TokenCipher,
		share.CreatedAt,
		share.RevokedAt,
	)
	return err
}

// RevokeShare marks a share as revoked.
func (r *PGRepo) RevokeShare(ctx context.Context, shareID string, revokedAt time.Time) error {
	const query = `
UPDATE analysis_shares
SET revoked_at = $1
WHERE id = $2::uuid
  AND revoked_at IS NULL`

	res, err := r.DB.ExecContext(ctx, query, revokedAt, shareID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrShareNotFound
	}
	return nil
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
WHERE id = $8::uuid AND status NOT IN ('completed','failed')`

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
		return r.finalizedOrNotFound(ctx, analysisID)
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
WHERE id = $3::uuid AND status NOT IN ('completed','failed')`

	payload, err := marshalJSONB(result)
	if err != nil {
		return err
	}
	res, err := r.DB.ExecContext(ctx, query, payload, completedAt, analysisID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return r.finalizedOrNotFound(ctx, analysisID)
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
       job_description, prompt_version, mode, analysis_version, prompt_hash, provider, model,
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
		var mode sql.NullString
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
			&mode,
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
		if mode.Valid {
			if parsed, err := ParseMode(mode.String); err == nil {
				a.Mode = parsed
			} else {
				a.Mode = ModeJobMatch
			}
		} else {
			a.Mode = ModeJobMatch
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
	job_description, prompt_version, mode, analysis_version, prompt_hash, provider, model, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

	rawPayload, err := marshalJSONB(analysis.AnalysisRaw)
	if err != nil {
		return err
	}
	resultPayload, err := marshalJSONB(analysis.Result)
	if err != nil {
		return err
	}

	mode := analysis.Mode
	if mode == "" {
		mode = ModeJobMatch
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
		mode,
		analysis.AnalysisVersion,
		analysis.PromptHash,
		analysis.Provider,
		analysis.Model,
		analysis.CreatedAt,
	)
	return err
}

// getLatestForDocument returns the latest analysis for a document that was run with the
// same mode and job description, so a cached result is never reused across modes or JDs.
func getLatestForDocument(ctx context.Context, q queryer, userID, documentID string, analysisMode AnalysisMode, jd string) (Analysis, error) {
	const query = `
SELECT id, document_id, user_id, status, result, analysis_raw, analysis_result, analysis_completed_at,
       job_description, prompt_version, mode, analysis_version, prompt_hash, provider, model,
       error_code, error_message, error_retryable, started_at, completed_at, created_at, updated_at
FROM analyses
WHERE document_id = $1 AND user_id = $2 AND deleted_at IS NULL
  AND COALESCE(NULLIF(mode, ''), 'JOB_MATCH') = $3
  AND COALESCE(job_description, '') = $4
ORDER BY created_at DESC
LIMIT 1`

	var a Analysis
	var result sql.NullString
	var analysisRaw sql.NullString
	var analysisResult sql.NullString
	var analysisCompletedAt sql.NullTime
	var jobDescription sql.NullString
	var promptVersion sql.NullString
	var mode sql.NullString
	var analysisVersion sql.NullString
	var promptHash sql.NullString
	var provider sql.NullString
	var model sql.NullString
	var errorCode sql.NullString
	var errorMessage sql.NullString
	var errorRetryable sql.NullBool
	var startedAt sql.NullTime
	var completedAt sql.NullTime

	err := q.QueryRowContext(ctx, query, documentID, userID, string(analysisMode), jd).Scan(
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
		&mode,
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
	if mode.Valid {
		if parsed, err := ParseMode(mode.String); err == nil {
			a.Mode = parsed
		} else {
			a.Mode = ModeJobMatch
		}
	} else {
		a.Mode = ModeJobMatch
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

func (r *PGRepo) finalizedOrNotFound(ctx context.Context, analysisID string) error {
	var exists bool
	if err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analyses WHERE id=$1)`, analysisID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrAnalysisFinalized
	}
	return ErrNotFound
}

func (r *PGRepo) ClaimProcessing(ctx context.Context, id string, startedAt time.Time) (bool, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE analyses a SET status='processing',started_at=$2,updated_at=now() WHERE a.id=$1 AND a.status='queued' AND a.started_at IS NULL AND a.deleted_at IS NULL
 AND (NOT $3::boolean OR EXISTS(SELECT 1 FROM analysis_usage_reservations u WHERE u.analysis_id=a.id AND u.state='reserved'))`, id, startedAt, r.DurableJobs())
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
