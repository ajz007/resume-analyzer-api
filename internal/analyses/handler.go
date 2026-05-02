package analyses

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/documents"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/shared/telemetry"
	"resume-backend/internal/usage"
)

// Handler wires HTTP handlers to the analyses service.
type Handler struct {
	Svc     *Service
	DocRepo documents.DocumentsRepo
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, docRepo documents.DocumentsRepo) *Handler {
	return &Handler{
		Svc:     svc,
		DocRepo: docRepo,
	}
}

// RegisterRoutes attaches analysis routes to the router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/documents/:id/analyze", h.startAnalysis)
	rg.GET("/analyses", h.listAnalyses)
	rg.GET("/analyses/:id", h.getAnalysis)
	rg.POST("/analyses/:id/shares", h.createShare)
	rg.GET("/shares/:token", h.getSharedAnalysis)
	rg.DELETE("/shares/:token", h.revokeShare)
}

type startAnalysisRequest struct {
	JobDescription string `json:"jobDescription"`
	PromptVersion  string `json:"promptVersion"`
	Mode           string `json:"mode"`
}

const defaultPollAfterMs = 2000

func (h *Handler) startAnalysis(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	ctx := withRequestID(c.Request.Context(), middleware.RequestIDFromContext(c))
	documentID := c.Param("id")
	c.Set("documentId", documentID)
	if documentID == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "document id is required", nil)
		return
	}

	var req startAnalysisRequest
	if err := decodeOptionalJSON(c.Request.Body, &req); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	req.PromptVersion = NormalizePromptVersion(req.PromptVersion)
	modeInput := strings.TrimSpace(req.Mode)
	if modeInput == "" {
		modeInput = string(ModeJobMatch)
	}
	mode, err := ParseMode(modeInput)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "mode is invalid", []map[string]string{
			{"field": "mode", "issue": "invalid"},
		})
		return
	}
	req.Mode = string(mode)
	if mode == ModeJobMatch {
		if len(strings.TrimSpace(req.JobDescription)) == 0 {
			respond.Error(c, http.StatusBadRequest, "validation_error", "jobDescription is required", []map[string]string{
				{"field": "jobDescription", "issue": "required"},
			})
			return
		}
		if utf8.RuneCountInString(req.JobDescription) < 300 {
			respond.Error(c, http.StatusBadRequest, "validation_error", "jobDescription too short", []map[string]string{
				{"field": "jobDescription", "issue": "min_length"},
			})
			return
		}
	}
	if utf8.RuneCountInString(req.JobDescription) > 50000 {
		respond.Error(c, http.StatusBadRequest, "validation_error", "jobDescription too long", []map[string]string{
			{"field": "jobDescription", "issue": "max_length"},
		})
		return
	}
	telemetry.Info("analysis.start", map[string]any{
		"request_id":  middleware.RequestIDFromContext(c),
		"user_id":     userID,
		"document_id": documentID,
		"mode":        mode,
	})

	doc, err := h.DocRepo.GetByID(c.Request.Context(), userID, documentID)
	if err != nil {
		switch {
		case errors.Is(err, documents.ErrNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "document not found", err)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to start analysis", err)
		}
		return
	}

	allowRetry := false
	if strings.EqualFold(c.Query("retry"), "true") {
		allowRetry = true
	}
	if strings.EqualFold(c.GetHeader("X-Retry-Analysis"), "true") {
		allowRetry = true
	}

	analysis, created, err := h.Svc.StartOrReuse(ctx, doc.ID, userID, req.JobDescription, req.PromptVersion, mode, allowRetry)
	if err != nil {
		switch {
		case errors.Is(err, ErrRetryRequired):
			respond.Error(c, http.StatusConflict, "retry_required", "analysis failed; set retry=true or X-Retry-Analysis: true to retry", nil)
		case errors.Is(err, ErrJobQueueNotConfigured):
			respond.Error(c, http.StatusInternalServerError, "internal_error", err.Error(), err)
		case errors.Is(err, usage.ErrLimitReached):
			respond.Error(c, http.StatusTooManyRequests, "limit_reached", "You've reached your analysis limit. Upgrade your plan to continue.", []map[string]string{
				{"field": "usage", "issue": "limit_reached"},
			})
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to start analysis", err)
		}
		return
	}
	c.Set("analysisId", analysis.ID)

	if !created && analysis.Status == StatusCompleted && analysis.Result != nil {
		respond.JSON(c, http.StatusOK, gin.H{
			"analysisId": analysis.ID,
			"status":     analysis.Status,
			"result":     analysis.Result,
		})
		return
	}

	respond.JSON(c, http.StatusAccepted, gin.H{
		"analysisId":  analysis.ID,
		"status":      analysis.Status,
		"pollAfterMs": defaultPollAfterMs,
	})
}

func (h *Handler) getAnalysis(c *gin.Context) {
	analysisID := c.Param("id")
	if analysisID == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "analysis id is required", nil)
		return
	}

	analysis, err := h.Svc.Get(c.Request.Context(), analysisID)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "analysis not found", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch analysis", nil)
		}
		return
	}
	if analysis.UserID != middleware.UserIDFromContext(c) {
		respond.Error(c, http.StatusNotFound, "not_found", "analysis not found", nil)
		return
	}
	c.Set("documentId", analysis.DocumentID)
	c.Set("analysisId", analysis.ID)

	resp := gin.H{
		"id":     analysis.ID,
		"status": analysis.Status,
		"mode":   analysis.Mode,
	}
	if analysis.StartedAt != nil {
		resp["startedAt"] = analysis.StartedAt
	}
	if analysis.CompletedAt != nil {
		resp["completedAt"] = analysis.CompletedAt
	}
	if analysis.Status == StatusFailed {
		resp["errorCode"] = analysis.ErrorCode
		resp["retryable"] = analysis.ErrorRetryable
		if analysis.ErrorMessage != nil {
			resp["errorMessage"] = *analysis.ErrorMessage
		} else {
			resp["errorMessage"] = ""
		}
	}
	if analysis.Status == StatusCompleted && analysis.Result != nil {
		resp["result"] = analysis.Result
	}
	if analysis.Status == StatusQueued || analysis.Status == StatusProcessing {
		resp["pollAfterMs"] = defaultPollAfterMs
	}

	respond.JSON(c, http.StatusOK, resp)
}

func (h *Handler) listAnalyses(c *gin.Context) {
	if isGuest, ok := c.Get("isGuest"); ok {
		if guest, ok2 := isGuest.(bool); ok2 && guest {
			respond.Error(c, http.StatusUnauthorized, "login_required", "Login required to view history", nil)
			return
		}
	}

	userID := middleware.UserIDFromContext(c)

	limit := 20
	offset := 0

	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}
	if limit < 0 {
		limit = 0
	}

	if v := c.Query("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			offset = parsed
		}
	}
	if offset < 0 {
		offset = 0
	}

	analyses, err := h.Svc.List(c.Request.Context(), userID, limit, offset)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to list analyses", nil)
		return
	}

	resp := make([]gin.H, 0, len(analyses))
	for _, a := range analyses {
		item := gin.H{
			"analysisId": a.ID,
			"documentId": a.DocumentID,
			"status":     a.Status,
			"mode":       a.Mode,
			"createdAt":  a.CreatedAt,
		}
		if a.StartedAt != nil {
			item["startedAt"] = a.StartedAt
		}
		if a.CompletedAt != nil {
			item["completedAt"] = a.CompletedAt
		}
		if a.Status == StatusCompleted && a.Result != nil {
			if finalScore, ok := extractFinalScore(a.Result, a.Mode); ok {
				item["finalScore"] = finalScore
			} else {
				item["finalScore"] = nil
			}
			if ms, ok := a.Result["matchScore"]; ok {
				item["matchScore"] = ms
			}
			if atsScore, ok := extractNestedFloat(a.Result, "ats", "score"); ok {
				item["atsScore"] = atsScore
			}
			if jobMatchScore, ok := extractNestedFloat(a.Result, "jobMatchScoring", "score"); ok {
				item["jobMatchScore"] = jobMatchScore
			} else if matchScore, ok := extractFloatAny(a.Result["matchScore"]); ok {
				item["jobMatchScore"] = matchScore
			}
			if aiScreeningScore, ok := extractNestedFloat(a.Result, "aiScreening", "score"); ok {
				item["aiScreeningScore"] = aiScreeningScore
			}
			item["primaryScoreLabel"] = primaryScoreLabel(a.Mode)
			if aiScreeningTier, ok := extractNestedString(a.Result, "aiScreening", "verdict", "tier"); ok {
				item["aiScreeningTier"] = aiScreeningTier
			}
			if summary, ok := a.Result["summary"]; ok {
				item["summary"] = summary
			}
		}
		resp = append(resp, item)
	}

	respond.JSON(c, http.StatusOK, resp)
}

func extractFinalScore(result map[string]any, mode AnalysisMode) (float64, bool) {
	if result == nil {
		return 0, false
	}
	if score, ok := extractFloatAny(result["finalScore"]); ok {
		return score, true
	}
	if mode == "" {
		mode = ModeJobMatch
	}
	if mode == ModeJobMatch {
		if score, ok := extractFloatAny(result["matchScore"]); ok {
			return score, true
		}
	}
	if atsRaw, ok := result["ats"]; ok {
		if ats, ok := atsRaw.(map[string]any); ok {
			if score, ok := extractFloatAny(ats["score"]); ok {
				return score, true
			}
		}
	}
	return 0, false
}

func extractNestedFloat(result map[string]any, path ...string) (float64, bool) {
	value, ok := extractNestedAny(result, path...)
	if !ok {
		return 0, false
	}
	return extractFloatAny(value)
}

func extractNestedString(result map[string]any, path ...string) (string, bool) {
	value, ok := extractNestedAny(result, path...)
	if !ok {
		return "", false
	}
	str, ok := value.(string)
	if !ok {
		return "", false
	}
	str = strings.TrimSpace(str)
	if str == "" {
		return "", false
	}
	return str, true
}

func extractNestedAny(result map[string]any, path ...string) (any, bool) {
	if result == nil || len(path) == 0 {
		return nil, false
	}
	var current any = result
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func primaryScoreLabel(mode AnalysisMode) string {
	switch mode {
	case ModeATS:
		return "ATS Readiness"
	default:
		return "Job Match"
	}
}

func extractFloatAny(value any) (float64, bool) {
	switch raw := value.(type) {
	case float64:
		return raw, true
	case float32:
		return float64(raw), true
	case int:
		return float64(raw), true
	case int64:
		return float64(raw), true
	case json.Number:
		parsed, err := raw.Float64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func decodeOptionalJSON(body io.ReadCloser, out any) error {
	if body == nil {
		return nil
	}
	var errInvalidJSON = errors.New("invalid json body")
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return errInvalidJSON
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errInvalidJSON
	}
	return nil
}

func (h *Handler) createShare(c *gin.Context) {
	analysisID := strings.TrimSpace(c.Param("id"))
	if analysisID == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "analysis id is required", nil)
		return
	}

	share, token, err := h.Svc.CreateShare(c.Request.Context(), analysisID, middleware.UserIDFromContext(c))
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "analysis not found", nil)
		case errors.Is(err, ErrForbidden):
			respond.Error(c, http.StatusForbidden, "forbidden", "not allowed", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to create share", err)
		}
		return
	}

	respond.JSON(c, http.StatusCreated, gin.H{
		"shareId":  share.ID,
		"token":    token,
		"shareUrl": buildShareURL(h.Svc.UIBaseURL, token),
	})
}

func (h *Handler) getSharedAnalysis(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		respond.Error(c, http.StatusNotFound, "not_found", "share not found", nil)
		return
	}

	analysis, err := h.Svc.GetSharedAnalysisByToken(c.Request.Context(), token)
	if err != nil {
		switch {
		case errors.Is(err, ErrShareNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "share not found", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch shared analysis", err)
		}
		return
	}

	resp := gin.H{
		"id":     analysis.ID,
		"mode":   analysis.Mode,
		"status": analysis.Status,
		"result": analysis.Result,
	}
	if analysis.StartedAt != nil {
		resp["startedAt"] = analysis.StartedAt
	}
	if analysis.CompletedAt != nil {
		resp["completedAt"] = analysis.CompletedAt
	}
	respond.JSON(c, http.StatusOK, resp)
}

func (h *Handler) revokeShare(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		respond.Error(c, http.StatusNotFound, "not_found", "share not found", nil)
		return
	}

	err := h.Svc.RevokeShareByToken(c.Request.Context(), token, middleware.UserIDFromContext(c))
	if err != nil {
		switch {
		case errors.Is(err, ErrShareNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "share not found", nil)
		case errors.Is(err, ErrForbidden):
			respond.Error(c, http.StatusForbidden, "forbidden", "not allowed", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to revoke share", err)
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func buildShareURL(baseURL, token string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return "/app/share/" + token
	}
	return baseURL + "/app/share/" + token
}
