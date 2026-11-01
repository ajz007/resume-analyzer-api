package payments

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

type recoveryProvider struct {
	mu        sync.Mutex
	calls     int
	order     ProviderOrder
	createErr error
	findErr   error
}

func (f *recoveryProvider) CreateOrder(_ context.Context, amount int64, currency, receipt string, _ map[string]string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.order = ProviderOrder{ID: "order_recovery", Receipt: receipt, Amount: amount, Currency: currency, Status: "created"}
	return f.order.ID, f.createErr
}
func (f *recoveryProvider) FindOrder(_ context.Context, receipt string) (ProviderOrder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.findErr != nil {
		return ProviderOrder{}, f.findErr
	}
	if f.order.ID == "" {
		return ProviderOrder{}, ErrNotFound
	}
	return f.order, nil
}
func recoveryService() (*Service, *recoveryProvider) {
	s, _ := newTestService()
	p := &recoveryProvider{}
	s.Orders = p
	return s, p
}

func TestConcurrentCheckoutAcrossServiceInstancesCreatesOneProviderOrder(t *testing.T) {
	svc, provider := recoveryService()
	second := *svc
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := svc
			if i%2 == 0 {
				s = &second
			}
			order, err := s.CreateOrder(context.Background(), "google:u1", "a1", "INR")
			if err != nil && !errors.Is(err, ErrPurchasePending) {
				t.Errorf("request: %v", err)
			}
			if err == nil && order.OrderID != "order_recovery" {
				t.Errorf("wrong order: %+v", order)
			}
		}(i)
	}
	wg.Wait()
	if provider.calls != 1 {
		t.Fatalf("created %d orders", provider.calls)
	}
}

func TestLostProviderResponseRecoversWithoutAnotherCreate(t *testing.T) {
	svc, provider := recoveryService()
	provider.createErr = errors.New("response lost after provider created order")
	if _, err := svc.CreateOrder(context.Background(), "google:u1", "a1", "INR"); !errors.Is(err, ErrPurchasePending) {
		t.Fatal(err)
	}
	// Simulate another Lambda instance and a new browser with no checkout proof.
	restarted := *svc
	state, err := restarted.RecoverPurchase(context.Background(), "google:u1", "a1")
	if err != nil || state.State != "ready" || state.Order == nil || state.Order.OrderID != "order_recovery" {
		t.Fatalf("recovery: %+v %v", state, err)
	}
	if _, err := restarted.CreateOrder(context.Background(), "google:u1", "a1", "INR"); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatal("recovery created another provider order")
	}
}

type attachFailureRepo struct {
	Repo
	failed bool
}

func (r *attachFailureRepo) AttachOrder(ctx context.Context, p Payment, orderID string) (Payment, error) {
	if !r.failed {
		r.failed = true
		return Payment{}, errors.New("database unavailable")
	}
	return r.Repo.AttachOrder(ctx, p, orderID)
}
func TestLocalSaveFailureRecoversByReceipt(t *testing.T) {
	svc, provider := recoveryService()
	svc.Repo = &attachFailureRepo{Repo: svc.Repo}
	if _, err := svc.CreateOrder(context.Background(), "google:u1", "a1", "INR"); !errors.Is(err, ErrPurchasePending) {
		t.Fatal(err)
	}
	state, err := svc.RecoverPurchase(context.Background(), "google:u1", "a1")
	if err != nil || state.State != "ready" {
		t.Fatalf("%+v %v", state, err)
	}
	if provider.calls != 1 {
		t.Fatal("created another provider order")
	}
}
func TestUncertainPurchaseNeverBlindlyRetriesCreate(t *testing.T) {
	svc, provider := recoveryService()
	provider.createErr = errors.New("timeout")
	provider.findErr = ErrNotFound
	for i := 0; i < 3; i++ {
		if _, err := svc.CreateOrder(context.Background(), "google:u1", "a1", "INR"); !errors.Is(err, ErrPurchasePending) {
			t.Fatal(err)
		}
	}
	state, err := svc.RecoverPurchase(context.Background(), "google:u1", "a1")
	if err != nil || state.State != "pending" || provider.calls != 1 {
		t.Fatalf("%+v %v creates=%d", state, err, provider.calls)
	}
}
func TestRecoveryConfirmsCapturedPaymentWithoutCheckoutProof(t *testing.T) {
	svc, provider := recoveryService()
	ctx := context.Background()
	_, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR")
	if err != nil {
		t.Fatal(err)
	}
	provider.order.Status = "paid"
	provider.order.Payments.Items = []ProviderPayment{{ID: "pay_recovery", OrderID: provider.order.ID, Amount: 49900, Currency: "INR", Status: "captured"}}
	state, err := svc.RecoverPurchase(ctx, "google:u1", "a1")
	if err != nil || state.State != "paid" {
		t.Fatalf("%+v %v", state, err)
	}
	if unlocked, _ := svc.IsUnlocked(ctx, "google:u1", "a1"); !unlocked {
		t.Fatal("capture not applied")
	}
	if _, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR"); !errors.Is(err, ErrAlreadyUnlocked) {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatal("another order created")
	}
}
func TestRecoveryProtectsAuthorizationRefundsAndMismatches(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ProviderOrder)
		state  string
		want   error
	}{
		{"authorized", func(o *ProviderOrder) {
			o.Payments.Items = []ProviderPayment{{ID: "pay_1", OrderID: o.ID, Amount: o.Amount, Currency: o.Currency, Status: "authorized"}}
		}, "pending", nil},
		{"refunded", func(o *ProviderOrder) {
			o.Payments.Items = []ProviderPayment{{ID: "pay_1", OrderID: o.ID, Amount: o.Amount, Currency: o.Currency, Status: "refunded", AmountRefunded: o.Amount}}
		}, "refunded", nil},
		{"wrong receipt", func(o *ProviderOrder) { o.Receipt = "other" }, "", ErrPurchaseConflict},
		{"wrong amount", func(o *ProviderOrder) { o.Amount = 1 }, "", ErrPurchaseConflict},
		{"wrong currency", func(o *ProviderOrder) { o.Currency = "USD" }, "", ErrPurchaseConflict},
		{"wrong order", func(o *ProviderOrder) { o.ID = "order_other" }, "", ErrPurchaseConflict},
		{"wrong payment order", func(o *ProviderOrder) {
			o.Payments.Items = []ProviderPayment{{ID: "pay_1", OrderID: "other", Amount: o.Amount, Currency: o.Currency, Status: "captured"}}
		}, "", ErrPurchaseConflict},
		{"paid without details", func(o *ProviderOrder) { o.Status = "paid" }, "pending", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, provider := recoveryService()
			ctx := context.Background()
			_, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR")
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&provider.order)
			state, err := svc.RecoverPurchase(ctx, "google:u1", "a1")
			if !errors.Is(err, tc.want) || state.State != tc.state {
				t.Fatalf("%+v %v", state, err)
			}
			if unlocked, _ := svc.IsUnlocked(ctx, "google:u1", "a1"); unlocked {
				t.Fatal("unsafe recovery unlocked report")
			}
		})
	}
}
func TestPurchaseRetainsOriginalQuoteAndCannotSwitchCurrency(t *testing.T) {
	svc, provider := recoveryService()
	ctx := context.Background()
	_, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR")
	if err != nil {
		t.Fatal(err)
	}
	svc.Prices[CurrencyINR] = 79900
	order, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR")
	if err != nil || order.Amount != 49900 {
		t.Fatalf("quote changed: %+v %v", order, err)
	}
	if _, err := svc.CreateOrder(ctx, "google:u1", "a1", "USD"); !errors.Is(err, ErrPurchaseCurrency) {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatal("created another order")
	}
}
func TestRecoveryOwnerScopeAndNoPurchase(t *testing.T) {
	svc, provider := recoveryService()
	ctx := context.Background()
	if _, err := svc.RecoverPurchase(ctx, "other", "a1"); !errors.Is(err, ErrAnalysisNotFound) {
		t.Fatal(err)
	}
	if state, err := svc.RecoverPurchase(ctx, "google:u1", "a1"); err != nil || state.State != "none" {
		t.Fatalf("%+v %v", state, err)
	}
	if provider.calls != 0 {
		t.Fatal("status read created order")
	}
}

func TestAttachOrderRejectsChangedBinding(t *testing.T) {
	svc, _ := recoveryService()
	ctx := context.Background()
	order, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR")
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.Repo.GetByOrderID(ctx, order.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Repo.AttachOrder(ctx, p, "order_other"); !errors.Is(err, ErrPurchaseConflict) {
		t.Fatal(fmt.Sprintf("changed order accepted: %v", err))
	}
}

func TestRecoveryKeepsCapturedPaymentWhenLateAttemptWasAutoRefunded(t *testing.T) {
	svc, provider := recoveryService()
	ctx := context.Background()
	_, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR")
	if err != nil {
		t.Fatal(err)
	}
	provider.order.Status = "paid"
	provider.order.Payments.Items = []ProviderPayment{
		{ID: "pay_late", OrderID: "order_recovery", Amount: 49900, Currency: "INR", Status: "refunded", AmountRefunded: 49900},
		{ID: "pay_captured", OrderID: "order_recovery", Amount: 49900, Currency: "INR", Status: "captured"},
	}
	state, err := svc.RecoverPurchase(ctx, "google:u1", "a1")
	if err != nil || state.State != "paid" {
		t.Fatalf("%+v %v", state, err)
	}
	p, err := svc.Repo.GetByOrderID(ctx, "order_recovery")
	if err != nil || p.RazorpayPaymentID != "pay_captured" {
		t.Fatalf("%+v %v", p, err)
	}
}

type failedReservationRepo struct{ Repo }

func (r failedReservationRepo) ReservePurchase(context.Context, Payment) (Payment, bool, error) {
	return Payment{}, false, errors.New("database unavailable")
}
func TestReservationFailureDoesNotContactProvider(t *testing.T) {
	svc, provider := recoveryService()
	svc.Repo = failedReservationRepo{Repo: svc.Repo}
	if _, err := svc.CreateOrder(context.Background(), "google:u1", "a1", "INR"); err == nil {
		t.Fatal("reservation failure ignored")
	}
	if provider.calls != 0 {
		t.Fatal("provider called before durable reservation")
	}
}
