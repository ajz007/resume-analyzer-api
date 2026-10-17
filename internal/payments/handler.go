package payments

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
)

// WebhookPath is exempt from auth; it is authenticated by its HMAC signature.
const WebhookPath = "/api/v1/payments/webhook"

const maxWebhookBody = 1 << 20

// Handler exposes payment endpoints.
type Handler struct {
	Svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

// RegisterRoutes attaches payment routes to the router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/payments/config", h.getConfig)
	rg.POST("/payments/orders", h.createOrder)
	rg.POST("/payments/verify", h.verify)
	rg.POST("/payments/webhook", h.webhook)
}

func isGuest(c *gin.Context) bool {
	val, ok := c.Get("isGuest")
	if !ok {
		return false
	}
	guest, _ := val.(bool)
	return guest
}

func (h *Handler) getConfig(c *gin.Context) {
	respond.JSON(c, http.StatusOK, gin.H{
		"paywallEnabled": h.Svc.PaywallActive(),
		"prices":         h.Svc.ListPrices(),
	})
}

type createOrderRequest struct {
	AnalysisID string `json:"analysisId"`
	Currency   string `json:"currency"`
}

func (h *Handler) createOrder(c *gin.Context) {
	if isGuest(c) {
		respond.Error(c, http.StatusUnauthorized, "login_required", ErrLoginRequired.Error(), nil)
		return
	}
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.AnalysisID) == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "analysisId is required", nil)
		return
	}
	order, err := h.Svc.CreateOrder(c.Request.Context(), middleware.UserIDFromContext(c), strings.TrimSpace(req.AnalysisID), req.Currency)
	if err != nil {
		switch {
		case errors.Is(err, ErrPaymentsDisabled):
			respond.Error(c, http.StatusServiceUnavailable, "payments_disabled", err.Error(), nil)
		case errors.Is(err, ErrInvalidCurrency):
			respond.Error(c, http.StatusBadRequest, "validation_error", err.Error(), nil)
		case errors.Is(err, ErrAnalysisNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "analysis not found", nil)
		case errors.Is(err, ErrAnalysisNotReady):
			respond.Error(c, http.StatusConflict, "analysis_not_ready", err.Error(), nil)
		case errors.Is(err, ErrAlreadyUnlocked):
			respond.Error(c, http.StatusConflict, "already_unlocked", err.Error(), nil)
		default:
			respond.Error(c, http.StatusBadGateway, "payment_provider_error", "failed to create order", err)
		}
		return
	}
	respond.JSON(c, http.StatusOK, order)
}

type verifyRequest struct {
	OrderID   string `json:"orderId"`
	PaymentID string `json:"paymentId"`
	Signature string `json:"signature"`
}

func (h *Handler) verify(c *gin.Context) {
	if isGuest(c) {
		respond.Error(c, http.StatusUnauthorized, "login_required", ErrLoginRequired.Error(), nil)
		return
	}
	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.OrderID == "" || req.PaymentID == "" || req.Signature == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "orderId, paymentId and signature are required", nil)
		return
	}
	p, err := h.Svc.VerifyCheckout(c.Request.Context(), middleware.UserIDFromContext(c), req.OrderID, req.PaymentID, req.Signature)
	if err != nil {
		switch {
		case errors.Is(err, ErrPaymentsDisabled):
			respond.Error(c, http.StatusServiceUnavailable, "payments_disabled", err.Error(), nil)
		case errors.Is(err, ErrInvalidSignature):
			respond.Error(c, http.StatusBadRequest, "invalid_signature", err.Error(), nil)
		case errors.Is(err, ErrNotFound), errors.Is(err, ErrOrderUserMismatch):
			respond.Error(c, http.StatusNotFound, "not_found", "order not found", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to verify payment", err)
		}
		return
	}
	respond.JSON(c, http.StatusOK, gin.H{
		"analysisId": p.AnalysisID,
		"status":     p.Status,
		"unlocked":   p.Status == StatusPaid,
	})
}

func (h *Handler) webhook(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBody))
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "failed to read body", nil)
		return
	}
	if err := h.Svc.HandleWebhook(c.Request.Context(), body, c.GetHeader("X-Razorpay-Signature")); err != nil {
		switch {
		case errors.Is(err, ErrInvalidSignature):
			respond.Error(c, http.StatusBadRequest, "invalid_signature", err.Error(), nil)
		case errors.Is(err, ErrAmountMismatch):
			// Acknowledge so Razorpay stops retrying; the mismatch is logged for review.
			respond.Error(c, http.StatusOK, "amount_mismatch", err.Error(), nil)
		default:
			// Non-2xx makes Razorpay retry the webhook later.
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to process webhook", err)
		}
		return
	}
	respond.JSON(c, http.StatusOK, gin.H{"ok": true})
}
