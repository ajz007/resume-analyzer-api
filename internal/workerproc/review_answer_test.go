package workerproc

import (
	"context"
	"resume-backend/internal/analyses"
	"resume-backend/internal/bootstrap"
	"resume-backend/internal/queue"
	"testing"
)

func TestReviewAnswerQueueDispatch(t *testing.T) {
	body := `{"type":"review_answer","analysisId":"analysis","jobId":"answer-request","requestId":"trace"}`
	msg, _, err := ParseMessage(body)
	if err != nil || msg.Type != queue.MessageTypeReviewAnswer {
		t.Fatal(msg, err)
	}
	for _, invalid := range []string{`{"type":"review_answer","jobId":"answer-request"}`, `{"type":"review_answer","analysisId":"analysis"}`} {
		if _, _, err := ParseMessage(invalid); err == nil {
			t.Fatal("missing answer identity accepted")
		}
	}
	repo := analyses.NewMemoryRepo()
	if err := repo.Create(context.Background(), analyses.Analysis{ID: "analysis", UserID: "owner", Status: analyses.StatusCompleted, Result: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	// A duplicate job with no pending reservation is acknowledged without generation.
	app := &bootstrap.App{AnalysesService: &analyses.Service{Repo: repo}}
	if err := HandleMessage(context.Background(), app, body); err != nil {
		t.Fatal(err)
	}
}
