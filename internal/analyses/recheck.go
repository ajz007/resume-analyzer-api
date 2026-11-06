package analyses

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/usage"
)

type recheckSummary struct {
	AnalysisID       string   `json:"analysisId"`
	DocumentID       string   `json:"documentId"`
	Status           string   `json:"status"`
	Score            *float64 `json:"score,omitempty"`
	PaidReviewCredit string   `json:"paidReviewCredit,omitempty"`
}

func (h *Handler) startRecheck(c *gin.Context) {
	ctx := withRequestID(c.Request.Context(), middleware.RequestIDFromContext(c))
	user := middleware.UserIDFromContext(c)
	if user == "" || usage.IsGuestUserID(user) || h.Unlocks == nil || !h.Unlocks.PaywallActive() {
		respond.Error(c, 403, "credits_unavailable", "Sign in to recheck a saved resume.", nil)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	original, err := h.Svc.Get(ctx, id.String())
	if err != nil || original.UserID != user {
		c.Status(http.StatusNotFound)
		return
	}
	if original.Status != StatusCompleted || original.Mode != ModeJobMatch || strings.TrimSpace(original.JobDescription) == "" {
		respond.Error(c, 409, "review_unavailable", "A completed job match review is required.", nil)
		return
	}
	unlocked, err := h.Unlocks.IsUnlocked(ctx, user, original.ID)
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if !unlocked {
		c.Status(http.StatusForbidden)
		return
	}
	repo, ok := h.Svc.Repo.(*PGRepo)
	if !ok {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	var isRecheck bool
	if err := repo.DB.QueryRowContext(ctx, `SELECT recheck_of IS NOT NULL FROM analyses WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, original.ID, user).Scan(&isRecheck); err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if isRecheck {
		respond.Error(c, 409, "review_unavailable", "Start a recheck from the original job review.", nil)
		return
	}
	var req struct {
		DocumentID string `json:"documentId"`
		Retry      bool   `json:"retry"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if err := decodeOptionalJSON(c.Request.Body, &req); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	docID, err := uuid.Parse(req.DocumentID)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	doc, err := h.DocRepo.GetByID(ctx, user, docID.String())
	if err != nil || doc.TailoredAnalysisID != original.ID {
		c.Status(http.StatusNotFound)
		return
	}
	analysis, _, err := h.Svc.startLinkedReview(ctx, doc.ID, user, original.JobDescription, original.PromptVersion, ModeJobMatch, req.Retry, true, original.ID)
	switch {
	case errors.Is(err, ErrNoReviewCredit):
		respond.Error(c, 409, "review_credit_unavailable", err.Error(), nil)
	case errors.Is(err, ErrRetryRequired):
		respond.Error(c, 409, "retry_required", "The last recheck failed. Choose Retry to try again.", nil)
	case err != nil:
		respond.Error(c, 503, "recheck_unavailable", "Could not start this recheck. Please try again.", nil)
	default:
		respond.JSON(c, http.StatusAccepted, gin.H{"analysisId": analysis.ID})
	}
}

func (h *Handler) listRechecks(c *gin.Context) {
	ctx := c.Request.Context()
	user := middleware.UserIDFromContext(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	original, err := h.Svc.Get(ctx, id.String())
	if err != nil || original.UserID != user {
		c.Status(http.StatusNotFound)
		return
	}
	repo, ok := h.Svc.Repo.(*PGRepo)
	if !ok {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	var parentID string
	err = repo.DB.QueryRowContext(ctx, `SELECT COALESCE(recheck_of::text, '') FROM analyses WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, original.ID, user).Scan(&parentID)
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	items := []recheckSummary{}
	if parentID == "" {
		rows, err := repo.DB.QueryContext(ctx, `
			SELECT id, document_id, status FROM (
				SELECT DISTINCT ON (document_id) id, document_id, status, created_at
				FROM analyses WHERE recheck_of=$1 AND user_id=$2 AND deleted_at IS NULL
				ORDER BY document_id, created_at DESC, id DESC
			) latest ORDER BY created_at DESC, id DESC`, original.ID, user)
		if err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		for rows.Next() {
			var item recheckSummary
			if err := rows.Scan(&item.AnalysisID, &item.DocumentID, &item.Status); err != nil {
				rows.Close()
				c.Status(http.StatusServiceUnavailable)
				return
			}
			items = append(items, item)
		}
		rows.Close()
		if rows.Err() != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		if len(items) > 0 && items[0].Status == StatusFailed {
			latest := Analysis{ID: items[0].AnalysisID, UserID: user, Status: StatusFailed}
			if credit, err := repo.paidCreditStatus(ctx, latest); err == nil {
				items[0].PaidReviewCredit = credit
			}
		}
		for i := range items {
			if items[i].Status != StatusCompleted {
				continue
			}
			child, err := h.Svc.Get(ctx, items[i].AnalysisID)
			if err != nil {
				c.Status(http.StatusServiceUnavailable)
				return
			}
			if score, ok := extractNestedFloat(child.Result, "jobMatchScoring", "score"); ok {
				items[i].Score = &score
			}
		}
	}
	c.Header("Cache-Control", "no-store")
	respond.JSON(c, http.StatusOK, gin.H{"originalAnalysisId": parentID, "rechecks": items})
}
