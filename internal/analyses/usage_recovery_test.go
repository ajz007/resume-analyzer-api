package analyses

import (
	"context"
	"errors"
	"resume-backend/internal/usage"
	"testing"
	"time"
)

func TestLateFailureCannotRefundCompletedReview(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepo()
	quota := usage.NewService()
	svc := &Service{Repo: repo, Usage: quota, JobQueue: &stubQueue{}}
	review, err := svc.Create(ctx, "document", "user", "job", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := repo.UpdateAnalysisResult(ctx, review.ID, map[string]any{"finished": true}, &now); err != nil {
		t.Fatal(err)
	}
	svc.failAnalysis(ctx, review.ID, "user", "document", errors.New("late worker failure"), nil)
	persisted, err := repo.GetByID(ctx, review.ID)
	if err != nil || persisted.Status != StatusCompleted {
		t.Fatalf("%+v %v", persisted, err)
	}
	if u, err := quota.Get(ctx, "user"); err != nil || u.Used != 1 {
		t.Fatalf("%+v %v", u, err)
	}
}
func TestQueueFailureReturnsAllowanceForBothSubmissionPaths(t *testing.T) {
	for _, method := range []string{"create", "start"} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			repo := NewMemoryRepo()
			quota := usage.NewService()
			svc := &Service{Repo: repo, Usage: quota, JobQueue: &stubQueue{err: errors.New("queue unavailable")}}
			var err error
			if method == "create" {
				_, err = svc.Create(ctx, "document", "user", "job", "")
			} else {
				_, _, err = svc.StartOrReuse(ctx, "document", "user", "job", "", ModeJobMatch, false)
			}
			if err == nil {
				t.Fatal("queue error ignored")
			}
			if u, err := quota.Get(ctx, "user"); err != nil || u.Used != 0 {
				t.Fatalf("%+v %v", u, err)
			}
			rows, err := repo.ListByUser(ctx, "user", 10, 0)
			if err != nil || len(rows) != 1 || rows[0].Status != StatusFailed {
				t.Fatalf("%+v %v", rows, err)
			}
		})
	}
}
func TestMissingQueueDoesNotReserveAllowanceOrCreateReview(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepo()
	quota := usage.NewService()
	svc := &Service{Repo: repo, Usage: quota}
	if _, err := svc.Create(ctx, "document", "user", "job", ""); !errors.Is(err, ErrJobQueueNotConfigured) {
		t.Fatal(err)
	}
	if _, _, err := svc.StartOrReuse(ctx, "document", "user", "job", "", ModeJobMatch, false); !errors.Is(err, ErrJobQueueNotConfigured) {
		t.Fatal(err)
	}
	if u, err := quota.Get(ctx, "user"); err != nil || u.Used != 0 {
		t.Fatalf("%+v %v", u, err)
	}
	if rows, err := repo.ListByUser(ctx, "user", 10, 0); err != nil || len(rows) != 0 {
		t.Fatalf("%+v %v", rows, err)
	}
}
