package workspace

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"resume-backend/internal/documents"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
)

type Handler struct {
	Store     Store
	Documents documents.DocumentsRepo
	// Returns only the owner-facing application context; no result or answer data.
	Application func(context.Context, string) (Application, string, error)
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/workspace/primary-resume", h.primary)
	r.PUT("/workspace/primary-resume", h.setPrimary)
	r.GET("/applications", h.list)
	r.GET("/applications/:id", h.get)
	r.PUT("/applications/:id", h.save)
}
func owner(c *gin.Context) string { return middleware.UserIDFromContext(c) }
func signedIn(c *gin.Context) bool {
	if guest, _ := c.Get("isGuest"); guest == true {
		respond.Error(c, 401, "login_required", "Sign in to manage saved resumes and applications", nil)
		return false
	}
	return true
}
func fail(c *gin.Context, err error) {
	if errors.Is(err, ErrNotFound) || errors.Is(err, documents.ErrNotFound) {
		respond.Error(c, 404, "not_found", "Item not found", nil)
	} else {
		respond.Error(c, 500, "internal_error", "Could not load or save this workspace item", nil)
	}
}
func documentResponse(d documents.Document, explicit bool) gin.H {
	return gin.H{"documentId": d.ID, "fileName": d.FileName, "mimeType": d.MimeType, "sizeBytes": d.SizeBytes, "uploadedAt": d.CreatedAt, "isPrimary": explicit}
}
func (h *Handler) primary(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := h.Store.Primary(c.Request.Context(), owner(c))
	if err != nil {
		fail(c, err)
		return
	}
	if id != "" {
		d, err := h.Documents.GetByID(c.Request.Context(), owner(c), id)
		if err == nil {
			respond.JSON(c, 200, documentResponse(d, true))
			return
		}
		if !errors.Is(err, documents.ErrNotFound) {
			fail(c, err)
			return
		}
	}
	d, err := h.Documents.GetCurrentByUser(c.Request.Context(), owner(c))
	if err != nil {
		fail(c, err)
		return
	}
	respond.JSON(c, 200, documentResponse(d, false))
}
func (h *Handler) setPrimary(c *gin.Context) {
	if !signedIn(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req struct {
		DocumentID string `json:"documentId"`
	}
	if c.ShouldBindJSON(&req) != nil || uuid.Validate(req.DocumentID) != nil {
		respond.Error(c, 400, "validation_error", "A valid saved document is required", nil)
		return
	}
	d, err := h.Documents.GetByID(c.Request.Context(), owner(c), req.DocumentID)
	if err != nil {
		fail(c, err)
		return
	}
	if err = h.Store.SetPrimary(c.Request.Context(), owner(c), d.ID); err != nil {
		fail(c, err)
		return
	}
	respond.JSON(c, 200, documentResponse(d, true))
}
func (h *Handler) authorized(c *gin.Context) (Application, bool) {
	c.Header("Cache-Control", "no-store")
	if uuid.Validate(c.Param("id")) != nil {
		fail(c, ErrNotFound)
		return Application{}, false
	}
	a, user, err := h.Application(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return a, false
	}
	if user != owner(c) {
		fail(c, ErrNotFound)
		return a, false
	}
	return a, true
}
func (h *Handler) get(c *gin.Context) {
	a, ok := h.authorized(c)
	if !ok {
		return
	}
	l, err := h.Store.GetLabels(c.Request.Context(), a.AnalysisID)
	if err != nil {
		fail(c, err)
		return
	}
	a.Labels = l
	respond.JSON(c, 200, a)
}
func (h *Handler) save(c *gin.Context) {
	a, ok := h.authorized(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var l Labels
	if c.ShouldBindJSON(&l) != nil {
		respond.Error(c, 400, "validation_error", "Company and role labels are invalid", nil)
		return
	}
	l.CompanyName = strings.TrimSpace(l.CompanyName)
	l.TargetRole = strings.TrimSpace(l.TargetRole)
	if utf8.RuneCountInString(l.CompanyName) > 120 || utf8.RuneCountInString(l.TargetRole) > 120 || strings.ContainsAny(l.CompanyName+l.TargetRole, "\r\n\x00") {
		respond.Error(c, 400, "validation_error", "Use a single line of at most 120 characters for each label", nil)
		return
	}
	if err := h.Store.SaveLabels(c.Request.Context(), a.AnalysisID, l); err != nil {
		fail(c, err)
		return
	}
	a.Labels = l
	respond.JSON(c, 200, a)
}
func (h *Handler) list(c *gin.Context) {
	if !signedIn(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	q := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(q) > 120 {
		respond.Error(c, 400, "validation_error", "Search is limited to 120 characters", nil)
		return
	}
	limit := 20
	offset := 0
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 50 {
			respond.Error(c, 400, "validation_error", "Limit must be between 1 and 50", nil)
			return
		}
		limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 1000000 {
			respond.Error(c, 400, "validation_error", "Offset is invalid", nil)
			return
		}
		offset = n
	}
	apps, err := h.Store.List(c.Request.Context(), owner(c), q, limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	respond.JSON(c, http.StatusOK, apps)
}
