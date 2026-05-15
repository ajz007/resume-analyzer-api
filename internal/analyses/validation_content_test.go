package analyses

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"resume-backend/internal/llm"
)

func TestValidateContentV2_2RejectsUnsupportedClaim(t *testing.T) {
	r := AnalysisResultV2_2{
		Meta:    MetaV2{PromptVersion: "v2_2", Model: "gpt-5-mini"},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2_2{
			Score: 80,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills: 20, Experience: 20, Impact: 20, Formatting: 20, RoleFit: 20,
			},
			ScoreReasoning: []string{"a", "b", "c"},
		},
		BulletRewrites: []BulletRewriteV2_1{
			{
				Section:       "Experience",
				After:         "Delivered double-digit growth.",
				MetricsSource: "resume",
			},
		},
	}
	if err := ValidateContentV2_2(&r); err == nil {
		t.Fatalf("expected unsupported claim to fail")
	}
}

func TestValidateContentV2_2AllowsPlaceholderWithNeeded(t *testing.T) {
	r := AnalysisResultV2_2{
		Meta:    MetaV2{PromptVersion: "v2_2", Model: "gpt-5-mini"},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2_2{
			Score: 80,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills: 20, Experience: 20, Impact: 20, Formatting: 20, RoleFit: 20,
			},
			ScoreReasoning: []string{"a", "b", "c"},
		},
		BulletRewrites: []BulletRewriteV2_1{
			{
				Section:            "Experience",
				After:              "Delivered significant growth (X%).",
				MetricsSource:      "placeholder",
				PlaceholdersNeeded: []string{"X%"},
			},
		},
	}
	if err := ValidateContentV2_2(&r); err != nil {
		t.Fatalf("expected placeholder case to pass, got %v", err)
	}
}

func TestValidateContentV2_2RejectsPlaceholderWithoutNeeded(t *testing.T) {
	r := AnalysisResultV2_2{
		Meta:    MetaV2{PromptVersion: "v2_2", Model: "gpt-5-mini"},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2_2{
			Score: 80,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills: 20, Experience: 20, Impact: 20, Formatting: 20, RoleFit: 20,
			},
			ScoreReasoning: []string{"a", "b", "c"},
		},
		BulletRewrites: []BulletRewriteV2_1{
			{
				Section:       "Experience",
				After:         "Delivered substantial growth.",
				MetricsSource: "placeholder",
			},
		},
	}
	if err := ValidateContentV2_2(&r); err == nil {
		t.Fatalf("expected placeholder without needed to fail")
	}
}

func TestSanitizeBulletRewriteTermsReplacesForbiddenTerms(t *testing.T) {
	r := AnalysisResultV2_3{
		Meta:    MetaV2{PromptVersion: "v2_3", Model: "gpt-5-mini"},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2_3{
			Score: 80,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills: 20, Experience: 20, Impact: 20, Formatting: 20, RoleFit: 20,
			},
			ScoreReasoning: []string{"a", "b", "c"},
			ScoreExplanation: ScoreExplanationV1{Components: []ScoreComponentV1{
				{Key: "atsReadability", Label: "ATS Readability", Score: 75, Weight: 25, Explanation: "x", Helped: []string{"a"}, Dragged: []string{"b"}},
				{Key: "skillMatch", Label: "Skill Match", Score: 70, Weight: 30, Explanation: "x", Helped: []string{"a"}, Dragged: []string{"b"}},
				{Key: "experienceRelevance", Label: "Experience Relevance", Score: 80, Weight: 30, Explanation: "x", Helped: []string{"a"}, Dragged: []string{"b"}},
				{Key: "resumeStructure", Label: "Resume Structure", Score: 78, Weight: 15, Explanation: "x", Helped: []string{"a"}, Dragged: []string{"b"}},
			}},
		},
		BulletRewrites: []BulletRewriteV2_3{
			{
				Section:       "Experience",
				After:         "Delivered double-digit growth through new pipeline.",
				MetricsSource: "resume",
				ClaimSupport:  "supported",
				Evidence:      "notFound",
			},
		},
	}

	changed, _ := sanitizeBulletRewriteTerms(&r)
	if !changed {
		t.Fatalf("expected sanitizer to change bullet rewrite")
	}
	after := r.BulletRewrites[0].After
	if strings.Contains(strings.ToLower(after), "double-digit") {
		t.Fatalf("expected forbidden term removed, got %q", after)
	}
	if !strings.Contains(after, "X% (replace with exact figure)") {
		t.Fatalf("expected placeholder replacement, got %q", after)
	}
	if r.BulletRewrites[0].ClaimSupport != "placeholder" {
		t.Fatalf("expected claimSupport placeholder, got %q", r.BulletRewrites[0].ClaimSupport)
	}
	if r.BulletRewrites[0].MetricsSource != "placeholder" {
		t.Fatalf("expected metricsSource placeholder, got %q", r.BulletRewrites[0].MetricsSource)
	}
	found := false
	for _, item := range r.BulletRewrites[0].PlaceholdersNeeded {
		if item == "revenue_growth_pct" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected revenue_growth_pct placeholder")
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("expected schema validation to pass after sanitize, got %v", err)
	}
	if err := ValidateContentV2_3(&r); err != nil {
		t.Fatalf("expected content validation to pass after sanitize, got %v", err)
	}
}

func TestSanitizeBulletRewriteTermsNoChange(t *testing.T) {
	r := AnalysisResultV2_3{
		Meta:    MetaV2{PromptVersion: "v2_3", Model: "gpt-5-mini"},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2_3{
			Score: 80,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills: 20, Experience: 20, Impact: 20, Formatting: 20, RoleFit: 20,
			},
			ScoreReasoning: []string{"a", "b", "c"},
			ScoreExplanation: ScoreExplanationV1{Components: []ScoreComponentV1{
				{Key: "atsReadability", Label: "ATS Readability", Score: 75, Weight: 25, Explanation: "x", Helped: []string{"a"}, Dragged: []string{"b"}},
				{Key: "skillMatch", Label: "Skill Match", Score: 70, Weight: 30, Explanation: "x", Helped: []string{"a"}, Dragged: []string{"b"}},
				{Key: "experienceRelevance", Label: "Experience Relevance", Score: 80, Weight: 30, Explanation: "x", Helped: []string{"a"}, Dragged: []string{"b"}},
				{Key: "resumeStructure", Label: "Resume Structure", Score: 78, Weight: 15, Explanation: "x", Helped: []string{"a"}, Dragged: []string{"b"}},
			}},
		},
		BulletRewrites: []BulletRewriteV2_3{
			{
				Section:       "Experience",
				After:         "Improved sales by 12%.",
				MetricsSource: "resume",
				ClaimSupport:  "supported",
				Evidence:      "Sales increased by 12%.",
			},
		},
	}

	changed, _ := sanitizeBulletRewriteTerms(&r)
	if changed {
		t.Fatalf("expected sanitizer to leave bullet rewrite unchanged")
	}
}

func TestValidateV2_4WithRetryAcceptsValidOutput(t *testing.T) {
	payload, err := json.Marshal(validAnalysisResultV2_4())
	if err != nil {
		t.Fatalf("marshal valid v2_4: %v", err)
	}

	raw, err := ValidateV2_4WithRetry(context.Background(), staticLLMResponse{resp: string(payload)}, llm.AnalyzeInput{PromptVersion: "v2_4"})
	if err != nil {
		t.Fatalf("expected valid v2_4 output to pass, got %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected raw output")
	}
}

func TestValidateV2_4WithRetryRecoversScoreExplanationArrays(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.Summary.Strengths = []string{" Backend ownership ", "", "Backend ownership"}
	out.ATS.ScoreExplanation.Components[0].Helped = []string{}
	out.ATS.ScoreExplanation.Components[0].Dragged = []string{"", "Dense bullets", "Dense bullets"}
	out.ATS.ScoreExplanation.Components[1].Helped = nil
	out.ATS.ScoreExplanation.Components[1].Dragged = []string{" Few queueing details "}
	out.JobRequirementProfile.HiddenExpectations = nil
	out.JobRequirementProfile.NiceToHaveSignals = nil
	payload, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal v2_4: %v", err)
	}

	raw, err := ValidateV2_4WithRetry(context.Background(), staticLLMResponse{resp: string(payload)}, llm.AnalyzeInput{PromptVersion: "v2_4"})
	if err != nil {
		t.Fatalf("expected recoverable v2_4 output to pass, got %v", err)
	}

	var got AnalysisResultV2_4
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal recovered payload: %v", err)
	}
	if got.ATS.ScoreExplanation.Components[0].Helped == nil || len(got.ATS.ScoreExplanation.Components[0].Helped) != 0 {
		t.Fatalf("expected empty helped array to be preserved, got %#v", got.ATS.ScoreExplanation.Components[0].Helped)
	}
	if got.ATS.ScoreExplanation.Components[1].Helped == nil || len(got.ATS.ScoreExplanation.Components[1].Helped) != 0 {
		t.Fatalf("expected nil helped array to normalize to empty array, got %#v", got.ATS.ScoreExplanation.Components[1].Helped)
	}
	if got.ATS.ScoreExplanation.Components[0].Dragged == nil || len(got.ATS.ScoreExplanation.Components[0].Dragged) != 1 || got.ATS.ScoreExplanation.Components[0].Dragged[0] != "Dense bullets" {
		t.Fatalf("expected dragged items to be trimmed and deduplicated, got %#v", got.ATS.ScoreExplanation.Components[0].Dragged)
	}
	if got.Summary.Strengths == nil || len(got.Summary.Strengths) != 1 || got.Summary.Strengths[0] != "Backend ownership" {
		t.Fatalf("expected summary strengths to be cleaned, got %#v", got.Summary.Strengths)
	}
	if got.JobRequirementProfile.HiddenExpectations == nil || got.JobRequirementProfile.NiceToHaveSignals == nil {
		t.Fatalf("expected optional arrays to normalize to empty arrays")
	}
}

func TestParseRecoverValidateV2_4ReportsWarnings(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.ATS.ScoreExplanation.Components[0].Helped = []string{}
	out.ATS.ScoreExplanation.Components[0].Dragged = []string{"", "Dense bullets"}
	payload, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal v2_4: %v", err)
	}

	var parsed AnalysisResultV2_4
	issues, err := parseRecoverValidateV2_4(payload, &parsed)
	if err != nil {
		t.Fatalf("expected validation to recover, got %v", err)
	}
	counts := countValidationIssuesBySeverity(issues)
	if counts[ValidationWarning] == 0 {
		t.Fatalf("expected warning issues, got %#v", issues)
	}
	if counts[ValidationRecoverable] == 0 {
		t.Fatalf("expected recoverable issues, got %#v", issues)
	}
}

func TestValidateV2_4WithRetryFailsMalformedJSON(t *testing.T) {
	_, err := ValidateV2_4WithRetry(context.Background(), staticLLMResponse{resp: `{"meta":`}, llm.AnalyzeInput{PromptVersion: "v2_4"})
	if err == nil {
		t.Fatalf("expected malformed JSON to fail")
	}
}

func TestValidateV2_4WithRetryFailsMissingATSSection(t *testing.T) {
	out := validAnalysisResultV2_4()
	payload, err := json.Marshal(struct {
		Meta                  MetaV2                  `json:"meta"`
		Summary               SummaryV1               `json:"summary"`
		Issues                []IssueV2_2             `json:"issues"`
		BulletRewrites        []BulletRewriteV2_3     `json:"bulletRewrites"`
		MissingInformation    []string                `json:"missingInformation"`
		ActionPlan            ActionPlanV1            `json:"actionPlan"`
		JobRequirementProfile JobRequirementProfileV1 `json:"jobRequirementProfile"`
		JobMatchScoring       JobMatchScoringV1       `json:"jobMatchScoring"`
		AIScreening           AIScreeningV1           `json:"aiScreening"`
		FixThisFirst          []FixThisFirstItemV1    `json:"fixThisFirst"`
	}{
		Meta:                  out.Meta,
		Summary:               out.Summary,
		Issues:                out.Issues,
		BulletRewrites:        out.BulletRewrites,
		MissingInformation:    out.MissingInformation,
		ActionPlan:            out.ActionPlan,
		JobRequirementProfile: out.JobRequirementProfile,
		JobMatchScoring:       out.JobMatchScoring,
		AIScreening:           out.AIScreening,
		FixThisFirst:          out.FixThisFirst,
	})
	if err != nil {
		t.Fatalf("marshal missing ats payload: %v", err)
	}

	if _, err := ValidateV2_4WithRetry(context.Background(), staticLLMResponse{resp: string(payload)}, llm.AnalyzeInput{PromptVersion: "v2_4"}); err == nil {
		t.Fatalf("expected missing ATS section to fail")
	}
}

func TestValidateV2_4WithRetryFailsInvalidScoreTotals(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.ATS.ScoreBreakdown.RoleFit = 1
	payload, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal v2_4: %v", err)
	}
	if _, err := ValidateV2_4WithRetry(context.Background(), staticLLMResponse{resp: string(payload)}, llm.AnalyzeInput{PromptVersion: "v2_4"}); err == nil {
		t.Fatalf("expected invalid score totals to fail")
	}
}

func TestValidateContentV2_4RejectsHardGuaranteePhrases(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.AIScreening.Verdict.Summary = "This resume will pass AI filter for the role."

	if err := ValidateContentV2_4(&out); err == nil {
		t.Fatalf("expected hard guarantee phrase to fail")
	}
}

type sequenceLLMResponse struct {
	calls int
	resp  []string
}

func (s *sequenceLLMResponse) AnalyzeResume(ctx context.Context, input llm.AnalyzeInput) (json.RawMessage, error) {
	_ = ctx
	_ = input
	if s.calls >= len(s.resp) {
		s.calls++
		return json.RawMessage(s.resp[len(s.resp)-1]), nil
	}
	resp := s.resp[s.calls]
	s.calls++
	return json.RawMessage(resp), nil
}

func TestValidateV2_4WithRetryPathWorks(t *testing.T) {
	first := validAnalysisResultV2_4()
	first.JobMatchScoring.Explanation = "This is a guaranteed shortlist after edits."
	firstPayload, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first v2_4: %v", err)
	}
	second := validAnalysisResultV2_4()
	second.JobMatchScoring.Explanation = "Strong shortlist readiness after edits."
	secondPayload, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second v2_4: %v", err)
	}
	mock := &sequenceLLMResponse{resp: []string{string(firstPayload), string(secondPayload)}}

	raw, err := ValidateV2_4WithRetry(context.Background(), mock, llm.AnalyzeInput{PromptVersion: "v2_4"})
	if err != nil {
		t.Fatalf("expected retry to pass, got %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected raw retry output")
	}
	if mock.calls != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", mock.calls)
	}
}
