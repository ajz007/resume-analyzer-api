package aimeter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryRecorder struct {
	mu               sync.Mutex
	starts, finishes []Attempt
	fail             bool
}

func (r *memoryRecorder) Start(ctx context.Context, a Attempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	r.starts = append(r.starts, a)
	if r.fail {
		return errors.New("private database failure")
	}
	return nil
}
func (r *memoryRecorder) Finish(ctx context.Context, a Attempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	r.finishes = append(r.finishes, a)
	if r.fail {
		return errors.New("private database failure")
	}
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const usageBody = `{"id":"chatcmpl_test","model":"gpt-6.1-sol","service_tier":"default","choices":[{"message":{"content":"DO_NOT_STORE_RESUME"}}],"usage":{"prompt_tokens":1000,"completion_tokens":300,"total_tokens":1300,"prompt_tokens_details":{"cached_tokens":400,"cache_write_tokens":100},"completion_tokens_details":{"reasoning_tokens":250}}}`

func observe(t *testing.T, body string, status int) (Attempt, string) {
	t.Helper()
	r := &memoryRecorder{}
	tr := &Transport{Model: "gpt-6.1-sol", Recorder: r, Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{"X-Request-Id": []string{"req_test"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	req, _ := http.NewRequestWithContext(WithOperation(context.Background(), "analysis", "11111111-1111-4111-8111-111111111111", ""), "POST", "https://provider.example", strings.NewReader("PRIVATE_PROMPT"))
	req.Header.Set("Authorization", "Bearer PRIVATE_KEY")
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	_ = resp.Body.Close()
	if len(r.starts) != 1 || len(r.finishes) != 1 {
		t.Fatalf("start=%d finish=%d", len(r.starts), len(r.finishes))
	}
	if r.starts[0].UsageState != "pending" || r.starts[0].EstimateNanoUSD != nil {
		t.Fatal("unobserved request was priced")
	}
	a := r.finishes[0]
	encoded, _ := json.Marshal(a)
	for _, private := range []string{"PRIVATE_PROMPT", "PRIVATE_KEY", "DO_NOT_STORE_RESUME"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private content entered usage record")
		}
	}
	return a, string(data)
}
func TestObservedUsageIncludesCachedWritesAndReasoningWithoutDoubleCounting(t *testing.T) {
	a, body := observe(t, usageBody, 200)
	if body != usageBody || a.UsageState != "known" || a.EstimateNanoUSD == nil || *a.EstimateNanoUSD != 4290000 || a.ProviderRequestID != "req_test" || *a.ReasoningTokens != 250 {
		t.Fatalf("%+v", a)
	}
}
func TestUnknownUsageNeverBecomesZeroCost(t *testing.T) {
	for _, tc := range []struct{ name, body, reason string }{
		{"missing", `{"model":"gpt-6.1-sol"}`, "missing_usage"},
		{"incomplete", `{"usage":{"prompt_tokens":3}}`, "incomplete_usage"},
		{"invalid JSON", `{`, "invalid_response"},
		{"negative", `{"usage":{"prompt_tokens":-1,"completion_tokens":1,"total_tokens":0}}`, "invalid_usage"},
		{"mismatched total", `{"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":4}}`, "invalid_usage"},
		{"invalid cache", `{"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"prompt_tokens_details":{"cached_tokens":4}}}`, "invalid_usage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := observe(t, tc.body, 200)
			if a.UsageState != "unknown" || a.UnknownReason != tc.reason || a.EstimateNanoUSD != nil || a.InputTokens != nil {
				t.Fatalf("%+v", a)
			}
		})
	}
}
func TestUsageRetainedForHTTPFailure(t *testing.T) {
	a, _ := observe(t, usageBody, 500)
	if a.UsageState != "known" || a.Outcome != "http_error" || a.EstimateNanoUSD == nil {
		t.Fatalf("%+v", a)
	}
}
func TestOversizedResponseIsUnmodifiedAndUsageUnknown(t *testing.T) {
	body := `{"content":"` + strings.Repeat("x", maxCaptureBytes) + `"}`
	a, received := observe(t, body, 200)
	if received != body || a.UnknownReason != "capture_limit" || a.EstimateNanoUSD != nil {
		t.Fatalf("%+v", a)
	}
}
func TestTransportTimeoutRecordsUnknownUsingIndependentCleanupContext(t *testing.T) {
	r := &memoryRecorder{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tr := &Transport{Recorder: r, Model: "gpt-6.1-sol", Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://provider.example", nil)
	if _, err := tr.RoundTrip(req); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if len(r.finishes) != 1 || r.finishes[0].UsageState != "unknown" || r.finishes[0].EstimateNanoUSD != nil || r.finishes[0].Outcome != "timeout_or_cancelled" {
		t.Fatalf("%+v", r.finishes)
	}
}
func TestAccountingFailureDoesNotRepeatProviderRequest(t *testing.T) {
	r := &memoryRecorder{fail: true}
	calls := 0
	tr := &Transport{Recorder: r, Model: "gpt-6.1-sol", Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(usageBody))}, nil
	})}
	req, _ := http.NewRequest("POST", "https://provider.example", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || string(data) != usageBody || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
func TestIncompleteBodyAndPreResponsePendingAreVisible(t *testing.T) {
	r := &memoryRecorder{}
	tr := &Transport{Recorder: r, Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(usageBody))}, nil
	})}
	req, _ := http.NewRequest("POST", "https://provider.example", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.starts) != 1 || len(r.finishes) != 0 {
		t.Fatal("pending call was finalized before reading response")
	}
	_ = resp.Body.Close()
	if len(r.finishes) != 1 || r.finishes[0].UnknownReason != "incomplete_body" {
		t.Fatalf("%+v", r.finishes)
	}
}
func TestConcurrentAttemptsShareOperationWithUniqueSequence(t *testing.T) {
	r := &memoryRecorder{}
	tr := &Transport{Recorder: r, Model: "gpt-6.1-sol", Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(usageBody))}, nil
	})}
	ctx := WithOperation(context.Background(), "analysis", "", "")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequestWithContext(WithRetry(WithStage(ctx, "validation_repair"), "transient"), "POST", "https://provider.example", nil)
			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()
	}
	wg.Wait()
	seen := map[int64]bool{}
	op := ""
	for _, a := range r.finishes {
		if op == "" {
			op = a.OperationID
		}
		if a.OperationID != op || seen[a.Number] || a.Stage != "validation_repair" || a.Retry != "transient" {
			t.Fatal(fmt.Sprintf("bad scope: %+v", a))
		}
		seen[a.Number] = true
	}
	if len(seen) != 20 {
		t.Fatal("missing attempts")
	}
}

func TestReplacingRecorderDoesNotDoubleCountPhysicalCall(t *testing.T) {
	old, current := &memoryRecorder{}, &memoryRecorder{}
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(usageBody))}, nil
	})
	tr := NewTransport(NewTransport(base, old, "gpt-6.1-sol"), current, "gpt-6.1-sol")
	req, _ := http.NewRequest("POST", "https://provider.example", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if len(old.finishes) != 0 || len(current.finishes) != 1 {
		t.Fatal("recorded a physical request twice")
	}
}
