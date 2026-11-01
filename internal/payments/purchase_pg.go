package payments

import (
	"context"
	"database/sql"
	"errors"
)

const purchaseColumns = `c.payment_id, c.user_id, c.analysis_id, COALESCE(p.razorpay_order_id, ''), COALESCE(p.razorpay_payment_id, ''), c.amount_minor, c.currency, COALESCE(p.status, 'created'), c.created_at, COALESCE(p.updated_at, c.created_at), c.legacy_ambiguous`

func (r *PGRepo) ReservePurchase(ctx context.Context, p Payment) (Payment, bool, error) {
	// Adopt any legacy order written after the migration but before this code deployed.
	if err := r.adoptLegacyPurchase(ctx, p.UserID, p.AnalysisID); err != nil {
		return Payment{}, false, err
	}
	// A unique key in PostgreSQL elects the creator across processes and Lambda instances.
	result, err := r.DB.ExecContext(ctx, `INSERT INTO payment_purchases (user_id,analysis_id,payment_id,amount_minor,currency,created_at)
 VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (user_id,analysis_id) DO NOTHING`, p.UserID, p.AnalysisID, p.ID, p.AmountMinor, p.Currency, p.CreatedAt)
	if err != nil {
		return Payment{}, false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Payment{}, false, err
	}
	existing, err := r.GetPurchase(ctx, p.UserID, p.AnalysisID)
	// If this read fails, the reservation remains. Never elect a second creator.
	return existing, n == 1, err
}
func (r *PGRepo) GetPurchase(ctx context.Context, userID, analysisID string) (Payment, error) {
	p, err := r.getPurchase(ctx, userID, analysisID)
	if !errors.Is(err, ErrNotFound) {
		return p, err
	}
	if err := r.adoptLegacyPurchase(ctx, userID, analysisID); err != nil {
		return Payment{}, err
	}
	return r.getPurchase(ctx, userID, analysisID)
}
func (r *PGRepo) getPurchase(ctx context.Context, userID, analysisID string) (Payment, error) {
	var p Payment
	err := r.DB.QueryRowContext(ctx, `SELECT `+purchaseColumns+` FROM payment_purchases c LEFT JOIN payments p ON p.id=c.payment_id WHERE c.user_id=$1 AND c.analysis_id=$2`, userID, analysisID).Scan(&p.ID, &p.UserID, &p.AnalysisID, &p.RazorpayOrderID, &p.RazorpayPaymentID, &p.AmountMinor, &p.Currency, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.NeedsSupport)
	if errors.Is(err, sql.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	return p, err
}
func (r *PGRepo) adoptLegacyPurchase(ctx context.Context, userID, analysisID string) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO payment_purchases(user_id,analysis_id,payment_id,amount_minor,currency,created_at,legacy_ambiguous)
 SELECT user_id,analysis_id,id,amount_minor,currency,created_at,count(*) OVER () > 1
 FROM payments WHERE user_id=$1 AND analysis_id=$2
 AND NOT EXISTS(SELECT 1 FROM payment_purchases WHERE user_id=$1 AND analysis_id=$2)
 ORDER BY CASE status WHEN 'paid' THEN 0 WHEN 'created' THEN 1 WHEN 'failed' THEN 2 ELSE 3 END,created_at,id
 LIMIT 1 ON CONFLICT(user_id,analysis_id) DO NOTHING`, userID, analysisID)
	return err
}

func (r *PGRepo) AttachOrder(ctx context.Context, p Payment, orderID string) (Payment, error) {
	if orderID == "" {
		return Payment{}, ErrPurchaseConflict
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Payment{}, err
	}
	defer tx.Rollback()
	var id, currency string
	var amount int64
	var needsSupport bool
	err = tx.QueryRowContext(ctx, `SELECT payment_id,amount_minor,currency,legacy_ambiguous FROM payment_purchases WHERE user_id=$1 AND analysis_id=$2 FOR UPDATE`, p.UserID, p.AnalysisID).Scan(&id, &amount, &currency, &needsSupport)
	if errors.Is(err, sql.ErrNoRows) {
		return Payment{}, ErrPurchaseConflict
	}
	if err != nil {
		return Payment{}, err
	}
	if needsSupport || id != p.ID || amount != p.AmountMinor || currency != p.Currency {
		return Payment{}, ErrPurchaseConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO payments (id,user_id,analysis_id,razorpay_order_id,amount_minor,currency,status,created_at,updated_at)
 VALUES ($1,$2,$3,$4,$5,$6,'created',$7,now()) ON CONFLICT (id) DO NOTHING`, p.ID, p.UserID, p.AnalysisID, orderID, p.AmountMinor, p.Currency, p.CreatedAt)
	if err != nil {
		return Payment{}, err
	}
	attached, err := scanPayment(tx.QueryRowContext(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id=$1`, p.ID))
	if err != nil {
		return Payment{}, err
	}
	if attached.RazorpayOrderID != orderID || attached.UserID != p.UserID || attached.AnalysisID != p.AnalysisID || attached.AmountMinor != p.AmountMinor || attached.Currency != p.Currency {
		return Payment{}, ErrPurchaseConflict
	}
	return attached, tx.Commit()
}
