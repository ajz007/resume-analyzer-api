package documents_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/bootstrap"
	"resume-backend/internal/shared/config"
)

func TestDocumentsUploadAndCurrent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Config{
		Port:            "0",
		CORSAllowOrigin: []string{"http://localhost:5173"},
		LocalStoreDir:   t.TempDir(),
		Env:             "dev",
		ObjectStoreType: "local",
	}

	app, err := bootstrap.Build(cfg)
	if err != nil {
		t.Fatalf("bootstrap build: %v", err)
	}
	router := app.Router

	// Upload a small file.
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fileWriter.Write([]byte("hello world")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.Code)
	}

	var created struct {
		DocumentID string `json:"documentId"`
		FileName   string `json:"fileName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.DocumentID == "" {
		t.Fatalf("expected documentId, got empty")
	}

	// Fetch current document.
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/documents/current", nil)
	addGuestHeader(reqGet)
	respGet := httptest.NewRecorder()
	router.ServeHTTP(respGet, reqGet)

	if respGet.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respGet.Code)
	}

	var current struct {
		DocumentID string `json:"documentId"`
		FileName   string `json:"fileName"`
	}
	if err := json.NewDecoder(respGet.Body).Decode(&current); err != nil {
		t.Fatalf("decode current response: %v", err)
	}
	if current.FileName != "hello.txt" {
		t.Fatalf("expected fileName hello.txt, got %s", current.FileName)
	}
}

func addGuestHeader(req *http.Request) {
	req.Header.Set("X-Guest-Id", "test-guest")
}
