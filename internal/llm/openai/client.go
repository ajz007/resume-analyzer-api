package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
	return &Client{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
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
	Temperature    float32        `json:"temperature"`
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

	raw, usage, err := c.analyzeOnce(ctx, input, systemPromptStrict, developerPrompt(input.PromptVersion), userPrompt(input))
	if err != nil {
		return nil, err
	}
	logUsage(c.model, input.PromptVersion, usage)

	if json.Valid(raw) {
		return raw, nil
	}

	raw, usage, err = c.analyzeOnce(ctx, input, systemPromptFixJSON, developerPrompt(input.PromptVersion), fixUserPrompt(raw))
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
	rawResp, usage, err := c.analyzeOnce(ctx, input, systemPromptFixJSON, developerPrompt(input.PromptVersion), fixUserPrompt([]byte(raw)))
	if err != nil {
		return nil, err
	}
	logUsage(c.model, input.PromptVersion, usage)
	if !json.Valid(rawResp) {
		return nil, fmt.Errorf("invalid JSON from OpenAI")
	}
	return rawResp, nil
}

func (c *Client) analyzeOnce(ctx context.Context, input llm.AnalyzeInput, systemPrompt, devPrompt, user string) (json.RawMessage, *chatResponseUsage, error) {
	reqBody := chatRequest{
		Model:       c.model,
		Messages:    []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "developer", Content: devPrompt}, {Role: "user", Content: user}},
		Temperature: 0,
		ResponseFormat: responseFormat{
			Type: "json_object",
		},
	}
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

const systemPromptStrict = "You are a resume analysis engine. Respond with JSON only. Output must match the schema exactly."

const systemPromptFixJSON = "You are a JSON repair tool. Return only valid JSON that matches the schema exactly."

func developerPrompt(promptVersion string) string {
	return fmt.Sprintf(`You must output JSON that matches this schema exactly:
{
  "summary": {
    "overallAssessment": "string",
    "strengths": ["string"],
    "weaknesses": ["string"]
  },
  "ats": {
    "score": "number (0-100)",
    "missingKeywords": ["string"],
    "formattingIssues": ["string"]
  },
  "issues": [
    {
      "severity": "critical | high | medium | low",
      "section": "string",
      "problem": "string",
      "whyItMatters": "string",
      "suggestion": "string"
    }
  ],
  "bulletRewrites": [
    {
      "section": "string",
      "before": "string",
      "after": "string",
      "rationale": "string"
    }
  ],
  "missingInformation": ["string"],
  "actionPlan": {
    "quickWins": ["string"],
    "mediumEffort": ["string"],
    "deepFixes": ["string"]
  }
}
Prompt version: %s`, promptVersion)
}

func userPrompt(input llm.AnalyzeInput) string {
	jobDescription := input.JobDescription
	if strings.TrimSpace(jobDescription) == "" {
		jobDescription = "N/A"
	}
	targetRole := input.TargetRole
	if strings.TrimSpace(targetRole) == "" {
		targetRole = "N/A"
	}
	return fmt.Sprintf("Resume Text:\n%s\n\nJob Description:\n%s\n\nTarget Role:\n%s",
		input.ResumeText, jobDescription, targetRole)
}

func fixUserPrompt(raw []byte) string {
	return fmt.Sprintf("Fix this JSON to match the schema exactly. Output JSON only:\n%s", string(raw))
}

var _ llm.Client = (*Client)(nil)
