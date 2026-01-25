package analyses

import (
	"reflect"
	"testing"
)

func TestRecommendationsDeterministicOrdering(t *testing.T) {
	result := NormalizedAnalysisResult{
		ATS: NormalizedATS{
			MissingKeywords:  MissingKeywordsV2{FromJobDescription: []string{}},
			FormattingIssues: []string{},
		},
		ActionPlan: ActionPlanV1{},
		Issues: []IssueV2_2{
			{
				Severity:     IssueSeverityCritical,
				Section:      "Experience",
				Problem:      "Missing impact metrics",
				WhyItMatters: "Recruiters look for measurable impact.",
				Suggestion:   "Add quantified outcomes.",
				Priority:     2,
			},
			{
				Severity:     IssueSeverityMedium,
				Section:      "Formatting",
				Problem:      "Inconsistent bullets",
				WhyItMatters: "Inconsistent bullets reduce readability.",
				Suggestion:   "Normalize bullet formatting.",
				Priority:     1,
			},
			{
				Severity:     IssueSeverityLow,
				Section:      "Summary",
				Problem:      "Summary is generic",
				WhyItMatters: "Generic summary weakens differentiation.",
				Suggestion:   "Customize summary for the role.",
				Priority:     3,
			},
		},
	}

	first := buildRecommendations(result)
	second := buildRecommendations(result)

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic recommendations ordering")
	}
}

func TestRecommendationsMaxSeven(t *testing.T) {
	missing := []string{"k1", "k2", "k3", "k4", "k5", "k6", "k7", "k8"}
	result := NormalizedAnalysisResult{
		ATS: NormalizedATS{
			MissingKeywords: MissingKeywordsV2{FromJobDescription: missing},
		},
	}

	recs := buildRecommendations(result)
	if len(recs) != 7 {
		t.Fatalf("expected 7 recommendations, got %d", len(recs))
	}
	for i, rec := range recs {
		if rec.Order != i+1 {
			t.Fatalf("expected order %d, got %d", i+1, rec.Order)
		}
	}
}

func TestRecommendationsDedup(t *testing.T) {
	result := NormalizedAnalysisResult{
		ATS: NormalizedATS{
			FormattingIssues: []string{"Extra spaces", "Extra spaces"},
		},
	}

	recs := buildRecommendations(result)
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation after dedupe, got %d", len(recs))
	}
}
