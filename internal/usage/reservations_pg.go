package usage

import (
	"context"
	"database/sql"
	"errors"
)

func (s *pgStore) ReserveAnalysis(ctx context.Context, userID, analysisID string) (Usage, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Usage{}, err
	}
	defer tx.Rollback()
	u, err := s.ReserveAnalysisTx(ctx, tx, userID, analysisID)
	if err != nil {
		return Usage{}, err
	}
	return u, tx.Commit()
}

// ReserveAnalysisTx admits a review inside the caller's job creation transaction.
func (s *pgStore) ReserveAnalysisTx(ctx context.Context, tx *sql.Tx, userID, analysisID string) (Usage, error) {
	var err error
	// Serialize reservation with terminal status writes. Reuse does not recharge.
	var owner, status string
	err = tx.QueryRowContext(ctx, `SELECT user_id,status FROM analyses WHERE id=$1 FOR UPDATE`, analysisID).Scan(&owner, &status)
	if err != nil {
		return Usage{}, err
	}
	if owner != userID {
		return Usage{}, ErrReservationConflict
	}
	u, err := s.lockAndEnsure(ctx, tx, userID)
	if err != nil {
		return Usage{}, err
	}
	var reservedOwner, state string
	err = tx.QueryRowContext(ctx, `SELECT user_id,state FROM analysis_usage_reservations WHERE analysis_id=$1`, analysisID).Scan(&reservedOwner, &state)
	if err == nil {
		if reservedOwner != userID || state == "released" {
			return Usage{}, ErrReservationConflict
		}
		return u, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Usage{}, err
	}
	if status != "queued" && status != "processing" {
		return Usage{}, ErrReservationConflict
	}
	if u.Used >= u.Limit {
		return Usage{}, ErrLimitReached
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO analysis_usage_reservations(analysis_id,user_id,resets_at,usage_epoch,state) VALUES($1,$2,$3,$4,'reserved')`, analysisID, userID, u.ResetsAt, u.Epoch); err != nil {
		return Usage{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE usage SET used=used+1 WHERE user_id=$1`, userID); err != nil {
		return Usage{}, err
	}
	u.Used++
	return u, nil
}

func (s *pgStore) SettleAnalysis(ctx context.Context, analysisID, status string) error {
	// The database status, not the caller's proposed status, determines settlement.
	var userID string
	err := s.DB.QueryRowContext(ctx, `SELECT user_id FROM analysis_usage_reservations WHERE analysis_id=$1`, analysisID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.ensure(ctx, userID)
	return err
}

func (s *pgStore) reconcileReservations(ctx context.Context, tx *sql.Tx, userID string, u Usage) (Usage, error) {
	// Terminal analyses cannot change terminal state. The usage row lock serializes
	// these ledger transitions with new reservations, reset and concurrent readers.
	var released int
	err := tx.QueryRowContext(ctx, `WITH terminal AS (
 SELECT r.analysis_id,a.status FROM analysis_usage_reservations r JOIN analyses a ON a.id=r.analysis_id
 WHERE r.user_id=$1 AND r.state='reserved' AND a.status IN ('completed','failed')
 ), settled AS (
 UPDATE analysis_usage_reservations r SET state=CASE t.status WHEN 'failed' THEN 'released' ELSE 'committed' END,settled_at=now()
 FROM terminal t WHERE r.analysis_id=t.analysis_id AND r.state='reserved'
 RETURNING r.state,r.resets_at,r.usage_epoch
 ) SELECT count(*) FROM settled WHERE state='released' AND resets_at=$2 AND usage_epoch=$3`, userID, u.ResetsAt, u.Epoch).Scan(&released)
	if err != nil {
		return Usage{}, err
	}
	if released > 0 {
		u.Used -= released
		if u.Used < 0 {
			u.Used = 0
		}
		if _, err = tx.ExecContext(ctx, `UPDATE usage SET used=$1 WHERE user_id=$2`, u.Used, userID); err != nil {
			return Usage{}, err
		}
	}
	return u, nil
}

// ClaimAnalysisReservationsTx carries current preview usage over with the
// claimed analyses. The guest counter is retained so claiming cannot refill it.
func (s *pgStore) ClaimAnalysisReservationsTx(ctx context.Context, tx *sql.Tx, guestID, accountID string) error {
	guest, err := s.lockAndEnsure(ctx, tx, guestID)
	if err != nil {
		return err
	}
	account, err := s.lockAndEnsure(ctx, tx, accountID)
	if err != nil {
		return err
	}
	moved, err := tx.ExecContext(ctx, `
UPDATE analysis_usage_reservations r
SET user_id=$2, resets_at=$5, usage_epoch=$6
FROM analyses a
WHERE a.id=r.analysis_id AND a.user_id=$2 AND a.deleted_at IS NULL
  AND r.user_id=$1 AND r.resets_at=$3 AND r.usage_epoch=$4
  AND r.state IN ('reserved','committed') AND r.resets_at=$5`,
		guestID, accountID, guest.ResetsAt, guest.Epoch, account.ResetsAt, account.Epoch)
	if err != nil {
		return err
	}
	count, err := moved.RowsAffected()
	if err != nil || count == 0 {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE usage SET used=used+$2 WHERE user_id=$1`, accountID, count)
	return err
}
