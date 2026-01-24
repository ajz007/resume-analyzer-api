package applies_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/applies"
	"resume-backend/internal/generatedresumes"
	"resume-backend/internal/shared/storage/object"
	"resume-backend/internal/shared/storage/object/local"
)

func TestGeneratedResumeDownloadGuestOwn(t *testing.T) {
	router, genRepo, store := newDownloadRouter(t, "guest:guest-1", true)
	resume := seedGeneratedResume(t, genRepo, store, "guest:guest-1", "resume-guest-own")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/generated-resumes/"+resume.ID+"/download", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("unexpected content type: %s", ct)
	}
	if cd := resp.Header().Get("Content-Disposition"); cd != "attachment; filename=\"generated_resume.docx\"" {
		t.Fatalf("unexpected content disposition: %s", cd)
	}
	if resp.Body.Len() == 0 {
		t.Fatalf("expected download body")
	}
}

func TestGeneratedResumeDownloadGuestForbidden(t *testing.T) {
	router, genRepo, store := newDownloadRouter(t, "guest:guest-2", true)
	resume := seedGeneratedResume(t, genRepo, store, "guest:owner", "resume-guest-forbidden")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/generated-resumes/"+resume.ID+"/download", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected json content type, got %s", ct)
	}
	if cd := resp.Header().Get("Content-Disposition"); cd != "" {
		t.Fatalf("expected empty content disposition, got %s", cd)
	}
	if !strings.Contains(resp.Body.String(), "access denied") {
		t.Fatalf("expected access denied error, got %s", resp.Body.String())
	}
}

func TestGeneratedResumeDownloadMissingIdentity(t *testing.T) {
	router, genRepo, store := newDownloadRouter(t, "", false)
	resume := seedGeneratedResume(t, genRepo, store, "guest:owner", "resume-missing-identity")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/generated-resumes/"+resume.ID+"/download", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected json content type, got %s", ct)
	}
	if cd := resp.Header().Get("Content-Disposition"); cd != "" {
		t.Fatalf("expected empty content disposition, got %s", cd)
	}
	if !strings.Contains(resp.Body.String(), "Missing identity") {
		t.Fatalf("expected missing identity error, got %s", resp.Body.String())
	}
}

func TestGeneratedResumeDownloadNotFound(t *testing.T) {
	router, _, _ := newDownloadRouter(t, "user-1", false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/generated-resumes/missing-id/download", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected json content type, got %s", ct)
	}
	if cd := resp.Header().Get("Content-Disposition"); cd != "" {
		t.Fatalf("expected empty content disposition, got %s", cd)
	}
}

func TestGeneratedResumeDownloadUserOwn(t *testing.T) {
	router, genRepo, store := newDownloadRouter(t, "user-1", false)
	resume := seedGeneratedResume(t, genRepo, store, "user-1", "resume-user-own")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/generated-resumes/"+resume.ID+"/download", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("unexpected content type: %s", ct)
	}
	if cd := resp.Header().Get("Content-Disposition"); cd != "attachment; filename=\"generated_resume.docx\"" {
		t.Fatalf("unexpected content disposition: %s", cd)
	}
	if resp.Body.Len() == 0 {
		t.Fatalf("expected download body")
	}
}

func TestGeneratedResumeDownloadReadFailureReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := readFailStore{}
	genRepo := generatedresumes.NewMemoryRepo()
	handler := applies.NewHandler(&applies.Service{}, genRepo, store)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "user-1")
		c.Set("isGuest", false)
		c.Next()
	})
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api)

	resume := generatedresumes.GeneratedResume{
		ID:         "resume-read-fail",
		UserID:     "user-1",
		DocumentID: "doc-1",
		AnalysisID: "analysis-1",
		TemplateID: "template-1",
		StorageKey: "missing",
		MimeType:   "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		SizeBytes:  10,
		CreatedAt:  time.Now().UTC(),
	}
	if err := genRepo.Create(context.Background(), resume); err != nil {
		t.Fatalf("create generated resume: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/generated-resumes/"+resume.ID+"/download", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected json content type, got %s", ct)
	}
	if cd := resp.Header().Get("Content-Disposition"); cd != "" {
		t.Fatalf("expected empty content disposition, got %s", cd)
	}
}

func newDownloadRouter(t *testing.T, userID string, isGuest bool) (*gin.Engine, *generatedresumes.MemoryRepo, object.ObjectStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	store := local.New(t.TempDir())
	genRepo := generatedresumes.NewMemoryRepo()
	handler := applies.NewHandler(&applies.Service{}, genRepo, store)

	router := gin.New()
	if userID != "" {
		router.Use(func(c *gin.Context) {
			c.Set("userId", userID)
			c.Set("isGuest", isGuest)
			c.Next()
		})
	}
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api)

	return router, genRepo, store
}

type readFailStore struct{}

func (readFailStore) Save(ctx context.Context, userID string, fileName string, r io.Reader) (string, int64, string, error) {
	return "", 0, "", nil
}

func (readFailStore) Open(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	return readFailCloser{}, nil
}

type readFailCloser struct{}

func (readFailCloser) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (readFailCloser) Close() error {
	return nil
}

func seedGeneratedResume(t *testing.T, repo *generatedresumes.MemoryRepo, store object.ObjectStore, userID, resumeID string) generatedresumes.GeneratedResume {
	t.Helper()

	data := []byte("fake docx data")
	key, size, _, err := store.Save(context.Background(), userID, "generated.docx", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("save resume: %v", err)
	}

	resume := generatedresumes.GeneratedResume{
		ID:         resumeID,
		UserID:     userID,
		DocumentID: "doc-1",
		AnalysisID: "analysis-1",
		TemplateID: "template-1",
		StorageKey: key,
		MimeType:   "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		SizeBytes:  size,
		CreatedAt:  time.Now().UTC(),
	}
	if err := repo.Create(context.Background(), resume); err != nil {
		t.Fatalf("create generated resume: %v", err)
	}
	return resume
}
