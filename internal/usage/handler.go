package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"resume-backend/internal/documents"
	"resume-backend/internal/generatedresumes"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/shared/storage/object"
	resumeservice "resume-backend/resume/service"
)

// Handler exposes usage endpoints.
type Handler struct {
	Svc          *Service
	AnalysisRepo AnalysisRepo
	DocRepo      documents.DocumentsRepo
	Store        object.ObjectStore
	Generated    *generatedresumes.Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, analysisRepo AnalysisRepo, docRepo documents.DocumentsRepo, store object.ObjectStore, generatedResumeSvc *generatedresumes.Service) *Handler {
	return &Handler{
		Svc:          svc,
		AnalysisRepo: analysisRepo,
		DocRepo:      docRepo,
		Store:        store,
		Generated:    generatedResumeSvc,
	}
}

// RegisterRoutes attaches usage routes to the router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/usage", h.getUsage)
	rg.POST("/analyses/:id/apply/plan", h.applyPlan)
	rg.POST("/apply-runs/:id/execute", h.executeApply)
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

type applyExecuteRequest struct {
	Header applyHeaderInput `json:"header"`
}

type applyHeaderInput struct {
	Name     string   `json:"name"`
	Title    string   `json:"title"`
	Email    string   `json:"email"`
	Phone    string   `json:"phone"`
	Location string   `json:"location"`
	Links    []string `json:"links"`
}

const analysisStatusCompleted = "completed"

func (h *Handler) applyPlan(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	analysisID := c.Param("id")
	if analysisID == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "analysis id is required", nil)
		return
	}

	analysis, err := h.AnalysisRepo.GetByID(c.Request.Context(), analysisID)
	if err != nil {
		switch {
		case errors.Is(err, ErrAnalysisNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "analysis not found", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch analysis", nil)
		}
		return
	}
	if analysis.UserID != userID {
		respond.Error(c, http.StatusNotFound, "not_found", "analysis not found", nil)
		return
	}
	if analysis.Status != analysisStatusCompleted || analysis.Result == nil {
		respond.Error(c, http.StatusConflict, "analysis_pending", "analysis not complete", nil)
		return
	}

	result, err := decodeAnalysisResult(analysis.Result)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "invalid_analysis", "analysis result is not compatible", nil)
		return
	}

	plan := h.Svc.BuildApplyPlan(result)

	run := ApplyRun{
		ID:                   uuid.NewString(),
		UserID:               userID,
		AnalysisID:           analysis.ID,
		Status:               ApplyRunStatusPlanned,
		AutoFixesCount:       len(plan.AutoFixes),
		SafeRewritesCount:    len(plan.SafeRewrites),
		BlockedRewritesCount: len(plan.BlockedRewrites),
		NeedsInputCount:      len(plan.NeedsInput),
		CreatedAt:            time.Now().UTC(),
	}

	if err := h.Svc.CreateApplyRun(c.Request.Context(), run); err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to create apply run", nil)
		return
	}

	respond.JSON(c, http.StatusOK, gin.H{
		"applyRunId": run.ID,
		"plan":       plan,
	})
}

func (h *Handler) executeApply(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	applyRunID := c.Param("id")
	if applyRunID == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "apply run id is required", nil)
		return
	}

	var req applyExecuteRequest
	if err := decodeOptionalJSON(c.Request.Body, &req); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid json body", nil)
		return
	}

	run, err := h.Svc.GetApplyRun(c.Request.Context(), userID, applyRunID)
	if err != nil {
		switch {
		case errors.Is(err, ErrApplyRunNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "apply run not found", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch apply run", nil)
		}
		return
	}

	analysis, err := h.AnalysisRepo.GetByID(c.Request.Context(), run.AnalysisID)
	if err != nil {
		switch {
		case errors.Is(err, ErrAnalysisNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "analysis not found", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch analysis", nil)
		}
		return
	}
	if analysis.UserID != userID {
		respond.Error(c, http.StatusNotFound, "not_found", "analysis not found", nil)
		return
	}
	if analysis.Result == nil {
		respond.Error(c, http.StatusConflict, "analysis_pending", "analysis not complete", nil)
		return
	}

	result, err := decodeAnalysisResult(analysis.Result)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "invalid_analysis", "analysis result is not compatible", nil)
		return
	}

	doc, err := h.DocRepo.GetByID(c.Request.Context(), userID, analysis.DocumentID)
	if err != nil {
		switch {
		case errors.Is(err, documents.ErrNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "document not found", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to load document", nil)
		}
		return
	}

	reader, err := h.Store.Open(c.Request.Context(), doc.StorageKey)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to open document", nil)
		return
	}
	defer reader.Close()

	raw, err := io.ReadAll(reader)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to read document", nil)
		return
	}

	execResult, err := resumeservice.ExecuteApply(c.Request.Context(), string(raw), result, resumeservice.ApplyHeaderInputs{
		Name:     req.Header.Name,
		Title:    req.Header.Title,
		Email:    req.Header.Email,
		Phone:    req.Header.Phone,
		Location: req.Header.Location,
		Links:    req.Header.Links,
	})
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to execute apply flow", nil)
		return
	}

	fileName := "resume_applied.docx"
	storageKey, size, mimeType, err := h.Store.Save(c.Request.Context(), userID, fileName, bytes.NewReader(execResult.DocxBytes))
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to store document", nil)
		return
	}

	version := DocumentVersion{
		ID:         uuid.NewString(),
		DocumentID: doc.ID,
		UserID:     userID,
		ApplyRunID: run.ID,
		FileName:   fileName,
		MimeType:   mimeType,
		SizeBytes:  size,
		StorageKey: storageKey,
		CreatedAt:  time.Now().UTC(),
	}
	if err := h.Svc.CreateDocumentVersion(c.Request.Context(), version); err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to persist document version", nil)
		return
	}

	update := ApplyRunUpdate{
		ID:                    run.ID,
		UserID:                userID,
		Status:                execResult.Status,
		AutoFixesCount:        len(execResult.Plan.AutoFixes),
		SafeRewritesCount:     execResult.SafeRewritesApplied,
		BlockedRewritesCount:  len(execResult.Plan.BlockedRewrites),
		NeedsInputCount:       len(execResult.Plan.NeedsInput),
		PlaceholdersRemaining: execResult.PlaceholdersRemaining,
		DocumentVersionID:     version.ID,
	}
	if err := h.Svc.UpdateApplyRun(c.Request.Context(), update); err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to update apply run", nil)
		return
	}

	respond.JSON(c, http.StatusOK, gin.H{
		"applyRunId":            run.ID,
		"documentVersionId":     version.ID,
		"status":                execResult.Status,
		"placeholdersRemaining": execResult.PlaceholdersRemaining,
		"autoFixesApplied":      execResult.AutoFixesApplied,
		"safeRewritesApplied":   execResult.SafeRewritesApplied,
	})
}

func decodeAnalysisResult(result map[string]any) (resumeservice.AnalysisResultV2_3, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return resumeservice.AnalysisResultV2_3{}, err
	}
	var decoded resumeservice.AnalysisResultV2_3
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return resumeservice.AnalysisResultV2_3{}, err
	}
	return decoded, nil
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
