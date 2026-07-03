package resumes

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/shared/telemetry"
	modelv1 "resume-backend/resume/modelv1"
)

type Handler struct {
	Svc *Service
}

const maxGenerateResumeRequestBytes int64 = 65536

func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/resumes", h.create)
	rg.POST("/resumes/generate", h.generate)
	rg.GET("/resume-generations/:generationId", h.getGeneration)
	rg.GET("/resumes", h.list)
	rg.GET("/resumes/:resumeId", h.get)
	rg.PUT("/resumes/:resumeId", h.update)
	rg.POST("/resumes/:resumeId/tailor", h.tailor)
	rg.GET("/resumes/:resumeId/versions", h.listVersions)
	rg.GET("/resumes/:resumeId/versions/:versionId", h.getVersion)
	rg.POST("/resumes/:resumeId/export/docx", h.exportDOCX)
}

type saveResumeRequest struct {
	Title         string              `json:"title"`
	Resume        modelv1.ResumeModel `json:"resume"`
	ChangeSummary map[string]any      `json:"changeSummary"`
}

type generateResumeRequest struct {
	Title                  string `json:"title"`
	TargetRole             string `json:"targetRole"`
	Seniority              string `json:"seniority"`
	GenerationMode         string `json:"generationMode"`
	JobDescription         string `json:"jobDescription"`
	ExperienceText         string `json:"experienceText"`
	SkillsText             string `json:"skillsText"`
	EducationText          string `json:"educationText"`
	AdditionalInstructions string `json:"additionalInstructions"`
}

type tailorResumeRequest struct {
	JobDescription         string `json:"jobDescription"`
	TargetRole             string `json:"targetRole"`
	AdditionalInstructions string `json:"additionalInstructions"`
}

type generateResumeResponse struct {
	ID                string                      `json:"id"`
	Title             string                      `json:"title"`
	Status            string                      `json:"status"`
	DraftType         string                      `json:"draftType,omitempty"`
	GenerationMode    string                      `json:"generationMode,omitempty"`
	SampleTemplate    bool                        `json:"sampleTemplate,omitempty"`
	CurrentVersionID  string                      `json:"currentVersionId"`
	Resume            modelv1.ResumeModel         `json:"resume"`
	RequiresUserInput []modelv1.RequiresUserInput `json:"requiresUserInput"`
	Assumptions       []modelv1.Assumption        `json:"assumptions"`
	Warnings          []modelv1.ResponseWarning   `json:"warnings"`
	ReadinessWarnings []modelv1.ValidationWarning `json:"readinessWarnings"`
	CreatedAt         time.Time                   `json:"createdAt"`
	UpdatedAt         time.Time                   `json:"updatedAt"`
}

type tailorResumeResponse struct {
	SourceResumeID      string                        `json:"sourceResumeId"`
	SourceVersionID     string                        `json:"sourceVersionId"`
	ID                  string                        `json:"id"`
	Title               string                        `json:"title"`
	Status              string                        `json:"status"`
	CurrentVersionID    string                        `json:"currentVersionId"`
	Resume              modelv1.ResumeModel           `json:"resume"`
	Changes             []modelv1.TailoringChange     `json:"changes"`
	MissingRequirements []modelv1.MissingRequirement  `json:"missingRequirements"`
	Suggestions         []modelv1.TailoringSuggestion `json:"suggestions"`
	Warnings            []modelv1.ResponseWarning     `json:"warnings"`
	ReadinessWarnings   []modelv1.ValidationWarning   `json:"readinessWarnings"`
	CreatedAt           time.Time                     `json:"createdAt"`
	UpdatedAt           time.Time                     `json:"updatedAt"`
}

type resumeResponse struct {
	ID                string                      `json:"id"`
	Title             string                      `json:"title"`
	Status            string                      `json:"status"`
	DraftType         string                      `json:"draftType,omitempty"`
	GenerationMode    string                      `json:"generationMode,omitempty"`
	SampleTemplate    bool                        `json:"sampleTemplate,omitempty"`
	CurrentVersionID  string                      `json:"currentVersionId"`
	Resume            modelv1.ResumeModel         `json:"resume"`
	ReadinessWarnings []modelv1.ValidationWarning `json:"readinessWarnings"`
	CreatedAt         time.Time                   `json:"createdAt"`
	UpdatedAt         time.Time                   `json:"updatedAt"`
}

type resumeListItemResponse struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	OriginType     string    `json:"originType"`
	DraftType      string    `json:"draftType,omitempty"`
	GenerationMode string    `json:"generationMode,omitempty"`
	SampleTemplate bool      `json:"sampleTemplate,omitempty"`
	SourceResumeID string    `json:"sourceResumeId,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type versionResponse struct {
	ID              string              `json:"id"`
	ResumeID        string              `json:"resumeId"`
	VersionNumber   int                 `json:"versionNumber"`
	SourceType      string              `json:"sourceType"`
	SourceVersionID string              `json:"sourceVersionId,omitempty"`
	Resume          modelv1.ResumeModel `json:"resume"`
	ChangeSummary   map[string]any      `json:"changeSummary,omitempty"`
	ParentVersionID *string             `json:"parentVersionId,omitempty"`
	CreatedAt       time.Time           `json:"createdAt"`
}

type generationAcceptedResponse struct {
	GenerationID string `json:"generationId"`
	Status       string `json:"status"`
}

func (h *Handler) create(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	var req saveResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid request body", bindJSONErrorDetails(err))
		return
	}
	if errs := validateSaveResumeRequest(req); len(errs) > 0 {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid resume request", errs)
		return
	}

	result, err := h.Svc.Create(c.Request.Context(), ownerID, req.Title, req.Resume)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}
	respond.JSON(c, http.StatusCreated, toResumeResponse(result))
}

func (h *Handler) generate(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	startedAt := time.Now()
	requestID := middleware.RequestIDFromContext(c)
	var req generateResumeRequest
	var responseCode string
	defer func() {
		telemetry.Info("resume.generate.request.end", map[string]any{
			"request_id":             requestID,
			"user_id":                ownerID,
			"method":                 c.Request.Method,
			"path":                   c.Request.URL.Path,
			"generation_mode":        strings.TrimSpace(req.GenerationMode),
			"job_description_length": utf8.RuneCountInString(strings.TrimSpace(req.JobDescription)),
			"experience_text_length": utf8.RuneCountInString(strings.TrimSpace(req.ExperienceText)),
			"status":                 c.Writer.Status(),
			"error_code":             responseCode,
			"duration_ms":            durationMilliseconds(time.Since(startedAt)),
		})
	}()

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxGenerateResumeRequestBytes)
	if err := c.ShouldBindJSON(&req); err != nil {
		responseCode = "validation_error"
		respond.Error(c, http.StatusBadRequest, responseCode, "invalid request body", bindJSONErrorDetails(err))
		return
	}
	telemetry.Info("resume.generate.request.start", map[string]any{
		"request_id":             requestID,
		"user_id":                ownerID,
		"method":                 c.Request.Method,
		"path":                   c.Request.URL.Path,
		"generation_mode":        strings.TrimSpace(req.GenerationMode),
		"job_description_length": utf8.RuneCountInString(strings.TrimSpace(req.JobDescription)),
		"experience_text_length": utf8.RuneCountInString(strings.TrimSpace(req.ExperienceText)),
	})
	if errs := validateGenerateResumeRequest(req); len(errs) > 0 {
		responseCode = "validation_error"
		respond.Error(c, http.StatusBadRequest, responseCode, "invalid resume request", errs)
		return
	}

	job, err := h.Svc.EnqueueGeneration(withRequestLogContext(c.Request.Context(), requestID, ownerID), ownerID, GenerateRequest{
		Title:                  req.Title,
		TargetRole:             req.TargetRole,
		Seniority:              req.Seniority,
		GenerationMode:         req.GenerationMode,
		JobDescription:         req.JobDescription,
		ExperienceText:         req.ExperienceText,
		SkillsText:             req.SkillsText,
		EducationText:          req.EducationText,
		AdditionalInstructions: req.AdditionalInstructions,
	})
	if err != nil {
		_, responseCode, _, _ = classifyServiceError(err)
		h.respondServiceError(c, err)
		return
	}
	respond.JSON(c, http.StatusAccepted, generationAcceptedResponse{
		GenerationID: job.ID,
		Status:       job.Status,
	})
}

func (h *Handler) update(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	resumeID := strings.TrimSpace(c.Param("resumeId"))
	var req saveResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid request body", bindJSONErrorDetails(err))
		return
	}
	if errs := validateSaveResumeRequest(req); len(errs) > 0 {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid resume request", errs)
		return
	}

	result, err := h.Svc.Update(c.Request.Context(), ownerID, resumeID, req.Title, req.Resume, req.ChangeSummary)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}
	respond.JSON(c, http.StatusOK, toResumeResponse(result))
}

func (h *Handler) tailor(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	resumeID := strings.TrimSpace(c.Param("resumeId"))
	var req tailorResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "invalid request body", nil)
		return
	}

	result, err := h.Svc.Tailor(c.Request.Context(), ownerID, resumeID, TailorRequest{
		JobDescription:         req.JobDescription,
		TargetRole:             req.TargetRole,
		AdditionalInstructions: req.AdditionalInstructions,
	})
	if err != nil {
		h.respondServiceError(c, err)
		return
	}
	respond.JSON(c, http.StatusCreated, toTailorResumeResponse(result))
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

func (h *Handler) getGeneration(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	job, err := h.Svc.GetGenerationJob(c.Request.Context(), ownerID, strings.TrimSpace(c.Param("generationId")))
	if err != nil {
		h.respondServiceError(c, err)
		return
	}

	response := gin.H{
		"generationId": job.ID,
		"status":       job.Status,
		"createdAt":    job.CreatedAt,
		"startedAt":    job.StartedAt,
		"completedAt":  job.CompletedAt,
	}

	switch job.Status {
	case GenerationJobStatusCompleted:
		if job.ResumeID == nil {
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to load generated resume", nil)
			return
		}
		resume, err := h.Svc.Get(c.Request.Context(), ownerID, *job.ResumeID)
		if err != nil {
			h.respondServiceError(c, err)
			return
		}
		result := SaveResult{
			Resume:            resume,
			ReadinessWarnings: modelv1.ValidateReadiness(resume.CurrentResume),
		}
		response["resumeId"] = resume.ID
		response["currentVersionId"] = resume.CurrentVersionID
		response["resume"] = resume.CurrentResume
		response["readinessWarnings"] = toResumeResponse(result).ReadinessWarnings
		if job.Result != nil {
			response["requiresUserInput"] = emptyRequiresUserInput(job.Result.RequiresUserInput)
			response["assumptions"] = emptyAssumptions(job.Result.Assumptions)
			response["warnings"] = emptyResponseWarnings(job.Result.Warnings)
			response["generationMode"] = job.Result.GenerationMode
			response["fallbackUsed"] = job.Result.FallbackUsed
			response["fallbackReason"] = job.Result.FallbackReason
			response["draftType"] = job.Result.DraftType
		} else {
			response["requiresUserInput"] = []modelv1.RequiresUserInput{}
			response["assumptions"] = []modelv1.Assumption{}
			response["warnings"] = []modelv1.ResponseWarning{}
			response["generationMode"] = ""
			response["fallbackUsed"] = false
			response["fallbackReason"] = ""
			response["draftType"] = ""
		}
	case GenerationJobStatusFailed:
		response["errorMessage"] = generationFailedMessage
	}

	respond.JSON(c, http.StatusOK, response)
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

func (h *Handler) exportDOCX(c *gin.Context) {
	ownerID, ok := authenticatedOwnerID(c)
	if !ok {
		return
	}

	result, err := h.Svc.ExportDOCX(c.Request.Context(), ownerID, strings.TrimSpace(c.Param("resumeId")))
	if err != nil {
		h.respondServiceError(c, err)
		return
	}

	c.Header("Content-Disposition", `attachment; filename="`+result.FileName+`"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", result.DocxBytes)
}

func (h *Handler) respondServiceError(c *gin.Context, err error) {
	status, code, message, details := classifyServiceError(err)
	respond.Error(c, status, code, message, details)
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

func toGenerateResumeResponse(result GenerateResult) generateResumeResponse {
	if result.RequiresUserInput == nil {
		result.RequiresUserInput = []modelv1.RequiresUserInput{}
	}
	if result.Assumptions == nil {
		result.Assumptions = []modelv1.Assumption{}
	}
	if result.Warnings == nil {
		result.Warnings = []modelv1.ResponseWarning{}
	}
	if result.ReadinessWarnings == nil {
		result.ReadinessWarnings = []modelv1.ValidationWarning{}
	}
	return generateResumeResponse{
		ID:                result.SavedResume.ID,
		Title:             result.SavedResume.Title,
		Status:            result.SavedResume.Status,
		DraftType:         resumeDraftType(result.SavedResume),
		GenerationMode:    resumeGenerationMode(result.SavedResume),
		SampleTemplate:    resumeSampleTemplate(result.SavedResume),
		CurrentVersionID:  result.SavedResume.CurrentVersionID,
		Resume:            result.SavedResume.CurrentResume,
		RequiresUserInput: result.RequiresUserInput,
		Assumptions:       result.Assumptions,
		Warnings:          result.Warnings,
		ReadinessWarnings: result.ReadinessWarnings,
		CreatedAt:         result.SavedResume.CreatedAt,
		UpdatedAt:         result.SavedResume.UpdatedAt,
	}
}

func toTailorResumeResponse(result TailorResult) tailorResumeResponse {
	if result.Changes == nil {
		result.Changes = []modelv1.TailoringChange{}
	}
	if result.MissingRequirements == nil {
		result.MissingRequirements = []modelv1.MissingRequirement{}
	}
	if result.Suggestions == nil {
		result.Suggestions = []modelv1.TailoringSuggestion{}
	}
	if result.Warnings == nil {
		result.Warnings = []modelv1.ResponseWarning{}
	}
	if result.ReadinessWarnings == nil {
		result.ReadinessWarnings = []modelv1.ValidationWarning{}
	}
	return tailorResumeResponse{
		SourceResumeID:      result.SourceResumeID,
		SourceVersionID:     result.SourceVersionID,
		ID:                  result.Resume.ID,
		Title:               result.Resume.Title,
		Status:              result.Resume.Status,
		CurrentVersionID:    result.Resume.CurrentVersionID,
		Resume:              result.Resume.CurrentResume,
		Changes:             result.Changes,
		MissingRequirements: result.MissingRequirements,
		Suggestions:         result.Suggestions,
		Warnings:            result.Warnings,
		ReadinessWarnings:   result.ReadinessWarnings,
		CreatedAt:           result.Resume.CreatedAt,
		UpdatedAt:           result.Resume.UpdatedAt,
	}
}

func toResumeResponse(result SaveResult) resumeResponse {
	if result.ReadinessWarnings == nil {
		result.ReadinessWarnings = []modelv1.ValidationWarning{}
	}
	return resumeResponse{
		ID:                result.Resume.ID,
		Title:             result.Resume.Title,
		Status:            result.Resume.Status,
		DraftType:         resumeDraftType(result.Resume),
		GenerationMode:    resumeGenerationMode(result.Resume),
		SampleTemplate:    resumeSampleTemplate(result.Resume),
		CurrentVersionID:  result.Resume.CurrentVersionID,
		Resume:            result.Resume.CurrentResume,
		ReadinessWarnings: result.ReadinessWarnings,
		CreatedAt:         result.Resume.CreatedAt,
		UpdatedAt:         result.Resume.UpdatedAt,
	}
}

func toResumeListItemResponse(resume Resume) resumeListItemResponse {
	return resumeListItemResponse{
		ID:             resume.ID,
		Title:          resume.Title,
		Status:         resume.Status,
		OriginType:     resume.OriginType,
		DraftType:      resumeDraftType(resume),
		GenerationMode: resumeGenerationMode(resume),
		SampleTemplate: resumeSampleTemplate(resume),
		SourceResumeID: resume.SourceResumeID,
		CreatedAt:      resume.CreatedAt,
		UpdatedAt:      resume.UpdatedAt,
	}
}

func resumeDraftType(resume Resume) string {
	if value, ok := stringSummaryValue(resume.CurrentChangeSummary, "draftType"); ok {
		return value
	}
	return ""
}

func resumeGenerationMode(resume Resume) string {
	if value, ok := stringSummaryValue(resume.CurrentChangeSummary, "generationMode"); ok {
		return value
	}
	return ""
}

func resumeSampleTemplate(resume Resume) bool {
	if value, ok := boolSummaryValue(resume.CurrentChangeSummary, "sampleTemplate"); ok {
		return value
	}
	return false
}

func stringSummaryValue(summary map[string]any, key string) (string, bool) {
	if summary == nil {
		return "", false
	}
	raw, ok := summary[key]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func boolSummaryValue(summary map[string]any, key string) (bool, bool) {
	if summary == nil {
		return false, false
	}
	raw, ok := summary[key]
	if !ok {
		return false, false
	}
	value, ok := raw.(bool)
	return value, ok
}

func toVersionResponse(version ResumeVersion) versionResponse {
	return versionResponse{
		ID:              version.ID,
		ResumeID:        version.ResumeID,
		VersionNumber:   version.VersionNumber,
		SourceType:      version.SourceType,
		SourceVersionID: version.SourceVersionID,
		Resume:          version.Resume,
		ChangeSummary:   version.ChangeSummary,
		ParentVersionID: version.ParentVersionID,
		CreatedAt:       version.CreatedAt,
	}
}

func emptyRequiresUserInput(items []modelv1.RequiresUserInput) []modelv1.RequiresUserInput {
	if items == nil {
		return []modelv1.RequiresUserInput{}
	}
	return items
}

func emptyAssumptions(items []modelv1.Assumption) []modelv1.Assumption {
	if items == nil {
		return []modelv1.Assumption{}
	}
	return items
}

func emptyResponseWarnings(items []modelv1.ResponseWarning) []modelv1.ResponseWarning {
	if items == nil {
		return []modelv1.ResponseWarning{}
	}
	return items
}

func validateSaveResumeRequest(req saveResumeRequest) []modelv1.ValidationError {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return []modelv1.ValidationError{{
			Field:   "title",
			Message: "title is required",
		}}
	}
	return nil
}

func validateGenerateResumeRequest(req generateResumeRequest) []modelv1.ValidationError {
	normalized := normalizeGenerateRequest(GenerateRequest{
		Title:                  req.Title,
		TargetRole:             req.TargetRole,
		Seniority:              req.Seniority,
		GenerationMode:         req.GenerationMode,
		JobDescription:         req.JobDescription,
		ExperienceText:         req.ExperienceText,
		SkillsText:             req.SkillsText,
		EducationText:          req.EducationText,
		AdditionalInstructions: req.AdditionalInstructions,
	})

	var errs []modelv1.ValidationError
	if normalized.Title == "" {
		errs = append(errs, modelv1.ValidationError{Field: "title", Message: "title is required"})
	}
	if utf8.RuneCountInString(normalized.Title) > maxTitleLength {
		errs = append(errs, modelv1.ValidationError{Field: "title", Message: "title must be at most 160 characters"})
	}
	if !validGenerationMode(normalized.GenerationMode) {
		errs = append(errs, modelv1.ValidationError{Field: "generationMode", Message: "generationMode is invalid"})
	}
	if normalized.GenerationMode == GenerationModeSampleFromJobDescription && normalized.JobDescription == "" {
		errs = append(errs, modelv1.ValidationError{Field: "jobDescription", Message: "jobDescription is required for sample_from_job_description mode"})
	}
	if normalized.GenerationMode == GenerationModeFromExperience &&
		normalized.ExperienceText == "" &&
		normalized.SkillsText == "" &&
		normalized.EducationText == "" {
		errs = append(errs, modelv1.ValidationError{Field: "experienceText", Message: "at least one experience field is required for from_experience mode"})
	}
	if utf8.RuneCountInString(normalized.JobDescription) > maxJobDescriptionLength {
		errs = append(errs, modelv1.ValidationError{Field: "jobDescription", Message: "jobDescription must be at most 30000 characters"})
	}
	if utf8.RuneCountInString(normalized.ExperienceText) > maxGenerationTextLength {
		errs = append(errs, modelv1.ValidationError{Field: "experienceText", Message: "experienceText must be at most 20000 characters"})
	}
	if utf8.RuneCountInString(normalized.SkillsText) > maxGenerationTextLength {
		errs = append(errs, modelv1.ValidationError{Field: "skillsText", Message: "skillsText must be at most 20000 characters"})
	}
	if utf8.RuneCountInString(normalized.EducationText) > maxGenerationTextLength {
		errs = append(errs, modelv1.ValidationError{Field: "educationText", Message: "educationText must be at most 20000 characters"})
	}
	if utf8.RuneCountInString(normalized.AdditionalInstructions) > maxAdditionalInstructionsLength {
		errs = append(errs, modelv1.ValidationError{Field: "additionalInstructions", Message: "additionalInstructions must be at most 4000 characters"})
	}
	return errs
}

func classifyServiceError(err error) (status int, code, message string, details any) {
	var validationErr ValidationError
	switch {
	case errors.As(err, &validationErr):
		return http.StatusBadRequest, "validation_error", "resume validation failed", validationErr.Errors
	case errors.Is(err, ErrInvalidInput):
		return http.StatusBadRequest, "validation_error", "invalid resume request", nil
	case errors.Is(err, ErrGenerationTimeout):
		return http.StatusGatewayTimeout, "RESUME_GENERATION_TIMEOUT", "resume generation timed out", nil
	case errors.Is(err, ErrInvalidLLMOutput):
		return http.StatusBadGateway, "RESUME_GENERATION_INVALID_OUTPUT", "invalid model output", nil
	case errors.Is(err, ErrJobQueueNotConfigured):
		return http.StatusInternalServerError, "internal_error", "resume generation queue is not configured", nil
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "not_found", "resume not found", nil
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, "forbidden", "not allowed", nil
	default:
		return http.StatusInternalServerError, "internal_error", "failed to process resume", nil
	}
}

func bindJSONErrorDetails(err error) []modelv1.ValidationError {
	var typeErr *json.UnmarshalTypeError
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &typeErr) {
		field := strings.TrimSpace(typeErr.Field)
		if field == "" {
			field = "body"
		}
		return []modelv1.ValidationError{{
			Field:   lowerFirst(field),
			Message: "has an invalid type",
		}}
	}
	if errors.As(err, &maxBytesErr) {
		return []modelv1.ValidationError{{
			Field:   "body",
			Message: "request body exceeds 65536 bytes",
		}}
	}
	if errors.Is(err, io.EOF) {
		return []modelv1.ValidationError{{
			Field:   "body",
			Message: "request body is required",
		}}
	}
	return []modelv1.ValidationError{{
		Field:   "body",
		Message: strings.TrimSpace(err.Error()),
	}}
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	first := strings.ToLower(string(runes[0]))
	runes[0] = []rune(first)[0]
	return string(runes)
}
