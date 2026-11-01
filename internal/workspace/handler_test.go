package workspace

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"resume-backend/internal/documents"
)

func setup(t *testing.T) (*gin.Engine, *MemoryStore, *documents.MemoryRepo, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := NewMemoryStore()
	docs := documents.NewMemoryRepo()
	docID := uuid.NewString()
	analysisID := uuid.NewString()
	if err := docs.Create(context.Background(), documents.Document{ID: docID, UserID: "owner", FileName: "source.pdf", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	app := Application{AnalysisID: analysisID, DocumentID: docID, ResumeFileName: "source.pdf", JobExcerpt: "Original job", Status: "completed"}
	store.Sources = func(ctx context.Context, user string) ([]Application, error) {
		if user == "owner" {
			return []Application{app}, nil
		}
		return []Application{}, nil
	}
	h := &Handler{Store: store, Documents: docs, Application: func(ctx context.Context, id string) (Application, string, error) {
		if id != analysisID {
			return Application{}, "", ErrNotFound
		}
		return app, "owner", nil
	}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", c.GetHeader("X-Test-User"))
		c.Set("isGuest", c.GetHeader("X-Test-Guest") == "true")
	})
	h.RegisterRoutes(r.Group("/api/v1"))
	return r, store, docs, docID, analysisID
}
func call(r *gin.Engine, method, path, user, body string, guest bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", user)
	if guest {
		req.Header.Set("X-Test-Guest", "true")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
func TestPrimarySelectionRetainsOldSourceAndSurvivesNewUpload(t *testing.T) {
	r, _, docs, first, _ := setup(t)
	w := call(r, "PUT", "/workspace/primary-resume", "owner", `{"documentId":"`+first+`"}`, false)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	second := uuid.NewString()
	_ = docs.Create(context.Background(), documents.Document{ID: second, UserID: "owner", FileName: "second.pdf", CreatedAt: time.Now().Add(time.Hour)})
	w = call(r, "GET", "/workspace/primary-resume", "owner", "", false)
	if !strings.Contains(w.Body.String(), first) || !strings.Contains(w.Body.String(), `"isPrimary":true`) {
		t.Fatal(w.Body)
	}
	if _, err := docs.GetByID(context.Background(), "owner", first); err != nil {
		t.Fatal("Selecting primary mutated source", err)
	}
}
func TestPrimaryIsOwnerScopedAndGuestCannotSet(t *testing.T) {
	r, _, _, id, _ := setup(t)
	for _, tt := range []struct {
		user  string
		guest bool
		want  int
	}{{"other", false, 404}, {"owner", true, 401}} {
		w := call(r, "PUT", "/workspace/primary-resume", tt.user, `{"documentId":"`+id+`"}`, tt.guest)
		if w.Code != tt.want {
			t.Fatal(w.Code, w.Body)
		}
	}
}
func TestPrimaryFallbackIsNotClaimedAsExplicit(t *testing.T) {
	r, _, _, id, _ := setup(t)
	w := call(r, "GET", "/workspace/primary-resume", "owner", "", false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), id) || !strings.Contains(w.Body.String(), `"isPrimary":false`) {
		t.Fatal(w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("Private source response must not be cached")
	}
}
func TestApplicationLabelsAreOwnerOnlyAndDoNotChangeSourceOrJob(t *testing.T) {
	r, _, _, doc, id := setup(t)
	w := call(r, "PUT", "/applications/"+id, "owner", `{"companyName":" Nimbus ","targetRole":"Engineer"}`, false)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = call(r, "GET", "/applications/"+id, "owner", "", false)
	var a Application
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if a.CompanyName != "Nimbus" || a.TargetRole != "Engineer" || a.DocumentID != doc || a.JobExcerpt != "Original job" {
		t.Fatal(a)
	}
	for _, method := range []string{"GET", "PUT"} {
		w = call(r, method, "/applications/"+id, "other", `{"companyName":"Intruder"}`, false)
		if w.Code != 404 || strings.Contains(w.Body.String(), "Nimbus") {
			t.Fatal(w.Code, w.Body)
		}
	}
}
func TestSearchMatchesLabelsBeforePaginationAndIsOwnerScoped(t *testing.T) {
	r, store, _, _, id := setup(t)
	_ = store.SaveLabels(context.Background(), id, Labels{CompanyName: "Nimbus", TargetRole: "Platform Engineer"})
	for _, q := range []string{"nimbus", "platform", "source.pdf"} {
		w := call(r, "GET", "/applications?q="+q, "owner", "", false)
		if w.Code != 200 || !strings.Contains(w.Body.String(), id) {
			t.Fatal(w.Code, w.Body)
		}
	}
	w := call(r, "GET", "/applications?q=nimbus", "other", "", false)
	if w.Code != 200 || strings.Contains(w.Body.String(), id) {
		t.Fatal(w.Code, w.Body)
	}
	w = call(r, "GET", "/applications?q=nimbus&offset=1", "owner", "", false)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal(w.Body)
	}
	w = call(r, "GET", "/applications", "owner", "", true)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestLabelValidationAndMissingID(t *testing.T) {
	r, _, _, _, id := setup(t)
	for _, body := range []string{`{"companyName":"` + strings.Repeat("界", 121) + `"}`, `{"targetRole":"First\nSecond"}`} {
		w := call(r, "PUT", "/applications/"+id, "owner", body, false)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
	for _, q := range []string{"limit=-1", "limit=51", "offset=-2", "q=" + strings.Repeat("x", 121)} {
		w := call(r, "GET", "/applications?"+q, "owner", "", false)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
	if w := call(r, "GET", "/applications/not-a-uuid", "owner", "", false); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
