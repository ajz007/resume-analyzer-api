package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"resume-backend/internal/llm"
)

func TestAnalyzeResumeOmitsTemperatureForDenylist(t *testing.T) {
	oldURL := apiURL
	t.Cleanup(func() { apiURL = oldURL })

	var bodyMu sync.Mutex
	var lastBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		bodyMu.Lock()
		lastBody = payload
		bodyMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer server.Close()

	apiURL = server.URL
	_ = os.Setenv("LLM_NO_TEMP0_MODELS", "gpt-5-mini")
	t.Cleanup(func() { _ = os.Unsetenv("LLM_NO_TEMP0_MODELS") })

	client, err := NewClient("test-key", "gpt-5-mini")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.AnalyzeResume(context.Background(), llm.AnalyzeInput{
		ResumeText:     "resume",
		JobDescription: "jd",
		PromptVersion:  "v2_1",
	})
	if err != nil {
		t.Fatalf("AnalyzeResume: %v", err)
	}

	bodyMu.Lock()
	_, hasTemp := lastBody["temperature"]
	bodyMu.Unlock()
	if hasTemp {
		t.Fatalf("expected temperature to be omitted for denylisted model")
	}
}

func TestAnalyzeResumeRetriesWithoutTemperature(t *testing.T) {
	oldURL := apiURL
	t.Cleanup(func() { apiURL = oldURL })

	var reqBodies []map[string]any
	var mu sync.Mutex
	var calls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		reqBodies = append(reqBodies, payload)
		calls++
		callNum := calls
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if callNum == 1 {
			_, _ = w.Write([]byte(`{"error":{"message":"Unsupported value: 'temperature' does not support 0 with this model. Only the default (1) value is supported.","type":"invalid_request_error"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer server.Close()

	apiURL = server.URL
	_ = os.Unsetenv("LLM_NO_TEMP0_MODELS")

	client, err := NewClient("test-key", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.AnalyzeResume(context.Background(), llm.AnalyzeInput{
		ResumeText:     "resume",
		JobDescription: "jd",
		PromptVersion:  "v2_1",
	})
	if err != nil {
		t.Fatalf("AnalyzeResume: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(reqBodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqBodies))
	}
	if _, ok := reqBodies[0]["temperature"]; !ok {
		t.Fatalf("expected first request to include temperature")
	}
	if _, ok := reqBodies[1]["temperature"]; ok {
		t.Fatalf("expected retry request to omit temperature")
	}
}

func TestAnalyzeResumeNoInfiniteRetry(t *testing.T) {
	oldURL := apiURL
	t.Cleanup(func() { apiURL = oldURL })

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":{"message":"Unsupported value: 'temperature' does not support 0 with this model.","type":"invalid_request_error"}}`))
	}))
	defer server.Close()

	apiURL = server.URL
	client, err := NewClient("test-key", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.AnalyzeResume(context.Background(), llm.AnalyzeInput{
		ResumeText:     "resume",
		JobDescription: "jd",
		PromptVersion:  "v2_1",
	})
	if err == nil {
		t.Fatalf("expected error on repeated temperature unsupported response")
	}
	if calls != 2 {
		t.Fatalf("expected 2 requests (one retry), got %d", calls)
	}
}
