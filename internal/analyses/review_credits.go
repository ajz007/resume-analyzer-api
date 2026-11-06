package analyses

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/usage"
)

var (
	ErrNoReviewCredit       = errors.New("No paid review credit is available.")
	ErrCreditsUnavailable   = errors.New("Review credits are unavailable.")
	ErrCreditCheckoutExists = errors.New("An earlier checkout exists for this review. Check that checkout first.")
)

func spendReviewCredit(ctx context.Context, tx *sql.Tx, user, id string) error {
	// The analysis lock makes two requests for the same report idempotent.
	var owner string
	if err := tx.QueryRowContext(ctx, `SELECT user_id FROM analyses WHERE id=$1 FOR UPDATE`, id).Scan(&owner); err != nil {
		return err
	}
	if owner != user {
		return ErrForbidden
	}
	var unlocked, legacyOrder bool
	if err := tx.QueryRowContext(ctx, `
 SELECT EXISTS(SELECT 1 FROM report_unlocks WHERE analysis_id=$1 AND user_id=$2)
 OR EXISTS(SELECT 1 FROM review_credit_uses u JOIN payments p ON p.id=u.payment_id
 WHERE u.analysis_id=$1 AND u.user_id=$2 AND u.released_at IS NULL AND p.status='paid'),
 EXISTS(SELECT 1 FROM payment_purchases WHERE analysis_id=$1 AND user_id=$2)`, id, user).Scan(&unlocked, &legacyOrder); err != nil {
		return err
	}
	if unlocked {
		return nil
	}
	if legacyOrder {
		return ErrCreditCheckoutExists
	}
	var paymentID string
	// Lock the purchased credit before attaching it. The partial unique index is the last line of defense.
	err := tx.QueryRowContext(ctx, `
 SELECT p.id FROM review_credit_orders o JOIN payments p ON p.analysis_id=o.id AND p.user_id=o.user_id
 WHERE o.user_id=$1 AND p.status='paid' AND NOT EXISTS (
 SELECT 1 FROM review_credit_uses u JOIN analyses a ON a.id=u.analysis_id
 WHERE u.payment_id=p.id AND u.released_at IS NULL AND a.status<>'failed')
 ORDER BY o.created_at,p.id LIMIT 1 FOR UPDATE OF p SKIP LOCKED`, user).Scan(&paymentID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoReviewCredit
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE review_credit_uses u SET released_at=now()
 FROM analyses a WHERE a.id=u.analysis_id AND a.status='failed' AND u.payment_id=$1 AND u.released_at IS NULL`, paymentID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE review_credit_uses u SET released_at=now()
 FROM payments p WHERE p.id=u.payment_id AND p.status='refunded' AND u.analysis_id=$1 AND u.released_at IS NULL`, id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO review_credit_uses(analysis_id,payment_id,user_id)
 VALUES($1,$2,$3)`, id, paymentID, user)
	return err
}

func (h *Handler) useReviewCredit(c *gin.Context) {
	user := middleware.UserIDFromContext(c)
	repo, ok := h.Svc.Repo.(*PGRepo)
	if !ok || user == "" || usage.IsGuestUserID(user) || h.Unlocks == nil || !h.Unlocks.PaywallActive() {
		respond.Error(c, http.StatusForbidden, "credits_unavailable", "Sign in to use review credits.", nil)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respond.Error(c, http.StatusNotFound, "not_found", "Review not found.", nil)
		return
	}
	ctx := c.Request.Context()
	tx, err := repo.DB.BeginTx(ctx, nil)
	if err != nil {
		respond.Error(c, http.StatusServiceUnavailable, "credits_unavailable", "Could not use a credit.", nil)
		return
	}
	defer tx.Rollback()
	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM analyses WHERE id=$1 AND user_id=$2 AND mode='JOB_MATCH' AND deleted_at IS NULL FOR UPDATE`, id, user).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && status != "completed") {
		respond.Error(c, http.StatusNotFound, "not_found", "Completed job review not found.", nil)
		return
	}
	if err == nil {
		err = spendReviewCredit(ctx, tx, user, id.String())
	}
	if errors.Is(err, ErrNoReviewCredit) || errors.Is(err, ErrCreditCheckoutExists) {
		respond.Error(c, http.StatusConflict, "review_credit_unavailable", err.Error(), nil)
		return
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respond.Error(c, http.StatusServiceUnavailable, "credits_unavailable", "Could not use a credit. Try again.", nil)
		return
	}
	c.Status(http.StatusNoContent)
}

// paidCreditStatus describes only the credit attached to this owner's review.
func (r *PGRepo) paidCreditStatus(ctx context.Context, a Analysis) (string, error) {
	var status string
	switch a.Status {
	case StatusFailed:
		status = "returned"
	case StatusQueued, StatusProcessing:
		status = "reserved"
	default:
		return "", nil
	}
	var found bool
	err := r.DB.QueryRowContext(ctx, `
SELECT EXISTS (
 SELECT 1 FROM review_credit_uses u JOIN payments p ON p.id=u.payment_id
 WHERE u.analysis_id=$1 AND u.user_id=$2 AND p.user_id=$2 AND p.status='paid'
   AND ($3='returned' OR u.released_at IS NULL)
)`, a.ID, a.UserID, status).Scan(&found)
	if err != nil || !found {
		return "", err
	}
	return status, nil
}
