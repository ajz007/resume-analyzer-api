package analyses

import (
	"context"
	"encoding/json"
	"errors"
	"resume-backend/internal/llm"
	"resume-backend/internal/usage"
	"sync/atomic"
	"testing"
	"time"
)

type blockedAnalysisLLM struct {
	calls            atomic.Int32
	entered, release chan struct{}
}

func (c *blockedAnalysisLLM) AnalyzeResume(ctx context.Context, _ llm.AnalyzeInput) (json.RawMessage, error) {
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > analysisExecutionBudget {
		panic("analysis provider missing execution budget")
	}
	c.calls.Add(1)
	close(c.entered)
	select {
	case <-c.release:
		return nil, errors.New("forced provider failure")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func TestDuplicateWorkerDoesNotRepeatAnalysisProviderCall(t *testing.T) {
	client := &blockedAnalysisLLM{entered: make(chan struct{}), release: make(chan struct{})}
	svc, _, _, doc := setupServiceWithDoc(t, client)
	svc.Usage = usage.NewService()
	svc.JobQueue = &stubQueue{}
	a, err := svc.Create(context.Background(), doc, "user-1", "job", "")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- svc.ProcessAnalysis(context.Background(), a.ID) }()
	select {
	case <-client.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provider not called")
	}
	if err := svc.ProcessAnalysis(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	if client.calls.Load() != 1 {
		t.Fatalf("provider calls=%d", client.calls.Load())
	}
	close(client.release)
	if err := <-done; err == nil {
		t.Fatal("expected controlled failure")
	}
	if err := svc.ProcessAnalysis(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	if client.calls.Load() != 1 {
		t.Fatal("failed attempt was automatically executed again")
	}
	if u, err := svc.Usage.Get(context.Background(), "user-1"); err != nil || u.Used != 0 {
		t.Fatalf("%+v %v", u, err)
	}
}

func TestProcessingClaimCannotBeReusedAfterStatusReset(t *testing.T) {
	repo := NewMemoryRepo()
	ctx := context.Background()
	a := Analysis{ID: "one", UserID: "user", Status: StatusQueued}
	if err := repo.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.ClaimProcessing(ctx, a.ID, time.Now()); err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if err := repo.UpdateStatus(ctx, a.ID, StatusQueued, nil); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.ClaimProcessing(ctx, a.ID, time.Now()); err != nil || ok {
		t.Fatalf("claim reused: %v %v", ok, err)
	}
}
