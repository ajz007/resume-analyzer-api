package payments

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Tests create an isolated schema on a disposable local PostgreSQL instance.
func purchaseTestDB(t *testing.T, seed func(*sql.DB)) *PGRepo {
	t.Helper()
	dsn := os.Getenv("PAYMENTS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PAYMENTS_TEST_DATABASE_URL to a disposable local PostgreSQL database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" {
		t.Fatal("integration tests require a local database")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "payment_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	db.SetMaxOpenConns(20)
	t.Cleanup(func() { _ = db.Close() })
	apply := func(name string) {
		body, err := os.ReadFile(filepath.Join("..", "shared", "storage", "db", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		up := strings.Split(string(body), "-- +goose Down")[0]
		if _, err = db.Exec(up); err != nil {
			t.Fatal(err)
		}
	}
	apply("0024_payments.sql")
	if seed != nil {
		seed(db)
	}
	apply("0026_purchase_reservations.sql")
	return &PGRepo{DB: db}
}
func TestPGConcurrentPurchaseReservationAndRecovery(t *testing.T) {
	repo := purchaseTestDB(t, nil)
	ctx := context.Background()
	svc, provider := recoveryService()
	svc.Repo = repo
	id := uuid.NewString()
	svc.Analyses = fakeAnalyses{id: {ID: id, UserID: "google:u1", Status: "completed"}}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := *svc
			order, err := other.CreateOrder(ctx, "google:u1", id, "INR")
			if err != nil && !errors.Is(err, ErrPurchasePending) {
				t.Errorf("checkout: %v", err)
			}
			if err == nil && order.OrderID != "order_recovery" {
				t.Errorf("wrong order: %+v", order)
			}
		}()
	}
	wg.Wait()
	if provider.calls != 1 {
		t.Fatalf("provider creates: %d", provider.calls)
	}
	state, err := svc.RecoverPurchase(ctx, "google:u1", id)
	if err != nil || state.State != "ready" {
		t.Fatalf("%+v %v", state, err)
	}
	var reservations, payments int
	if err := repo.DB.QueryRow(`SELECT count(*) FROM payment_purchases`).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if err := repo.DB.QueryRow(`SELECT count(*) FROM payments`).Scan(&payments); err != nil {
		t.Fatal(err)
	}
	if reservations != 1 || payments != 1 {
		t.Fatalf("reservations=%d payments=%d", reservations, payments)
	}
	provider.order.Status = "paid"
	provider.order.Payments.Items = []ProviderPayment{{ID: "pay_pg", OrderID: "order_recovery", Amount: 49900, Currency: "INR", Status: "captured"}}
	state, err = svc.RecoverPurchase(ctx, "google:u1", id)
	if err != nil || state.State != "paid" {
		t.Fatalf("%+v %v", state, err)
	}
	if unlocked, err := repo.IsUnlocked(ctx, "google:u1", id); err != nil || !unlocked {
		t.Fatalf("%v %v", unlocked, err)
	}
}
func TestPGRecoveryAfterAttachFailure(t *testing.T) {
	repo := purchaseTestDB(t, nil)
	svc, provider := recoveryService()
	svc.Repo = &attachFailureRepo{Repo: repo}
	id := uuid.NewString()
	svc.Analyses = fakeAnalyses{id: {ID: id, UserID: "google:u1", Status: "completed"}}
	ctx := context.Background()
	if _, err := svc.CreateOrder(ctx, "google:u1", id, "INR"); !errors.Is(err, ErrPurchasePending) {
		t.Fatal(err)
	}
	state, err := svc.RecoverPurchase(ctx, "google:u1", id)
	if err != nil || state.State != "ready" || provider.calls != 1 {
		t.Fatalf("%+v %v", state, err)
	}
	p, err := repo.GetPurchase(ctx, "google:u1", id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AttachOrder(ctx, p, "order_wrong"); !errors.Is(err, ErrPurchaseConflict) {
		t.Fatalf("binding changed: %v", err)
	}
	persisted, err := repo.GetPurchase(ctx, "google:u1", id)
	if err != nil || persisted.RazorpayOrderID != "order_recovery" {
		t.Fatalf("%+v %v", persisted, err)
	}
}
func TestPGMigrationPreservesLegacyOrdersAndSelectsExistingPurchase(t *testing.T) {
	ctx := context.Background()
	analysis := uuid.NewString()
	other := uuid.NewString()
	oldID, newID, paidID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	repo := purchaseTestDB(t, func(db *sql.DB) {
		for i, p := range []Payment{{ID: oldID, AnalysisID: analysis, RazorpayOrderID: "order_old", Status: StatusCreated}, {ID: newID, AnalysisID: analysis, RazorpayOrderID: "order_new", Status: StatusCreated}, {ID: paidID, AnalysisID: other, RazorpayOrderID: "order_paid", Status: StatusPaid}} {
			if _, err := db.Exec(`INSERT INTO payments(id,user_id,analysis_id,razorpay_order_id,amount_minor,currency,status,created_at) VALUES($1,'google:u1',$2,$3,49900,'INR',$4,$5)`, p.ID, p.AnalysisID, p.RazorpayOrderID, p.Status, time.Now().Add(time.Duration(i)*time.Second)); err != nil {
				t.Fatal(err)
			}
		}
	})
	p, err := repo.GetPurchase(ctx, "google:u1", analysis)
	if err != nil || p.ID != oldID || !p.NeedsSupport {
		t.Fatalf("legacy recovery: %+v %v", p, err)
	}
	p, creator, err := repo.ReservePurchase(ctx, Payment{ID: uuid.NewString(), UserID: "google:u1", AnalysisID: analysis, AmountMinor: 49900, Currency: "INR", CreatedAt: time.Now()})
	if err != nil || creator || p.ID != oldID {
		t.Fatalf("new legacy reservation: %+v %v %v", p, creator, err)
	}
	var count int
	if err := repo.DB.QueryRow(`SELECT count(*) FROM payments`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("historical orders changed: %d %v", count, err)
	}
	p, err = repo.GetPurchase(ctx, "google:u1", other)
	if err != nil || p.ID != paidID || p.Status != StatusPaid {
		t.Fatalf("paid order lost: %+v %v", p, err)
	}
}

func TestPGAmbiguousLegacyPurchasesRequireSupport(t *testing.T) {
	id := uuid.NewString()
	repo := purchaseTestDB(t, func(db *sql.DB) {
		for _, order := range []string{"order_legacy1", "order_legacy2"} {
			if _, err := db.Exec(`INSERT INTO payments(id,user_id,analysis_id,razorpay_order_id,amount_minor,currency,status) VALUES($1,'google:u1',$2,$3,49900,'INR','created')`, uuid.NewString(), id, order); err != nil {
				t.Fatal(err)
			}
		}
	})
	svc, provider := recoveryService()
	svc.Repo = repo
	svc.Analyses = fakeAnalyses{id: {ID: id, UserID: "google:u1", Status: "completed"}}
	if _, err := svc.CreateOrder(context.Background(), "google:u1", id, "INR"); !errors.Is(err, ErrPurchaseConflict) {
		t.Fatalf("ambiguous legacy checkout: %v", err)
	}
	if provider.calls != 0 {
		t.Fatal("created provider order for ambiguous legacy purchase")
	}
}

func TestPGAdoptsLegacyOrderWrittenAfterMigration(t *testing.T) {
	repo := purchaseTestDB(t, nil)
	id, paymentID := uuid.NewString(), uuid.NewString()
	if _, err := repo.DB.Exec(`INSERT INTO payments(id,user_id,analysis_id,razorpay_order_id,amount_minor,currency,status) VALUES($1,'google:u1',$2,'order_during_rollout',49900,'INR','created')`, paymentID, id); err != nil {
		t.Fatal(err)
	}
	p, creator, err := repo.ReservePurchase(context.Background(), Payment{ID: uuid.NewString(), UserID: "google:u1", AnalysisID: id, AmountMinor: 900, Currency: "USD", CreatedAt: time.Now()})
	if err != nil || creator || p.ID != paymentID || p.Currency != "INR" || p.RazorpayOrderID != "order_during_rollout" {
		t.Fatalf("%+v creator=%v err=%v", p, creator, err)
	}
}
