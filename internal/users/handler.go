package users

import (
	"net/http"

	"github.com/gin-gonic/gin"

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
	rg.GET("/me", h.me)
}

func (h *Handler) me(c *gin.Context) {
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
	userID := middleware.UserIDFromContext(c)
	user, err := h.Svc.GetByID(c.Request.Context(), userID)
	if err != nil {
		if err == ErrNotFound {
			respond.Error(c, http.StatusNotFound, "not_found", "user not found", nil)
			return
		}
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to load user", nil)
		return
	}
	respond.JSON(c, http.StatusOK, gin.H{
		"id":         user.ID,
		"email":      user.Email,
		"fullName":   user.FullName,
		"pictureUrl": user.PictureURL,
	})
}
