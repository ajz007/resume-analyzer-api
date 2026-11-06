package payments

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
)

func (h *Handler) creditAccount(c *gin.Context) (*PGRepo, string, bool) {
	user := middleware.UserIDFromContext(c)
	if isGuest(c) || user == "" {
		respond.Error(c, http.StatusUnauthorized, "login_required", "Sign in to use review credits.", nil)
		return nil, "", false
	}
	repo, ok := h.Svc.Repo.(*PGRepo)
	if !ok || !h.Svc.PaywallActive() || h.Svc.Orders == nil || h.Svc.Prices[CurrencyINR] <= 0 {
		respond.Error(c, http.StatusNotFound, "credits_unavailable", "Review credits are unavailable.", nil)
		return nil, "", false
	}
	return repo, user, true
}

func (h *Handler) getCredits(c *gin.Context) {
	repo, user, ok := h.creditAccount(c)
	if !ok {
		return
	}
	var count int
	err := repo.DB.QueryRowContext(c.Request.Context(), `
 SELECT count(*) FROM review_credit_orders o JOIN payments p ON p.analysis_id=o.id AND p.user_id=o.user_id
 WHERE o.user_id=$1 AND p.status='paid' AND NOT EXISTS (
 SELECT 1 FROM review_credit_uses u JOIN analyses a ON a.id=u.analysis_id
 WHERE u.payment_id=p.id AND u.released_at IS NULL AND a.status<>'failed')`, user).Scan(&count)
	if err != nil {
		respond.Error(c, http.StatusServiceUnavailable, "credits_unavailable", "Could not check your review credits.", nil)
		return
	}
	respond.JSON(c, http.StatusOK, gin.H{"available": count, "price": h.Svc.Prices[CurrencyINR], "currency": CurrencyINR})
}

func (h *Handler) buyCredit(c *gin.Context)     { h.creditOrder(c, true) }
func (h *Handler) recoverCredit(c *gin.Context) { h.creditOrder(c, false) }

func (h *Handler) creditOrder(c *gin.Context, create bool) {
	repo, user, ok := h.creditAccount(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "invalid_order", "Invalid checkout reference.", nil)
		return
	}
	ctx := c.Request.Context()
	if create {
		_, err = repo.DB.ExecContext(ctx, `INSERT INTO review_credit_orders(id,user_id) VALUES($1,$2) ON CONFLICT(id) DO NOTHING`, id, user)
		if err != nil {
			respond.Error(c, http.StatusServiceUnavailable, "credits_unavailable", "Could not prepare checkout.", nil)
			return
		}
	}
	if err = repo.checkCreditOwner(ctx, id, user); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respond.Error(c, http.StatusNotFound, "not_found", "Checkout not found.", nil)
			return
		}
		respond.Error(c, http.StatusServiceUnavailable, "credits_unavailable", "Could not check checkout.", nil)
		return
	}
	if create {
		order, err := h.Svc.createPurchaseOrder(ctx, user, id.String(), CurrencyINR, h.Svc.Prices[CurrencyINR])
		if err == nil {
			respond.JSON(c, http.StatusOK, PurchaseStatus{State: "ready", Order: &order})
			return
		}
		if !errors.Is(err, ErrAlreadyUnlocked) && !errors.Is(err, ErrPurchasePending) {
			respond.Error(c, http.StatusConflict, "credit_checkout_unavailable", "Could not start this checkout. Check its status before trying again.", nil)
			return
		}
	}
	state, err := h.Svc.recoverPurchaseTarget(ctx, user, id.String())
	if err != nil {
		respond.Error(c, http.StatusServiceUnavailable, "credits_unavailable", "Could not confirm this checkout. Try checking again.", nil)
		return
	}
	respond.JSON(c, http.StatusOK, state)
}

func (r *PGRepo) checkCreditOwner(ctx context.Context, id uuid.UUID, user string) error {
	var found int
	return r.DB.QueryRowContext(ctx, `SELECT 1 FROM review_credit_orders WHERE id=$1 AND user_id=$2`, id, user).Scan(&found)
}
