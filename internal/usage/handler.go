package usage

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
)

// Handler exposes usage endpoints.
type Handler struct {
	Svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

// RegisterRoutes attaches usage routes to the router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/usage", h.getUsage)
}

// RegisterDevRoutes attaches dev-only usage routes.
func (h *Handler) RegisterDevRoutes(rg *gin.RouterGroup) {
	rg.POST("/usage/reset", h.resetUsage)
}

func (h *Handler) getUsage(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	u, err := h.Svc.EnsurePeriod(c.Request.Context(), userID)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			respond.Error(c, http.StatusRequestTimeout, "timeout", "request canceled", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch usage", nil)
		}
		return
	}

	respond.JSON(c, http.StatusOK, gin.H{
		"plan":     u.Plan,
		"limit":    u.Limit,
		"used":     u.Used,
		"resetsAt": u.ResetsAt,
	})
}

func (h *Handler) resetUsage(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	u, err := h.Svc.Reset(c.Request.Context(), userID)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			respond.Error(c, http.StatusRequestTimeout, "timeout", "request canceled", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to reset usage", nil)
		}
		return
	}

	respond.JSON(c, http.StatusOK, gin.H{
		"plan":     u.Plan,
		"limit":    u.Limit,
		"used":     u.Used,
		"resetsAt": u.ResetsAt,
	})
}
