package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const analysisStatusCompleted = "completed"

// AnalysisLookup fetches analysis ownership and status.
type AnalysisLookup interface {
	GetAnalysisInfo(ctx context.Context, analysisID string) (AnalysisInfo, error)
}

// OrderCreator creates provider orders. *RazorpayClient implements it.
type OrderCreator interface {
	CreateOrder(ctx context.Context, amountMinor int64, currency, receipt string, notes map[string]string) (string, error)
}

// PaymentFetcher retrieves authoritative provider state before granting access.
type PaymentFetcher interface {
	FetchPayment(ctx context.Context, paymentID string) (ProviderPayment, error)
}

// ProviderPayment contains only fields needed to verify an entitlement.
type ProviderPayment struct {
	ID             string `json:"id"`
	OrderID        string `json:"order_id"`
	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	Status         string `json:"status"`
	AmountRefunded int64  `json:"amount_refunded"`
}

// Service implements one-time report unlocks via Razorpay.
type Service struct {
	Repo          Repo
	Orders        OrderCreator
	Payments      PaymentFetcher
	Analyses      AnalysisLookup
	KeyID         string
	KeySecret     string
	WebhookSecret string
	// Prices maps currency -> amount in minor units (paise, cents).
	Prices map[string]int64
	// Enabled gates the paywall: when false, every report is treated as unlocked.
	Enabled bool
}

// Order is returned to the client to open Razorpay Checkout.
type Order struct {
	OrderID  string `json:"orderId"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	KeyID    string `json:"keyId"`
}

// Price is a single display price.
type Price struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}

// PaywallActive reports whether reports should be redacted until unlocked.
func (s *Service) PaywallActive() bool {
	return s != nil && s.Enabled
}

// ListPrices returns the configured prices.
func (s *Service) ListPrices() []Price {
	out := make([]Price, 0, len(s.Prices))
	for _, currency := range []string{CurrencyINR, CurrencyUSD} {
		if amount, ok := s.Prices[currency]; ok && amount > 0 {
			out = append(out, Price{Currency: currency, Amount: amount})
		}
	}
	return out
}

// IsUnlocked reports whether the user can see the full report. When the paywall is off,
// every report is unlocked.
func (s *Service) IsUnlocked(ctx context.Context, userID, analysisID string) (bool, error) {
	if !s.PaywallActive() {
		return true, nil
	}
	return s.Repo.IsUnlocked(ctx, userID, analysisID)
}

// CreateOrder creates a Razorpay order for unlocking the given analysis.
func (s *Service) CreateOrder(ctx context.Context, userID, analysisID, currency string) (Order, error) {
	if !s.PaywallActive() || s.Orders == nil {
		return Order{}, ErrPaymentsDisabled
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	amount, ok := s.Prices[currency]
	if !ok || amount <= 0 {
		return Order{}, ErrInvalidCurrency
	}

	analysis, err := s.Analyses.GetAnalysisInfo(ctx, analysisID)
	if err != nil {
		return Order{}, err
	}
	if analysis.UserID != userID {
		return Order{}, ErrAnalysisNotFound
	}
	if analysis.Status != analysisStatusCompleted {
		return Order{}, ErrAnalysisNotReady
	}
	unlocked, err := s.Repo.IsUnlocked(ctx, userID, analysisID)
	if err != nil {
		return Order{}, err
	}
	if unlocked {
		return Order{}, ErrAlreadyUnlocked
	}

	paymentID := uuid.NewString()
	orderID, err := s.Orders.CreateOrder(ctx, amount, currency, paymentID, map[string]string{
		"analysis_id": analysisID,
		"payment_id":  paymentID,
	})
	if err != nil {
		return Order{}, err
	}
	now := time.Now().UTC()
	if err := s.Repo.Create(ctx, Payment{
		ID:              paymentID,
		UserID:          userID,
		AnalysisID:      analysisID,
		RazorpayOrderID: orderID,
		AmountMinor:     amount,
		Currency:        currency,
		Status:          StatusCreated,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		return Order{}, err
	}
	return Order{OrderID: orderID, Amount: amount, Currency: currency, KeyID: s.KeyID}, nil
}

// VerifyCheckout confirms a payment from the Razorpay Checkout handler and unlocks the report.
func (s *Service) VerifyCheckout(ctx context.Context, userID, orderID, paymentID, signature string) (Payment, error) {
	if !s.PaywallActive() {
		return Payment{}, ErrPaymentsDisabled
	}
	if !VerifyPaymentSignature(orderID, paymentID, signature, s.KeySecret) {
		return Payment{}, ErrInvalidSignature
	}
	p, err := s.Repo.GetByOrderID(ctx, orderID)
	if err != nil {
		return Payment{}, err
	}
	if p.UserID != userID {
		return Payment{}, ErrOrderUserMismatch
	}
	if s.Payments == nil {
		return Payment{}, ErrPaymentsDisabled
	}
	confirmed, err := s.Payments.FetchPayment(ctx, paymentID)
	if err != nil {
		return Payment{}, fmt.Errorf("%w: %v", ErrPaymentVerificationUnavailable, err)
	}
	if confirmed.ID != paymentID || confirmed.OrderID != orderID || confirmed.Amount != p.AmountMinor || confirmed.Currency != p.Currency {
		return Payment{}, ErrPaymentMismatch
	}
	if confirmed.Status != "captured" || confirmed.AmountRefunded != 0 {
		return Payment{}, ErrPaymentNotCaptured
	}
	return s.Repo.MarkPaidAndUnlock(ctx, orderID, paymentID)
}

type webhookEvent struct {
	Event   string `json:"event"`
	Payload struct {
		Payment struct {
			Entity struct {
				ID       string `json:"id"`
				OrderID  string `json:"order_id"`
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"entity"`
		} `json:"payment"`
		Refund struct {
			Entity struct {
				PaymentID string `json:"payment_id"`
			} `json:"entity"`
		} `json:"refund"`
	} `json:"payload"`
}

// HandleWebhook processes a signed Razorpay webhook. Unknown events and orders not
// created by this service are acknowledged and ignored.
func (s *Service) HandleWebhook(ctx context.Context, body []byte, signature string) error {
	if !VerifyWebhookSignature(body, signature, s.WebhookSecret) {
		return ErrInvalidSignature
	}
	var evt webhookEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return fmt.Errorf("decode webhook: %w", err)
	}
	switch evt.Event {
	case "payment.captured", "order.paid":
		pay := evt.Payload.Payment.Entity
		if pay.OrderID == "" {
			return nil
		}
		p, err := s.Repo.GetByOrderID(ctx, pay.OrderID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if pay.Amount != p.AmountMinor || !strings.EqualFold(pay.Currency, p.Currency) {
			return ErrAmountMismatch
		}
		_, err = s.Repo.MarkPaidAndUnlock(ctx, pay.OrderID, pay.ID)
		return err
	case "refund.processed":
		paymentID := evt.Payload.Refund.Entity.PaymentID
		if paymentID == "" {
			paymentID = evt.Payload.Payment.Entity.ID
		}
		if paymentID == "" {
			return nil
		}
		if err := s.Repo.MarkRefunded(ctx, paymentID); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		return nil
	default:
		return nil
	}
}
