package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"resume-backend/internal/llm"
)

func TestReportV3RequestsStrictSchemaAndExplicitReasoning(t *testing.T) {
	original := apiURL
	t.Cleanup(func() { apiURL = original })
	t.Setenv("ANALYSIS_REASONING_EFFORT", "medium")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		format := request["response_format"].(map[string]any)
		if format["type"] != "json_schema" || format["json_schema"].(map[string]any)["strict"] != true {
			t.Error("strict schema was not sent")
		}
		if request["reasoning_effort"] != "medium" {
			t.Error("reasoning setting missing")
		}
		if _, ok := request["temperature"]; ok {
			t.Error("unsupported temperature sent")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	apiURL = server.URL
	client, err := NewClient("test-key", "gpt-6.1-sol")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.AnalyzeResume(context.Background(), llm.AnalyzeInput{ResumeText: "source", PromptVersion: "v3", ResponseSchema: json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`)})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReportV3HandlesRefusalAndTruncationWithoutSourceFreeRepair(t *testing.T) {
	for _, tc := range []struct{ name, body, expected string }{
		{"refusal", `{"choices":[{"message":{"content":"","refusal":"Cannot comply"},"finish_reason":"stop"}]}`, "refused"},
		{"truncation", `{"choices":[{"message":{"content":"{}"},"finish_reason":"length"}]}`, "truncated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := apiURL
			t.Cleanup(func() { apiURL = original })
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; _, _ = w.Write([]byte(tc.body)) }))
			defer server.Close()
			apiURL = server.URL
			client, _ := NewClient("test-key", "gpt-6.1-sol")
			_, err := client.AnalyzeResume(context.Background(), llm.AnalyzeInput{PromptVersion: "v3", ResponseSchema: json.RawMessage(`{"type":"object"}`)})
			if err == nil || !strings.Contains(err.Error(), tc.expected) || calls != 1 {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestAnswerRevisionUsesIsolatedPromptAndBoundedOutput(t *testing.T) {
	original := apiURL
	t.Cleanup(func() { apiURL = original })
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["max_completion_tokens"] != float64(8192) {
			t.Error("revision output is unbounded")
		}
		messages := request["messages"].([]any)
		if len(messages) != 2 || messages[0].(map[string]any)["content"] != "isolated revision instructions" || messages[1].(map[string]any)["content"] != "candidate context" {
			t.Error("used the full-analysis prompt instead of delta context")
		}
		if request["response_format"].(map[string]any)["type"] != "json_schema" {
			t.Error("missing strict schema")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"action\":\"no_change\"}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	apiURL = server.URL
	client, _ := NewClient("test-key", "gpt-6.1-sol")
	_, err := client.ReviseReview(context.Background(), llm.ReviewRevisionInput{SystemPrompt: "isolated revision instructions", Payload: "candidate context", ResponseSchema: json.RawMessage(`{"type":"object"}`)})
	if err != nil || calls != 1 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}
