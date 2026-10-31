package analyses

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"resume-backend/internal/llm"
)

type answerClient struct {
	delta   ReviewDelta
	calls   atomic.Int32
	entered chan bool
	release chan bool
}

func (c *answerClient) AnalyzeResume(context.Context, llm.AnalyzeInput) (json.RawMessage, error) {
	return nil, errors.New("full analysis must not run")
}
func (c *answerClient) ReviseReview(_ context.Context, input llm.ReviewRevisionInput) (json.RawMessage, error) {
	c.calls.Add(1)
	if c.entered != nil {
		c.entered <- true
		<-c.release
	}
	if !strings.Contains(input.Payload, "TradingCo") || !json.Valid(input.ResponseSchema) {
		return nil, errors.New("missing original context or strict schema")
	}
	return json.Marshal(c.delta)
}

func sessionFixture(t *testing.T) (*Service, Analysis, ReviewAnswerRequest, *answerClient) {
	t.Helper()
	result := reviewFixture(t)
	raw, _ := json.Marshal(result)
	data := map[string]any{}
	_ = json.Unmarshal(raw, &data)
	a := Analysis{ID: "review-1", UserID: "owner", DocumentID: "doc-1", Status: StatusCompleted, JobDescription: "Job requires evidence of recovery validation.", Result: data}
	repo := NewMemoryRepo()
	if err := repo.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	answer := "At TradingCo, I validated automated failover with 12 recovery drills."
	client := &answerClient{delta: ReviewDelta{Action: "add_evidence", Passage: "Validated automated failover with 12 recovery drills.", Section: "TradingCo", InsertAfter: "TradingCo", SourceQuote: "TradingCo", AnswerQuote: answer, Explanation: "Adds the confirmed recovery validation.", Establishes: "Personally performed recovery drills at TradingCo.", StillUnproven: "Does not establish payments experience."}}
	req := ReviewAnswerRequest{Version: 0, RequestID: uuid.NewString(), RequirementID: result.JobRequirementProfile.TopPriorities[0].ID, QuestionID: "recovery", Kind: "experience", Answer: answer, Confirmed: true, ExcludedIDs: []string{"failover"}}
	return &Service{Repo: repo, LLM: client}, a, req, client
}

func TestAnswerReviewAddsOnlyDeltaAndPreservesOriginalChoices(t *testing.T) {
	svc, original, req, client := sessionFixture(t)
	state, err := svc.answerReview(context.Background(), original, reviewSource, req)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != 2 || len(state.Answers) != 1 || state.Pending != nil || state.ExcludedIDs[0] != "failover" {
		t.Fatalf("unexpected state: %+v", state)
	}
	saved, _ := svc.Repo.GetByID(context.Background(), original.ID)
	before, _ := json.Marshal(original.Result["report"])
	after, _ := json.Marshal(saved.Result["report"])
	if string(before) != string(after) {
		t.Fatal("original report was rewritten")
	}
	for _, key := range []string{"jobMatchScoring", "ats", "aiScreening"} {
		x, _ := json.Marshal(original.Result[key])
		y, _ := json.Marshal(saved.Result[key])
		if string(x) != string(y) {
			t.Fatalf("changed original %s", key)
		}
	}
	report, requirements, err := mergedReview(saved.Result, state)
	if err != nil {
		t.Fatal(err)
	}
	if err = report.Validate(reviewSource, requirements, state.Answers); err != nil {
		t.Fatal(err)
	}
	if err = report.Validate(reviewSource, requirements); err == nil {
		t.Fatal("answer-backed claims accepted without saved answer")
	}
	selected := []string{state.Answers[0].Edit.ID}
	if _, err = RenderReviewedResume(reviewSource, saved.Result, &selected); err != nil {
		t.Fatal(err)
	}
	// An idempotent replay must not spend another request.
	if _, err = svc.answerReview(context.Background(), saved, reviewSource, req); err != nil {
		t.Fatal(err)
	}
	if client.calls.Load() != 1 {
		t.Fatal("idempotent replay called model twice")
	}
}

func TestReviewNoExperienceDoesNotCallModel(t *testing.T) {
	for _, kind := range []string{"none", "not_sure"} {
		t.Run(kind, func(t *testing.T) {
			svc, a, req, client := sessionFixture(t)
			req.Kind = kind
			req.Answer = ""
			state, err := svc.answerReview(context.Background(), a, reviewSource, req)
			if err != nil {
				t.Fatal(err)
			}
			if client.calls.Load() != 0 || len(state.Answers) != 1 || state.Answers[0].Edit != nil || state.Answers[0].Delta.Action != "no_change" {
				t.Fatal("invented an edit or called model")
			}
		})
	}
}

func TestUnsafeAnswerDeltaFailsWithoutChangingEarlierReport(t *testing.T) {
	for _, failure := range []string{"number", "anchor", "answer evidence", "overlap", "nochange passage"} {
		t.Run(failure, func(t *testing.T) {
			svc, a, req, client := sessionFixture(t)
			switch failure {
			case "number":
				client.delta.Passage = "Validated automated failover with 99 recovery drills."
			case "anchor":
				client.delta.InsertAfter = "Imaginary employer"
			case "answer evidence":
				client.delta.AnswerQuote = "Something the candidate never said"
			case "overlap":
				client.delta.InsertAfter = "Implemented automated failover"
			case "nochange passage":
				client.delta.Action = "no_change"
			}
			state, err := svc.answerReview(context.Background(), a, reviewSource, req)
			if err == nil || len(state.Answers) != 0 || state.Pending != nil || state.LastError == "" || state.Attempts != 1 {
				t.Fatalf("unsafe output accepted or lost reservation: %+v %v", state, err)
			}
			saved, _ := svc.Repo.GetByID(context.Background(), a.ID)
			stored, _ := reviewState(saved.Result)
			if stored.Pending != nil || stored.Attempts != 1 {
				t.Fatal("failed generation lock not persisted")
			}
		})
	}
}

func TestConcurrentAnswerReservationAllowsOneModelCall(t *testing.T) {
	svc, a, req, client := sessionFixture(t)
	client.entered = make(chan bool)
	client.release = make(chan bool)
	done := make(chan error, 1)
	go func() { _, err := svc.answerReview(context.Background(), a, reviewSource, req); done <- err }()
	<-client.entered
	other := req
	other.RequestID = uuid.NewString()
	if _, err := svc.answerReview(context.Background(), a, reviewSource, other); !errors.Is(err, ErrReviewConflict) {
		t.Fatalf("expected stale CAS to fail: %v", err)
	}
	close(client.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if client.calls.Load() != 1 {
		t.Fatal("concurrent requests both invoked model")
	}
}

func TestReviewValidationBeforeModelAndNoAnswerLeak(t *testing.T) {
	for _, change := range []string{"confirmation", "requirement", "choice", "question", "limit"} {
		t.Run(change, func(t *testing.T) {
			svc, a, req, client := sessionFixture(t)
			switch change {
			case "confirmation":
				req.Confirmed = false
			case "requirement":
				req.RequirementID = "unknown"
			case "choice":
				req.ExcludedIDs = []string{"unknown"}
			case "question":
				req.QuestionID = "unknown"
			case "limit":
				a.Result[reviewSessionKey] = ReviewSession{Attempts: 6}
			}
			if _, err := svc.answerReview(context.Background(), a, reviewSource, req); err == nil || client.calls.Load() != 0 {
				t.Fatal("invalid answer called model")
			}
		})
	}
	svc, a, req, _ := sessionFixture(t)
	req.Kind = "none"
	state, err := svc.answerReview(context.Background(), a, reviewSource, req)
	if err != nil {
		t.Fatal(err)
	}
	a.Result[reviewSessionKey] = state
	redacted, _ := RedactForFreeTier(a.Result)
	if _, exists := redacted[reviewSessionKey]; exists {
		t.Fatal("answer leaked through redaction")
	}
	h := &Handler{}
	public, _, _, err := h.applyPaywall(context.Background(), a.UserID, a.ID, a.Result)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := public[reviewSessionKey]; exists {
		t.Fatal("answer leaked into public/shared result")
	}
}

func TestPGReviewCASIsOwnerScopedAndDetectsStaleWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &PGRepo{DB: db}
	for _, n := range []int64{1, 0} {
		mock.ExpectExec("UPDATE analyses").WithArgs(sqlmock.AnyArg(), "id", "owner", 3).WillReturnResult(sqlmock.NewResult(0, n))
		err = repo.CompareAndSwapReview(context.Background(), "id", "owner", 3, ReviewSession{Version: 4})
		if n == 0 && !errors.Is(err, ErrReviewConflict) {
			t.Fatal("stale version accepted")
		}
		if n == 1 && err != nil {
			t.Fatal(err)
		}
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReviewSectionNameReusesOnlyUniqueWholeHeading(t *testing.T) {
	report := ReviewReport{Sections: []ReviewSection{{Heading: "Example Platforms"}}}
	if got := reviewSectionName(report, "Example Platforms | Backend Engineer | 2021-2024"); got != "Example Platforms" {
		t.Fatal(got)
	}
	if got := reviewSectionName(report, "Example PlatformsOther"); got != "Example PlatformsOther" {
		t.Fatal("matched partial name")
	}
	report.Sections = append(report.Sections, ReviewSection{Heading: "Example"})
	if got := reviewSectionName(report, "Example Platforms | Backend Engineer"); got != "Example Platforms | Backend Engineer" {
		t.Fatal("ambiguous heading was guessed")
	}
}
