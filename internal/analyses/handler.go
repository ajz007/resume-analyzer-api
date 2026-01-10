package analyses

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/documents"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/usage"
)

// Handler wires HTTP handlers to the analyses service.
type Handler struct {
	Svc     *Service
	DocRepo documents.DocumentsRepo
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, docRepo documents.DocumentsRepo) *Handler {
	return &Handler{Svc: svc, DocRepo: docRepo}
}

// RegisterRoutes attaches analysis routes to the router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/documents/:id/analyze", h.startAnalysis)
	rg.GET("/analyses", h.listAnalyses)
	rg.GET("/analyses/:id", h.getAnalysis)
}

type startAnalysisRequest struct {
	JobDescription string `json:"jobDescription"`
	PromptVersion  string `json:"promptVersion"`
}

func (h *Handler) startAnalysis(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	documentID := c.Param("id")
	if documentID == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "document id is required", nil)
		return
	}

	req := startAnalysisRequest{PromptVersion: "v1"}
	if err := decodeOptionalJSON(c.Request.Body, &req); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if utf8.RuneCountInString(req.JobDescription) > 50000 {
		respond.Error(c, http.StatusBadRequest, "validation_error", "jobDescription too long", []map[string]string{
			{"field": "jobDescription", "issue": "max_length"},
		})
		return
	}
	log.Printf("Starting analysis for user %s on document %s", userID, documentID)

	doc, err := h.DocRepo.GetByID(c.Request.Context(), userID, documentID)
	if err != nil {
		switch {
		case errors.Is(err, documents.ErrNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "document not found", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to start analysis", nil)
		}
		return
	}

	analysis, err := h.Svc.Create(c.Request.Context(), doc.ID, userID, req.JobDescription, req.PromptVersion)
	if err != nil {
		switch {
		case errors.Is(err, usage.ErrLimitReached):
			respond.Error(c, http.StatusTooManyRequests, "limit_reached", "You've reached your analysis limit. Upgrade your plan to continue.", []map[string]string{
				{"field": "usage", "issue": "limit_reached"},
			})
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to start analysis", nil)
		}
		return
	}

	respond.JSON(c, http.StatusAccepted, gin.H{
		"analysisId": analysis.ID,
		"status":     analysis.Status,
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

	resp := gin.H{
		"id":     analysis.ID,
		"status": analysis.Status,
	}
	if analysis.Status == StatusCompleted && analysis.Result != nil {
		resp["result"] = analysis.Result
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
			"createdAt":  a.CreatedAt,
		}
		if a.Status == StatusCompleted && a.Result != nil {
			if ms, ok := a.Result["matchScore"]; ok {
				item["matchScore"] = ms
			}
			if summary, ok := a.Result["summary"]; ok {
				item["summary"] = summary
			}
		}
		resp = append(resp, item)
	}

	respond.JSON(c, http.StatusOK, resp)
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
