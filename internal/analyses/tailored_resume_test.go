package analyses

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"resume-backend/internal/documents"
	"resume-backend/internal/shared/storage/object/local"
)

func TestTailoredResumeSnapshotMatchesExportAndCanBeReused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, analysis, _, client := sessionFixture(t)
	svc.Store = local.New(t.TempDir())
	ctx := context.Background()
	key, _, _, err := svc.Store.Save(ctx, analysis.UserID, "source.txt", strings.NewReader(reviewSource))
	if err != nil {
		t.Fatal(err)
	}
	docs := documents.NewMemoryRepo()
	if err := docs.Create(ctx, documents.Document{ID: analysis.DocumentID, UserID: analysis.UserID, ExtractedTextKey: key}); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(svc, docs)
	h.TailoredName = func(context.Context, string) (string, error) { return "Example · Engineer", nil }
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userId", c.GetHeader("Test-Owner")) })
	h.RegisterRoutes(r.Group("/api/v1"))
	documents.NewHandler(&documents.Service{Repo: docs, Store: svc.Store}).RegisterRoutes(r.Group("/api/v1"))
	call := func(method, path, owner, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Test-Owner", owner)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	base := "/analyses/" + analysis.ID
	report, _, err := mergedReview(analysis.Result, ReviewSession{})
	if err != nil || len(report.Edits) == 0 {
		t.Fatalf("fixture has no edits: %v", err)
	}
	raw, _ := json.Marshal(map[string]any{"editIds": []string{report.Edits[0].ID}, "reviewVersion": 0})
	selection := string(raw)
	export := call("POST", base+"/reviewed-resume", analysis.UserID, selection)
	if export.Code != 200 {
		t.Fatal(export.Code, export.Body.String())
	}
	snapshot := base + "/tailored-resumes"
	if w := call("POST", snapshot+"/preview", analysis.UserID, selection); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	save := func(body string) documents.DocumentResponse {
		w := call("POST", snapshot, analysis.UserID, body)
		var doc documents.DocumentResponse
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &doc) != nil || doc.DocumentID == "" {
			t.Fatal(w.Code, w.Body.String())
		}
		return doc
	}
	first := save(selection)
	if again := save(selection); again.DocumentID != first.DocumentID {
		t.Fatal("duplicate snapshot")
	}
	if changed := save(`{"editIds":[],"reviewVersion":0}`); changed.DocumentID == first.DocumentID {
		t.Fatal("changed choices overwrote snapshot")
	}
	if first.TailoredAnalysisID != analysis.ID || first.FileName != "Example · Engineer.docx" {
		t.Fatal(first)
	}
	download := "/documents/" + first.DocumentID + "/tailored-download"
	if w := call("GET", download, analysis.UserID, ""); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), export.Body.Bytes()) {
		t.Fatal("saved export differs", w.Code)
	}
	if w := call("GET", download, "another-user", ""); w.Code != 404 {
		t.Fatal("snapshot visible to another user", w.Code)
	}
	if w := call("GET", snapshot, "another-user", ""); w.Code != 404 {
		t.Fatal("snapshot list visible to another user", w.Code)
	}
	if doc, err := docs.GetCurrentByUser(ctx, analysis.UserID); err != nil || doc.ID != analysis.DocumentID {
		t.Fatal("default source changed", doc, err)
	}
	if client.calls.Load() != 0 {
		t.Fatal("saving invoked AI")
	}
}
