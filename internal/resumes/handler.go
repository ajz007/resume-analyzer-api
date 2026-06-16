package resumes

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	modelv1 "resume-backend/resume/modelv1"
)

type Handler struct {
	Svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/resumes", h.create)
	rg.GET("/resumes", h.list)
	rg.GET("/resumes/:resumeId", h.get)
	rg.PUT("/resumes/:resumeId", h.update)
	rg.GET("/resumes/:resumeId/versions", h.listVersions)
	rg.GET("/resumes/:resumeId/versions/:versionId", h.getVersion)
}

type saveResumeRequest struct {
	Title         string              `json:"title"`
	Resume        modelv1.ResumeModel `json:"resume"`
	ChangeSummary map[string]any      `json:"changeSummary"`
}

type resumeResponse struct {
	ID                string                      `json:"resumeId"`
	Title             string                      `json:"title"`
	CurrentVersionID  string                      `json:"currentVersionId"`
	Resume            modelv1.ResumeModel         `json:"resume"`
	ReadinessWarnings []modelv1.ValidationWarning `json:"readinessWarnings,omitempty"`
	CreatedAt         time.Time                   `json:"createdAt"`
	UpdatedAt         time.Time                   `json:"updatedAt"`
}

type resumeListItemResponse struct {
	ID               string    `json:"resumeId"`
	Title            string    `json:"title"`
	Status           string    `json:"status"`
	CurrentVersionID string    `json:"currentVersionId"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type versionResponse struct {
	ID              string              `json:"id"`
	ResumeID        string              `json:"resumeId"`
	VersionNumber   int                 `json:"versionNumber"`
	SourceType      string              `json:"sourceType"`
	Resume          modelv1.ResumeModel `json:"resume"`
	ChangeSummary   map[string]any      `json:"changeSummary,omitempty"`
	ParentVersionID *string             `json:"parentVersionId,omitempty"`
	CreatedAt       time.Time           `json:"createdAt"`
}

func (h *Handler) create(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	var req saveResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid request body", nil)
		return
	}

	result, err := h.Svc.Create(c.Request.Context(), ownerID, req.Title, req.Resume)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}
	respond.JSON(c, http.StatusCreated, toResumeResponse(result))
}

func (h *Handler) update(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	resumeID := strings.TrimSpace(c.Param("resumeId"))
	var req saveResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid request body", nil)
		return
	}

	result, err := h.Svc.Update(c.Request.Context(), ownerID, resumeID, req.Title, req.Resume, req.ChangeSummary)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}
	respond.JSON(c, http.StatusOK, toResumeResponse(result))
}

func (h *Handler) get(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	resume, err := h.Svc.Get(c.Request.Context(), ownerID, strings.TrimSpace(c.Param("resumeId")))
	if err != nil {
		h.respondServiceError(c, err)
		return
	}
	respond.JSON(c, http.StatusOK, toResumeResponse(SaveResult{
		Resume:            resume,
		ReadinessWarnings: modelv1.ValidateReadiness(resume.CurrentResume),
	}))
}

func (h *Handler) list(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	resumes, err := h.Svc.List(c.Request.Context(), ownerID, parseLimit(c, 20, 50), parseOffset(c))
	if err != nil {
		h.respondServiceError(c, err)
		return
	}

	resp := make([]resumeListItemResponse, 0, len(resumes))
	for _, resume := range resumes {
		resp = append(resp, toResumeListItemResponse(resume))
	}
	respond.JSON(c, http.StatusOK, resp)
}

func (h *Handler) listVersions(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	versions, err := h.Svc.ListVersions(c.Request.Context(), ownerID, strings.TrimSpace(c.Param("resumeId")))
	if err != nil {
		h.respondServiceError(c, err)
		return
	}

	resp := make([]versionResponse, 0, len(versions))
	for _, version := range versions {
		resp = append(resp, toVersionResponse(version))
	}
	respond.JSON(c, http.StatusOK, resp)
}

func (h *Handler) getVersion(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	version, err := h.Svc.GetVersion(
		c.Request.Context(),
		ownerID,
		strings.TrimSpace(c.Param("resumeId")),
		strings.TrimSpace(c.Param("versionId")),
	)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}
	respond.JSON(c, http.StatusOK, toVersionResponse(version))
}

func (h *Handler) respondServiceError(c *gin.Context, err error) {
	var validationErr ValidationError
	switch {
	case errors.As(err, &validationErr):
		respond.Error(c, http.StatusBadRequest, "validation_error", "resume validation failed", validationErr.Errors)
	case errors.Is(err, ErrInvalidInput):
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid resume request", nil)
	case errors.Is(err, ErrNotFound):
		respond.Error(c, http.StatusNotFound, "not_found", "resume not found", nil)
	case errors.Is(err, ErrForbidden):
		respond.Error(c, http.StatusForbidden, "forbidden", "not allowed", nil)
	default:
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to process resume", err)
	}
}

func authenticatedOwnerID(c *gin.Context) (string, bool) {
	if isGuest, ok := c.Get("isGuest"); ok {
		if guest, ok2 := isGuest.(bool); ok2 && guest {
			respond.Error(c, http.StatusUnauthorized, "login_required", "Login required to manage resumes", nil)
			return "", false
		}
	}
	ownerID := strings.TrimSpace(middleware.UserIDFromContext(c))
	if ownerID == "" {
		respond.Error(c, http.StatusUnauthorized, "unauthorized", "missing identity", nil)
		return "", false
	}
	return ownerID, true
}

func parseLimit(c *gin.Context, defaultValue, maxValue int) int {
	limit := defaultValue
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit < 0 {
		limit = 0
	}
	if limit > maxValue {
		limit = maxValue
	}
	return limit
}

func parseOffset(c *gin.Context) int {
	offset := 0
	if raw := c.Query("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			offset = parsed
		}
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

func toResumeResponse(result SaveResult) resumeResponse {
	return resumeResponse{
		ID:                result.Resume.ID,
		Title:             result.Resume.Title,
		CurrentVersionID:  result.Resume.CurrentVersionID,
		Resume:            result.Resume.CurrentResume,
		ReadinessWarnings: result.ReadinessWarnings,
		CreatedAt:         result.Resume.CreatedAt,
		UpdatedAt:         result.Resume.UpdatedAt,
	}
}

func toResumeListItemResponse(resume Resume) resumeListItemResponse {
	return resumeListItemResponse{
		ID:               resume.ID,
		Title:            resume.Title,
		Status:           resume.Status,
		CurrentVersionID: resume.CurrentVersionID,
		CreatedAt:        resume.CreatedAt,
		UpdatedAt:        resume.UpdatedAt,
	}
}

func toVersionResponse(version ResumeVersion) versionResponse {
	return versionResponse{
		ID:              version.ID,
		ResumeID:        version.ResumeID,
		VersionNumber:   version.VersionNumber,
		SourceType:      version.SourceType,
		Resume:          version.Resume,
		ChangeSummary:   version.ChangeSummary,
		ParentVersionID: version.ParentVersionID,
		CreatedAt:       version.CreatedAt,
	}
}
