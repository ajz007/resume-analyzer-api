package aimeter

import (
	"context"
	"encoding/json"
	"errors"
)

const (
	CodeOutputLimit      = "AI_OUTPUT_LIMIT"
	CodeDailyBudget      = "AI_DAILY_BUDGET"
	CodePaused           = "AI_PAUSED"
	CodeInputLimit       = "AI_INPUT_LIMIT"
	CodeAttemptLimit     = "AI_ATTEMPT_LIMIT"
	CodeUnavailable      = "AI_GUARD_UNAVAILABLE"
	absoluteRequestBytes = 1048576
)

// ControlError is a safeguard stop. It must never be retried as a network failure.
type ControlError struct {
	Code            string
	NoProviderSpend bool
}

func (e *ControlError) Error() string {
	switch e.Code {
	case CodeDailyBudget:
		return "AI processing is temporarily unavailable. Your saved work is available. Please try again later."
	case CodePaused:
		return "AI requests are temporarily paused. Your saved work is available. Please try again later."
	case CodeInputLimit:
		return "This request is too large for AI processing. Please shorten the resume, job description or instructions."
	case CodeOutputLimit:
		return "This request reached its AI response limit. Please shorten the input before trying again."
	case CodeAttemptLimit:
		return "This request reached its AI attempt limit. Please simplify the input before trying again."
	default:
		return "AI requests are temporarily unavailable. Your saved work is available. Please try again later."
	}
}
func Control(err error) (*ControlError, bool) {
	var e *ControlError
	ok := errors.As(err, &e)
	return e, ok
}

type guard interface {
	Admit(context.Context, *Attempt, []byte) ([]byte, error)
}

type spendPolicy struct {
	Enabled                              bool
	Daily                                int64
	RequestBytes, OutputTokens, Attempts int
}

func prepareRequest(raw []byte, p spendPolicy, model string) ([]byte, int64, error) {
	if len(raw) > p.RequestBytes || len(raw) > absoluteRequestBytes {
		return nil, 0, &ControlError{Code: CodeInputLimit}
	}
	if model != "gpt-6.1-sol" {
		return nil, 0, &ControlError{Code: CodeUnavailable}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, 0, &ControlError{Code: CodeInputLimit}
	}
	for key := range fields {
		switch key {
		case "model", "messages", "temperature", "response_format", "reasoning_effort", "max_completion_tokens", "service_tier":
		default:
			return nil, 0, &ControlError{Code: CodeInputLimit}
		}
	}
	var requested string
	if json.Unmarshal(fields["model"], &requested) != nil || requested != model {
		return nil, 0, &ControlError{Code: CodeUnavailable}
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(fields["messages"], &messages) != nil || len(messages) == 0 || len(messages) > 32 {
		return nil, 0, &ControlError{Code: CodeInputLimit}
	}
	for _, m := range messages {
		for key := range m {
			if key != "role" && key != "content" {
				return nil, 0, &ControlError{Code: CodeInputLimit}
			}
		}
		var content, role string
		if json.Unmarshal(m["content"], &content) != nil || json.Unmarshal(m["role"], &role) != nil {
			return nil, 0, &ControlError{Code: CodeInputLimit}
		}
		switch role {
		case "system", "developer", "user", "assistant":
		default:
			return nil, 0, &ControlError{Code: CodeInputLimit}
		}
	}
	if tier, exists := fields["service_tier"]; exists {
		var value string
		if json.Unmarshal(tier, &value) != nil || value != "default" {
			return nil, 0, &ControlError{Code: CodeUnavailable}
		}
	}
	output := p.OutputTokens
	if value, exists := fields["max_completion_tokens"]; exists {
		var requested int
		if json.Unmarshal(value, &requested) != nil || requested < 0 {
			return nil, 0, &ControlError{Code: CodeInputLimit}
		}
		if requested > 0 && requested < output {
			output = requested
		}
	}
	fields["max_completion_tokens"], _ = json.Marshal(output)
	fields["service_tier"] = json.RawMessage(`"default"`)
	prepared, err := json.Marshal(fields)
	if err != nil {
		return nil, 0, &ControlError{Code: CodeInputLimit}
	}
	// Conservative byte-based input allowance with extra framing/schema headroom.
	// Reserve the highest text-input/cache-write rate and a 2x safety margin.
	inputBound := int64(len(prepared) + 32768)
	inputRate, outputRate := int64(2500), int64(10000)
	if inputBound > 272000 {
		inputRate *= 2
		outputRate = 15000
	}
	reserve := 2 * (inputBound*inputRate + int64(output)*outputRate)
	return prepared, reserve, nil
}
