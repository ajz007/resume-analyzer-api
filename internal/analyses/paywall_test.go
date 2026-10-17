package analyses

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/shared/server/middleware"
)

func paywallFixture(t *testing.T) map[string]any {
	t.Helper()
	raw := []byte(`{
  "finalScore": 78,
  "summary": {"overallAssessment": "ok"},
  "ats": {"score": 80, "missingKeywords": {"fromJobDescription": ["kafka"]}},
  "aiScreening": {"score": 70},
  "fixThisFirst": [{"title": "a"}, {"title": "b"}],
  "issues": [{"t": 1}, {"t": 2}, {"t": 3}, {"t": 4}, {"t": 5}],
  "bulletRewrites": [{"before": "x", "after": "y"}],
  "recommendations": [{"r": 1}],
  "missingInformation": ["metrics"],
  "actionPlan": {"quickWins": ["q"], "mediumEffort": [], "deepFixes": []},
  "jobRequirementProfile": {"isApplicable": true, "primaryRole": "Architect", "topPriorities": [{"name": "Java"}]},
  "jobMatchScoring": {"score": 76, "requirementScores": [{"name": "Java", "score": 90}]}
}`)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return out
}

func TestRedactForFreeTierKeepsScoresAndRemovesPaidSections(t *testing.T) {
	original := paywallFixture(t)
	redacted, locked := RedactForFreeTier(original)

	wantLocked := []string{LockedIssues, LockedBulletRewrites, LockedRecommendations, LockedMissingInformation, LockedActionPlan, LockedJobRequirements}
	if !reflect.DeepEqual(locked, wantLocked) {
		t.Fatalf("locked sections = %v, want %v", locked, wantLocked)
	}
	for _, key := range []string{"finalScore", "summary", "ats", "aiScreening", "fixThisFirst"} {
		if !reflect.DeepEqual(redacted[key], original[key]) {
			t.Fatalf("expected %s to stay free", key)
		}
	}
	if got := len(redacted["issues"].([]any)); got != freeTierIssueLimit {
		t.Fatalf("expected %d issues, got %d", freeTierIssueLimit, got)
	}
	if got := len(redacted["bulletRewrites"].([]any)); got != 0 {
		t.Fatalf("expected bulletRewrites removed, got %d", got)
	}
	profile := redacted["jobRequirementProfile"].(map[string]any)
	if profile["primaryRole"] != "Architect" || len(profile["topPriorities"].([]any)) != 0 {
		t.Fatalf("expected primaryRole kept and topPriorities removed, got %v", profile)
	}
	scoring := redacted["jobMatchScoring"].(map[string]any)
	if scoring["score"] != float64(76) || len(scoring["requirementScores"].([]any)) != 0 {
		t.Fatalf("expected job match score kept and requirementScores removed, got %v", scoring)
	}

	// The stored result must not be mutated.
	if len(original["issues"].([]any)) != 5 || len(original["jobMatchScoring"].(map[string]any)["requirementScores"].([]any)) != 1 {
		t.Fatalf("RedactForFreeTier mutated its input")
	}
}

func TestRedactForFreeTierNothingToLock(t *testing.T) {
	redacted, locked := RedactForFreeTier(map[string]any{"finalScore": 70.0, "issues": []any{map[string]any{"t": 1}}})
	if len(locked) != 0 {
		t.Fatalf("expected no locked sections, got %v", locked)
	}
	if len(redacted["issues"].([]any)) != 1 {
		t.Fatalf("expected issues untouched")
	}
}

type fakeUnlocks struct {
	active   bool
	unlocked map[string]bool
}

func (f fakeUnlocks) PaywallActive() bool { return f.active }

func (f fakeUnlocks) IsUnlocked(_ context.Context, userID, analysisID string) (bool, error) {
	return f.unlocked[userID+"|"+analysisID], nil
}

func getAnalysisWithUnlocks(t *testing.T, unlocks UnlockChecker) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := NewMemoryRepo()
	if err := repo.Create(context.Background(), Analysis{
		ID:        "a-paywall",
		UserID:    "guest:test-guest",
		Status:    StatusCompleted,
		Result:    paywallFixture(t),
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create analysis: %v", err)
	}
	handler := NewHandler(&Service{Repo: repo}, nil)
	handler.Unlocks = unlocks
	router := gin.New()
	router.Use(middleware.Auth("dev"))
	handler.RegisterRoutes(router.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analyses/a-paywall", nil)
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func TestGetAnalysisLockedWhenPaywallActiveAndNotUnlocked(t *testing.T) {
	body := getAnalysisWithUnlocks(t, fakeUnlocks{active: true})
	if body["locked"] != true {
		t.Fatalf("expected locked=true, got %v", body["locked"])
	}
	result := body["result"].(map[string]any)
	if len(result["bulletRewrites"].([]any)) != 0 {
		t.Fatalf("expected bulletRewrites redacted")
	}
	if len(body["lockedSections"].([]any)) == 0 {
		t.Fatalf("expected lockedSections")
	}
}

func TestGetAnalysisFullWhenUnlockedOrPaywallOff(t *testing.T) {
	for name, unlocks := range map[string]UnlockChecker{
		"unlocked":    fakeUnlocks{active: true, unlocked: map[string]bool{"guest:test-guest|a-paywall": true}},
		"paywall off": fakeUnlocks{active: false},
		"no checker":  nil,
	} {
		body := getAnalysisWithUnlocks(t, unlocks)
		if body["locked"] != false {
			t.Fatalf("%s: expected locked=false, got %v", name, body["locked"])
		}
		if got := len(body["result"].(map[string]any)["bulletRewrites"].([]any)); got != 1 {
			t.Fatalf("%s: expected full bulletRewrites, got %d", name, got)
		}
	}
}
