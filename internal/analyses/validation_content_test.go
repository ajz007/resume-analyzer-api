package analyses

import "testing"

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
