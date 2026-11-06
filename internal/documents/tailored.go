package documents

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"resume-backend/internal/shared/server/middleware"
)

const TailoredMIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

type TailoredRepository interface {
	ListTailored(context.Context, string, string) ([]Document, error)
}

func (r *PGRepo) ListTailored(ctx context.Context, userID, analysisID string) ([]Document, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, file_name, mime_type, size_bytes, created_at
	FROM documents WHERE user_id = $1 AND tailored_analysis_id = $2 AND deleted_at IS NULL
	ORDER BY created_at DESC, id DESC`, userID, analysisID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Document{}
	for rows.Next() {
		doc := Document{UserID: userID, TailoredAnalysisID: analysisID}
		if err := rows.Scan(&doc.ID, &doc.FileName, &doc.MimeType, &doc.SizeBytes, &doc.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, doc)
	}
	return result, rows.Err()
}

func (r *MemoryRepo) ListTailored(ctx context.Context, userID, analysisID string) ([]Document, error) {
	docs, err := r.ListByUser(ctx, userID, 0, 0)
	result := []Document{}
	for _, doc := range docs {
		if doc.TailoredAnalysisID == analysisID {
			result = append(result, doc)
		}
	}
	return result, err
}

func (h *Handler) downloadTailored(c *gin.Context) {
	if _, err := uuid.Parse(c.Param("id")); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	ctx := c.Request.Context()
	doc, err := h.Svc.Repo.GetByID(ctx, middleware.UserIDFromContext(c), c.Param("id"))
	if err != nil || doc.TailoredAnalysisID == "" {
		c.Status(http.StatusNotFound)
		return
	}
	file, err := h.Svc.Store.Open(ctx, doc.StorageKey)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	defer file.Close()
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", `attachment; filename="reviewed-resume.docx"`)
	c.Header("Content-Length", fmt.Sprint(doc.SizeBytes))
	c.Header("Content-Type", TailoredMIME)
	_, _ = io.Copy(c.Writer, file)
}
