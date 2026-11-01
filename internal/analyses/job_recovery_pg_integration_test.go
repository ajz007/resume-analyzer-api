package analyses_test

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"net/url"
	"os"
	"path/filepath"
	"resume-backend/internal/analyses"
	"resume-backend/internal/queue"
	"resume-backend/internal/usage"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func recoveryDBWithSeed(t *testing.T, beforeDispatch func(*sql.DB)) *sql.DB {
	t.Helper()
	dsn := os.Getenv("JOBS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set JOBS_TEST_DATABASE_URL to a disposable local database")
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
	schema := "jobs_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
		if filepath.Base(file) == "0028_analysis_dispatch.sql" && beforeDispatch != nil {
			beforeDispatch(db)
		}
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

type recoveryQueue struct {
	calls            atomic.Int32
	err              error
	lastMu           sync.Mutex
	last             queue.Message
	entered, release chan struct{}
}

func (q *recoveryQueue) Send(ctx context.Context, msg queue.Message) error {
	q.calls.Add(1)
	q.lastMu.Lock()
	q.last = msg
	q.lastMu.Unlock()
	if q.entered != nil {
		select {
		case q.entered <- struct{}{}:
		default:
		}
		select {
		case <-q.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return q.err
}
func recoveryService(t *testing.T) (*analyses.Service, *analyses.PGRepo, *recoveryQueue, *sql.DB, string) {
	t.Helper()
	db := recoveryDB(t)
	pgUsage := usage.NewPGStore(db)
	repo := &analyses.PGRepo{DB: db, Reservations: pgUsage}
	q := &recoveryQueue{}
	doc := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO documents(id,user_id,file_name,mime_type,size_bytes,storage_key) VALUES($1,'user','resume.pdf','application/pdf',100,'test')`, doc); err != nil {
		t.Fatal(err)
	}
	return &analyses.Service{Repo: repo, Usage: usage.NewPostgresService(pgUsage), JobQueue: q}, repo, q, db, doc
}
func savedReview(t *testing.T, repo *analyses.PGRepo, doc string) analyses.Analysis {
	t.Helper()
	a := analyses.Analysis{ID: uuid.NewString(), DocumentID: doc, UserID: "user", Status: analyses.StatusQueued, Mode: analyses.ModeJobMatch, CreatedAt: time.Now().UTC()}
	if err := repo.Create(analyses.WithRequestID(context.Background(), "request-test"), a); err != nil {
		t.Fatal(err)
	}
	return a
}
func assertUsage(t *testing.T, svc *analyses.Service, want int) {
	t.Helper()
	u, err := svc.Usage.Get(context.Background(), "user")
	if err != nil || u.Used != want {
		t.Fatalf("%+v err=%v want=%d", u, err, want)
	}
}
func TestPGJobAdmissionRollsBackReviewAndAllowanceOnOutboxFailure(t *testing.T) {
	svc, repo, _, db, doc := recoveryService(t)
	if _, err := db.Exec(`DROP TABLE analysis_dispatch`); err != nil {
		t.Fatal(err)
	}
	a := analyses.Analysis{ID: uuid.NewString(), DocumentID: doc, UserID: "user", Status: analyses.StatusQueued, CreatedAt: time.Now()}
	if err := repo.Create(context.Background(), a); err == nil {
		t.Fatal("missing dispatch table did not roll back admission")
	}
	for _, table := range []string{"analyses", "analysis_usage_reservations", "usage"} {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	_ = svc
}
func TestPGJobCrashBeforeDispatchRecoversOnOwnerRead(t *testing.T) {
	svc, repo, q, db, doc := recoveryService(t)
	a := savedReview(t, repo, doc)
	assertUsage(t, svc, 1)
	// No queue send occurred: admission committed immediately before a simulated crash.
	if _, err := svc.GetOwnedAndRecover(context.Background(), "other", a.ID); !errors.Is(err, analyses.ErrNotFound) {
		t.Fatal(err)
	}
	if q.calls.Load() != 0 {
		t.Fatal("foreign owner triggered recovery")
	}
	recovered, err := svc.GetOwnedAndRecover(context.Background(), "user", a.ID)
	if err != nil || recovered.Status != analyses.StatusQueued || q.calls.Load() != 1 {
		t.Fatalf("%+v err=%v sends=%d", recovered, err, q.calls.Load())
	}
	if q.last.RequestID != "request-test" || q.last.AnalysisID != a.ID {
		t.Fatalf("message %+v", q.last)
	}
	var published bool
	if err := db.QueryRow(`SELECT published_at IS NOT NULL FROM analysis_dispatch WHERE analysis_id=$1`, a.ID).Scan(&published); err != nil || !published {
		t.Fatalf("published=%v err=%v", published, err)
	}
	assertUsage(t, svc, 1)
}
func TestPGJobConcurrentRecoveryUsesOneDispatchLease(t *testing.T) {
	svc, repo, q, db, doc := recoveryService(t)
	db.SetMaxOpenConns(2)
	a := savedReview(t, repo, doc)
	q.entered = make(chan struct{}, 1)
	q.release = make(chan struct{})
	first := make(chan error, 1)
	go func() { _, err := svc.GetOwnedAndRecover(context.Background(), "user", a.ID); first <- err }()
	select {
	case <-q.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("no dispatch")
	}
	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.GetOwnedAndRecover(context.Background(), "user", a.ID); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	close(q.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if q.calls.Load() != 1 {
		t.Fatalf("sends=%d", q.calls.Load())
	}
}
func TestPGJobQueueResponseLossKeepsDurableAcceptedReview(t *testing.T) {
	svc, _, q, db, doc := recoveryService(t)
	q.err = errors.New("response lost after SQS accepted message")
	a, err := svc.Create(context.Background(), doc, "user", "job", "")
	if err != nil || a.Status != analyses.StatusQueued {
		t.Fatalf("%+v %v", a, err)
	}
	assertUsage(t, svc, 1)
	if _, err := db.Exec(`UPDATE analysis_dispatch SET next_attempt_at=now()-interval '1 second' WHERE analysis_id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	q.err = nil
	if checked, err := svc.RecoverBatch(context.Background()); err != nil || checked != 1 {
		t.Fatalf("checked=%d err=%v", checked, err)
	}
	if q.calls.Load() != 2 {
		t.Fatalf("sends=%d", q.calls.Load())
	}
	assertUsage(t, svc, 1)
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := svc.Repo.ClaimProcessing(context.Background(), a.ID, time.Now())
			if err != nil {
				t.Error(err)
			}
			if ok {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("processing winners=%d", winners.Load())
	}
	if _, err := svc.GetOwnedAndRecover(context.Background(), "user", a.ID); err != nil {
		t.Fatal(err)
	}
	if q.calls.Load() != 2 {
		t.Fatal("processing review was dispatched again")
	}
}
func TestPGJobInterruptedProcessingFailsWithoutReclaimAndReturnsAllowance(t *testing.T) {
	svc, repo, q, db, doc := recoveryService(t)
	a := savedReview(t, repo, doc)
	if ok, err := repo.ClaimProcessing(context.Background(), a.ID, time.Now().Add(-21*time.Minute)); err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	recovered, err := svc.GetOwnedAndRecover(context.Background(), "user", a.ID)
	if err != nil || recovered.Status != analyses.StatusFailed || recovered.ErrorCode != "ANALYSIS_INTERRUPTED" || !recovered.ErrorRetryable {
		t.Fatalf("%+v %v", recovered, err)
	}
	assertUsage(t, svc, 0)
	if q.calls.Load() != 0 {
		t.Fatal("interrupted worker was automatically retried")
	}
	if err := svc.ProcessAnalysis(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.ClaimProcessing(context.Background(), a.ID, time.Now()); err != nil || ok {
		t.Fatalf("reclaimed failed attempt: %v %v", ok, err)
	}
	now := time.Now()
	if err := repo.UpdateAnalysisResult(context.Background(), a.ID, map[string]any{"late": true}, &now); !errors.Is(err, analyses.ErrAnalysisFinalized) {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM analysis_usage_reservations WHERE state='released'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
func TestPGJobQueuedDeadlineFailsAndRestoresAllowance(t *testing.T) {
	svc, repo, q, db, doc := recoveryService(t)
	a := savedReview(t, repo, doc)
	if _, err := db.Exec(`UPDATE analyses SET created_at=now()-interval '61 minutes' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecoverBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(context.Background(), a.ID)
	if err != nil || got.Status != analyses.StatusFailed {
		t.Fatalf("%+v %v", got, err)
	}
	if q.calls.Load() != 0 {
		t.Fatal("expired review was dispatched")
	}
	assertUsage(t, svc, 0)
}
func TestPGJobDispatchLeaseTokenFencesLateAcknowledgement(t *testing.T) {
	_, repo, _, db, doc := recoveryService(t)
	a := savedReview(t, repo, doc)
	ctx := context.Background()
	old, _, ok, err := repo.ClaimDispatch(ctx, a.ID)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if _, err := db.Exec(`UPDATE analysis_dispatch SET claim_until=now()-interval '1 second' WHERE analysis_id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	current, _, ok, err := repo.ClaimDispatch(ctx, a.ID)
	if err != nil || !ok || current == old {
		t.Fatalf("%v %v", ok, err)
	}
	if err := repo.FinishDispatch(ctx, a.ID, old, true); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow(`SELECT claim_token FROM analysis_dispatch WHERE analysis_id=$1`, a.ID).Scan(&stored); err != nil || stored != current {
		t.Fatalf("token=%s err=%v", stored, err)
	}
	if err := repo.FinishDispatch(ctx, a.ID, current, true); err != nil {
		t.Fatal(err)
	}
}
func TestPGJobAdmissionEnforcesQuotaAcrossConcurrentNewReviews(t *testing.T) {
	_, repo, _, db, doc := recoveryService(t)
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := analyses.Analysis{ID: uuid.NewString(), DocumentID: doc, UserID: "user", Status: analyses.StatusQueued, CreatedAt: time.Now()}
			err := repo.Create(context.Background(), a)
			if err == nil {
				admitted.Add(1)
			} else if !errors.Is(err, usage.ErrLimitReached) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != int32(usage.FreeUserMonthlyAnalysisLimit) {
		t.Fatalf("admitted=%d", admitted.Load())
	}
	for _, table := range []string{"analyses", "analysis_usage_reservations", "analysis_dispatch"} {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != usage.FreeUserMonthlyAnalysisLimit {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}

func recoveryDB(t *testing.T) *sql.DB { return recoveryDBWithSeed(t, nil) }
func TestPGJobMigrationAdoptsEstablishedQueuedReservation(t *testing.T) {
	var id string
	db := recoveryDBWithSeed(t, func(db *sql.DB) {
		doc := uuid.NewString()
		if _, err := db.Exec(`INSERT INTO documents(id,user_id,file_name,mime_type,size_bytes,storage_key) VALUES($1,'user','resume.pdf','application/pdf',100,'test')`, doc); err != nil {
			t.Fatal(err)
		}
		a := savedReview(t, &analyses.PGRepo{DB: db}, doc)
		id = a.ID
		if _, err := usage.NewPGStore(db).ReserveAnalysis(context.Background(), "user", id); err != nil {
			t.Fatal(err)
		}
	})
	pgUsage := usage.NewPGStore(db)
	q := &recoveryQueue{}
	svc := &analyses.Service{Repo: &analyses.PGRepo{DB: db, Reservations: pgUsage}, Usage: usage.NewPostgresService(pgUsage), JobQueue: q}
	if checked, err := svc.RecoverBatch(context.Background()); err != nil || checked != 1 || q.calls.Load() != 1 {
		t.Fatalf("checked=%d sends=%d err=%v", checked, q.calls.Load(), err)
	}
	assertUsage(t, svc, 1)
}
func TestPGJobLegacyUntrackedAllowanceIsNotGuessed(t *testing.T) {
	svc, _, q, db, doc := recoveryService(t)
	legacy := &analyses.PGRepo{DB: db}
	a := analyses.Analysis{ID: uuid.NewString(), DocumentID: doc, UserID: "user", Status: analyses.StatusQueued, CreatedAt: time.Now().Add(-61 * time.Minute)}
	if err := legacy.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if ok, err := svc.Repo.ClaimProcessing(context.Background(), a.ID, time.Now()); err != nil || ok {
		t.Fatalf("unadmitted legacy execution: %v %v", ok, err)
	}
	if _, err := svc.Usage.Consume(context.Background(), "user", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecoverBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertUsage(t, svc, 1)
	if q.calls.Load() != 0 {
		t.Fatal("untracked legacy work was automatically dispatched")
	}
}
func TestPGJobDeletedReviewSettlesAndCannotRun(t *testing.T) {
	for _, status := range []string{analyses.StatusQueued, analyses.StatusFailed} {
		t.Run(status, func(t *testing.T) {
			svc, repo, q, db, doc := recoveryService(t)
			a := savedReview(t, repo, doc)
			if _, err := db.Exec(`UPDATE analyses SET deleted_at=now(),status=$2 WHERE id=$1`, a.ID, status); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.RecoverBatch(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertUsage(t, svc, 0)
			if err := svc.ProcessAnalysis(context.Background(), a.ID); err != nil {
				t.Fatal(err)
			}
			if q.calls.Load() != 0 {
				t.Fatal("deleted review was dispatched")
			}
		})
	}
}

func TestPGJobRepeatedPollsHaveBoundedDispatchAttempts(t *testing.T) {
	svc, repo, q, db, doc := recoveryService(t)
	a := savedReview(t, repo, doc)
	q.err = errors.New("SQS unavailable")
	for i := 0; i < 10; i++ {
		if _, err := db.Exec(`UPDATE analysis_dispatch SET next_attempt_at=now()-interval '1 second' WHERE analysis_id=$1`, a.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.GetOwnedAndRecover(context.Background(), "user", a.ID); err != nil {
			t.Fatal(err)
		}
	}
	if q.calls.Load() != 8 {
		t.Fatalf("sends=%d", q.calls.Load())
	}
	assertUsage(t, svc, 1)
	if _, err := db.Exec(`UPDATE analyses SET created_at=now()-interval '61 minutes' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := svc.GetOwnedAndRecover(context.Background(), "user", a.ID)
	if err != nil || recovered.Status != analyses.StatusFailed {
		t.Fatalf("%+v %v", recovered, err)
	}
	assertUsage(t, svc, 0)
	if q.calls.Load() != 8 {
		t.Fatal("deadline restarted dispatch")
	}
}

func TestPGJobSweepSettlesSavedTerminalOutcomeWithoutUserRead(t *testing.T) {
	svc, repo, q, _, doc := recoveryService(t)
	a := savedReview(t, repo, doc)
	if err := repo.UpdateStatus(context.Background(), a.ID, analyses.StatusFailed, nil); err != nil {
		t.Fatal(err)
	}
	if checked, err := svc.RecoverBatch(context.Background()); err != nil || checked != 1 {
		t.Fatalf("checked=%d err=%v", checked, err)
	}
	assertUsage(t, svc, 0)
	if q.calls.Load() != 0 {
		t.Fatal("terminal review was dispatched")
	}
}

func TestPGJobProcessingClaimCannotBeReusedAfterStatusReset(t *testing.T) {
	_, repo, _, _, doc := recoveryService(t)
	a := savedReview(t, repo, doc)
	ctx := context.Background()
	if ok, err := repo.ClaimProcessing(ctx, a.ID, time.Now()); err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if err := repo.UpdateStatus(ctx, a.ID, analyses.StatusQueued, nil); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.ClaimProcessing(ctx, a.ID, time.Now()); err != nil || ok {
		t.Fatalf("claim reused: %v %v", ok, err)
	}
}
