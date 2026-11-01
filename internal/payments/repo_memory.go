package payments

import (
	"context"
	"sync"
	"time"
)

// MemoryRepo is an in-memory Repo for dev and tests.
type MemoryRepo struct {
	mu        sync.Mutex
	byOrder   map[string]Payment
	purchases map[string]Payment
	unlocks   map[string]string // userID|analysisID -> payment id
}

// NewMemoryRepo constructs a MemoryRepo.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		byOrder:   make(map[string]Payment),
		purchases: make(map[string]Payment),
		unlocks:   make(map[string]string),
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

func (r *MemoryRepo) ReservePurchase(ctx context.Context, p Payment) (Payment, bool, error) {
	if err := ctx.Err(); err != nil {
		return Payment{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := unlockKey(p.UserID, p.AnalysisID)
	if old, ok := r.purchases[key]; ok {
		return r.purchaseLocked(old), false, nil
	}
	r.purchases[key] = p
	return p, true, nil
}
func (r *MemoryRepo) purchaseLocked(p Payment) Payment {
	for _, existing := range r.byOrder {
		if existing.ID == p.ID {
			return existing
		}
	}
	return p
}
func (r *MemoryRepo) GetPurchase(ctx context.Context, userID, analysisID string) (Payment, error) {
	if err := ctx.Err(); err != nil {
		return Payment{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.purchases[unlockKey(userID, analysisID)]
	if !ok {
		return Payment{}, ErrNotFound
	}
	return r.purchaseLocked(p), nil
}
func (r *MemoryRepo) AttachOrder(ctx context.Context, p Payment, orderID string) (Payment, error) {
	if err := ctx.Err(); err != nil {
		return Payment{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	reserved, ok := r.purchases[unlockKey(p.UserID, p.AnalysisID)]
	if !ok || reserved.ID != p.ID || reserved.AmountMinor != p.AmountMinor || reserved.Currency != p.Currency || orderID == "" {
		return Payment{}, ErrPurchaseConflict
	}
	if old := r.purchaseLocked(reserved); old.RazorpayOrderID != "" {
		if old.RazorpayOrderID != orderID {
			return Payment{}, ErrPurchaseConflict
		}
		return old, nil
	}
	if _, exists := r.byOrder[orderID]; exists {
		return Payment{}, ErrPurchaseConflict
	}
	reserved.RazorpayOrderID = orderID
	reserved.Status = StatusCreated
	reserved.UpdatedAt = time.Now().UTC()
	r.byOrder[orderID] = reserved
	return reserved, nil
}
