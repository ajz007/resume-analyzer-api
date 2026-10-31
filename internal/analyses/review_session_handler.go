package analyses

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"resume-backend/internal/shared/server/middleware"
)

func (h *Handler) reviewAccess(c *gin.Context) (Analysis, bool) {
	a, err := h.Svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil || a.UserID != middleware.UserIDFromContext(c) {
		c.JSON(404, gin.H{"error": "report not found"})
		return a, false
	}
	if a.Status != StatusCompleted {
		c.JSON(409, gin.H{"error": "report is not complete"})
		return a, false
	}
	_, locked, _, err := h.applyPaywall(c.Request.Context(), a.UserID, a.ID, a.Result)
	if err != nil {
		c.JSON(500, gin.H{"error": "could not verify report access"})
		return a, false
	}
	if locked {
		c.JSON(403, gin.H{"error": "unlock the report before strengthening it"})
		return a, false
	}
	if _, _, err = originalReview(a.Result); err != nil || strings.TrimSpace(a.JobDescription) == "" {
		c.JSON(422, gin.H{"error": "a job-specific V3 report is required"})
		return a, false
	}
	c.Header("Cache-Control", "no-store")
	return a, true
}

func decodeReviewRequest(c *gin.Context, value any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("expected one JSON request")
	}
	return nil
}

func (h *Handler) getReviewSession(c *gin.Context) {
	a, ok := h.reviewAccess(c)
	if !ok {
		return
	}
	state, err := reviewState(a.Result)
	if err != nil {
		c.JSON(500, gin.H{"error": "review could not be loaded"})
		return
	}
	// A crashed process may leave a reservation. Expiration records failure and keeps
	// its attempt charged against the limit; it never automatically resends a model call.
	if state.Pending != nil && time.Since(state.Pending.StartedAt) > 6*time.Minute {
		if repo, supported := h.Svc.Repo.(reviewSessionRepo); supported {
			expected := state.Version
			state.Version++
			if state.Pending.Request != nil {
				state.LastRequest = state.Pending.Request
			}
			state.Pending = nil
			state.LastError = "The previous answer request did not finish. Earlier edits are unchanged."
			if err = repo.CompareAndSwapReview(c.Request.Context(), a.ID, a.UserID, expected, state); err != nil {
				c.JSON(409, gin.H{"error": "review changed; reload"})
				return
			}
		}
	}
	h.sendReviewState(c, a, state)
}

func (h *Handler) saveReviewChoices(c *gin.Context) {
	a, ok := h.reviewAccess(c)
	if !ok {
		return
	}
	var request struct {
		Version     int      `json:"version"`
		ExcludedIDs []string `json:"excludedIds"`
	}
	if err := decodeReviewRequest(c, &request); err != nil {
		c.JSON(400, gin.H{"error": "invalid review choices"})
		return
	}
	state, err := reviewState(a.Result)
	if err != nil {
		c.JSON(500, gin.H{"error": "review could not be loaded"})
		return
	}
	if state.Pending != nil || state.Version != request.Version {
		c.JSON(409, gin.H{"error": "review changed; reload before saving"})
		return
	}
	report, _, err := mergedReview(a.Result, state)
	if err == nil {
		err = validateReviewChoices(report, request.ExcludedIDs)
	}
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid edit choices"})
		return
	}
	repo, ok := h.Svc.Repo.(reviewSessionRepo)
	if !ok {
		c.JSON(503, gin.H{"error": "review persistence unavailable"})
		return
	}
	state.ExcludedIDs = append([]string{}, request.ExcludedIDs...)
	state.Version++
	if err = repo.CompareAndSwapReview(c.Request.Context(), a.ID, a.UserID, request.Version, state); err != nil {
		c.JSON(409, gin.H{"error": "review changed; reload before saving"})
		return
	}
	h.sendReviewState(c, a, state)
}

func (h *Handler) answerReviewQuestion(c *gin.Context) {
	a, ok := h.reviewAccess(c)
	if !ok {
		return
	}
	var request ReviewAnswerRequest
	if err := decodeReviewRequest(c, &request); err != nil {
		c.JSON(400, gin.H{"error": "invalid answer request"})
		return
	}
	doc, err := h.DocRepo.GetByID(c.Request.Context(), a.UserID, a.DocumentID)
	if err != nil {
		c.JSON(404, gin.H{"error": "source document not found"})
		return
	}
	source, err := loadText(c.Request.Context(), h.Svc.Store, doc.ExtractedTextKey)
	if err != nil || strings.TrimSpace(source) == "" {
		c.JSON(422, gin.H{"error": "original source is unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 240*time.Second)
	defer cancel()
	state, err := h.Svc.answerReview(ctx, a, source, request)
	if err != nil {
		code := 422
		if errors.Is(err, ErrReviewConflict) {
			code = 409
		}
		// Never return provider responses or candidate content in error messages.
		message := "The answer could not be processed. Check the requirement, answer and confirmation, then reload the review before retrying."
		if code == 409 {
			message = "This review has changed or an answer is processing. Reload the latest review before retrying."
		}
		c.JSON(code, gin.H{"error": message})
		return
	}
	h.sendReviewState(c, a, state)
}

func (h *Handler) sendReviewState(c *gin.Context, a Analysis, state ReviewSession) {
	doc, err := h.DocRepo.GetByID(c.Request.Context(), a.UserID, a.DocumentID)
	if err != nil {
		c.JSON(404, gin.H{"error": "source document not found"})
		return
	}
	source, err := loadText(c.Request.Context(), h.Svc.Store, doc.ExtractedTextKey)
	if err != nil || strings.TrimSpace(source) == "" {
		c.JSON(422, gin.H{"error": "original source is unavailable"})
		return
	}
	report, requirements, err := mergedReview(a.Result, state)
	if err == nil {
		err = report.Validate(source, requirements, state.Answers)
	}
	if err != nil {
		c.JSON(422, gin.H{"error": "review could not be safely assembled"})
		return
	}
	status := http.StatusOK
	if state.Pending != nil {
		status = http.StatusAccepted
	}
	c.JSON(status, gin.H{"session": state, "report": report})
}
