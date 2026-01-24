package analyses

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"resume-backend/internal/llm"
)

const v2RepairSystemMessage = "Fix the JSON to satisfy all schema constraints. Keep content same, but ensure ats.scoreBreakdown integers sum to 100. Output JSON only."

// ValidateV2WithRetry calls the LLM and validates the v2 schema with a single retry.
func ValidateV2WithRetry(ctx context.Context, client llm.Client, input llm.AnalyzeInput) (json.RawMessage, error) {
	raw, err := client.AnalyzeResume(ctx, input)
	if err != nil {
		return nil, err
	}
	if err := validateV2(raw); err == nil {
		return raw, nil
	} else {
		log.Printf("v2 validation attempt=1 error=%s", sanitizeError(err))
	}

	ctxRetry := llm.WithExtraSystemMessage(ctx, v2RepairSystemMessage)
	rawRetry, err := client.AnalyzeResume(ctxRetry, input)
	if err != nil {
		return nil, err
	}
	if err := validateV2(rawRetry); err != nil {
		log.Printf("v2 validation attempt=2 error=%s", sanitizeError(err))
		return nil, err
	}
	return rawRetry, nil
}

func validateV2(raw json.RawMessage) error {
	var parsed AnalysisResultV2
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	parsed.ATS.Score = clampScore(parsed.ATS.Score)
	parsed.ATS.ScoreBreakdown = clampScoreBreakdown(parsed.ATS.ScoreBreakdown)
	if err := parsed.Validate(); err != nil {
		return err
	}
	return nil
}
