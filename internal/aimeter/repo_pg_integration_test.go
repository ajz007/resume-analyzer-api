package aimeter

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func meterDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("AI_USAGE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AI_USAGE_TEST_DATABASE_URL to a disposable local database")
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
	schema := "meter_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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

func pendingAttempt() Attempt {
	return Attempt{ID: uuid.NewString(), OperationID: uuid.NewString(), Operation: "analysis", Stage: "primary", Number: 1, Provider: "openai", RequestedModel: "gpt-6.1-sol", StartedAt: time.Now().UTC(), Outcome: "pending", UsageState: "pending", UnknownReason: "awaiting_response"}
}
func pricedAttempt(a Attempt) Attempt {
	var e usageEnvelope
	_ = json.Unmarshal([]byte(usageBody), &e)
	u := e.Usage
	a.UsageState = "known"
	a.UnknownReason = ""
	a.Outcome = "response"
	a.HTTPStatus = 200
	a.ResponseModel = e.Model
	a.ServiceTier = e.ServiceTier
	a.InputTokens = u.Input
	a.OutputTokens = u.Output
	a.TotalTokens = u.Total
	a.CachedTokens = &u.Prompt.Cached
	a.CacheWriteTokens = &u.Prompt.Write
	a.ReasoningTokens = &u.Completion.Reasoning
	now := time.Now().UTC()
	a.FinishedAt = &now
	estimate(&a, false)
	return a
}
func TestPGMeterPendingSurvivesMissingFinishAndDoesNotShowZeroCost(t *testing.T) {
	db := meterDB(t)
	r := &PGRecorder{DB: db}
	ctx := context.Background()
	a := pendingAttempt()
	if err := r.Start(ctx, a); err != nil {
		t.Fatal(err)
	}
	// A new recorder sees the same durable pending row after simulated process loss.
	var state string
	var cost sql.NullInt64
	if err := db.QueryRow(`SELECT usage_state,estimate_nano_usd FROM ai_usage_attempts WHERE id=$1`, a.ID).Scan(&state, &cost); err != nil || state != "pending" || cost.Valid {
		t.Fatalf("state=%s cost=%+v err=%v", state, cost, err)
	}
	var incomplete int
	var full sql.NullFloat64
	if err := db.QueryRow(`SELECT pending_or_unknown_attempts,recorded_estimate_usd FROM ai_usage_daily`).Scan(&incomplete, &full); err != nil || incomplete != 1 || full.Valid {
		t.Fatalf("incomplete=%d full=%+v err=%v", incomplete, full, err)
	}
}
func TestPGMeterFinishIsIdempotentAndCanRecoverFailedInitialWrite(t *testing.T) {
	db := meterDB(t)
	r := &PGRecorder{DB: db}
	ctx := context.Background()
	a := pricedAttempt(pendingAttempt())
	// Finish can salvage a missing start row without any provider replay.
	if err := r.Finish(ctx, a); err != nil {
		t.Fatal(err)
	}
	original := *a.EstimateNanoUSD
	other := int64(1)
	a.EstimateNanoUSD = &other
	if err := r.Finish(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(ctx, pendingAttemptWithSameID(a)); err != nil {
		t.Fatal(err)
	}
	var cost int64
	var count int
	if err := db.QueryRow(`SELECT estimate_nano_usd FROM ai_usage_attempts WHERE id=$1`, a.ID).Scan(&cost); err != nil || cost != original {
		t.Fatalf("cost=%d err=%v", cost, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM ai_usage_attempts`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
func pendingAttemptWithSameID(a Attempt) Attempt {
	a.UsageState = "pending"
	a.EstimateNanoUSD = nil
	a.FinishedAt = nil
	return a
}
func TestPGMeterReportsUnknownsAlongsidePricedSubtotals(t *testing.T) {
	db := meterDB(t)
	r := &PGRecorder{DB: db}
	ctx := context.Background()
	analysisID, docID := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(`INSERT INTO documents(id,user_id,file_name,mime_type,size_bytes,storage_key) VALUES($1,'user','resume.pdf','application/pdf',100,'test')`, docID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO analyses(id,document_id,user_id,status) VALUES($1,$2,'user','completed')`, analysisID, docID); err != nil {
		t.Fatal(err)
	}
	pending := pendingAttempt()
	pending.AnalysisID = analysisID
	known := pricedAttempt(pending)
	if err := r.Start(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(ctx, known); err != nil {
		t.Fatal(err)
	}
	unknown := pendingAttempt()
	unknown.AnalysisID = analysisID
	if err := r.Start(ctx, unknown); err != nil {
		t.Fatal(err)
	}
	var attempts, unresolved int
	var subtotal float64
	var full sql.NullFloat64
	if err := db.QueryRow(`SELECT recorded_attempts,pending_or_unknown_attempts,observed_priced_subtotal_usd,recorded_estimate_usd FROM ai_analysis_costs WHERE analysis_id=$1`, analysisID).Scan(&attempts, &unresolved, &subtotal, &full); err != nil || attempts != 2 || unresolved != 1 || subtotal != 0.00429 || full.Valid {
		t.Fatalf("attempts=%d unknown=%d subtotal=%g full=%+v err=%v", attempts, unresolved, subtotal, full, err)
	}
}
func TestPGMeterMissingAttributionAndLegacyReportsRemainUnmeasured(t *testing.T) {
	db := meterDB(t)
	id, doc := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(`INSERT INTO documents(id,user_id,file_name,mime_type,size_bytes,storage_key) VALUES($1,'user','resume.pdf','application/pdf',100,'test')`, doc); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO analyses(id,document_id,user_id,status) VALUES($1,$2,'user','completed')`, id, doc); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var full sql.NullFloat64
	if err := db.QueryRow(`SELECT recorded_attempts,recorded_estimate_usd FROM ai_analysis_costs WHERE analysis_id=$1`, id).Scan(&attempts, &full); err != nil || attempts != 0 || full.Valid {
		t.Fatalf("attempts=%d full=%+v err=%v", attempts, full, err)
	}
}

func TestPGMeterOperatorReportsExecuteReadOnly(t *testing.T) {
	db := meterDB(t)
	body, err := os.ReadFile("../../docs/launch-readiness/ai-usage-reports.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
}
