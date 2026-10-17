package payments

import (
	"context"
	"sync"
	"time"
)

// MemoryRepo is an in-memory Repo for dev and tests.
type MemoryRepo struct {
	mu      sync.Mutex
	byOrder map[string]Payment
	unlocks map[string]string // userID|analysisID -> payment id
}

// NewMemoryRepo constructs a MemoryRepo.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		byOrder: make(map[string]Payment),
		unlocks: make(map[string]string),
	}
}

func unlockKey(userID, analysisID string) string {
	return userID + "|" + analysisID
}

func (r *MemoryRepo) Create(ctx context.Context, p Payment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byOrder[p.RazorpayOrderID] = p
	return nil
}

func (r *MemoryRepo) GetByOrderID(ctx context.Context, orderID string) (Payment, error) {
	if err := ctx.Err(); err != nil {
		return Payment{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byOrder[orderID]
	if !ok {
		return Payment{}, ErrNotFound
	}
	return p, nil
}

func (r *MemoryRepo) MarkPaidAndUnlock(ctx context.Context, orderID, paymentID string) (Payment, error) {
	if err := ctx.Err(); err != nil {
		return Payment{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byOrder[orderID]
	if !ok {
		return Payment{}, ErrNotFound
	}
	if p.Status == StatusRefunded {
		return p, nil
	}
	p.Status = StatusPaid
	if p.RazorpayPaymentID == "" {
		p.RazorpayPaymentID = paymentID
	}
	p.UpdatedAt = time.Now().UTC()
	r.byOrder[orderID] = p
	key := unlockKey(p.UserID, p.AnalysisID)
	if _, exists := r.unlocks[key]; !exists {
		r.unlocks[key] = p.ID
	}
	return p, nil
}

func (r *MemoryRepo) MarkRefunded(ctx context.Context, paymentID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for orderID, p := range r.byOrder {
		if p.RazorpayPaymentID != paymentID {
			continue
		}
		p.Status = StatusRefunded
		p.UpdatedAt = time.Now().UTC()
		r.byOrder[orderID] = p
		key := unlockKey(p.UserID, p.AnalysisID)
		if r.unlocks[key] == p.ID {
			delete(r.unlocks, key)
		}
		return nil
	}
	return ErrNotFound
}

func (r *MemoryRepo) IsUnlocked(ctx context.Context, userID, analysisID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.unlocks[unlockKey(userID, analysisID)]
	return ok, nil
}
