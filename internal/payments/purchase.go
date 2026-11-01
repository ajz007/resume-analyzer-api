package payments

import (
	"context"
	"errors"
	"fmt"
)

// OrderFinder locates an uncertain order using the durable purchase id as receipt.
type OrderFinder interface {
	FindOrder(ctx context.Context, receipt string) (ProviderOrder, error)
}
type ProviderOrder struct {
	ID       string `json:"id"`
	Receipt  string `json:"receipt"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
	Payments struct {
		Items []ProviderPayment `json:"items"`
	} `json:"payments"`
}
type PurchaseStatus struct {
	State string `json:"state"`
	Order *Order `json:"order,omitempty"`
}

func (s *Service) checkoutOrder(p Payment) Order {
	return Order{OrderID: p.RazorpayOrderID, Amount: p.AmountMinor, Currency: p.Currency, KeyID: s.KeyID}
}

// recoverPurchase performs reads at the provider and idempotent local writes only.
func (s *Service) recoverPurchase(ctx context.Context, p Payment) (Payment, error) {
	if p.NeedsSupport {
		return Payment{}, ErrPurchaseConflict
	}
	if p.Status == StatusRefunded {
		return Payment{}, ErrPurchaseClosed
	}
	if p.Status == StatusPaid {
		return p, nil
	}
	finder, ok := s.Orders.(OrderFinder)
	if !ok {
		return Payment{}, ErrPurchasePending
	}
	order, err := finder.FindOrder(ctx, p.ID)
	if errors.Is(err, ErrPurchaseConflict) {
		return Payment{}, err
	}
	if err != nil {
		return Payment{}, fmt.Errorf("%w: %v", ErrPurchasePending, err)
	}
	if order.ID == "" || order.Receipt != p.ID || order.Amount != p.AmountMinor || order.Currency != p.Currency || (p.RazorpayOrderID != "" && p.RazorpayOrderID != order.ID) {
		return Payment{}, ErrPurchaseConflict
	}
	p, err = s.Repo.AttachOrder(ctx, p, order.ID)
	if err != nil {
		return Payment{}, err
	}
	if p.Status == StatusPaid {
		return p, nil
	}
	if p.Status == StatusRefunded {
		return Payment{}, ErrPurchaseClosed
	}

	// Razorpay can auto-refund a late second authorization on an already-paid
	// order. That refunded attempt must not hide the valid captured payment.
	capturedID := ""
	hasRefund := false
	for _, payment := range order.Payments.Items {
		if payment.OrderID != order.ID || payment.ID == "" || payment.Amount != p.AmountMinor || payment.Currency != p.Currency {
			return Payment{}, ErrPurchaseConflict
		}
		if payment.AmountRefunded > 0 || payment.Status == "refunded" {
			hasRefund = true
			continue
		}
		if payment.Status == "captured" {
			if capturedID != "" {
				return Payment{}, ErrPurchaseConflict
			}
			capturedID = payment.ID
		}
	}
	if capturedID != "" {
		return s.Repo.MarkPaidAndUnlock(ctx, order.ID, capturedID)
	}
	if hasRefund {
		return Payment{}, ErrPurchaseClosed
	}
	for _, payment := range order.Payments.Items {
		if payment.Status == "authorized" {
			return Payment{}, ErrPurchasePending
		}
	}
	if order.Status != "created" && order.Status != "attempted" {
		return Payment{}, ErrPurchasePending
	}
	return p, nil
}

// RecoverPurchase is owner-scoped and cannot create a new purchase or provider order.
func (s *Service) RecoverPurchase(ctx context.Context, userID, analysisID string) (PurchaseStatus, error) {
	analysis, err := s.Analyses.GetAnalysisInfo(ctx, analysisID)
	if err != nil {
		return PurchaseStatus{}, err
	}
	if analysis.UserID != userID {
		return PurchaseStatus{}, ErrAnalysisNotFound
	}
	if !s.PaywallActive() {
		return PurchaseStatus{State: "paid"}, nil
	}
	unlocked, err := s.Repo.IsUnlocked(ctx, userID, analysisID)
	if err != nil {
		return PurchaseStatus{}, err
	}
	if unlocked {
		return PurchaseStatus{State: "paid"}, nil
	}
	p, err := s.Repo.GetPurchase(ctx, userID, analysisID)
	if errors.Is(err, ErrNotFound) {
		return PurchaseStatus{State: "none"}, nil
	}
	if err != nil {
		return PurchaseStatus{}, err
	}
	p, err = s.recoverPurchase(ctx, p)
	if errors.Is(err, ErrPurchasePending) {
		return PurchaseStatus{State: "pending"}, nil
	}
	if errors.Is(err, ErrPurchaseClosed) {
		return PurchaseStatus{State: "refunded"}, nil
	}
	if err != nil {
		return PurchaseStatus{}, err
	}
	if p.Status == StatusPaid {
		return PurchaseStatus{State: "paid"}, nil
	}
	order := s.checkoutOrder(p)
	return PurchaseStatus{State: "ready", Order: &order}, nil
}
