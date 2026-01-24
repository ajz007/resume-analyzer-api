package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// PromptClient implements prompt completion for JSON outputs.
type PromptClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewPromptClient constructs a prompt client for JSON completions.
func NewPromptClient(apiKey, model string) (*PromptClient, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("LLM_MODEL is required for OpenAI")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required")
	}
	timeout := 120 * time.Second
	if raw := strings.TrimSpace(os.Getenv("OPENAI_TIMEOUT_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			timeout = time.Duration(parsed) * time.Second
		}
	}
	return &PromptClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// Complete returns the raw model response for the prompt.
func (c *PromptClient) Complete(ctx context.Context, prompt string) (string, error) {
	if strings.TrimSpace(c.model) == "" {
		return "", fmt.Errorf("LLM_MODEL is required for OpenAI")
	}
	messages := []chatMessage{
		{Role: "user", Content: prompt},
	}
	temp := float32(0)
	reqBody := chatRequest{
		Model:    c.model,
		Messages: messages,
		ResponseFormat: responseFormat{
			Type: "json_object",
		},
	}
	if !isGPT5(c.model) {
		reqBody.Temperature = &temp
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Client.Timeout") {
			return "", fmt.Errorf("openai request timeout: %w", err)
		}
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("openai http status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return "", fmt.Errorf("openai response parse: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("openai http status %d: %s (%s)", resp.StatusCode, parsed.Error.Message, parsed.Error.Type)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("openai http status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai response missing choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("openai response empty content")
	}
	return content, nil
}
