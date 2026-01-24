package openai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"resume-backend/internal/llm"
)

const (
	apiURL = "https://api.openai.com/v1/chat/completions"
)

// Client implements llm.Client using OpenAI Chat Completions.
type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewClient constructs a new OpenAI client.
func NewClient(apiKey, model string) (*Client, error) {
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
	return &Client{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    *float32       `json:"temperature,omitempty"`
	ResponseFormat responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *Client) AnalyzeResume(ctx context.Context, input llm.AnalyzeInput) (json.RawMessage, error) {
	if strings.TrimSpace(c.model) == "" {
		return nil, fmt.Errorf("LLM_MODEL is required for OpenAI")
	}

	rawFix, hasFix := llm.FixJSONFromContext(ctx)
	if hasFix {
		return c.analyzeFixJSON(ctx, input, rawFix)
	}

	messages := BuildPrompt(input.PromptVersion, input.ResumeText, input.JobDescription, c.model)
	if extra, ok := llm.ExtraSystemMessageFromContext(ctx); ok && strings.TrimSpace(extra) != "" {
		messages = prependSystemMessage(messages, extra)
	}
	raw, usage, err := c.analyzeOnce(ctx, input, messages)
	if err != nil {
		return nil, err
	}
	logUsage(c.model, input.PromptVersion, usage)

	if json.Valid(raw) {
		return raw, nil
	}

	fixMessages := buildFixPrompt(input.PromptVersion, input.JobDescription, c.model, raw)
	raw, usage, err = c.analyzeOnce(ctx, input, fixMessages)
	if err != nil {
		return nil, err
	}
	logUsage(c.model, input.PromptVersion, usage)
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid JSON from OpenAI")
	}
	return raw, nil
}

func (c *Client) analyzeFixJSON(ctx context.Context, input llm.AnalyzeInput, raw string) (json.RawMessage, error) {
	fixMessages := buildFixPrompt(input.PromptVersion, input.JobDescription, c.model, []byte(raw))
	rawResp, usage, err := c.analyzeOnce(ctx, input, fixMessages)
	if err != nil {
		return nil, err
	}
	logUsage(c.model, input.PromptVersion, usage)
	if !json.Valid(rawResp) {
		return nil, fmt.Errorf("invalid JSON from OpenAI")
	}
	return rawResp, nil
}

func (c *Client) analyzeOnce(ctx context.Context, input llm.AnalyzeInput, messages []Message) (json.RawMessage, *chatResponseUsage, error) {
	temp := float32(0)
	if sink, ok := llm.PromptHashSinkFromContext(ctx); ok && sink != nil {
		prompt := promptStringFromMessages(messages)
		*sink = hashPromptString(prompt)
	}
	reqMessages := make([]chatMessage, 0, len(messages))
	for _, m := range messages {
		reqMessages = append(reqMessages, chatMessage{Role: m.Role, Content: m.Content})
	}
	reqBody := chatRequest{
		Model:    c.model,
		Messages: reqMessages,
		ResponseFormat: responseFormat{
			Type: "json_object",
		},
	}
	reqBody.Temperature = &temp
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Client.Timeout") {
			return nil, nil, fmt.Errorf("openai request timeout: %w", err)
		}
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, nil, fmt.Errorf("openai response parse: %w", err)
	}
	if parsed.Error != nil {
		return nil, nil, fmt.Errorf("openai error: %s (%s)", parsed.Error.Message, parsed.Error.Type)
	}
	if len(parsed.Choices) == 0 {
		return nil, nil, fmt.Errorf("openai response missing choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return nil, nil, fmt.Errorf("openai response empty content")
	}
	return json.RawMessage(content), toUsage(parsed.Usage), nil
}

type chatResponseUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func toUsage(raw *struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}) *chatResponseUsage {
	if raw == nil {
		return nil
	}
	return &chatResponseUsage{
		PromptTokens:     raw.PromptTokens,
		CompletionTokens: raw.CompletionTokens,
		TotalTokens:      raw.TotalTokens,
	}
}

func logUsage(model, promptVersion string, usage *chatResponseUsage) {
	if usage == nil {
		log.Printf("llm response model=%s prompt_version=%s", model, promptVersion)
		return
	}
	log.Printf("llm response model=%s prompt_version=%s prompt_tokens=%d completion_tokens=%d total_tokens=%d",
		model, promptVersion, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
}

func isGPT5(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-5")
}

func promptStringFromMessages(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	var b strings.Builder
	for i, m := range messages {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(m.Content)
	}
	return b.String()
}

func hashPromptString(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

var _ llm.Client = (*Client)(nil)
