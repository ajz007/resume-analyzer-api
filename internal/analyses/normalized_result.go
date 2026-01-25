package analyses

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// NormalizedAnalysisResult is the single normalized response schema returned by the API.
type NormalizedAnalysisResult struct {
	Meta               MetaV2                    `json:"meta"`
	Summary            SummaryV1                 `json:"summary"`
	ATS                NormalizedATS             `json:"ats"`
	Issues             []IssueV2_2               `json:"issues"`
	BulletRewrites     []NormalizedBulletRewrite `json:"bulletRewrites"`
	MissingInformation []string                  `json:"missingInformation"`
	ActionPlan         ActionPlanV1              `json:"actionPlan"`
	Recommendations    []Recommendation          `json:"recommendations"`
}

type NormalizedATS struct {
	Score            float64            `json:"score"`
	ScoreBreakdown   ScoreBreakdownV2   `json:"scoreBreakdown"`
	ScoreReasoning   []string           `json:"scoreReasoning"`
	ScoreExplanation ScoreExplanationV1 `json:"scoreExplanation"`
	MissingKeywords  MissingKeywordsV2  `json:"missingKeywords"`
	FormattingIssues []string           `json:"formattingIssues"`
}

type NormalizedBulletRewrite struct {
	Section            string   `json:"section"`
	Before             string   `json:"before"`
	After              string   `json:"after"`
	Rationale          string   `json:"rationale"`
	MetricsSource      string   `json:"metricsSource"`
	PlaceholdersNeeded []string `json:"placeholdersNeeded"`
	ClaimSupport       string   `json:"claimSupport"`
	Evidence           string   `json:"evidence"`
}

func normalizeAnalysisResult(raw json.RawMessage, analysis Analysis) (map[string]any, error) {
	normalized, err := normalizeToFinal(raw, analysis)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	normalizeResultOrdering(result)
	return result, nil
}

func normalizeToFinal(raw json.RawMessage, analysis Analysis) (NormalizedAnalysisResult, error) {
	if len(raw) == 0 {
		return NormalizedAnalysisResult{}, errors.New("empty analysis result")
	}
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return NormalizedAnalysisResult{}, err
	}
	if err := requireTopLevelFields(top); err != nil {
		return NormalizedAnalysisResult{}, err
	}

	var envelope struct {
		Meta struct {
			PromptVersion string `json:"promptVersion"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(raw, &envelope)

	hasMeta := false
	if _, ok := top["meta"]; ok {
		hasMeta = true
	}

	switch {
	case hasMeta && strings.EqualFold(envelope.Meta.PromptVersion, "v2_3"):
		var parsed AnalysisResultV2_3
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		out := normalizeFromV2_3(parsed, analysis)
		out.Recommendations = normalizeRecommendations(buildRecommendations(out))
		return out, validateNormalized(out)
	case hasMeta && strings.EqualFold(envelope.Meta.PromptVersion, "v2_2"):
		var parsed AnalysisResultV2_2
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		out := normalizeFromV2_2(parsed, analysis)
		out.Recommendations = normalizeRecommendations(buildRecommendations(out))
		return out, validateNormalized(out)
	case hasMeta && strings.EqualFold(envelope.Meta.PromptVersion, "v2_1"):
		var parsed AnalysisResultV2_1
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		out := normalizeFromV2_1(parsed, analysis)
		out.Recommendations = normalizeRecommendations(buildRecommendations(out))
		return out, validateNormalized(out)
	case hasMeta && strings.EqualFold(envelope.Meta.PromptVersion, "v2"):
		var parsed AnalysisResultV2
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		out := normalizeFromV2(parsed, analysis)
		out.Recommendations = normalizeRecommendations(buildRecommendations(out))
		return out, validateNormalized(out)
	default:
		var parsed AnalysisResultV1
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		topMissing := extractStringSlice(top["missingKeywords"])
		topFormatting := extractStringSlice(top["formattingIssues"])
		out := normalizeFromV1(parsed, analysis, topMissing, topFormatting)
		out.Recommendations = normalizeRecommendations(buildRecommendations(out))
		return out, validateNormalized(out)
	}
}

func requireTopLevelFields(raw map[string]any) error {
	required := []string{"summary", "ats", "issues", "bulletRewrites", "missingInformation", "actionPlan"}
	for _, key := range required {
		if _, ok := raw[key]; !ok {
			return fmt.Errorf("missing field: %s", key)
		}
	}
	return nil
}

func validateNormalized(out NormalizedAnalysisResult) error {
	if strings.TrimSpace(out.Summary.OverallAssessment) == "" {
		return errors.New("summary.overallAssessment is required")
	}
	if strings.TrimSpace(out.Meta.PromptVersion) == "" || strings.TrimSpace(out.Meta.Model) == "" {
		return errors.New("meta.promptVersion and meta.model are required")
	}
	return nil
}

func normalizeFromV1(r AnalysisResultV1, analysis Analysis, topMissing, topFormatting []string) NormalizedAnalysisResult {
	meta := MetaV2{
		PromptVersion:          fallbackString(analysis.PromptVersion, "v1"),
		Model:                  analysis.Model,
		JobDescriptionProvided: strings.TrimSpace(analysis.JobDescription) != "",
		Confidence:             0,
		Assumptions:            []string{},
		Limitations:            []string{},
	}
	if meta.Model == "" {
		meta.Model = "unknown"
	}
	missingKeywords := []string(r.ATS.MissingKeywords)
	if len(topMissing) > 0 {
		missingKeywords = topMissing
	}
	formattingIssues := r.ATS.FormattingIssues
	if len(topFormatting) > 0 {
		formattingIssues = topFormatting
	}
	ats := NormalizedATS{
		Score:            clampScore(r.ATS.Score),
		ScoreBreakdown:   ScoreBreakdownV2{},
		ScoreReasoning:   []string{},
		ScoreExplanation: ScoreExplanationV1{},
		MissingKeywords:  MissingKeywordsV2{FromJobDescription: ensureStringSlice(missingKeywords), IndustryCommon: []string{}},
		FormattingIssues: ensureStringSlice(formattingIssues),
	}
	issues := make([]IssueV2_2, 0, len(r.Issues))
	for _, issue := range r.Issues {
		issues = append(issues, IssueV2_2{
			Severity:          issue.Severity,
			Section:           issue.Section,
			Problem:           issue.Problem,
			WhyItMatters:      issue.WhyItMatters,
			Suggestion:        issue.Suggestion,
			Evidence:          "",
			FixEffort:         "",
			Priority:          0,
			AutoFixable:       false,
			RequiresUserInput: []string{},
		})
	}
	bullets := make([]NormalizedBulletRewrite, 0, len(r.BulletRewrites))
	for _, br := range r.BulletRewrites {
		bullets = append(bullets, NormalizedBulletRewrite{
			Section:            br.Section,
			Before:             br.Before,
			After:              br.After,
			Rationale:          br.Rationale,
			MetricsSource:      "resume",
			PlaceholdersNeeded: []string{},
			ClaimSupport:       "inferred",
			Evidence:           "notFound",
		})
	}

	out := NormalizedAnalysisResult{
		Meta:               normalizeMeta(meta, analysis),
		Summary:            normalizeSummary(r.Summary),
		ATS:                normalizeATS(ats),
		Issues:             ensureIssueList(issues),
		BulletRewrites:     ensureBulletList(bullets),
		MissingInformation: ensureStringSlice(r.MissingInformation),
		ActionPlan:         normalizeActionPlan(r.ActionPlan),
		Recommendations:    []Recommendation{},
	}
	return out
}

func normalizeFromV2(r AnalysisResultV2, analysis Analysis) NormalizedAnalysisResult {
	meta := normalizeMeta(r.Meta, analysis)
	ats := NormalizedATS{
		Score:            clampScore(r.ATS.Score),
		ScoreBreakdown:   clampScoreBreakdown(r.ATS.ScoreBreakdown),
		ScoreReasoning:   []string{},
		ScoreExplanation: ScoreExplanationV1{},
		MissingKeywords:  normalizeMissingKeywords(r.ATS.MissingKeywords),
		FormattingIssues: ensureStringSlice(r.ATS.FormattingIssues),
	}
	issues := make([]IssueV2_2, 0, len(r.Issues))
	for _, issue := range r.Issues {
		issues = append(issues, IssueV2_2{
			Severity:          issue.Severity,
			Section:           issue.Section,
			Problem:           issue.Problem,
			WhyItMatters:      issue.WhyItMatters,
			Suggestion:        issue.Suggestion,
			Evidence:          issue.Evidence,
			FixEffort:         issue.FixEffort,
			Priority:          0,
			AutoFixable:       false,
			RequiresUserInput: []string{},
		})
	}
	bullets := make([]NormalizedBulletRewrite, 0, len(r.BulletRewrites))
	for _, br := range r.BulletRewrites {
		bullets = append(bullets, NormalizedBulletRewrite{
			Section:            br.Section,
			Before:             br.Before,
			After:              br.After,
			Rationale:          br.Rationale,
			MetricsSource:      "resume",
			PlaceholdersNeeded: []string{},
			ClaimSupport:       "inferred",
			Evidence:           "notFound",
		})
	}
	return NormalizedAnalysisResult{
		Meta:               meta,
		Summary:            normalizeSummary(r.Summary),
		ATS:                normalizeATS(ats),
		Issues:             ensureIssueList(issues),
		BulletRewrites:     ensureBulletList(bullets),
		MissingInformation: ensureStringSlice(r.MissingInformation),
		ActionPlan:         normalizeActionPlan(r.ActionPlan),
		Recommendations:    []Recommendation{},
	}
}

func normalizeFromV2_1(r AnalysisResultV2_1, analysis Analysis) NormalizedAnalysisResult {
	meta := normalizeMeta(r.Meta, analysis)
	ats := NormalizedATS{
		Score:            clampScore(r.ATS.Score),
		ScoreBreakdown:   clampScoreBreakdown(r.ATS.ScoreBreakdown),
		ScoreReasoning:   []string{},
		ScoreExplanation: ScoreExplanationV1{},
		MissingKeywords:  normalizeMissingKeywords(r.ATS.MissingKeywords),
		FormattingIssues: ensureStringSlice(r.ATS.FormattingIssues),
	}
	issues := make([]IssueV2_2, 0, len(r.Issues))
	for _, issue := range r.Issues {
		issues = append(issues, IssueV2_2{
			Severity:          issue.Severity,
			Section:           issue.Section,
			Problem:           issue.Problem,
			WhyItMatters:      issue.WhyItMatters,
			Suggestion:        issue.Suggestion,
			Evidence:          issue.Evidence,
			FixEffort:         issue.FixEffort,
			Priority:          issue.Priority,
			AutoFixable:       false,
			RequiresUserInput: []string{},
		})
	}
	bullets := make([]NormalizedBulletRewrite, 0, len(r.BulletRewrites))
	for _, br := range r.BulletRewrites {
		bullets = append(bullets, NormalizedBulletRewrite{
			Section:            br.Section,
			Before:             br.Before,
			After:              br.After,
			Rationale:          br.Rationale,
			MetricsSource:      normalizeMetricsSource(br.MetricsSource),
			PlaceholdersNeeded: ensureStringSlice(br.PlaceholdersNeeded),
			ClaimSupport:       "inferred",
			Evidence:           "notFound",
		})
	}
	return NormalizedAnalysisResult{
		Meta:               meta,
		Summary:            normalizeSummary(r.Summary),
		ATS:                normalizeATS(ats),
		Issues:             ensureIssueList(issues),
		BulletRewrites:     ensureBulletList(bullets),
		MissingInformation: ensureStringSlice(r.MissingInformation),
		ActionPlan:         normalizeActionPlan(r.ActionPlan),
		Recommendations:    []Recommendation{},
	}
}

func normalizeFromV2_2(r AnalysisResultV2_2, analysis Analysis) NormalizedAnalysisResult {
	meta := normalizeMeta(r.Meta, analysis)
	ats := NormalizedATS{
		Score:            clampScore(r.ATS.Score),
		ScoreBreakdown:   clampScoreBreakdown(r.ATS.ScoreBreakdown),
		ScoreReasoning:   ensureStringSlice(r.ATS.ScoreReasoning),
		ScoreExplanation: ScoreExplanationV1{},
		MissingKeywords:  normalizeMissingKeywords(r.ATS.MissingKeywords),
		FormattingIssues: ensureStringSlice(r.ATS.FormattingIssues),
	}
	bullets := make([]NormalizedBulletRewrite, 0, len(r.BulletRewrites))
	for _, br := range r.BulletRewrites {
		bullets = append(bullets, NormalizedBulletRewrite{
			Section:            br.Section,
			Before:             br.Before,
			After:              br.After,
			Rationale:          br.Rationale,
			MetricsSource:      normalizeMetricsSource(br.MetricsSource),
			PlaceholdersNeeded: ensureStringSlice(br.PlaceholdersNeeded),
			ClaimSupport:       "inferred",
			Evidence:           "notFound",
		})
	}
	return NormalizedAnalysisResult{
		Meta:               meta,
		Summary:            normalizeSummary(r.Summary),
		ATS:                normalizeATS(ats),
		Issues:             ensureIssueList(r.Issues),
		BulletRewrites:     ensureBulletList(bullets),
		MissingInformation: ensureStringSlice(r.MissingInformation),
		ActionPlan:         normalizeActionPlan(r.ActionPlan),
		Recommendations:    []Recommendation{},
	}
}

func normalizeFromV2_3(r AnalysisResultV2_3, analysis Analysis) NormalizedAnalysisResult {
	meta := normalizeMeta(r.Meta, analysis)
	ats := NormalizedATS{
		Score:            clampScore(r.ATS.Score),
		ScoreBreakdown:   clampScoreBreakdown(r.ATS.ScoreBreakdown),
		ScoreReasoning:   ensureStringSlice(r.ATS.ScoreReasoning),
		ScoreExplanation: r.ATS.ScoreExplanation,
		MissingKeywords:  normalizeMissingKeywords(r.ATS.MissingKeywords),
		FormattingIssues: ensureStringSlice(r.ATS.FormattingIssues),
	}
	bullets := make([]NormalizedBulletRewrite, 0, len(r.BulletRewrites))
	for _, br := range r.BulletRewrites {
		bullets = append(bullets, NormalizedBulletRewrite{
			Section:            br.Section,
			Before:             br.Before,
			After:              br.After,
			Rationale:          br.Rationale,
			MetricsSource:      normalizeMetricsSource(br.MetricsSource),
			PlaceholdersNeeded: ensureStringSlice(br.PlaceholdersNeeded),
			ClaimSupport:       normalizeClaimSupport(br.ClaimSupport),
			Evidence:           normalizeEvidence(br.Evidence),
		})
	}
	return NormalizedAnalysisResult{
		Meta:               meta,
		Summary:            normalizeSummary(r.Summary),
		ATS:                normalizeATS(ats),
		Issues:             ensureIssueList(r.Issues),
		BulletRewrites:     ensureBulletList(bullets),
		MissingInformation: ensureStringSlice(r.MissingInformation),
		ActionPlan:         normalizeActionPlan(r.ActionPlan),
		Recommendations:    []Recommendation{},
	}
}

func normalizeMeta(meta MetaV2, analysis Analysis) MetaV2 {
	meta.PromptVersion = fallbackString(meta.PromptVersion, analysis.PromptVersion)
	meta.Model = fallbackString(meta.Model, analysis.Model)
	if meta.Model == "" {
		meta.Model = "unknown"
	}
	if meta.Assumptions == nil {
		meta.Assumptions = []string{}
	}
	if meta.Limitations == nil {
		meta.Limitations = []string{}
	}
	return meta
}

func normalizeSummary(summary SummaryV1) SummaryV1 {
	if summary.Strengths == nil {
		summary.Strengths = []string{}
	}
	if summary.Weaknesses == nil {
		summary.Weaknesses = []string{}
	}
	return summary
}

func normalizeATS(ats NormalizedATS) NormalizedATS {
	ats.Score = clampScore(ats.Score)
	ats.ScoreBreakdown = clampScoreBreakdown(ats.ScoreBreakdown)
	ats.ScoreReasoning = ensureStringSlice(ats.ScoreReasoning)
	ats.ScoreExplanation = normalizeScoreExplanation(ats.ScoreExplanation)
	ats.MissingKeywords = normalizeMissingKeywords(ats.MissingKeywords)
	ats.FormattingIssues = ensureStringSlice(ats.FormattingIssues)
	return ats
}

func normalizeMissingKeywords(m MissingKeywordsV2) MissingKeywordsV2 {
	m.FromJobDescription = ensureStringSlice(m.FromJobDescription)
	m.IndustryCommon = ensureStringSlice(m.IndustryCommon)
	return m
}

func normalizeActionPlan(plan ActionPlanV1) ActionPlanV1 {
	if plan.QuickWins == nil {
		plan.QuickWins = []string{}
	}
	if plan.MediumEffort == nil {
		plan.MediumEffort = []string{}
	}
	if plan.DeepFixes == nil {
		plan.DeepFixes = []string{}
	}
	return plan
}

func ensureStringSlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

func ensureIssueList(value []IssueV2_2) []IssueV2_2 {
	if value == nil {
		return []IssueV2_2{}
	}
	for i := range value {
		if value[i].RequiresUserInput == nil {
			value[i].RequiresUserInput = []string{}
		}
	}
	return value
}

func ensureBulletList(value []NormalizedBulletRewrite) []NormalizedBulletRewrite {
	if value == nil {
		return []NormalizedBulletRewrite{}
	}
	for i := range value {
		if value[i].PlaceholdersNeeded == nil {
			value[i].PlaceholdersNeeded = []string{}
		}
		if strings.TrimSpace(value[i].MetricsSource) == "" {
			value[i].MetricsSource = "resume"
		}
		if strings.TrimSpace(value[i].ClaimSupport) == "" {
			value[i].ClaimSupport = "inferred"
		}
		if strings.TrimSpace(value[i].Evidence) == "" {
			value[i].Evidence = "notFound"
		}
	}
	return value
}

func normalizeMetricsSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "resume", "placeholder":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "resume"
	}
}

func normalizeClaimSupport(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "supported", "inferred", "placeholder":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "inferred"
	}
}

func normalizeEvidence(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "notFound"
	}
	return trimmed
}

func clampScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func clampScoreBreakdown(b ScoreBreakdownV2) ScoreBreakdownV2 {
	b.Skills = clampScore(b.Skills)
	b.Experience = clampScore(b.Experience)
	b.Impact = clampScore(b.Impact)
	b.Formatting = clampScore(b.Formatting)
	b.RoleFit = clampScore(b.RoleFit)
	return b
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func extractStringSlice(value any) []string {
	switch raw := value.(type) {
	case []string:
		return raw
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}
