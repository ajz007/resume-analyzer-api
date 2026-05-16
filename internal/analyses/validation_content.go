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

// ValidateContentV2_4 enforces content guardrails for v2_4 outputs.
func ValidateContentV2_4(r *AnalysisResultV2_4) error {
	if r == nil {
		return errors.New("analysis result is nil")
	}
	base := AnalysisResultV2_3{
		Meta:               r.Meta,
		Summary:            r.Summary,
		ATS:                r.ATS,
		Issues:             r.Issues,
		BulletRewrites:     r.BulletRewrites,
		MissingInformation: r.MissingInformation,
		ActionPlan:         r.ActionPlan,
	}
	if err := ValidateContentV2_3(&base); err != nil {
		return err
	}
	if field, phrase, ok := containsHardGuaranteeV2_4(r); ok {
		return fmt.Errorf("%s contains unsafe guarantee phrase %q", field, phrase)
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
	issues, err := parseRecoverValidateV2_3(raw, &parsed)
	logValidationTelemetry(ctx, "analysis.validation.attempt", "v2_3", issues)
	if err != nil {
		return nil, err
	}
	if err := ValidateContentV2_3(&parsed); err != nil {
		log.Printf("v2_3 content attempt=1 error=%s", sanitizeError(err))
		ctxRetry := llm.WithExtraSystemMessage(ctx, contentRepairSystemMessage)
		rawRetry, retryErr := client.AnalyzeResume(ctxRetry, input)
		if retryErr != nil {
			return nil, retryErr
		}
		issues, err := parseRecoverValidateV2_3(rawRetry, &parsed)
		logValidationTelemetry(ctx, "analysis.validation.retry", "v2_3", issues)
		if err != nil {
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
		return marshalRecoveredPayload(rawRetry, parsed, issues)
	}
	return marshalRecoveredPayload(raw, parsed, issues)
}

// ValidateV2_4WithRetry validates v2_4 schema and content guardrails with one retry.
func ValidateV2_4WithRetry(ctx context.Context, client llm.Client, input llm.AnalyzeInput) (rawJSON []byte, err error) {
	raw, err := client.AnalyzeResume(ctx, input)
	if err != nil {
		return nil, err
	}
	var parsed AnalysisResultV2_4
	issues, err := parseRecoverValidateV2_4(raw, &parsed)
	logValidationTelemetry(ctx, "analysis.validation.attempt", "v2_4", issues)
	if err != nil {
		return nil, err
	}
	if err := ValidateContentV2_4(&parsed); err != nil {
		log.Printf("v2_4 content attempt=1 error=%s", sanitizeError(err))
		ctxRetry := llm.WithExtraSystemMessage(ctx, contentRepairSystemMessage)
		rawRetry, retryErr := client.AnalyzeResume(ctxRetry, input)
		if retryErr != nil {
			return nil, retryErr
		}
		issues, err := parseRecoverValidateV2_4(rawRetry, &parsed)
		logValidationTelemetry(ctx, "analysis.validation.retry", "v2_4", issues)
		if err != nil {
			return nil, err
		}
		if err := ValidateContentV2_4(&parsed); err != nil {
			log.Printf("v2_4 content attempt=2 error=%s", sanitizeError(err))
			changedTerms, _ := sanitizeBulletRewriteTermsV2_4(&parsed)
			changedGuarantees := sanitizeHardGuaranteesV2_4(&parsed)
			if changedTerms || changedGuarantees {
				SanitizeV2_4(&parsed)
				if err := parsed.Validate(); err != nil {
					return nil, err
				}
				if err := ValidateContentV2_4(&parsed); err == nil {
					payload, marshalErr := json.Marshal(parsed)
					if marshalErr != nil {
						return nil, marshalErr
					}
					return payload, nil
				}
			}
			return nil, err
		}
		return marshalRecoveredPayload(rawRetry, parsed, issues)
	}
	return marshalRecoveredPayload(raw, parsed, issues)
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

func parseAndValidateV2_4(raw []byte, out *AnalysisResultV2_4) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	SanitizeV2_4(out)
	return out.Validate()
}

func parseRecoverValidateV2_3(raw []byte, out *AnalysisResultV2_3) ([]ValidationIssue, error) {
	if err := json.Unmarshal(raw, out); err != nil {
		return []ValidationIssue{{Path: "$", Message: err.Error(), Severity: ValidationFatal}}, err
	}
	issues := recoverAnalysisV2_3(out)
	issues = append(issues, validateAnalysisV2_3(out)...)
	if fatal := fatalValidationIssues(issues); len(fatal) > 0 {
		return issues, ValidationIssuesError{Issues: fatal}
	}
	return issues, nil
}

func parseRecoverValidateV2_4(raw []byte, out *AnalysisResultV2_4) ([]ValidationIssue, error) {
	if err := json.Unmarshal(raw, out); err != nil {
		return []ValidationIssue{{Path: "$", Message: err.Error(), Severity: ValidationFatal}}, err
	}
	issues := recoverAnalysisV2_4(out)
	issues = append(issues, validateAnalysisV2_4(out)...)
	if fatal := fatalValidationIssues(issues); len(fatal) > 0 {
		return issues, ValidationIssuesError{Issues: fatal}
	}
	return issues, nil
}

func marshalRecoveredPayload[T any](raw []byte, parsed T, issues []ValidationIssue) ([]byte, error) {
	counts := countValidationIssuesBySeverity(issues)
	if counts[ValidationRecoverable] == 0 && counts[ValidationWarning] == 0 {
		return raw, nil
	}
	payload, err := json.Marshal(parsed)
	if err != nil {
		return nil, err
	}
	return payload, nil
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

func sanitizeBulletRewriteTermsV2_4(r *AnalysisResultV2_4) (bool, []string) {
	if r == nil {
		return false, nil
	}
	base := AnalysisResultV2_3{
		Meta:               r.Meta,
		Summary:            r.Summary,
		ATS:                r.ATS,
		Issues:             r.Issues,
		BulletRewrites:     r.BulletRewrites,
		MissingInformation: r.MissingInformation,
		ActionPlan:         r.ActionPlan,
	}
	changed, notes := sanitizeBulletRewriteTerms(&base)
	r.BulletRewrites = base.BulletRewrites
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
		r.Issues[i].RequiresUserInput = sanitizeIssueRequiresUserInput(
			r.Issues[i].RequiresUserInput,
			r.Issues[i].AutoFixable,
		)
	}
	for i := range r.BulletRewrites {
		r.BulletRewrites[i].Evidence = sanitizeEvidence(r.BulletRewrites[i].Evidence, 160)
	}
}

func sanitizeIssueRequiresUserInput(values []string, autoFixable bool) []string {
	if autoFixable {
		return []string{}
	}
	if values == nil {
		return []string{}
	}

	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized, ok := normalizeUserInputKey(value)
		if !ok || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	return out
}

// SanitizeV2_4 trims and normalizes display-only fields before content validation.
func SanitizeV2_4(r *AnalysisResultV2_4) {
	if r == nil {
		return
	}
	base := AnalysisResultV2_3{
		Meta:               r.Meta,
		Summary:            r.Summary,
		ATS:                r.ATS,
		Issues:             r.Issues,
		BulletRewrites:     r.BulletRewrites,
		MissingInformation: r.MissingInformation,
		ActionPlan:         r.ActionPlan,
	}
	SanitizeV2_3(&base)
	r.Issues = base.Issues
	r.BulletRewrites = base.BulletRewrites

	if r.Meta.Assumptions == nil {
		r.Meta.Assumptions = []string{}
	}
	if r.Meta.Limitations == nil {
		r.Meta.Limitations = []string{}
	}
	if r.Summary.Strengths == nil {
		r.Summary.Strengths = []string{}
	}
	if r.Summary.Weaknesses == nil {
		r.Summary.Weaknesses = []string{}
	}
	if r.ATS.ScoreReasoning == nil {
		r.ATS.ScoreReasoning = []string{}
	}
	if r.ATS.ScoreExplanation.Components == nil {
		r.ATS.ScoreExplanation.Components = []ScoreComponentV1{}
	}
	for i := range r.ATS.ScoreExplanation.Components {
		if r.ATS.ScoreExplanation.Components[i].Helped == nil {
			r.ATS.ScoreExplanation.Components[i].Helped = []string{}
		}
		if r.ATS.ScoreExplanation.Components[i].Dragged == nil {
			r.ATS.ScoreExplanation.Components[i].Dragged = []string{}
		}
	}
	if r.ATS.MissingKeywords.FromJobDescription == nil {
		r.ATS.MissingKeywords.FromJobDescription = []string{}
	}
	if r.ATS.MissingKeywords.IndustryCommon == nil {
		r.ATS.MissingKeywords.IndustryCommon = []string{}
	}
	if r.ATS.FormattingIssues == nil {
		r.ATS.FormattingIssues = []string{}
	}
	for i := range r.Issues {
		if r.Issues[i].RequiresUserInput == nil {
			r.Issues[i].RequiresUserInput = []string{}
		}
	}
	for i := range r.BulletRewrites {
		if r.BulletRewrites[i].PlaceholdersNeeded == nil {
			r.BulletRewrites[i].PlaceholdersNeeded = []string{}
		}
	}
	if r.MissingInformation == nil {
		r.MissingInformation = []string{}
	}
	if r.ActionPlan.QuickWins == nil {
		r.ActionPlan.QuickWins = []string{}
	}
	if r.ActionPlan.MediumEffort == nil {
		r.ActionPlan.MediumEffort = []string{}
	}
	if r.ActionPlan.DeepFixes == nil {
		r.ActionPlan.DeepFixes = []string{}
	}
	if r.JobRequirementProfile.TopPriorities == nil {
		r.JobRequirementProfile.TopPriorities = []JobPriorityV1{}
	}
	if r.JobRequirementProfile.HiddenExpectations == nil {
		r.JobRequirementProfile.HiddenExpectations = []HiddenExpectationV1{}
	}
	if r.JobRequirementProfile.NiceToHaveSignals == nil {
		r.JobRequirementProfile.NiceToHaveSignals = []NiceToHaveSignalV1{}
	}
	if r.JobMatchScoring.RequirementScores == nil {
		r.JobMatchScoring.RequirementScores = []RequirementScoreV1{}
	}
	for i := range r.JobMatchScoring.RequirementScores {
		r.JobMatchScoring.RequirementScores[i].Evidence = sanitizeEvidence(r.JobMatchScoring.RequirementScores[i].Evidence, 200)
	}
	if r.AIScreening.ScoreBreakdown == nil {
		r.AIScreening.ScoreBreakdown = []AIScreeningBreakdownItemV1{}
	}
	r.AIScreening.AIRecruiterVerdict.OneLineVerdict = sanitizeTextFieldV2_4(r.AIScreening.AIRecruiterVerdict.OneLineVerdict, 500)
	r.AIScreening.AIRecruiterVerdict.MainConcern = sanitizeTextFieldV2_4(r.AIScreening.AIRecruiterVerdict.MainConcern, 500)
	r.AIScreening.AIRecruiterVerdict.StrongestSignal = sanitizeTextFieldV2_4(r.AIScreening.AIRecruiterVerdict.StrongestSignal, 500)
	r.AIScreening.AIRecruiterVerdict.WeakestSignal = sanitizeTextFieldV2_4(r.AIScreening.AIRecruiterVerdict.WeakestSignal, 500)
	if r.FixThisFirst == nil {
		r.FixThisFirst = []FixThisFirstItemV1{}
	}
}

var hardGuaranteePhrasesV2_4 = []string{
	"guaranteed interview",
	"guaranteed shortlist",
	"will be selected",
	"will pass ai filter",
	"bypass ats",
	"beat the ats",
}

func containsHardGuaranteeV2_4(r *AnalysisResultV2_4) (string, string, bool) {
	if r == nil {
		return "", "", false
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "jobMatchScoring.explanation", value: r.JobMatchScoring.Explanation},
		{name: "aiScreening.verdict.title", value: r.AIScreening.Verdict.Title},
		{name: "aiScreening.verdict.summary", value: r.AIScreening.Verdict.Summary},
		{name: "aiScreening.aiRecruiterVerdict.oneLineVerdict", value: r.AIScreening.AIRecruiterVerdict.OneLineVerdict},
		{name: "aiScreening.aiRecruiterVerdict.mainConcern", value: r.AIScreening.AIRecruiterVerdict.MainConcern},
		{name: "aiScreening.aiRecruiterVerdict.strongestSignal", value: r.AIScreening.AIRecruiterVerdict.StrongestSignal},
		{name: "aiScreening.aiRecruiterVerdict.weakestSignal", value: r.AIScreening.AIRecruiterVerdict.WeakestSignal},
	}
	for i, item := range r.JobMatchScoring.RequirementScores {
		fields = append(fields,
			struct {
				name  string
				value string
			}{name: "jobMatchScoring.requirementScores[" + strconv.Itoa(i) + "].requirement", value: item.Requirement},
			struct {
				name  string
				value string
			}{name: "jobMatchScoring.requirementScores[" + strconv.Itoa(i) + "].evidence", value: item.Evidence},
			struct {
				name  string
				value string
			}{name: "jobMatchScoring.requirementScores[" + strconv.Itoa(i) + "].gap", value: item.Gap},
		)
	}
	for i, item := range r.AIScreening.ScoreBreakdown {
		fields = append(fields,
			struct {
				name  string
				value string
			}{name: "aiScreening.scoreBreakdown[" + strconv.Itoa(i) + "].label", value: item.Label},
			struct {
				name  string
				value string
			}{name: "aiScreening.scoreBreakdown[" + strconv.Itoa(i) + "].explanation", value: item.Explanation},
			struct {
				name  string
				value string
			}{name: "aiScreening.scoreBreakdown[" + strconv.Itoa(i) + "].improvementFocus", value: item.ImprovementFocus},
		)
	}
	for _, field := range fields {
		if phrase, ok := containsHardGuaranteePhraseV2_4(field.value); ok {
			return field.name, phrase, true
		}
	}
	return "", "", false
}

func containsHardGuaranteePhraseV2_4(value string) (string, bool) {
	lower := strings.ToLower(value)
	for _, phrase := range hardGuaranteePhrasesV2_4 {
		if strings.Contains(lower, phrase) {
			return phrase, true
		}
	}
	return "", false
}

func sanitizeHardGuaranteesV2_4(r *AnalysisResultV2_4) bool {
	if r == nil {
		return false
	}
	changed := false
	replace := func(value *string) {
		updated := replaceHardGuaranteePhrasesV2_4(*value)
		if updated != *value {
			*value = updated
			changed = true
		}
	}
	replace(&r.JobMatchScoring.Explanation)
	for i := range r.JobMatchScoring.RequirementScores {
		replace(&r.JobMatchScoring.RequirementScores[i].Requirement)
		replace(&r.JobMatchScoring.RequirementScores[i].Evidence)
		replace(&r.JobMatchScoring.RequirementScores[i].Gap)
	}
	replace(&r.AIScreening.Verdict.Title)
	replace(&r.AIScreening.Verdict.Summary)
	for i := range r.AIScreening.ScoreBreakdown {
		replace(&r.AIScreening.ScoreBreakdown[i].Label)
		replace(&r.AIScreening.ScoreBreakdown[i].Explanation)
		replace(&r.AIScreening.ScoreBreakdown[i].ImprovementFocus)
	}
	replace(&r.AIScreening.AIRecruiterVerdict.OneLineVerdict)
	replace(&r.AIScreening.AIRecruiterVerdict.MainConcern)
	replace(&r.AIScreening.AIRecruiterVerdict.StrongestSignal)
	replace(&r.AIScreening.AIRecruiterVerdict.WeakestSignal)
	return changed
}

func replaceHardGuaranteePhrasesV2_4(input string) string {
	replacements := map[string]string{
		"guaranteed interview": "stronger screening readiness",
		"guaranteed shortlist": "stronger shortlist readiness",
		"will be selected":     "may be more competitive",
		"will pass ai filter":  "may improve AI screening readability",
		"bypass ats":           "improve ATS readability",
		"beat the ats":         "improve ATS readability",
	}
	out := input
	for phrase, replacement := range replacements {
		out = replaceAllCaseInsensitiveV2_4(out, phrase, replacement)
	}
	return out
}

func replaceAllCaseInsensitiveV2_4(input, old, replacement string) string {
	if old == "" {
		return input
	}
	lowerInput := strings.ToLower(input)
	lowerOld := strings.ToLower(old)
	var out strings.Builder
	start := 0
	for {
		idx := strings.Index(lowerInput[start:], lowerOld)
		if idx < 0 {
			out.WriteString(input[start:])
			return out.String()
		}
		idx += start
		out.WriteString(input[start:idx])
		out.WriteString(replacement)
		start = idx + len(old)
	}
}

func sanitizeTextFieldV2_4(value string, maxRunes int) string {
	return truncateWithEllipsis(normalizeWhitespace(value), maxRunes)
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
