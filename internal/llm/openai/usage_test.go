package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"resume-backend/internal/aimeter"
	"resume-backend/internal/llm"
)

type usageSpy struct {
	mu     sync.Mutex
	events []aimeter.Attempt
}

func (r *usageSpy) Start(context.Context, aimeter.Attempt) error { return nil }
func (r *usageSpy) Finish(_ context.Context, a aimeter.Attempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, a)
	return nil
}
func usageResponse(content, refusal, finish string) map[string]any {
	return map[string]any{
		"id": "chatcmpl_test", "model": "gpt-6.1-sol", "service_tier": "default",
		"choices": []any{map[string]any{"message": map[string]any{"content": content, "refusal": refusal}, "finish_reason": finish}},
		"usage":   map[string]int{"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150},
	}
}
func TestUsageCountsInternalJSONRepairSeparately(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		content := "broken JSON"
		if calls == 2 {
			content = "{}"
		}
		_ = json.NewEncoder(w).Encode(usageResponse(content, "", "stop"))
	}))
	defer server.Close()
	old := apiURL
	apiURL = server.URL
	defer func() { apiURL = old }()
	client, err := NewClient("fictional-key", "gpt-6.1-sol")
	if err != nil {
		t.Fatal(err)
	}
	spy := &usageSpy{}
	client.SetUsageRecorder(spy)
	raw, err := client.AnalyzeResume(aimeter.WithOperation(context.Background(), "analysis", "11111111-1111-4111-8111-111111111111", ""), llm.AnalyzeInput{ResumeText: "Private source", PromptVersion: "v1"})
	if err != nil || string(raw) != "{}" || len(spy.events) != 2 {
		t.Fatalf("raw=%s err=%v events=%d", raw, err, len(spy.events))
	}
	if spy.events[0].Stage != "primary" || spy.events[1].Stage != "json_repair" || spy.events[0].OperationID != spy.events[1].OperationID || spy.events[1].Number != 2 {
		t.Fatalf("%+v", spy.events)
	}
	for _, event := range spy.events {
		if event.UsageState != "known" || event.EstimateNanoUSD == nil {
			t.Fatalf("%+v", event)
		}
	}
}
func TestUsageSurvivesRefusalAndOutputTruncation(t *testing.T) {
	for _, kind := range []string{"refusal", "truncation"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				refusal, finish := "", "stop"
				if kind == "refusal" {
					refusal = "Cannot comply"
				} else {
					finish = "length"
				}
				_ = json.NewEncoder(w).Encode(usageResponse("{}", refusal, finish))
			}))
			defer server.Close()
			old := apiURL
			apiURL = server.URL
			defer func() { apiURL = old }()
			client, _ := NewClient("fictional-key", "gpt-6.1-sol")
			spy := &usageSpy{}
			client.SetUsageRecorder(spy)
			if _, err := client.AnalyzeResume(context.Background(), llm.AnalyzeInput{PromptVersion: "v1"}); err == nil {
				t.Fatal("expected application rejection")
			}
			if len(spy.events) != 1 || spy.events[0].UsageState != "known" || spy.events[0].OutputTokens == nil || *spy.events[0].OutputTokens != 50 {
				t.Fatalf("%+v", spy.events)
			}
		})
	}
}
func TestUsageCountsParameterRetryAndPreservesPromptClientCoverage(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"temperature does not support 0","type":"invalid_request_error"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(usageResponse("{}", "", "stop"))
	}))
	defer server.Close()
	old := apiURL
	apiURL = server.URL
	defer func() { apiURL = old }()
	client, _ := NewClient("fictional-key", "gpt-4o")
	client.noTemp0Models = nil
	spy := &usageSpy{}
	client.SetUsageRecorder(spy)
	if _, err := client.AnalyzeResume(context.Background(), llm.AnalyzeInput{PromptVersion: "v1"}); err != nil {
		t.Fatal(err)
	}
	if len(spy.events) != 2 || spy.events[0].UsageState != "unknown" || spy.events[1].Retry != "parameter" || spy.events[0].OperationID != spy.events[1].OperationID {
		t.Fatalf("%+v", spy.events)
	}
	prompt, _ := NewPromptClient("fictional-key", "gpt-6.1-sol")
	prompt.SetUsageRecorder(spy)
	ctx := aimeter.WithOperation(context.Background(), "resume_generation", "", "22222222-2222-4222-8222-222222222222")
	if _, err := prompt.Complete(ctx, "Private resume prompt"); err != nil {
		t.Fatal(err)
	}
	if len(spy.events) != 3 || spy.events[2].Operation != "resume_generation" || spy.events[2].ObjectID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("%+v", spy.events)
	}
}
