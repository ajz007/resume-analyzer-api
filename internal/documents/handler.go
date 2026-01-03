package documents

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
)

const maxUploadSize = 10 << 20 // 10MB

// Handler wires HTTP handlers to the service.
type Handler struct {
	Svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

// RegisterRoutes attaches document routes to the router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/documents", h.upload)
	rg.GET("/documents/current", h.current)
	rg.GET("/documents", h.list)
}

func (h *Handler) upload(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "file is required", nil)
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "validation_error", "unable to read file", nil)
		return
	}
	defer file.Close()

	doc, err := h.Svc.Upload(c.Request.Context(), userID, fileHeader.Filename, file)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			respond.Error(c, http.StatusBadRequest, "validation_error", err.Error(), nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "failed to upload document", err.Error(), nil)
		}
		return
	}

	respond.JSON(c, http.StatusCreated, toResponse(doc))
}

func (h *Handler) current(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)

	doc, err := h.Svc.Current(c.Request.Context(), userID)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			respond.Error(c, http.StatusNotFound, "not_found", "document not found", nil)
		case errors.Is(err, ErrInvalidInput):
			respond.Error(c, http.StatusBadRequest, "validation_error", err.Error(), nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to fetch document", nil)
		}
		return
	}

	respond.JSON(c, http.StatusOK, toResponse(doc))
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

	docs, err := h.Svc.List(c.Request.Context(), userID, limit, offset)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			respond.Error(c, http.StatusBadRequest, "validation_error", err.Error(), nil)
		default:
			respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to list documents", nil)
		}
		return
	}

	resp := make([]gin.H, 0, len(docs))
	for _, doc := range docs {
		resp = append(resp, gin.H{
			"documentId": doc.ID,
			"fileName":   doc.FileName,
			"mimeType":   doc.MimeType,
			"sizeBytes":  doc.SizeBytes,
			"uploadedAt": doc.CreatedAt,
		})
	}

	respond.JSON(c, http.StatusOK, resp)
}
