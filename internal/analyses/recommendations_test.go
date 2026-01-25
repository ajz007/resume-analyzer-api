package analyses

import (
	"reflect"
	"testing"

	"resume-backend/internal/analyses/recommendations"
)

func TestRecommendationsDeterministicOrdering(t *testing.T) {
	input := recommendations.Input{
		Issues: []recommendations.Issue{
			{
				Severity:     string(IssueSeverityCritical),
				Section:      "Experience",
				Problem:      "Missing impact metrics",
				WhyItMatters: "Recruiters look for measurable impact.",
				Suggestion:   "Add quantified outcomes.",
			},
			{
				Severity:     string(IssueSeverityMedium),
				Section:      "Formatting",
				Problem:      "Inconsistent bullets",
				WhyItMatters: "Inconsistent bullets reduce readability.",
				Suggestion:   "Normalize bullet formatting.",
			},
			{
				Severity:     string(IssueSeverityLow),
				Section:      "Summary",
				Problem:      "Summary is generic",
				WhyItMatters: "Generic summary weakens differentiation.",
				Suggestion:   "Customize summary for the role.",
			},
		},
	}

	first := recommendations.GenerateRecommendations(input)
	second := recommendations.GenerateRecommendations(input)

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic recommendations ordering")
	}
}

func TestRecommendationsMaxSeven(t *testing.T) {
	issues := make([]recommendations.Issue, 0, 12)
	for i := 0; i < 12; i++ {
		issues = append(issues, recommendations.Issue{
			Severity:     string(IssueSeverityLow),
			Section:      "Experience",
			Problem:      "Issue " + string(rune('A'+i)),
			WhyItMatters: "Why",
			Suggestion:   "Do something",
		})
	}
	input := recommendations.Input{
		Issues: issues,
	}

	recs := recommendations.GenerateRecommendations(input)
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
	input := recommendations.Input{
		Issues: []recommendations.Issue{
			{
				Severity:     string(IssueSeverityMedium),
				Section:      "Formatting",
				Problem:      "Inconsistent bullets",
				WhyItMatters: "Why",
				Suggestion:   "Fix bullets",
			},
			{
				Severity:     string(IssueSeverityMedium),
				Section:      "Formatting",
				Problem:      "Inconsistent bullets",
				WhyItMatters: "Why",
				Suggestion:   "Fix bullets",
			},
		},
	}

	recs := recommendations.GenerateRecommendations(input)
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation after dedupe, got %d", len(recs))
	}
}

func TestRecommendationsMissingJDKeywords(t *testing.T) {
	input := recommendations.Input{
		MissingJDKeywords: []string{"Kafka", "Golang", "AWS"},
	}

	recs := recommendations.GenerateRecommendations(input)
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
	if recs[0].ID != "ATS_MISSING_JD_KEYWORDS" {
		t.Fatalf("expected ATS_MISSING_JD_KEYWORDS id, got %q", recs[0].ID)
	}
}
