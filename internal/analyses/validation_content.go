package analyses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"resume-backend/internal/llm"
)

const contentRepairSystemMessage = "Remove any unsupported impact claims (e.g., double-digit, significant) unless explicitly stated in resume. Never use \"double-digit\" unless it appears verbatim in resume evidence. If an exact value is missing, replace with placeholder \"X% (replace with exact figure)\", set claimSupport=placeholder, metricsSource=placeholder, and add placeholdersNeeded (e.g., revenue_growth_pct). Keep JSON only."

var forbiddenImpactTerms = []string{
	"double-digit",
	"double digit",
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
			changed, _ := sanitizeBulletRewriteTerms(&parsed)
			if changed {
				if err := parsed.Validate(); err != nil {
					return nil, err
				}
				if err := ValidateContentV2_3(&parsed); err == nil {
					payload, marshalErr := json.Marshal(parsed)
					if marshalErr != nil {
						return nil, marshalErr
					}
					return payload, nil
				}
			}
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
	out.ATS.Score = clampScore(out.ATS.Score)
	out.ATS.ScoreBreakdown = clampScoreBreakdown(out.ATS.ScoreBreakdown)
	return out.Validate()
}

func parseAndValidateV2_3(raw []byte, out *AnalysisResultV2_3) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	out.ATS.Score = clampScore(out.ATS.Score)
	out.ATS.ScoreBreakdown = clampScoreBreakdown(out.ATS.ScoreBreakdown)
	return out.Validate()
}

func containsForbiddenTerm(text string) (string, bool) {
	lower := normalizeForMatch(text)
	for _, term := range forbiddenImpactTerms {
		if strings.Contains(lower, term) {
			return term, true
		}
	}
	return "", false
}

func sanitizeBulletRewriteTerms(r *AnalysisResultV2_3) (bool, []string) {
	if r == nil {
		return false, nil
	}
	changed := false
	var notes []string
	for i := range r.BulletRewrites {
		after := r.BulletRewrites[i].After
		if after == "" {
			continue
		}
		updated, replacements := replaceForbiddenTerms(after)
		if len(replacements) == 0 {
			continue
		}
		r.BulletRewrites[i].After = updated
		r.BulletRewrites[i].ClaimSupport = "placeholder"
		r.BulletRewrites[i].MetricsSource = "placeholder"
		r.BulletRewrites[i].Evidence = "notFound"
		if r.BulletRewrites[i].PlaceholdersNeeded == nil {
			r.BulletRewrites[i].PlaceholdersNeeded = []string{}
		}
		addPlaceholderNeeded(&r.BulletRewrites[i], "revenue_growth_pct")
		appendRationalePlaceholder(&r.BulletRewrites[i])
		changed = true
		for _, repl := range replacements {
			notes = append(notes, "bulletRewrites["+strconv.Itoa(i)+"] replaced "+repl)
		}
	}
	return changed, notes
}

func replaceForbiddenTerms(input string) (string, []string) {
	replacements := map[string]string{
		"double-digit": "X% (replace with exact figure)",
		"double digit": "X% (replace with exact figure)",
		"significant":  "measurable",
		"substantial":  "measurable",
		"massive":      "measurable",
		"remarkable":   "measurable",
	}
	updated := input
	normalized := normalizeForMatch(updated)
	var applied []string
	for term, repl := range replacements {
		if strings.Contains(normalized, term) {
			for _, variant := range termVariants(term) {
				updated = replaceInsensitive(updated, variant, repl)
			}
			applied = append(applied, term+"->"+repl)
			normalized = normalizeForMatch(updated)
		}
	}
	return updated, applied
}

func normalizeForMatch(text string) string {
	lower := strings.ToLower(text)
	for _, r := range []string{"\u2010", "\u2011", "\u2012", "\u2013", "\u2014", "\u2212"} {
		lower = strings.ReplaceAll(lower, r, "-")
	}
	lower = strings.Join(strings.Fields(lower), " ")
	return lower
}

func termVariants(term string) []string {
	var variants []string
	variants = append(variants, term)
	if strings.Contains(term, "-") {
		variants = append(variants, strings.ReplaceAll(term, "-", " "))
		for _, r := range []string{"\u2010", "\u2011", "\u2012", "\u2013", "\u2014", "\u2212"} {
			variants = append(variants, strings.ReplaceAll(term, "-", r))
		}
	}
	return variants
}

func replaceInsensitive(input, term, replacement string) string {
	out := input
	out = strings.ReplaceAll(out, term, replacement)
	out = strings.ReplaceAll(out, strings.ToUpper(term), replacement)
	out = strings.ReplaceAll(out, strings.Title(term), replacement)
	return out
}

func addPlaceholderNeeded(br *BulletRewriteV2_3, placeholder string) {
	if br == nil {
		return
	}
	for _, item := range br.PlaceholdersNeeded {
		if strings.EqualFold(item, placeholder) {
			return
		}
	}
	br.PlaceholdersNeeded = append(br.PlaceholdersNeeded, placeholder)
}

func appendRationalePlaceholder(br *BulletRewriteV2_3) {
	if br == nil {
		return
	}
	if strings.Contains(strings.ToLower(br.Rationale), "replace placeholders before final submission") {
		return
	}
	if strings.TrimSpace(br.Rationale) == "" {
		br.Rationale = "Replace placeholders before final submission."
		return
	}
	br.Rationale = strings.TrimSpace(br.Rationale) + " Replace placeholders before final submission."
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
