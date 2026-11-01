package usage

import (
	"context"
	"time"
)

type memoryReservation struct {
	UserID   string
	Epoch    int64
	ResetsAt time.Time
	State    string
}

func (s *memoryStore) ReserveAnalysis(ctx context.Context, userID, analysisID string) (Usage, error) {
	if err := ctx.Err(); err != nil {
		return Usage{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.data[userID]
	if !ok {
		u = defaultUsage(userID)
	}
	now := time.Now().UTC()
	if !now.Before(u.ResetsAt) {
		u.Used = 0
		u.Epoch++
		u.ResetsAt = nextMonthlyReset(now)
	}
	u.Plan = defaultPlan(userID)
	u.Limit = defaultLimit(userID)
	if old, exists := s.reservations[analysisID]; exists {
		if old.UserID != userID || old.State == "released" {
			return Usage{}, ErrReservationConflict
		}
		s.data[userID] = u
		return u, nil
	}
	if u.Used >= u.Limit {
		return Usage{}, ErrLimitReached
	}
	s.reservations[analysisID] = memoryReservation{UserID: userID, Epoch: u.Epoch, ResetsAt: u.ResetsAt, State: "reserved"}
	u.Used++
	s.data[userID] = u
	return u, nil
}
func (s *memoryStore) SettleAnalysis(ctx context.Context, analysisID, status string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reservations[analysisID]
	if !ok || r.State != "reserved" {
		return nil
	}
	if status != "failed" && status != "completed" {
		return nil
	}
	r.State = "committed"
	if status == "failed" {
		r.State = "released"
		u := s.data[r.UserID]
		if u.Epoch == r.Epoch && u.ResetsAt.Equal(r.ResetsAt) && time.Now().UTC().Before(u.ResetsAt) && u.Used > 0 {
			u.Used--
			s.data[r.UserID] = u
		}
	}
	s.reservations[analysisID] = r
	return nil
}
