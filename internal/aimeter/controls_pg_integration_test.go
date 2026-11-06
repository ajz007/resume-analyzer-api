package aimeter

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func budgetDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("AI_BUDGET_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AI_BUDGET_TEST_DATABASE_URL to a disposable local database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" {
		t.Fatal("local database required")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "budget_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); _ = admin.Close() })
	q := parsed.Query()
	q.Set("search_path", schema)
	parsed.RawQuery = q.Encode()
	db, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(25)
	t.Cleanup(func() { _ = db.Close() })
	files, err := filepath.Glob("../shared/storage/db/migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(strings.Split(string(body), "-- +goose Down")[0]); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
	return db
}

const budgetRequest = `{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"Private resume"}]}`

func budgetAttempt() Attempt { a := pendingAttempt(); a.Operation = "analysis"; return a }
func budgetDay(t *testing.T, db *sql.DB) string {
	t.Helper()
	var day string
	if err := db.QueryRow(`SELECT to_char(now() AT TIME ZONE 'UTC','YYYY-MM-DD')`).Scan(&day); err != nil {
		t.Fatal(err)
	}
	return day
}
func TestPGBudgetParallelCallsCannotOversubscribe(t *testing.T) {
	db := budgetDB(t)
	r := &PGRecorder{DB: db}
	policy := spendPolicy{RequestBytes: 262144, OutputTokens: 32768}
	_, reserve, err := prepareRequest([]byte(budgetRequest), policy, "gpt-6.1-sol")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE ai_spend_policy SET daily_limit_nano_usd=$1`, reserve*3); err != nil {
		t.Fatal(err)
	}
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := budgetAttempt()
			_, err := r.Admit(context.Background(), &a, []byte(budgetRequest))
			if err == nil {
				admitted.Add(1)
			} else if c, ok := Control(err); !ok || c.Code != CodeDailyBudget {
				t.Errorf("%v", err)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 3 {
		t.Fatalf("admitted=%d", admitted.Load())
	}
	var booked int64
	if err := db.QueryRow(`SELECT booked_nano_usd FROM ai_spend_days`).Scan(&booked); err != nil || booked != reserve*3 {
		t.Fatalf("booked=%d err=%v", booked, err)
	}
}
func TestPGBudgetKnownUsageSettlesOnceAndUnknownRetainsHold(t *testing.T) {
	db := budgetDB(t)
	r := &PGRecorder{DB: db}
	ctx := context.Background()
	a := budgetAttempt()
	if _, err := r.Admit(ctx, &a, []byte(budgetRequest)); err != nil {
		t.Fatal(err)
	}
	var reserved int64
	if err := db.QueryRow(`SELECT booked_nano_usd FROM ai_spend_days`).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	known := pricedAttempt(a)
	if err := r.Finish(ctx, known); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(ctx, known); err != nil {
		t.Fatal(err)
	}
	var booked int64
	if err := db.QueryRow(`SELECT booked_nano_usd FROM ai_spend_days`).Scan(&booked); err != nil || booked != 2*(*known.EstimateNanoUSD) {
		t.Fatalf("booked=%d err=%v", booked, err)
	}
	unknown := budgetAttempt()
	if _, err := r.Admit(ctx, &unknown, []byte(budgetRequest)); err != nil {
		t.Fatal(err)
	}
	unknown.UsageState = "unknown"
	unknown.UnknownReason = "transport_error"
	unknown.Outcome = "transport_error"
	if err := r.Finish(ctx, unknown); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT booked_nano_usd FROM ai_spend_days`).Scan(&booked); err != nil || booked != reserved+2*(*known.EstimateNanoUSD) {
		t.Fatalf("unknown hold released: %d %v", booked, err)
	}
}
func TestPGBudgetSharedAttemptLimitAcrossInstances(t *testing.T) {
	db := budgetDB(t)
	r := &PGRecorder{DB: db}
	a := budgetAttempt()
	op := a.OperationID
	for i := 0; i < 4; i++ {
		a = budgetAttempt()
		a.OperationID = op
		a.Number = int64(i + 1)
		if _, err := r.Admit(context.Background(), &a, []byte(budgetRequest)); err != nil {
			t.Fatal(err)
		}
	}
	another := &PGRecorder{DB: db}
	a = budgetAttempt()
	a.OperationID = op
	a.Number = 5
	if _, err := another.Admit(context.Background(), &a, []byte(budgetRequest)); err == nil {
		t.Fatal("attempt limit bypassed")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM ai_spend_holds`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
func TestPGBudgetStopSwitchAndStorageFailureBlockBeforeSend(t *testing.T) {
	db := budgetDB(t)
	r := &PGRecorder{DB: db}
	a := budgetAttempt()
	if _, err := db.Exec(`UPDATE ai_spend_policy SET calls_enabled=false`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Admit(context.Background(), &a, []byte(budgetRequest)); err == nil {
		t.Fatal("stop switch ignored")
	}
	if _, err := db.Exec(`DROP TABLE ai_spend_policy`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Admit(context.Background(), &a, []byte(budgetRequest)); err == nil {
		t.Fatal("missing controls allowed a request")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM ai_usage_attempts`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
func TestPGBudgetLegacyUnknownIsNotAssumedFree(t *testing.T) {
	db := budgetDB(t)
	r := &PGRecorder{DB: db}
	ctx := context.Background()
	legacy := pendingAttempt()
	if err := r.Start(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	next := budgetAttempt()
	if _, err := r.Admit(ctx, &next, []byte(budgetRequest)); err == nil {
		t.Fatal("legacy unknown spend ignored")
	}
	if err := r.Finish(ctx, pricedAttempt(legacy)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Admit(ctx, &next, []byte(budgetRequest)); err != nil {
		t.Fatal(err)
	}
}
func TestPGBudgetAdmissionRollbackLeavesNoPartialHold(t *testing.T) {
	db := budgetDB(t)
	r := &PGRecorder{DB: db}
	if _, err := db.Exec(`DROP TABLE ai_spend_holds`); err != nil {
		t.Fatal(err)
	}
	a := budgetAttempt()
	if _, err := r.Admit(context.Background(), &a, []byte(budgetRequest)); err == nil {
		t.Fatal("invalid storage admitted work")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM ai_usage_attempts`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial usage count=%d %v", count, err)
	}
}
func TestBudgetPreparationCapsOutputAndRejectsUnsafeInputs(t *testing.T) {
	p := spendPolicy{RequestBytes: 262144, OutputTokens: 32768}
	raw, reserve, err := prepareRequest([]byte(budgetRequest), p, "gpt-6.1-sol")
	if err != nil || reserve <= 0 {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["max_completion_tokens"] != float64(32768) || fields["service_tier"] != "default" {
		t.Fatalf("%v", fields)
	}
	small := strings.TrimSuffix(budgetRequest, "}") + `,"max_completion_tokens":8192}`
	raw, _, err = prepareRequest([]byte(small), p, "gpt-6.1-sol")
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &fields)
	if fields["max_completion_tokens"] != float64(8192) {
		t.Fatal("raised existing lower cap")
	}
	for _, body := range []string{strings.Repeat("x", 262145), `{"model":"other","messages":[]}`, strings.TrimSuffix(budgetRequest, "}") + `,"n":2}`, strings.TrimSuffix(budgetRequest, "}") + `,"service_tier":"priority"}`, strings.TrimSuffix(budgetRequest, "}") + `,"tools":[]}`} {
		if _, _, err := prepareRequest([]byte(body), p, "gpt-6.1-sol"); err == nil {
			t.Fatal("unsafe request admitted")
		}
	}
}

func TestPGBudgetLateSettlementStaysInOriginalDay(t *testing.T) {
	db := budgetDB(t)
	r := &PGRecorder{DB: db}
	ctx := context.Background()
	old := budgetAttempt()
	if _, err := r.Admit(ctx, &old, []byte(budgetRequest)); err != nil {
		t.Fatal(err)
	}
	var reserve int64
	if err := db.QueryRow(`SELECT booked_nano_usd FROM ai_spend_days`).Scan(&reserve); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ai_spend_days(day_utc,booked_nano_usd) SELECT day_utc-1,booked_nano_usd FROM ai_spend_days`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE ai_spend_holds SET day_utc=day_utc-1 WHERE attempt_id=$1`, old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE ai_spend_days SET booked_nano_usd=0 WHERE day_utc=$1`, budgetDay(t, db)); err != nil {
		t.Fatal(err)
	}
	fresh := budgetAttempt()
	if _, err := r.Admit(ctx, &fresh, []byte(budgetRequest)); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(ctx, pricedAttempt(old)); err != nil {
		t.Fatal(err)
	}
	var current int64
	if err := db.QueryRow(`SELECT booked_nano_usd FROM ai_spend_days WHERE day_utc=$1`, budgetDay(t, db)).Scan(&current); err != nil || current != reserve {
		t.Fatalf("current=%d err=%v", current, err)
	}
}
func TestPGBudgetProtectedTransportRejectsBeforeProviderAndKeepsLowerAnswerCap(t *testing.T) {
	db := budgetDB(t)
	r := &PGRecorder{DB: db}
	calls := 0
	tr := NewTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["max_completion_tokens"] != float64(8192) || body["service_tier"] != "default" {
			t.Errorf("%v", body)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(usageBody))}, nil
	}), r, "gpt-6.1-sol")
	ctx := WithOperation(context.Background(), "review_answer", "", "")
	request := strings.TrimSuffix(budgetRequest, "}") + `,"max_completion_tokens":8192}`
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://provider.example", strings.NewReader(request))
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if _, err := db.Exec(`UPDATE ai_spend_policy SET calls_enabled=false`); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequestWithContext(WithOperation(context.Background(), "analysis", "", ""), "POST", "https://provider.example", strings.NewReader(request))
	if _, err := tr.RoundTrip(req); err == nil {
		t.Fatal("provider request allowed while paused")
	} else if control, ok := Control(err); !ok || !control.NoProviderSpend {
		t.Fatalf("%v", err)
	}
	if calls != 1 {
		t.Fatalf("provider calls=%d", calls)
	}
}

func TestPGBudgetNeverReusesAnAlreadyRecordedAttempt(t *testing.T) {
	db := budgetDB(t)
	r := &PGRecorder{DB: db}
	ctx := context.Background()
	a := pricedAttempt(budgetAttempt())
	if err := r.Finish(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Admit(ctx, &a, []byte(budgetRequest)); err == nil {
		t.Fatal("reused an existing provider attempt")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM ai_spend_holds`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("holds=%d %v", count, err)
	}
}
func TestPGBudgetUnexpectedOverrunStopsFurtherCalls(t *testing.T) {
	db := budgetDB(t)
	r := &PGRecorder{DB: db}
	ctx := context.Background()
	a := budgetAttempt()
	if _, err := r.Admit(ctx, &a, []byte(budgetRequest)); err != nil {
		t.Fatal(err)
	}
	var reserve int64
	if err := db.QueryRow(`SELECT reserved_nano_usd FROM ai_spend_holds`).Scan(&reserve); err != nil {
		t.Fatal(err)
	}
	actual := pricedAttempt(a)
	actual.EstimateNanoUSD = &reserve // 2x settlement charge exceeds this hold.
	if err := r.Finish(ctx, actual); err != nil {
		t.Fatal(err)
	}
	next := budgetAttempt()
	if _, err := r.Admit(ctx, &next, []byte(budgetRequest)); err == nil {
		t.Fatal("overrun did not stop further calls")
	}
}
