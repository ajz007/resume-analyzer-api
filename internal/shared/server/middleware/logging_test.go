package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoggingIncludesRequiredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestID(), Auth("dev"), Logging())
	router.GET("/test", func(c *gin.Context) {
		c.Set("documentId", "doc-1")
		c.Set("analysisId", "analysis-1")
		c.Set("statusTransition", "queued->processing")
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
	}()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Guest-Id", "guest1")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	_ = w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read log output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected log output")
	}
	last := lines[len(lines)-1]
	var payload map[string]any
	if err := json.Unmarshal([]byte(last), &payload); err != nil {
		t.Fatalf("decode log json: %v", err)
	}

	required := []string{"request_id", "user_id", "document_id", "analysis_id", "duration_ms", "status", "status_transition"}
	for _, key := range required {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing log field: %s", key)
		}
	}
	if payload["user_id"] != "guest:guest1" {
		t.Fatalf("unexpected user_id: %v", payload["user_id"])
	}
	if payload["document_id"] != "doc-1" {
		t.Fatalf("unexpected document_id: %v", payload["document_id"])
	}
	if payload["analysis_id"] != "analysis-1" {
		t.Fatalf("unexpected analysis_id: %v", payload["analysis_id"])
	}
	if payload["status_transition"] != "queued->processing" {
		t.Fatalf("unexpected status_transition: %v", payload["status_transition"])
	}
}
