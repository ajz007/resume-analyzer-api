package usage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type pgStore struct {
	DB *sql.DB
}

// NewPGStore constructs a Postgres-backed usage store.
func NewPGStore(db *sql.DB) *pgStore {
	return &pgStore{DB: db}
}

func (s *pgStore) Get(ctx context.Context, userID string) (Usage, error) {
	u, err := s.ensure(ctx, userID)
	return u, err
}

func (s *pgStore) EnsurePeriod(ctx context.Context, userID string) (Usage, error) {
	return s.ensure(ctx, userID)
}

func (s *pgStore) Consume(ctx context.Context, userID string, n int) (Usage, error) {
	if n <= 0 {
		return s.ensure(ctx, userID)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Usage{}, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	u, err := s.lockAndEnsure(ctx, tx, userID)
	if err != nil {
		return Usage{}, err
	}

	if u.Used+n > u.Limit {
		err = ErrLimitReached
		return Usage{}, err
	}
	u.Used += n
	if _, err = tx.ExecContext(ctx, `
UPDATE usage SET used = $1 WHERE user_id = $2`, u.Used, userID); err != nil {
		return Usage{}, err
	}
	if err = tx.Commit(); err != nil {
		return Usage{}, err
	}
	return u, nil
}

func (s *pgStore) Reset(ctx context.Context, userID string) (Usage, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Usage{}, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	now := time.Now().UTC()
	resetsAt := now.Add(7 * 24 * time.Hour)
	if _, err = tx.ExecContext(ctx, `
INSERT INTO usage (user_id, plan, limit_amount, used, resets_at)
VALUES ($1, 'Starter', 10, 0, $2)
ON CONFLICT (user_id) DO UPDATE SET used = 0, resets_at = EXCLUDED.resets_at`, userID, resetsAt); err != nil {
		return Usage{}, err
	}
	if err = tx.Commit(); err != nil {
		return Usage{}, err
	}
	return Usage{Plan: "Starter", Limit: 10, Used: 0, ResetsAt: resetsAt}, nil
}

func (s *pgStore) CreateApplyRun(ctx context.Context, run ApplyRun) error {
	const query = `
INSERT INTO apply_runs (
    id, user_id, analysis_id, status, auto_fixes_count, safe_rewrites_count,
    blocked_rewrites_count, needs_input_count, placeholders_remaining, document_version_id, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := s.DB.ExecContext(ctx, query,
		run.ID,
		run.UserID,
		run.AnalysisID,
		run.Status,
		run.AutoFixesCount,
		run.SafeRewritesCount,
		run.BlockedRewritesCount,
		run.NeedsInputCount,
		run.PlaceholdersRemaining,
		nullableString(run.DocumentVersionID),
		run.CreatedAt,
	)
	return err
}

func (s *pgStore) GetApplyRun(ctx context.Context, userID, runID string) (ApplyRun, error) {
	const query = `
SELECT id, user_id, analysis_id, status, auto_fixes_count, safe_rewrites_count,
       blocked_rewrites_count, needs_input_count, placeholders_remaining, document_version_id, created_at
FROM apply_runs
WHERE id = $1 AND user_id = $2
LIMIT 1`
	var run ApplyRun
	var documentVersionID sql.NullString
	err := s.DB.QueryRowContext(ctx, query, runID, userID).Scan(
		&run.ID,
		&run.UserID,
		&run.AnalysisID,
		&run.Status,
		&run.AutoFixesCount,
		&run.SafeRewritesCount,
		&run.BlockedRewritesCount,
		&run.NeedsInputCount,
		&run.PlaceholdersRemaining,
		&documentVersionID,
		&run.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ApplyRun{}, ErrApplyRunNotFound
		}
		return ApplyRun{}, err
	}
	if documentVersionID.Valid {
		run.DocumentVersionID = documentVersionID.String
	}
	return run, nil
}

func (s *pgStore) UpdateApplyRun(ctx context.Context, update ApplyRunUpdate) error {
	const query = `
UPDATE apply_runs
SET status = $1,
    auto_fixes_count = $2,
    safe_rewrites_count = $3,
    blocked_rewrites_count = $4,
    needs_input_count = $5,
    placeholders_remaining = $6,
    document_version_id = $7
WHERE id = $8 AND user_id = $9`
	res, err := s.DB.ExecContext(ctx, query,
		update.Status,
		update.AutoFixesCount,
		update.SafeRewritesCount,
		update.BlockedRewritesCount,
		update.NeedsInputCount,
		update.PlaceholdersRemaining,
		nullableString(update.DocumentVersionID),
		update.ID,
		update.UserID,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrApplyRunNotFound
	}
	return nil
}

func (s *pgStore) CreateDocumentVersion(ctx context.Context, version DocumentVersion) error {
	const query = `
INSERT INTO document_versions (
    id, document_id, user_id, apply_run_id, file_name, mime_type, size_bytes, storage_key, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := s.DB.ExecContext(ctx, query,
		version.ID,
		version.DocumentID,
		version.UserID,
		nullableString(version.ApplyRunID),
		version.FileName,
		version.MimeType,
		version.SizeBytes,
		version.StorageKey,
		version.CreatedAt,
	)
	return err
}

func nullableString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func (s *pgStore) ensure(ctx context.Context, userID string) (Usage, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Usage{}, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	u, err := s.lockAndEnsure(ctx, tx, userID)
	if err != nil {
		return Usage{}, err
	}
	if err = tx.Commit(); err != nil {
		return Usage{}, err
	}
	return u, nil
}

func (s *pgStore) lockAndEnsure(ctx context.Context, tx *sql.Tx, userID string) (Usage, error) {
	var u Usage
	row := tx.QueryRowContext(ctx, `
SELECT plan, limit_amount, used, resets_at FROM usage WHERE user_id = $1 FOR UPDATE`, userID)
	err := row.Scan(&u.Plan, &u.Limit, &u.Used, &u.ResetsAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			u = defaultUsage()
			u.ResetsAt = time.Now().UTC().Add(7 * 24 * time.Hour)
			if _, err = tx.ExecContext(ctx, `
INSERT INTO usage (user_id, plan, limit_amount, used, resets_at) VALUES ($1, $2, $3, $4, $5)`,
				userID, u.Plan, u.Limit, u.Used, u.ResetsAt); err != nil {
				return Usage{}, err
			}
			return u, nil
		}
		return Usage{}, err
	}

	now := time.Now().UTC()
	if now.After(u.ResetsAt) || now.Equal(u.ResetsAt) {
		u.Used = 0
		u.ResetsAt = now.Add(7 * 24 * time.Hour)
		if _, err = tx.ExecContext(ctx, `UPDATE usage SET used = $1, resets_at = $2 WHERE user_id = $3`, u.Used, u.ResetsAt, userID); err != nil {
			return Usage{}, err
		}
	}
	return u, nil
}
