package analyses

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"resume-backend/internal/aimeter"
	openai "resume-backend/internal/llm/openai"
)

type analysisUsageRecorder struct {
	mu     sync.Mutex
	events []aimeter.Attempt
}

func (r *analysisUsageRecorder) Start(context.Context, aimeter.Attempt) error { return nil }
func (r *analysisUsageRecorder) Finish(_ context.Context, a aimeter.Attempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, a)
	return nil
}

type meterFakeTransport struct{ calls int }

func (f *meterFakeTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.calls++
	status := 200
	body := `{"model":"gpt-6.1-sol","service_tier":"default","choices":[{"message":{"content":"{}"}}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`
	if f.calls == 1 {
		status = 503
		body = `{"error":{"message":"forced server failure","type":"server_error"}}`
	}
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
}
func TestAnalysisUsageLinksTransientRetryToSameReview(t *testing.T) {
	// All HTTP exchanges are intercepted in memory. No network/provider request occurs.
	original := http.DefaultTransport
	fake := &meterFakeTransport{}
	http.DefaultTransport = fake
	defer func() { http.DefaultTransport = original }()
	client, err := openai.NewClient("fictional-test-key", "gpt-6.1-sol")
	if err != nil {
		t.Fatal(err)
	}
	recorder := &analysisUsageRecorder{}
	client.SetUsageRecorder(recorder)
	svc, _, _, doc := setupServiceWithDoc(t, client)
	svc.JobQueue = &stubQueue{}
	review, err := svc.Create(context.Background(), doc, "user-1", "job", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = svc.ProcessAnalysis(context.Background(), review.ID)
	if len(recorder.events) < 2 || len(recorder.events) != fake.calls {
		t.Fatalf("events=%d exchanges=%d", len(recorder.events), fake.calls)
	}
	first, second := recorder.events[0], recorder.events[1]
	if first.UsageState != "unknown" || second.Retry != "transient" || second.Number != 2 {
		t.Fatalf("%+v", recorder.events)
	}
	for _, event := range recorder.events {
		if event.AnalysisID != review.ID || event.Operation != "analysis" || event.OperationID != first.OperationID {
			t.Fatalf("incorrect attribution: %+v", event)
		}
	}
}
