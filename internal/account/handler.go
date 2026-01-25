package account

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
)

type Handler struct {
	Svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/account/claim-guest", h.claimGuest)
}

func (h *Handler) claimGuest(c *gin.Context) {
	if h.Svc == nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "service unavailable", nil)
		return
	}
	if isGuest, ok := c.Get("isGuest"); ok {
		if guest, ok2 := isGuest.(bool); ok2 && guest {
			respond.Error(c, http.StatusUnauthorized, "unauthorized", "login required", nil)
			return
		}
	}

	authedUserID := strings.TrimSpace(middleware.UserIDFromContext(c))
	if authedUserID == "" {
		respond.Error(c, http.StatusUnauthorized, "unauthorized", "login required", nil)
		return
	}

	guestID := strings.TrimSpace(c.GetHeader("X-Guest-Id"))
	if guestID == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "missing X-Guest-Id header", []map[string]string{
			{"field": "X-Guest-Id", "issue": "required"},
		})
		return
	}
	if _, err := uuid.Parse(guestID); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid guest id", []map[string]string{
			{"field": "X-Guest-Id", "issue": "invalid"},
		})
		return
	}

	guestUserID := "guest:" + guestID
	result, err := h.Svc.ClaimGuest(c.Request.Context(), guestUserID, authedUserID)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to claim guest data", nil)
		return
	}
	respond.JSON(c, http.StatusOK, result)
}
