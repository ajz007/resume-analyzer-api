package analyses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"resume-backend/internal/llm"
)

const contentRepairSystemMessage = "Remove any unsupported impact claims (e.g., double-digit, significant) unless explicitly stated in resume. Keep JSON only."

var forbiddenImpactTerms = []string{
	"double-digit",
	"significant",
	"substantial",
	"massive",
	"remarkable",
}

// ValidateContentV2_2 enforces content guardrails for v2_2 outputs.
func ValidateContentV2_2(r *AnalysisResultV2_2) error {
	if r == nil {
		return errors.New("analysis result is nil")
	}
	for i, br := range r.BulletRewrites {
		if term, ok := containsForbiddenTerm(br.After); ok {
			switch strings.ToLower(strings.TrimSpace(br.MetricsSource)) {
			case "resume":
				return fmt.Errorf("bulletRewrites[%d].after contains unsupported term %q", i, term)
			case "placeholder":
				if len(br.PlaceholdersNeeded) == 0 {
					return fmt.Errorf("bulletRewrites[%d].placeholdersNeeded required when using placeholders with %q", i, term)
				}
			}
		}
	}
	return nil
}

// ValidateContentV2_3 enforces content guardrails for v2_3 outputs.
func ValidateContentV2_3(r *AnalysisResultV2_3) error {
	if r == nil {
		return errors.New("analysis result is nil")
	}
	for i, br := range r.BulletRewrites {
		if term, ok := containsForbiddenTerm(br.After); ok {
			switch strings.ToLower(strings.TrimSpace(br.MetricsSource)) {
			case "resume":
				return fmt.Errorf("bulletRewrites[%d].after contains unsupported term %q", i, term)
			case "placeholder":
				if len(br.PlaceholdersNeeded) == 0 {
					return fmt.Errorf("bulletRewrites[%d].placeholdersNeeded required when using placeholders with %q", i, term)
				}
			}
		}
	}
	return nil
}

// ValidateV2_2WithRetry validates v2_2 schema and content guardrails with one retry.
func ValidateV2_2WithRetry(ctx context.Context, client llm.Client, input llm.AnalyzeInput) (rawJSON []byte, err error) {
	raw, err := client.AnalyzeResume(ctx, input)
	if err != nil {
		return nil, err
	}
	var parsed AnalysisResultV2_2
	if err := parseAndValidateV2_2(raw, &parsed); err != nil {
		return nil, err
	}
	if err := ValidateContentV2_2(&parsed); err != nil {
		log.Printf("v2_2 content attempt=1 error=%s", sanitizeError(err))
		ctxRetry := llm.WithExtraSystemMessage(ctx, contentRepairSystemMessage)
		rawRetry, retryErr := client.AnalyzeResume(ctxRetry, input)
		if retryErr != nil {
			return nil, retryErr
		}
		if err := parseAndValidateV2_2(rawRetry, &parsed); err != nil {
			return nil, err
		}
		if err := ValidateContentV2_2(&parsed); err != nil {
			log.Printf("v2_2 content attempt=2 error=%s", sanitizeError(err))
			return nil, err
		}
		return rawRetry, nil
	}
	return raw, nil
}

// ValidateV2_3WithRetry validates v2_3 schema and content guardrails with one retry.
func ValidateV2_3WithRetry(ctx context.Context, client llm.Client, input llm.AnalyzeInput) (rawJSON []byte, err error) {
	raw, err := client.AnalyzeResume(ctx, input)
	if err != nil {
		return nil, err
	}
	var parsed AnalysisResultV2_3
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	SanitizeV2_3(&parsed)
	if err := parsed.Validate(); err != nil {
		return nil, err
	}
	if err := ValidateContentV2_3(&parsed); err != nil {
		log.Printf("v2_3 content attempt=1 error=%s", sanitizeError(err))
		ctxRetry := llm.WithExtraSystemMessage(ctx, contentRepairSystemMessage)
		rawRetry, retryErr := client.AnalyzeResume(ctxRetry, input)
		if retryErr != nil {
			return nil, retryErr
		}
		if err := json.Unmarshal(rawRetry, &parsed); err != nil {
			return nil, err
		}
		SanitizeV2_3(&parsed)
		if err := parsed.Validate(); err != nil {
			return nil, err
		}
		if err := ValidateContentV2_3(&parsed); err != nil {
			log.Printf("v2_3 content attempt=2 error=%s", sanitizeError(err))
			return nil, err
		}
		return rawRetry, nil
	}
	return raw, nil
}

func parseAndValidateV2_2(raw []byte, out *AnalysisResultV2_2) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	return out.Validate()
}

func parseAndValidateV2_3(raw []byte, out *AnalysisResultV2_3) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	return out.Validate()
}

func containsForbiddenTerm(text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, term := range forbiddenImpactTerms {
		if strings.Contains(lower, term) {
			return term, true
		}
	}
	return "", false
}

// SanitizeV2_3 trims and normalizes display-only fields before content validation.
func SanitizeV2_3(r *AnalysisResultV2_3) {
	if r == nil {
		return
	}
	for i := range r.Issues {
		r.Issues[i].Evidence = sanitizeEvidence(r.Issues[i].Evidence, 160)
	}
	for i := range r.BulletRewrites {
		r.BulletRewrites[i].Evidence = sanitizeEvidence(r.BulletRewrites[i].Evidence, 160)
	}
}

func sanitizeEvidence(value string, maxRunes int) string {
	normalized := normalizeWhitespace(value)
	if strings.EqualFold(normalized, "notFound") {
		return "notFound"
	}
	return truncateWithEllipsis(normalized, maxRunes)
}

func normalizeWhitespace(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.Join(strings.Fields(trimmed), " ")
}

func truncateWithEllipsis(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}
