package aimeter

import (
	"context"
	"database/sql"
	"errors"
)

func (r *PGRecorder) Admit(ctx context.Context, a *Attempt, raw []byte) (prepared []byte, err error) {
	// Fail closed before provider send, without exposing raw storage errors.
	defer func() {
		if err != nil {
			if _, ok := Control(err); !ok {
				err = &ControlError{Code: CodeUnavailable}
			}
		}
	}()
	if r == nil || r.DB == nil {
		return nil, &ControlError{Code: CodeUnavailable}
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var policy spendPolicy
	err = tx.QueryRowContext(ctx, `SELECT calls_enabled,daily_limit_nano_usd,max_request_bytes,max_completion_tokens,max_attempts_per_operation FROM ai_spend_policy WHERE id=true FOR SHARE`).Scan(&policy.Enabled, &policy.Daily, &policy.RequestBytes, &policy.OutputTokens, &policy.Attempts)
	if err != nil {
		return nil, err
	}
	if !policy.Enabled {
		return nil, &ControlError{Code: CodePaused}
	}
	switch a.Operation {
	case "analysis", "review_answer", "resume_generation", "resume_tailoring", "apply":
	default:
		return nil, &ControlError{Code: CodeUnavailable}
	}
	prepared, reserved, err := prepareRequest(raw, policy, a.RequestedModel)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO ai_spend_operations(operation_id) VALUES($1) ON CONFLICT DO NOTHING`, a.OperationID); err != nil {
		return nil, err
	}
	var attempts int
	if err = tx.QueryRowContext(ctx, `SELECT admitted_attempts FROM ai_spend_operations WHERE operation_id=$1 FOR UPDATE`, a.OperationID).Scan(&attempts); err != nil {
		return nil, err
	}
	if attempts >= policy.Attempts {
		return nil, &ControlError{Code: CodeAttemptLimit}
	}
	var day string
	if err = tx.QueryRowContext(ctx, `SELECT to_char(now() AT TIME ZONE 'UTC','YYYY-MM-DD'),now()`).Scan(&day, &a.StartedAt); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO ai_spend_days(day_utc) VALUES($1) ON CONFLICT DO NOTHING`, day); err != nil {
		return nil, err
	}
	var booked int64
	var legacyCover sql.NullInt64
	var breach bool
	if err = tx.QueryRowContext(ctx, `SELECT booked_nano_usd,legacy_uncertain_cover_nano_usd,breach_detected FROM ai_spend_days WHERE day_utc=$1 FOR UPDATE`, day).Scan(&booked, &legacyCover, &breach); err != nil {
		return nil, err
	}
	if err = reconcileDay(ctx, tx, day); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT booked_nano_usd,breach_detected FROM ai_spend_days WHERE day_utc=$1`, day).Scan(&booked, &breach); err != nil {
		return nil, err
	}
	if breach {
		return nil, &ControlError{Code: CodeUnavailable}
	}
	// Include usage from older/unprotected instances during the deployment day.
	var legacyKnown int64
	var uncertain int
	err = tx.QueryRowContext(ctx, `SELECT coalesce(sum(u.estimate_nano_usd)*2,0),count(*) FILTER(WHERE u.estimate_nano_usd IS NULL)
 FROM ai_usage_attempts u WHERE started_at>=($1::date::timestamp AT TIME ZONE 'UTC') AND started_at<(($1::date+1)::timestamp AT TIME ZONE 'UTC')
 AND NOT EXISTS(SELECT 1 FROM ai_spend_holds h WHERE h.attempt_id=u.id)`, day).Scan(&legacyKnown, &uncertain)
	if err != nil {
		return nil, err
	}
	if uncertain > 0 && !legacyCover.Valid {
		return nil, &ControlError{Code: CodeUnavailable}
	}
	if booked+legacyKnown+legacyCover.Int64+reserved > policy.Daily {
		return nil, &ControlError{Code: CodeDailyBudget}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_usage_attempts(id,operation_id,operation,analysis_id,object_id,stage,retry_kind,attempt_number,provider,requested_model,started_at,outcome,usage_state,unknown_reason)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'pending','pending','awaiting_response')`, a.ID, a.OperationID, a.Operation, nullableID(a.AnalysisID), nullableID(a.ObjectID), a.Stage, a.Retry, a.Number, a.Provider, a.RequestedModel, a.StartedAt)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO ai_spend_holds(attempt_id,operation_id,day_utc,reserved_nano_usd,booked_nano_usd) VALUES($1,$2,$3,$4,$4)`, a.ID, a.OperationID, day, reserved); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ai_spend_days SET booked_nano_usd=booked_nano_usd+$2 WHERE day_utc=$1`, day, reserved); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ai_spend_operations SET admitted_attempts=admitted_attempts+1 WHERE operation_id=$1`, a.OperationID); err != nil {
		return nil, err
	}
	return prepared, tx.Commit()
}

func (r *PGRecorder) finishBudget(ctx context.Context, a Attempt) error {
	var day string
	err := r.DB.QueryRowContext(ctx, `SELECT day_utc::text FROM ai_spend_holds WHERE attempt_id=$1`, a.ID).Scan(&day)
	if errors.Is(err, sql.ErrNoRows) {
		return r.save(ctx, a, true)
	}
	if err != nil {
		return err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked string
	if err := tx.QueryRowContext(ctx, `SELECT day_utc::text FROM ai_spend_days WHERE day_utc=$1 FOR UPDATE`, day).Scan(&locked); err != nil {
		return err
	}
	if err := saveWith(ctx, tx, a, true); err != nil {
		return err
	}
	if err := reconcileDay(ctx, tx, day); err != nil {
		return err
	}
	return tx.Commit()
}

func reconcileDay(ctx context.Context, tx *sql.Tx, day string) error {
	// Unknown/unpriced outcomes keep their full reservation, even after a timeout.
	rows, err := tx.QueryContext(ctx, `SELECT h.attempt_id,h.reserved_nano_usd,h.booked_nano_usd,u.estimate_nano_usd
 FROM ai_spend_holds h JOIN ai_usage_attempts u ON u.id=h.attempt_id
 WHERE h.day_utc=$1 AND h.state='held' AND u.usage_state='known' AND u.estimate_nano_usd IS NOT NULL FOR UPDATE OF h`, day)
	if err != nil {
		return err
	}
	type adjustment struct {
		id                    string
		reserved, old, actual int64
	}
	var changes []adjustment
	for rows.Next() {
		var c adjustment
		if err := rows.Scan(&c.id, &c.reserved, &c.old, &c.actual); err != nil {
			rows.Close()
			return err
		}
		changes = append(changes, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var delta int64
	breach := false
	for _, c := range changes {
		charged := 2 * c.actual
		delta += charged - c.old
		if charged > c.reserved {
			breach = true
		}
		if _, err := tx.ExecContext(ctx, `UPDATE ai_spend_holds SET booked_nano_usd=$2,state='settled' WHERE attempt_id=$1`, c.id, charged); err != nil {
			return err
		}
	}
	if len(changes) > 0 {
		result, err := tx.ExecContext(ctx, `UPDATE ai_spend_days SET booked_nano_usd=booked_nano_usd+$2,breach_detected=breach_detected OR $3 WHERE day_utc=$1 AND booked_nano_usd+$2>=0`, day, delta, breach)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.New("inconsistent spend total")
		}
	}
	return nil
}
