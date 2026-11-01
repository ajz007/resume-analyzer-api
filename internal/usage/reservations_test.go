package usage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMemoryReservationAndReleaseAreIdempotent(t *testing.T) {
	svc := NewService()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.ReserveAnalysis(ctx, "guest:one", "review"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if u, err := svc.Get(ctx, "guest:one"); err != nil || u.Used != 1 {
		t.Fatalf("%+v %v", u, err)
	}
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.SettleAnalysis(ctx, "review", "failed"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if u, err := svc.Get(ctx, "guest:one"); err != nil || u.Used != 0 {
		t.Fatalf("%+v %v", u, err)
	}
	if _, err := svc.ReserveAnalysis(ctx, "guest:one", "review"); !errors.Is(err, ErrReservationConflict) {
		t.Fatal("closed reservation recharged")
	}
}
func TestMemoryFailedReviewCannotReduceNewWindow(t *testing.T) {
	for _, reset := range []string{"manual", "expired"} {
		t.Run(reset, func(t *testing.T) {
			svc := NewService()
			ctx := context.Background()
			if _, err := svc.ReserveAnalysis(ctx, "user", "old"); err != nil {
				t.Fatal(err)
			}
			if reset == "manual" {
				if _, err := svc.Reset(ctx, "user"); err != nil {
					t.Fatal(err)
				}
			} else {
				store := svc.store.(*memoryStore)
				store.mu.Lock()
				u := store.data["user"]
				u.ResetsAt = time.Now().Add(-time.Second)
				store.data["user"] = u
				store.mu.Unlock()
				if _, err := svc.Get(ctx, "user"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := svc.ReserveAnalysis(ctx, "user", "new"); err != nil {
				t.Fatal(err)
			}
			if err := svc.SettleAnalysis(ctx, "old", "failed"); err != nil {
				t.Fatal(err)
			}
			if u, err := svc.Get(ctx, "user"); err != nil || u.Used != 1 {
				t.Fatalf("%+v %v", u, err)
			}
		})
	}
}
func TestMemoryCommittedReviewDoesNotRefund(t *testing.T) {
	svc := NewService()
	ctx := context.Background()
	_, _ = svc.ReserveAnalysis(ctx, "user", "review")
	if err := svc.SettleAnalysis(ctx, "review", "completed"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SettleAnalysis(ctx, "review", "failed"); err != nil {
		t.Fatal(err)
	}
	if u, err := svc.Get(ctx, "user"); err != nil || u.Used != 1 {
		t.Fatalf("%+v %v", u, err)
	}
	if _, err := svc.ReserveAnalysis(ctx, "other", "review"); !errors.Is(err, ErrReservationConflict) {
		t.Fatal("owner changed")
	}
}
