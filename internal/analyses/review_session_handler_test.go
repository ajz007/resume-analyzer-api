package analyses

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"resume-backend/internal/documents"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/storage/object/local"
)

func TestReviewSessionHTTPFlowAndAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, a, request, client := sessionFixture(t)
	a.UserID = "guest:test-guest"
	svc.Repo = NewMemoryRepo()
	if err := svc.Repo.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	store := local.New(t.TempDir())
	svc.Store = store
	key, _, _, err := store.Save(context.Background(), a.UserID, "source.txt", strings.NewReader(reviewSource))
	if err != nil {
		t.Fatal(err)
	}
	docs := documents.NewMemoryRepo()
	svc.DocRepo = docs
	if err = docs.Create(context.Background(), documents.Document{ID: a.DocumentID, UserID: a.UserID, FileName: "source.txt", ExtractedTextKey: key}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(svc, docs)
	router := gin.New()
	router.Use(middleware.Auth("dev"))
	h.RegisterRoutes(router.Group("/api/v1"))
	call := func(method, path, guest string, payload any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest(method, "/api/v1/analyses/"+a.ID+path, bytes.NewReader(raw))
		req.Header.Set("X-Guest-Id", guest)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	for _, methodPath := range [][2]string{{"GET", "/review-session"}, {"POST", "/review-session/answers"}, {"PUT", "/review-session/choices"}} {
		if w := call(methodPath[0], methodPath[1], "other-owner", request); w.Code != 404 {
			t.Fatalf("other owner got %d", w.Code)
		}
	}
	if client.calls.Load() != 0 {
		t.Fatal("unauthorized request called model")
	}
	h.Unlocks = fakeUnlocks{active: true}
	if w := call("POST", "/review-session/answers", "test-guest", request); w.Code != 403 {
		t.Fatalf("locked answer got %d", w.Code)
	}
	h.Unlocks = nil
	w := call("GET", "/review-session", "test-guest", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing private response caching policy")
	}
	w = call("POST", "/review-session/answers", "test-guest", request)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Session ReviewSession `json:"session"`
		Report  ReviewReport  `json:"report"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Session.Version != 2 || len(response.Report.Edits) != 2 || len(response.Report.Sections[0].Items) != 2 {
		t.Fatalf("delta not assembled with original edits: %+v", response)
	}
	w = call("PUT", "/review-session/choices", "test-guest", map[string]any{"version": 0, "excludedIds": []string{}})
	if w.Code != 409 {
		t.Fatal("accepted stale choices", w.Code)
	}
	w = call("POST", "/reviewed-resume", "test-guest", map[string]any{"editIds": []string{response.Session.Answers[0].Edit.ID}})
	if w.Code != 409 {
		t.Fatal("export did not reject stale review version", w.Code)
	}
	w = call("POST", "/reviewed-resume", "test-guest", map[string]any{"editIds": []string{response.Session.Answers[0].Edit.ID}, "reviewVersion": 2})
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "wordprocessingml") {
		t.Fatal("export failed", w.Code, w.Body.String())
	}
	if client.calls.Load() != 1 {
		t.Fatal("export or reads invoked another model")
	}
	// Simulate a crashed generation. Recovery never repeats the model call.
	saved, _ := svc.Repo.GetByID(context.Background(), a.ID)
	state, _ := reviewState(saved.Result)
	state.Pending = &ReviewPending{RequestID: "abandoned", StartedAt: time.Now().Add(-7 * time.Minute)}
	state.Version++
	if err = svc.Repo.(reviewSessionRepo).CompareAndSwapReview(context.Background(), a.ID, a.UserID, 2, state); err != nil {
		t.Fatal(err)
	}
	w = call("GET", "/review-session", "test-guest", nil)
	if w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Session.Pending != nil || response.Session.LastError == "" || client.calls.Load() != 1 {
		t.Fatal("crash recovery resubmitted or stayed locked")
	}
}

func TestQueuedReviewAnswerIsClaimedOnceAndFailureDoesNotRetryModel(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		svc, a, request, client := sessionFixture(t)
		svc.Store = local.New(t.TempDir())
		docs := documents.NewMemoryRepo()
		svc.DocRepo = docs
		key, _, _, err := svc.Store.Save(context.Background(), a.UserID, "source.txt", strings.NewReader(reviewSource))
		if err != nil {
			t.Fatal(err)
		}
		if err = docs.Create(context.Background(), documents.Document{ID: a.DocumentID, UserID: a.UserID, ExtractedTextKey: key}); err != nil {
			t.Fatal(err)
		}
		q := &stubQueue{}
		svc.JobQueue = q
		if unsafe {
			client.delta.Passage = "Validated automated failover with 99 recovery drills."
		}
		state, err := svc.answerReview(context.Background(), a, reviewSource, request)
		if err != nil {
			t.Fatal(err)
		}
		if state.Pending == nil || state.Pending.Processing || client.calls.Load() != 0 || len(q.messages) != 1 || q.messages[0].Type != "review_answer" {
			t.Fatal("HTTP path did not reserve and enqueue without a paid call")
		}
		saved, _ := svc.Repo.GetByID(context.Background(), a.ID)
		if _, err = svc.answerReview(context.Background(), saved, reviewSource, request); err != nil || len(q.messages) != 1 {
			t.Fatal("pending replay enqueued twice", err)
		}
		if err = svc.ProcessReviewAnswer(context.Background(), a.ID, request.RequestID); err != nil {
			t.Fatal(err)
		}
		if err = svc.ProcessReviewAnswer(context.Background(), a.ID, request.RequestID); err != nil {
			t.Fatal(err)
		}
		saved, _ = svc.Repo.GetByID(context.Background(), a.ID)
		final, _ := reviewState(saved.Result)
		if client.calls.Load() != 1 || final.Pending != nil || final.Attempts != 1 {
			t.Fatal("queue redelivery regenerated or left the review locked")
		}
		if unsafe && (len(final.Answers) != 0 || final.LastRequest == nil || final.LastError == "") {
			t.Fatal("failed answer was not recoverable")
		}
		if !unsafe && (len(final.Answers) != 1 || final.Answers[0].QuestionID != request.QuestionID || final.ExcludedIDs[0] != "failover") {
			t.Fatal("queued answer lost context or choices")
		}
	}
}
