package payments

import (
	"context"
	"database/sql"
	"errors"
)

// PGRepo is a Postgres-backed Repo.
type PGRepo struct {
	DB *sql.DB
}

const paymentColumns = `id, user_id, analysis_id, razorpay_order_id, COALESCE(razorpay_payment_id, ''),
       amount_minor, currency, status, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPayment(row rowScanner) (Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.UserID, &p.AnalysisID, &p.RazorpayOrderID, &p.RazorpayPaymentID,
		&p.AmountMinor, &p.Currency, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	return p, err
}

func (r *PGRepo) Create(ctx context.Context, p Payment) error {
	_, err := r.DB.ExecContext(ctx, `
INSERT INTO payments (id, user_id, analysis_id, razorpay_order_id, amount_minor, currency, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		p.ID, p.UserID, p.AnalysisID, p.RazorpayOrderID, p.AmountMinor, p.Currency, p.Status, p.CreatedAt)
	return err
}

func (r *PGRepo) GetByOrderID(ctx context.Context, orderID string) (Payment, error) {
	row := r.DB.QueryRowContext(ctx, `SELECT `+paymentColumns+` FROM payments WHERE razorpay_order_id = $1`, orderID)
	return scanPayment(row)
}

func (r *PGRepo) MarkPaidAndUnlock(ctx context.Context, orderID, paymentID string) (Payment, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Payment{}, err
	}
	defer tx.Rollback()

	p, err := scanPayment(tx.QueryRowContext(ctx, `SELECT `+paymentColumns+` FROM payments WHERE razorpay_order_id = $1 FOR UPDATE`, orderID))
	if err != nil {
		return Payment{}, err
	}
	// A refund is final; a late checkout callback or webhook must not re-unlock.
	if p.Status == StatusRefunded {
		return p, tx.Commit()
	}
	if p.Status != StatusPaid {
		row := tx.QueryRowContext(ctx, `
UPDATE payments
SET status = $1, razorpay_payment_id = COALESCE(razorpay_payment_id, $2), updated_at = now()
WHERE razorpay_order_id = $3
RETURNING `+paymentColumns, StatusPaid, paymentID, orderID)
		if p, err = scanPayment(row); err != nil {
			return Payment{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO report_unlocks (user_id, analysis_id, payment_id)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, analysis_id) DO NOTHING`, p.UserID, p.AnalysisID, p.ID); err != nil {
		return Payment{}, err
	}
	return p, tx.Commit()
}

func (r *PGRepo) MarkRefunded(ctx context.Context, paymentID string) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id string
	err = tx.QueryRowContext(ctx, `
UPDATE payments SET status = $1, updated_at = now()
WHERE razorpay_payment_id = $2
RETURNING id`, StatusRefunded, paymentID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM report_unlocks WHERE payment_id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PGRepo) IsUnlocked(ctx context.Context, userID, analysisID string) (bool, error) {
	var exists bool
	err := r.DB.QueryRowContext(ctx, `
SELECT EXISTS (SELECT 1 FROM report_unlocks WHERE user_id = $1 AND analysis_id = $2)`, userID, analysisID).Scan(&exists)
	return exists, err
}
