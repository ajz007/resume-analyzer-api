package analyses

import (
	"strings"
	"testing"
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
