package payments

import "context"

// Repo persists payments and report unlocks.
type Repo interface {
	Create(ctx context.Context, p Payment) error
	GetByOrderID(ctx context.Context, orderID string) (Payment, error)
	// MarkPaidAndUnlock marks the order paid and records the unlock. It is idempotent.
	MarkPaidAndUnlock(ctx context.Context, orderID, paymentID string) (Payment, error)
	// MarkRefunded marks the payment refunded and revokes its unlock. It is idempotent.
	MarkRefunded(ctx context.Context, paymentID string) error
	IsUnlocked(ctx context.Context, userID, analysisID string) (bool, error)
}
