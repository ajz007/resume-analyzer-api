package usage_test

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

func usageDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("USAGE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set USAGE_TEST_DATABASE_URL to a disposable local database")
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
	schema := "usage_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
func addAnalysis(t *testing.T, db *sql.DB, user string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO analyses(id,document_id,user_id,status) VALUES($1,$2,$3,'queued')`, id, addDocument(t, db, user), user); err != nil {
		t.Fatal(err)
	}
	return id
}
func mustUsage(t *testing.T, svc *usage.Service, user string, want int) {
	t.Helper()
	u, err := svc.Get(context.Background(), user)
	if err != nil || u.Used != want {
		t.Fatalf("usage=%+v err=%v want=%d", u, err, want)
	}
}
func TestPGFailedReviewRecoversAfterSettlementWasInterrupted(t *testing.T) {
	db := usageDB(t)
	svc := usage.NewPostgresService(usage.NewPGStore(db))
	ctx := context.Background()
	id := addAnalysis(t, db, "guest:one")
	if _, err := svc.ReserveAnalysis(ctx, "guest:one", id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE analyses SET status='failed' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	// Simulate a worker crash after persisting failure but before quota settlement.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_ = svc.SettleAnalysis(cancelled, id, "failed")
	mustUsage(t, svc, "guest:one", 0)
	mustUsage(t, svc, "guest:one", 0)
	var state string
	if err := db.QueryRow(`SELECT state FROM analysis_usage_reservations WHERE analysis_id=$1`, id).Scan(&state); err != nil || state != "released" {
		t.Fatalf("%s %v", state, err)
	}
}
func TestPGConcurrentFirstUseCannotExceedQuota(t *testing.T) {
	db := usageDB(t)
	svc := usage.NewPostgresService(usage.NewPGStore(db))
	ctx := context.Background()
	ids := make([]string, 20)
	for i := range ids {
		ids[i] = addAnalysis(t, db, "guest:race")
	}
	var wg sync.WaitGroup
	var success atomic.Int32
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := svc.ReserveAnalysis(ctx, "guest:race", id)
			if err == nil {
				success.Add(1)
			} else if !errors.Is(err, usage.ErrLimitReached) {
				t.Errorf("reserve: %v", err)
			}
		}(id)
	}
	wg.Wait()
	if success.Load() != int32(usage.GuestMonthlyAnalysisLimit) {
		t.Fatalf("accepted %d", success.Load())
	}
	mustUsage(t, svc, "guest:race", usage.GuestMonthlyAnalysisLimit)
}
func TestPGConcurrentReservationAndSettlementOfSameReview(t *testing.T) {
	db := usageDB(t)
	svc := usage.NewPostgresService(usage.NewPGStore(db))
	ctx := context.Background()
	id := addAnalysis(t, db, "user")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.ReserveAnalysis(ctx, "user", id); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	mustUsage(t, svc, "user", 1)
	if _, err := db.Exec(`UPDATE analyses SET status='failed' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.SettleAnalysis(ctx, id, "failed"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	mustUsage(t, svc, "user", 0)
}
func TestPGOldFailureCannotReduceNewWindow(t *testing.T) {
	for _, reset := range []string{"manual", "expired"} {
		t.Run(reset, func(t *testing.T) {
			db := usageDB(t)
			svc := usage.NewPostgresService(usage.NewPGStore(db))
			ctx := context.Background()
			old := addAnalysis(t, db, "user")
			_, err := svc.ReserveAnalysis(ctx, "user", old)
			if err != nil {
				t.Fatal(err)
			}
			if reset == "manual" {
				if _, err := svc.Reset(ctx, "user"); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := db.Exec(`UPDATE usage SET resets_at=now()-interval '1 second' WHERE user_id='user'`); err != nil {
					t.Fatal(err)
				}
				mustUsage(t, svc, "user", 0)
			}
			fresh := addAnalysis(t, db, "user")
			if _, err := svc.ReserveAnalysis(ctx, "user", fresh); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE analyses SET status='failed' WHERE id=$1`, old); err != nil {
				t.Fatal(err)
			}
			mustUsage(t, svc, "user", 1)
		})
	}
}
func TestPGTerminalStatesCannotBeReversedForRefunds(t *testing.T) {
	for _, first := range []string{"completed", "failed"} {
		t.Run(first, func(t *testing.T) {
			db := usageDB(t)
			svc := usage.NewPostgresService(usage.NewPGStore(db))
			repo := &analyses.PGRepo{DB: db}
			ctx := context.Background()
			id := addAnalysis(t, db, "user")
			if _, err := svc.ReserveAnalysis(ctx, "user", id); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			if err := repo.UpdateStatusResultAndError(ctx, id, first, nil, nil, nil, nil, nil, &now); err != nil {
				t.Fatal(err)
			}
			second := "failed"
			want := 1
			if first == "failed" {
				second = "completed"
				want = 0
			}
			if err := repo.UpdateStatus(ctx, id, second, nil); !errors.Is(err, analyses.ErrAnalysisFinalized) {
				t.Fatalf("status reversed: %v", err)
			}
			if err := repo.UpdateAnalysisResult(ctx, id, map[string]any{"done": true}, &now); !errors.Is(err, analyses.ErrAnalysisFinalized) {
				t.Fatalf("result overwrote final state: %v", err)
			}
			mustUsage(t, svc, "user", want)
		})
	}
}
func TestPGSettlementUsesPersistedOutcomeAndOriginalBillingOwner(t *testing.T) {
	db := usageDB(t)
	svc := usage.NewPostgresService(usage.NewPGStore(db))
	ctx := context.Background()
	id := addAnalysis(t, db, "guest:original")
	if _, err := svc.ReserveAnalysis(ctx, "guest:original", id); err != nil {
		t.Fatal(err)
	}
	// Caller saying 'failed' is insufficient while the review is still queued.
	if err := svc.SettleAnalysis(ctx, id, "failed"); err != nil {
		t.Fatal(err)
	}
	mustUsage(t, svc, "guest:original", 1)
	other := addAnalysis(t, db, "authenticated")
	if _, err := svc.ReserveAnalysis(ctx, "authenticated", other); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE analyses SET user_id='authenticated',status='failed' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := svc.SettleAnalysis(ctx, id, "failed"); err != nil {
		t.Fatal(err)
	}
	mustUsage(t, svc, "guest:original", 0)
	mustUsage(t, svc, "authenticated", 1)
}

type creditQueue struct {
	calls int
	err   error
}

func (q *creditQueue) Send(context.Context, queue.Message) error { q.calls++; return q.err }
func TestPGServiceLifecycleReleasesFailuresAndReusesCompletedReview(t *testing.T) {
	db := usageDB(t)
	ctx := context.Background()
	quota := usage.NewPostgresService(usage.NewPGStore(db))
	repo := &analyses.PGRepo{DB: db}
	q := &creditQueue{}
	doc := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO documents(id,user_id,file_name,mime_type,size_bytes,storage_key) VALUES($1,'user','resume.pdf','application/pdf',100,'test')`, doc); err != nil {
		t.Fatal(err)
	}
	svc := &analyses.Service{Repo: repo, Usage: quota, JobQueue: q}
	first, created, err := svc.StartOrReuse(ctx, doc, "user", "job", "", analyses.ModeJobMatch, false)
	if err != nil || !created {
		t.Fatalf("%+v %v", first, err)
	}
	mustUsage(t, quota, "user", 1)
	reused, created, err := svc.StartOrReuse(ctx, doc, "user", "job", "", analyses.ModeJobMatch, false)
	if err != nil || created || reused.ID != first.ID || q.calls != 1 {
		t.Fatalf("reuse: %+v %v", reused, err)
	}
	// Missing worker dependencies are a technical failure, with no provider call.
	if err := svc.ProcessAnalysis(ctx, first.ID); err == nil {
		t.Fatal("expected technical failure")
	}
	mustUsage(t, quota, "user", 0)
	next, created, err := svc.StartOrReuse(ctx, doc, "user", "job", "", analyses.ModeJobMatch, true)
	if err != nil || !created || next.ID == first.ID {
		t.Fatalf("retry: %+v %v", next, err)
	}
	mustUsage(t, quota, "user", 1)
	now := time.Now()
	if err := repo.UpdateAnalysisResult(ctx, next.ID, map[string]any{"done": true}, &now); err != nil {
		t.Fatal(err)
	}
	mustUsage(t, quota, "user", 1)
	if _, err := quota.Consume(ctx, "user", usage.FreeUserMonthlyAnalysisLimit-1); err != nil {
		t.Fatal(err)
	}
	if _, created, err := svc.StartOrReuse(ctx, doc, "user", "job", "", analyses.ModeJobMatch, false); err != nil || created || q.calls != 2 {
		t.Fatalf("completed reuse: %v", err)
	}
}
func TestPGServiceQueueFailureReturnsAllowance(t *testing.T) {
	db := usageDB(t)
	ctx := context.Background()
	quota := usage.NewPostgresService(usage.NewPGStore(db))
	repo := &analyses.PGRepo{DB: db}
	q := &creditQueue{err: errors.New("queue unavailable")}
	svc := &analyses.Service{Repo: repo, Usage: quota, JobQueue: q}
	if _, err := svc.Create(ctx, addDocument(t, db, "user"), "user", "job", ""); err == nil {
		t.Fatal("queue failure ignored")
	}
	mustUsage(t, quota, "user", 0)
	var state string
	if err := db.QueryRow(`SELECT state FROM analysis_usage_reservations`).Scan(&state); err != nil || state != "released" {
		t.Fatalf("%s %v", state, err)
	}
}

func addDocument(t *testing.T, db *sql.DB, user string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO documents(id,user_id,file_name,mime_type,size_bytes,storage_key) VALUES($1,$2,'resume.pdf','application/pdf',100,'test')`, id, user); err != nil {
		t.Fatal(err)
	}
	return id
}
