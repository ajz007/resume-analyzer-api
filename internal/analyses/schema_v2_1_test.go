package analyses

import (
	"encoding/json"
	"testing"
)

func TestAnalysisResultV2_1ValidateGood(t *testing.T) {
	r := AnalysisResultV2_1{
		Meta:    MetaV2{PromptVersion: "v2_1", Model: "gpt-5-mini", JobDescriptionProvided: false},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2{
			Score: 80,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills:     20,
				Experience: 20,
				Impact:     20,
				Formatting: 20,
				RoleFit:    20,
			},
			MissingKeywords: MissingKeywordsV2{
				FromJobDescription: []string{},
				IndustryCommon:     []string{"crm"},
			},
		},
		Issues: []IssueV2_1{
			{
				Severity:   IssueSeverityLow,
				Section:    "Summary",
				Problem:    "Vague statement",
				Suggestion: "Add metrics",
				Evidence:   "notFound",
				FixEffort:  "5min",
				Priority:   5,
			},
		},
		BulletRewrites: []BulletRewriteV2_1{
			{
				Section:       "Experience",
				Before:        "Improved sales.",
				After:         "Improved sales by 10%.",
				Rationale:     "Adds impact.",
				MetricsSource: "resume",
			},
		},
	}

	if err := r.Validate(); err != nil {
		t.Fatalf("expected valid v2_1 result, got error: %v", err)
	}
}

func TestAnalysisResultV2_1ValidateBadSum(t *testing.T) {
	r := AnalysisResultV2_1{
		Meta:    MetaV2{PromptVersion: "v2_1", Model: "gpt-5-mini", JobDescriptionProvided: false},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2{
			Score: 80,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills:     10,
				Experience: 10,
				Impact:     10,
				Formatting: 10,
				RoleFit:    10,
			},
			MissingKeywords: MissingKeywordsV2{
				FromJobDescription: []string{},
				IndustryCommon:     []string{"crm"},
			},
		},
	}

	if err := r.Validate(); err == nil {
		t.Fatalf("expected validation error for scoreBreakdown sum")
	}
}

func TestAnalysisResultV2_1ValidateMissingPlaceholders(t *testing.T) {
	r := AnalysisResultV2_1{
		Meta:    MetaV2{PromptVersion: "v2_1", Model: "gpt-5-mini", JobDescriptionProvided: false},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2{
			Score: 80,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills:     20,
				Experience: 20,
				Impact:     20,
				Formatting: 20,
				RoleFit:    20,
			},
			MissingKeywords: MissingKeywordsV2{
				FromJobDescription: []string{},
				IndustryCommon:     []string{"crm"},
			},
		},
		BulletRewrites: []BulletRewriteV2_1{
			{
				Section:       "Experience",
				Before:        "Improved sales.",
				After:         "Improved sales by X%.",
				Rationale:     "Adds impact.",
				MetricsSource: "placeholder",
			},
		},
	}

	if err := r.Validate(); err == nil {
		t.Fatalf("expected validation error for missing placeholdersNeeded")
	}
}

func TestAnalysisResultV2_1Fixture(t *testing.T) {
	payload := loadFixture(t, "testdata/v2_1_good.json")

	var out AnalysisResultV2_1
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2_1 fixture to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("expected v2_1 fixture to validate, got error: %v", err)
	}
}

func TestAnalysisResultV2_1BadSumFixture(t *testing.T) {
	payload := loadFixture(t, "testdata/v2_1_bad_sum.json")

	var out AnalysisResultV2_1
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2_1 bad sum fixture to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err == nil {
		t.Fatalf("expected validation error for scoreBreakdown sum")
	}
}

func TestAnalysisResultV2_1BadPlaceholdersFixture(t *testing.T) {
	payload := loadFixture(t, "testdata/v2_1_bad_placeholders.json")

	var out AnalysisResultV2_1
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2_1 bad placeholders fixture to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err == nil {
		t.Fatalf("expected validation error for placeholdersNeeded")
	}
}
