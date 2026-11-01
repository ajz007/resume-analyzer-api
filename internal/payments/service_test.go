package payments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testKeySecret     = "test_secret"
	testWebhookSecret = "whsec"
)

func TestVerifyPaymentSignatureKnownVector(t *testing.T) {
	// hex(HMAC_SHA256("order_ABC123|pay_XYZ789", "test_secret")), computed independently.
	const sig = "85cbc6036124891c4d0280fbb7cd83804f87a66f2eb485a89af574086f592cbc"
	if !VerifyPaymentSignature("order_ABC123", "pay_XYZ789", sig, testKeySecret) {
		t.Fatalf("expected known signature to verify")
	}
	if VerifyPaymentSignature("order_ABC123", "pay_OTHER", sig, testKeySecret) {
		t.Fatalf("expected signature for a different payment to fail")
	}
	if VerifyPaymentSignature("order_ABC123", "pay_XYZ789", sig, "") {
		t.Fatalf("expected empty secret to fail")
	}
}

func TestVerifyWebhookSignatureKnownVector(t *testing.T) {
	const sig = "4673dd707ef4c41b987cb7fefe1583142dc702388c93145b7814b9ad3d3c183e"
	if !VerifyWebhookSignature([]byte(`{"event":"payment.captured"}`), sig, testWebhookSecret) {
		t.Fatalf("expected known webhook signature to verify")
	}
	if VerifyWebhookSignature([]byte(`{"event":"payment.captured" }`), sig, testWebhookSecret) {
		t.Fatalf("expected tampered body to fail")
	}
}

type fakeOrders struct {
	n     int
	calls []int64
}

func (f *fakeOrders) CreateOrder(_ context.Context, amount int64, _ string, _ string, _ map[string]string) (string, error) {
	f.n++
	f.calls = append(f.calls, amount)
	return fmt.Sprintf("order_%d", f.n), nil
}

type fakeAnalyses map[string]AnalysisInfo

func (f fakeAnalyses) GetAnalysisInfo(_ context.Context, id string) (AnalysisInfo, error) {
	a, ok := f[id]
	if !ok {
		return AnalysisInfo{}, ErrAnalysisNotFound
	}
	return a, nil
}

func newTestService() (*Service, *fakeOrders) {
	orders := &fakeOrders{}
	return &Service{
		Repo:     NewMemoryRepo(),
		Orders:   orders,
		Payments: fakePaymentFetcher{payment: ProviderPayment{ID: "pay_1", OrderID: "order_1", Amount: 49900, Currency: "INR", Status: "captured"}},
		Analyses: fakeAnalyses{
			"a1":      {ID: "a1", UserID: "google:u1", Status: "completed"},
			"pending": {ID: "pending", UserID: "google:u1", Status: "processing"},
		},
		KeyID:         "rzp_test_key",
		KeySecret:     testKeySecret,
		WebhookSecret: testWebhookSecret,
		Prices:        map[string]int64{CurrencyINR: 49900, CurrencyUSD: 900},
		Enabled:       true,
	}, orders
}

func TestCreateOrderUsesServerPriceAndValidatesOwnership(t *testing.T) {
	ctx := context.Background()
	svc, orders := newTestService()

	order, err := svc.CreateOrder(ctx, "google:u1", "a1", "usd")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Amount != 900 || order.Currency != CurrencyUSD || order.KeyID != "rzp_test_key" {
		t.Fatalf("unexpected order %+v", order)
	}
	if len(orders.calls) != 1 || orders.calls[0] != 900 {
		t.Fatalf("expected provider order for 900 cents, got %v", orders.calls)
	}

	if _, err := svc.CreateOrder(ctx, "google:other", "a1", "INR"); !errors.Is(err, ErrAnalysisNotFound) {
		t.Fatalf("expected ErrAnalysisNotFound for another user, got %v", err)
	}
	if _, err := svc.CreateOrder(ctx, "google:u1", "pending", "INR"); !errors.Is(err, ErrAnalysisNotReady) {
		t.Fatalf("expected ErrAnalysisNotReady, got %v", err)
	}
	if _, err := svc.CreateOrder(ctx, "google:u1", "a1", "EUR"); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("expected ErrInvalidCurrency, got %v", err)
	}
}

func TestCreateOrderDisabledWhenPaywallOff(t *testing.T) {
	svc, _ := newTestService()
	svc.Enabled = false
	if _, err := svc.CreateOrder(context.Background(), "google:u1", "a1", "INR"); !errors.Is(err, ErrPaymentsDisabled) {
		t.Fatalf("expected ErrPaymentsDisabled, got %v", err)
	}
	unlocked, err := svc.IsUnlocked(context.Background(), "google:u1", "a1")
	if err != nil || !unlocked {
		t.Fatalf("expected reports unlocked when paywall is off, got %v %v", unlocked, err)
	}
}

func TestVerifyCheckoutUnlocksOnceAndRejectsBadSignature(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	order, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	if _, err := svc.VerifyCheckout(ctx, "google:u1", order.OrderID, "pay_1", "bad"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
	sig := hmacHex([]byte(order.OrderID+"|pay_1"), testKeySecret)
	if _, err := svc.VerifyCheckout(ctx, "google:other", order.OrderID, "pay_1", sig); !errors.Is(err, ErrOrderUserMismatch) {
		t.Fatalf("expected ErrOrderUserMismatch, got %v", err)
	}
	for i := 0; i < 2; i++ {
		p, err := svc.VerifyCheckout(ctx, "google:u1", order.OrderID, "pay_1", sig)
		if err != nil || p.Status != StatusPaid {
			t.Fatalf("verify attempt %d: status=%q err=%v", i+1, p.Status, err)
		}
	}
	unlocked, _ := svc.IsUnlocked(ctx, "google:u1", "a1")
	if !unlocked {
		t.Fatalf("expected report unlocked after verify")
	}
	if _, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR"); !errors.Is(err, ErrAlreadyUnlocked) {
		t.Fatalf("expected ErrAlreadyUnlocked, got %v", err)
	}
}

func signedWebhook(body string) (string, string) {
	return body, hmacHex([]byte(body), testWebhookSecret)
}

func TestWebhookCapturedIsIdempotentAndRefundRevokes(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	order, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	captured, sig := signedWebhook(fmt.Sprintf(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_9","order_id":%q,"amount":49900,"currency":"INR"}}}}`, order.OrderID))
	for i := 0; i < 2; i++ {
		if err := svc.HandleWebhook(ctx, []byte(captured), sig); err != nil {
			t.Fatalf("captured webhook attempt %d: %v", i+1, err)
		}
	}
	if unlocked, _ := svc.IsUnlocked(ctx, "google:u1", "a1"); !unlocked {
		t.Fatalf("expected unlock after captured webhook")
	}

	refund, refundSig := signedWebhook(`{"event":"refund.processed","payload":{"refund":{"entity":{"payment_id":"pay_9"}}}}`)
	if err := svc.HandleWebhook(ctx, []byte(refund), refundSig); err != nil {
		t.Fatalf("refund webhook: %v", err)
	}
	if unlocked, _ := svc.IsUnlocked(ctx, "google:u1", "a1"); unlocked {
		t.Fatalf("expected refund to revoke unlock")
	}
	// A late captured retry must not re-unlock a refunded payment.
	if err := svc.HandleWebhook(ctx, []byte(captured), sig); err != nil {
		t.Fatalf("late captured webhook: %v", err)
	}
	if unlocked, _ := svc.IsUnlocked(ctx, "google:u1", "a1"); unlocked {
		t.Fatalf("expected refunded payment to stay locked")
	}
}

func TestWebhookRejectsBadSignatureAndAmountMismatch(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	order, _ := svc.CreateOrder(ctx, "google:u1", "a1", "INR")

	body := fmt.Sprintf(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_2","order_id":%q,"amount":100,"currency":"INR"}}}}`, order.OrderID)
	if err := svc.HandleWebhook(ctx, []byte(body), "bad"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
	_, sig := signedWebhook(body)
	if err := svc.HandleWebhook(ctx, []byte(body), sig); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("expected ErrAmountMismatch, got %v", err)
	}
	if unlocked, _ := svc.IsUnlocked(ctx, "google:u1", "a1"); unlocked {
		t.Fatalf("expected no unlock on amount mismatch")
	}

	unknown, unknownSig := signedWebhook(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_3","order_id":"order_unknown","amount":1,"currency":"INR"}}}}`)
	if err := svc.HandleWebhook(ctx, []byte(unknown), unknownSig); err != nil {
		t.Fatalf("expected unknown order to be ignored, got %v", err)
	}
}

func TestRazorpayClientCreateOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "rzp_test_key" || pass != testKeySecret || r.URL.Path != "/v1/orders" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"amount":49900`) || !strings.Contains(string(body), `"currency":"INR"`) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"id":"order_live1","status":"created"}`))
	}))
	defer srv.Close()

	client := NewRazorpayClient("rzp_test_key", testKeySecret)
	client.BaseURL = srv.URL
	id, err := client.CreateOrder(context.Background(), 49900, "INR", "receipt-1", nil)
	if err != nil || id != "order_live1" {
		t.Fatalf("expected order_live1, got %q err=%v", id, err)
	}

	client.KeySecret = "wrong"
	if _, err := client.CreateOrder(context.Background(), 49900, "INR", "receipt-1", nil); err == nil {
		t.Fatalf("expected error on provider 401")
	}
}

type fakePaymentFetcher struct {
	payment ProviderPayment
	err     error
}

func (f fakePaymentFetcher) FetchPayment(context.Context, string) (ProviderPayment, error) {
	return f.payment, f.err
}

func TestVerifyCheckoutRequiresCapturedMatchingProviderPayment(t *testing.T) {
	cases := []struct {
		name        string
		mutate      func(*ProviderPayment)
		providerErr error
		want        error
	}{
		{name: "authorized", mutate: func(p *ProviderPayment) { p.Status = "authorized" }, want: ErrPaymentNotCaptured},
		{name: "failed", mutate: func(p *ProviderPayment) { p.Status = "failed" }, want: ErrPaymentNotCaptured},
		{name: "refunded", mutate: func(p *ProviderPayment) { p.Status = "refunded" }, want: ErrPaymentNotCaptured},
		{name: "partial refund", mutate: func(p *ProviderPayment) { p.AmountRefunded = 100 }, want: ErrPaymentNotCaptured},
		{name: "wrong payment", mutate: func(p *ProviderPayment) { p.ID = "pay_other" }, want: ErrPaymentMismatch},
		{name: "wrong order", mutate: func(p *ProviderPayment) { p.OrderID = "order_other" }, want: ErrPaymentMismatch},
		{name: "wrong amount", mutate: func(p *ProviderPayment) { p.Amount = 1 }, want: ErrPaymentMismatch},
		{name: "wrong currency", mutate: func(p *ProviderPayment) { p.Currency = "USD" }, want: ErrPaymentMismatch},
		{name: "provider unavailable", providerErr: errors.New("timeout"), want: ErrPaymentVerificationUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTestService()
			ctx := context.Background()
			order, err := svc.CreateOrder(ctx, "google:u1", "a1", "INR")
			if err != nil {
				t.Fatal(err)
			}
			payment := ProviderPayment{ID: "pay_1", OrderID: order.OrderID, Amount: 49900, Currency: "INR", Status: "captured"}
			if tc.mutate != nil {
				tc.mutate(&payment)
			}
			svc.Payments = fakePaymentFetcher{payment: payment, err: tc.providerErr}
			sig := hmacHex([]byte(order.OrderID+"|pay_1"), testKeySecret)
			if _, err := svc.VerifyCheckout(ctx, "google:u1", order.OrderID, "pay_1", sig); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			p, _ := svc.Repo.GetByOrderID(ctx, order.OrderID)
			if p.Status != StatusCreated {
				t.Fatalf("state changed: %+v", p)
			}
			if unlocked, _ := svc.IsUnlocked(ctx, "google:u1", "a1"); unlocked {
				t.Fatal("unverified payment unlocked report")
			}
		})
	}
}

func TestVerifyCheckoutCanRetrySamePaymentAfterCapture(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	order, _ := svc.CreateOrder(ctx, "google:u1", "a1", "INR")
	p := ProviderPayment{ID: "pay_1", OrderID: order.OrderID, Amount: 49900, Currency: "INR", Status: "authorized"}
	sig := hmacHex([]byte(order.OrderID+"|pay_1"), testKeySecret)
	svc.Payments = fakePaymentFetcher{payment: p}
	if _, err := svc.VerifyCheckout(ctx, "google:u1", order.OrderID, "pay_1", sig); !errors.Is(err, ErrPaymentNotCaptured) {
		t.Fatal(err)
	}
	p.Status = "captured"
	svc.Payments = fakePaymentFetcher{payment: p}
	if got, err := svc.VerifyCheckout(ctx, "google:u1", order.OrderID, "pay_1", sig); err != nil || got.Status != StatusPaid {
		t.Fatalf("retry: %+v %v", got, err)
	}
}

func TestRazorpayFetchPayment(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantErr    bool
	}{
		{"captured", `{"id":"pay_1","order_id":"order_1","amount":49900,"currency":"INR","status":"captured","amount_refunded":0}`, 200, false},
		{"unauthorized", `{}`, 401, true},
		{"unavailable", `{}`, 503, true},
		{"bad JSON", `{`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				if !ok || user != "key" || pass != "secret" || r.Method != "GET" || r.URL.Path != "/v1/payments/pay_1" {
					t.Error("incorrect provider request")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := NewRazorpayClient("key", "secret")
			client.BaseURL = server.URL
			p, err := client.FetchPayment(context.Background(), "pay_1")
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantErr && (p.ID != "pay_1" || p.Amount != 49900 || p.OrderID != "order_1" || p.Status != "captured") {
				t.Fatalf("incorrect decode: %+v", p)
			}
		})
	}
}
