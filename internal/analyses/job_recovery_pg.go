package analyses

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
)

func (r *PGRepo) AdoptDispatch(ctx context.Context, id string) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO analysis_dispatch(analysis_id,created_at)
 SELECT a.id,a.created_at FROM analyses a JOIN analysis_usage_reservations u ON u.analysis_id=a.id
 WHERE a.id=$1 AND a.status='queued' AND u.state='reserved' ON CONFLICT(analysis_id) DO NOTHING`, id)
	return err
}
func (r *PGRepo) ClaimDispatch(ctx context.Context, id string) (token, requestID string, claimed bool, err error) {
	token = uuid.NewString()
	err = r.DB.QueryRowContext(ctx, `UPDATE analysis_dispatch d SET claim_token=$2,claim_until=now()+interval '1 minute',attempts=attempts+1
 WHERE analysis_id=$1 AND next_attempt_at<=now() AND (claim_until IS NULL OR claim_until<=now()) AND attempts<8
 AND EXISTS(SELECT 1 FROM analyses a WHERE a.id=d.analysis_id AND a.status='queued' AND a.deleted_at IS NULL)
 RETURNING request_id`, id, token).Scan(&requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	return token, requestID, err == nil, err
}
func (r *PGRepo) FinishDispatch(ctx context.Context, id, token string, published bool) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE analysis_dispatch SET claim_token=NULL,claim_until=NULL,
 next_attempt_at=now()+CASE WHEN $3 THEN interval '5 minutes' ELSE interval '1 minute' END,
 published_at=CASE WHEN $3 THEN now() ELSE published_at END WHERE analysis_id=$1 AND claim_token=$2`, id, token, published)
	return err
}
func (r *PGRepo) ExpireJob(ctx context.Context, id string) (bool, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE analyses SET status='failed',error_code='ANALYSIS_INTERRUPTED',
 error_message='This review was interrupted. Please retry when ready.',error_retryable=true,completed_at=now(),updated_at=now()
 WHERE id=$1 AND ((deleted_at IS NOT NULL AND status IN ('queued','processing')) OR (status='processing' AND COALESCE(started_at,created_at)<now()-interval '20 minutes')
 OR (status='queued' AND created_at<now()-interval '1 hour'))`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
func (r *PGRepo) RecoveryIDs(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 25 {
		limit = 25
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT a.id FROM analyses a LEFT JOIN analysis_dispatch d ON d.analysis_id=a.id
 WHERE (a.status IN ('completed','failed') AND EXISTS(SELECT 1 FROM analysis_usage_reservations u WHERE u.analysis_id=a.id AND u.state='reserved'))
 OR (a.deleted_at IS NOT NULL AND a.status IN ('queued','processing')) OR (a.status='processing' AND COALESCE(a.started_at,a.created_at)<now()-interval '20 minutes')
 OR (a.status='queued' AND (a.created_at<now()-interval '1 hour' OR
 (EXISTS(SELECT 1 FROM analysis_usage_reservations u WHERE u.analysis_id=a.id AND u.state='reserved')
 AND (d.analysis_id IS NULL OR (d.next_attempt_at<=now() AND (d.claim_until IS NULL OR d.claim_until<=now()) AND d.attempts<8)))))
 ORDER BY a.created_at,a.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
