package payments

import (
	"errors"
	"time"
)

// Payment statuses.
const (
	StatusCreated  = "created"
	StatusPaid     = "paid"
	StatusFailed   = "failed"
	StatusRefunded = "refunded"
)

// Supported currencies.
const (
	CurrencyINR = "INR"
	CurrencyUSD = "USD"
)

var (
	ErrPaymentNotCaptured             = errors.New("payment is not captured; check the existing payment again before attempting another purchase")
	ErrPaymentMismatch                = errors.New("payment details do not match order")
	ErrPaymentVerificationUnavailable = errors.New("payment confirmation is temporarily unavailable; check the existing payment again")
	ErrNotFound                       = errors.New("payment not found")
	ErrAnalysisNotFound               = errors.New("analysis not found")
	ErrAnalysisNotReady               = errors.New("analysis is not completed")
	ErrAlreadyUnlocked                = errors.New("report already unlocked")
	ErrInvalidCurrency                = errors.New("unsupported currency")
	ErrInvalidSignature               = errors.New("invalid signature")
	ErrAmountMismatch                 = errors.New("payment amount does not match order")
	ErrPaymentsDisabled               = errors.New("payments are not configured")
	ErrLoginRequired                  = errors.New("login required to unlock a report")
	ErrOrderUserMismatch              = errors.New("order belongs to another user")
)

// Payment is a Razorpay order created to unlock one analysis report.
type Payment struct {
	ID                string
	UserID            string
	AnalysisID        string
	RazorpayOrderID   string
	RazorpayPaymentID string
	AmountMinor       int64
	Currency          string
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// AnalysisInfo is the subset of an analysis the payments flow needs.
type AnalysisInfo struct {
	ID     string
	UserID string
	Status string
}
