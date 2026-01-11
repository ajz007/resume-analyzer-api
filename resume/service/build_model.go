package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"resume-backend/llm/prompts"
	"resume-backend/resume/model"
)

// LLMClient defines the interface for completing prompts.
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Client is the configured LLM client used by BuildResumeModel.
var Client LLMClient

// BuildResumeModel builds a ResumeModel by calling the LLM and validating output.
func BuildResumeModel(ctx context.Context, resumeText string) (model.ResumeModel, error) {
	if Client == nil {
		return model.ResumeModel{}, errors.New("llm client is not configured")
	}

	prompt := buildResumeModelPrompt(resumeText)

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := Client.Complete(ctx, prompt)
		if err != nil {
			return model.ResumeModel{}, err
		}

		jsonPayload, err := extractJSONObject(raw)
		if err != nil {
			lastErr = err
			continue
		}

		var resumeModel model.ResumeModel
		if err := json.Unmarshal([]byte(jsonPayload), &resumeModel); err != nil {
			lastErr = err
			continue
		}

		if err := resumeModel.Validate(); err != nil {
			lastErr = err
			continue
		}

		return resumeModel, nil
	}

	if lastErr == nil {
		lastErr = errors.New("failed to build resume model")
	}
	return model.ResumeModel{}, lastErr
}

func buildResumeModelPrompt(resumeText string) string {
	return strings.TrimSpace(prompts.ResumeToModel) + "\n" + resumeText
}

func extractJSONObject(raw string) (string, error) {
	payload := strings.TrimSpace(raw)
	if payload == "" {
		return "", errors.New("empty llm response")
	}
	if json.Valid([]byte(payload)) {
		return payload, nil
	}

	start := strings.Index(payload, "{")
	end := strings.LastIndex(payload, "}")
	if start == -1 || end == -1 || end <= start {
		return "", errors.New("no json object found")
	}

	candidate := payload[start : end+1]
	if !json.Valid([]byte(candidate)) {
		return "", errors.New("invalid json object")
	}
	return candidate, nil
}
