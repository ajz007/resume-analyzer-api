package analyses

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"resume-backend/internal/documents"
	"resume-backend/internal/extract"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/server/respond"
	"resume-backend/internal/shared/storage/object"
	"resume-backend/internal/shared/telemetry"
	"resume-backend/internal/shared/util"
)

func (h *Handler) saveTailoredResume(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if guest, _ := c.Get("isGuest"); guest == true {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "sign in to save a tailored resume"})
		return
	}
	data, analysis, source := h.renderReviewDownload(c)
	if data == nil {
		return
	}
	ctx := c.Request.Context()
	id := uuid.NewSHA1(uuid.NameSpaceOID, append([]byte(analysis.ID+":"), data...)).String()
	doc, err := h.DocRepo.GetByID(ctx, analysis.UserID, id)
	if err == nil {
		c.JSON(http.StatusOK, documents.ToResponse(doc))
		return
	}
	if !errors.Is(err, documents.ErrNotFound) {
		tailoredSaveError(c, "lookup", err)
		return
	}

	name := "Tailored resume"
	if h.TailoredName != nil {
		label, err := h.TailoredName(ctx, analysis.ID)
		if err != nil {
			tailoredSaveError(c, "labels", err)
			return
		}
		if label != "" {
			name = label
		}
	}
	key := tailoredStorageKey(source, id)
	size, err := saveTailoredObject(ctx, h.Svc.Store, key, data)
	if err != nil {
		tailoredSaveError(c, "storage", err)
		return
	}
	doc = documents.Document{
		ID: id, UserID: analysis.UserID, TailoredAnalysisID: analysis.ID,
		FileName: name + ".docx", MimeType: documents.TailoredMIME, SizeBytes: size,
		StorageProvider: source.StorageProvider, StorageKey: key, CreatedAt: time.Now().UTC(),
	}
	if err := h.DocRepo.Create(ctx, doc); err != nil {
		tailoredSaveError(c, "insert", err)
		return
	}
	// Read back the winning row when two identical saves race.
	doc, err = h.DocRepo.GetByID(ctx, analysis.UserID, id)
	if err != nil {
		tailoredSaveError(c, "readback", err)
		return
	}
	c.JSON(http.StatusOK, documents.ToResponse(doc))
}

func tailoredStorageKey(source documents.Document, id string) string {
	dir := path.Dir(source.StorageKey)
	if dir == "." {
		dir = util.HashUserKey(source.UserID)
	}
	return path.Join(dir, "tailored-"+id+".docx")
}

func tailoredSaveError(c *gin.Context, stage string, err error) {
	fields := map[string]any{"stage": stage, "request_id": middleware.RequestIDFromContext(c)}
	var awsError interface{ ErrorCode() string }
	var dbError interface{ SQLState() string }
	if errors.As(err, &awsError) {
		fields["code"] = awsError.ErrorCode()
	} else if errors.As(err, &dbError) {
		fields["code"] = dbError.SQLState()
	}
	telemetry.Error("tailored_resume.save_failed", fields)
	respond.Error(c, http.StatusInternalServerError, "tailored_save_failed", "Could not save this resume version. Your edit choices are still available.", nil)
}

func saveTailoredObject(ctx context.Context, store object.ObjectStore, key string, data []byte) (int64, error) {
	saver, ok := store.(interface {
		SaveWithKey(context.Context, string, string, io.Reader) (int64, error)
	})
	if !ok {
		return 0, errors.New("object store cannot save tailored resumes")
	}
	return saver.SaveWithKey(ctx, key, documents.TailoredMIME, bytes.NewReader(data))
}

func (h *Handler) listTailoredResumes(c *gin.Context) {
	ctx := c.Request.Context()
	user := middleware.UserIDFromContext(c)
	analysis, err := h.Svc.Get(ctx, c.Param("id"))
	if err != nil || analysis.UserID != user {
		c.Status(http.StatusNotFound)
		return
	}
	repo, ok := h.DocRepo.(documents.TailoredRepository)
	if !ok {
		c.Status(http.StatusInternalServerError)
		return
	}
	docs, err := repo.ListTailored(ctx, user, analysis.ID)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	response := make([]documents.DocumentResponse, 0, len(docs))
	for _, doc := range docs {
		response = append(response, documents.ToResponse(doc))
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, response)
}

func (h *Handler) previewTailoredResume(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	data, _, _ := h.renderReviewDownload(c)
	if data == nil {
		return
	}
	text, err := extract.ExtractTextFromBytes(c.Request.Context(), data, documents.TailoredMIME, "reviewed-resume.docx")
	if err != nil {
		c.Status(http.StatusUnprocessableEntity)
		return
	}
	blocks, err := previewDOCXBlocks(data)
	if err != nil {
		c.Status(http.StatusUnprocessableEntity)
		return
	}
	c.JSON(http.StatusOK, gin.H{"text": text, "blocks": blocks})
}
