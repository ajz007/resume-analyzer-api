package bootstrap

import (
	"context"
	"errors"
	"resume-backend/internal/payments"
	"resume-backend/internal/shared/config"
	"testing"
)

func TestPaymentConfigurationDoesNotDisablePaywallOnMissingCredentials(t *testing.T) {
	for _, missing := range []string{"keyID", "keySecret", "webhookSecret"} {
		t.Run(missing, func(t *testing.T) {
			cfg := config.Config{PaywallEnabled: true, RazorpayKeyID: "key", RazorpayKeySecret: "secret", RazorpayWebhookSecret: "webhook"}
			switch missing {
			case "keyID":
				cfg.RazorpayKeyID = ""
			case "keySecret":
				cfg.RazorpayKeySecret = ""
			case "webhookSecret":
				cfg.RazorpayWebhookSecret = ""
			}
			svc := buildPaymentsService(cfg, nil, nil)
			if !svc.PaywallActive() {
				t.Fatal("missing credentials disabled paywall")
			}
			ctx := context.Background()
			if unlocked, err := svc.IsUnlocked(ctx, "user", "analysis"); err != nil || unlocked {
				t.Fatalf("unpaid report: %v %v", unlocked, err)
			}
			if _, err := svc.CreateOrder(ctx, "user", "analysis", "INR"); !errors.Is(err, payments.ErrPaymentsDisabled) {
				t.Fatalf("checkout: %v", err)
			}
			if err := svc.Repo.Create(ctx, payments.Payment{ID: "local", UserID: "user", AnalysisID: "analysis", RazorpayOrderID: "order_1"}); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Repo.MarkPaidAndUnlock(ctx, "order_1", "pay_1"); err != nil {
				t.Fatal(err)
			}
			if unlocked, err := svc.IsUnlocked(ctx, "user", "analysis"); err != nil || !unlocked {
				t.Fatalf("existing paid access lost: %v %v", unlocked, err)
			}
		})
	}
}
func TestPaywallCanBeExplicitlyDisabled(t *testing.T) {
	svc := buildPaymentsService(config.Config{PaywallEnabled: false}, nil, nil)
	if svc.PaywallActive() {
		t.Fatal("explicit disable ignored")
	}
	if unlocked, err := svc.IsUnlocked(context.Background(), "user", "analysis"); err != nil || !unlocked {
		t.Fatalf("%v %v", unlocked, err)
	}
}

func TestCompletePaymentConfigurationWiresProvider(t *testing.T) {
	svc := buildPaymentsService(config.Config{PaywallEnabled: true, RazorpayKeyID: "key", RazorpayKeySecret: "secret", RazorpayWebhookSecret: "webhook"}, nil, nil)
	if !svc.PaywallActive() || svc.Orders == nil || svc.Payments == nil {
		t.Fatal("configured payment provider not wired")
	}
}
