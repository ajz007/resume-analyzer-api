package applies

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/generatedresumes"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/shared/storage/object"
)

// Handler wires HTTP handlers to the apply service.
type Handler struct {
	Svc           *Service
	GeneratedRepo generatedresumes.Repo
	Store         object.ObjectStore
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, generatedRepo generatedresumes.Repo, store object.ObjectStore) *Handler {
	return &Handler{
		Svc:           svc,
		GeneratedRepo: generatedRepo,
		Store:         store,
	}
}

// RegisterRoutes attaches apply routes to the router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/analyses/:id/apply", h.apply)
	rg.GET("/generated-resumes", h.list)
	rg.GET("/generated-resumes/:id", h.get)
	rg.GET("/generated-resumes/:id/download", h.download)
}

type applyRequest struct {
	TemplateID string `json:"templateId"`
}

func (h *Handler) apply(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	analysisID := c.Param("id")
	if analysisID == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "analysis id is required", nil)
		return
	}

	req := applyRequest{}
	if err := decodeOptionalJSON(c.Request.Body, &req); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}

	resume, err := h.Svc.Apply(c.Request.Context(), userID, analysisID, req.TemplateID)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			respond.Error(c, http.StatusBadRequest, "validation_error", "invalid input", nil)
		case errors.Is(err, ErrNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "analysis not found", nil)
		case errors.Is(err, ErrAnalysisNotComplete):
			respond.Error(c, http.StatusConflict, "analysis_pending", "analysis not complete", nil)
		case errors.Is(err, ErrMissingExtracted):
			respond.Error(c, http.StatusConflict, "document_not_ready", "document text not extracted", nil)
		case errors.Is(err, ErrInvalidLLMOutput):
			respond.Error(c, http.StatusBadGateway, "invalid_llm_output", "invalid model output", nil)
		case errors.Is(err, ErrInvalidResumeModel):
			respond.Error(c, http.StatusBadGateway, "invalid_resume_model", "invalid resume model", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to apply resume", nil)
		}
		return
	}

	respond.JSON(c, http.StatusCreated, toGeneratedResumeResponse(resume))
}

func (h *Handler) list(c *gin.Context) {
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
	if limit > 50 {
		limit = 50
	}

	if v := c.Query("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			offset = parsed
		}
	}
	if offset < 0 {
		offset = 0
	}

	resumes, err := h.GeneratedRepo.ListByUser(c.Request.Context(), userID, limit, offset)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to list generated resumes", nil)
		return
	}

	resp := make([]GeneratedResumeResponse, 0, len(resumes))
	for _, resume := range resumes {
		resp = append(resp, toGeneratedResumeResponse(resume))
	}

	respond.JSON(c, http.StatusOK, resp)
}

func (h *Handler) get(c *gin.Context) {
	if isGuest, ok := c.Get("isGuest"); ok {
		if guest, ok2 := isGuest.(bool); ok2 && guest {
			respond.Error(c, http.StatusUnauthorized, "login_required", "Login required to view history", nil)
			return
		}
	}

	userID := middleware.UserIDFromContext(c)
	resumeID := c.Param("id")
	if resumeID == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "generated resume id is required", nil)
		return
	}

	resume, err := h.GeneratedRepo.GetByID(c.Request.Context(), userID, resumeID)
	if err != nil {
		switch {
		case errors.Is(err, generatedresumes.ErrNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "generated resume not found", nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch generated resume", nil)
		}
		return
	}

	respond.JSON(c, http.StatusOK, toGeneratedResumeResponse(resume))
}

func (h *Handler) download(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	if userID == "" {
		respond.Error(c, http.StatusUnauthorized, "unauthorized", "Missing identity", nil)
		return
	}

	resumeID := c.Param("id")
	if resumeID == "" {
		respond.Error(c, http.StatusBadRequest, "validation_error", "generated resume id is required", nil)
		return
	}

	resume, err := h.GeneratedRepo.GetByID(c.Request.Context(), userID, resumeID)
	if err != nil {
		switch {
		case errors.Is(err, generatedresumes.ErrForbidden):
			respond.Error(c, http.StatusForbidden, "forbidden", "access denied", nil)
			return
		case errors.Is(err, generatedresumes.ErrNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "generated resume not found", nil)
		}
		if !errors.Is(err, generatedresumes.ErrNotFound) {
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch generated resume", nil)
		}
		return
	}

	reader, err := h.Store.Open(c.Request.Context(), resume.StorageKey)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to load generated resume", nil)
		return
	}
	defer reader.Close()

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	c.Header("Content-Disposition", "attachment; filename=\"generated_resume.docx\"")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, reader)
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
