package recommendations

import (
	"reflect"
	"testing"
)

func TestGenerateRecommendationsDeterminism(t *testing.T) {
	input := Input{
		Issues: []Issue{
			{
				Severity:     "critical",
				Section:      "Experience",
				Problem:      "Missing impact metrics",
				WhyItMatters: "Recruiters look for measurable impact.",
				Suggestion:   "Add quantified outcomes.",
			},
			{
				Severity:     "medium",
				Section:      "Formatting",
				Problem:      "Inconsistent bullets",
				WhyItMatters: "Inconsistent bullets reduce readability.",
				Suggestion:   "Normalize bullet formatting.",
			},
		},
		MissingJDKeywords: []string{"Kafka", "Golang"},
		FormattingIssues:  []string{"Bullet style mismatch"},
		ActionPlan: ActionPlan{
			QuickWins: []string{"Tighten summary"},
		},
	}

	first := GenerateRecommendations(input)
	second := GenerateRecommendations(input)

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic recommendations ordering")
	}
}

func TestGenerateRecommendationsRanking(t *testing.T) {
	cases := []struct {
		name     string
		items    []Recommendation
		expected string
	}{
		{
			name: "critical_high_above_warning_high",
			items: []Recommendation{
				{ID: "a", Severity: "warning", Impact: "high", Title: "B"},
				{ID: "b", Severity: "critical", Impact: "high", Title: "A"},
			},
			expected: "b",
		},
		{
			name: "warning_high_above_warning_low",
			items: []Recommendation{
				{ID: "a", Severity: "warning", Impact: "low", Title: "B"},
				{ID: "b", Severity: "warning", Impact: "high", Title: "A"},
			},
			expected: "b",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := append([]Recommendation{}, tc.items...)
			sortRecommendations(items)
			if len(items) == 0 || items[0].ID != tc.expected {
				t.Fatalf("expected first id %q, got %q", tc.expected, items[0].ID)
			}
		})
	}
}

func TestGenerateRecommendationsDedup(t *testing.T) {
	input := Input{
		Issues: []Issue{
			{
				Severity:     "medium",
				Section:      "Formatting",
				Problem:      "Inconsistent bullets",
				WhyItMatters: "Why",
				Suggestion:   "Fix bullets",
			},
			{
				Severity:     "medium",
				Section:      "Formatting",
				Problem:      "Inconsistent bullets",
				WhyItMatters: "Why",
				Suggestion:   "Fix bullets",
			},
		},
	}

	recs := GenerateRecommendations(input)
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation after dedupe, got %d", len(recs))
	}
}

func TestGenerateRecommendationsMaxSeven(t *testing.T) {
	issues := make([]Issue, 0, 20)
	for i := 0; i < 20; i++ {
		issues = append(issues, Issue{
			Severity:     "low",
			Section:      "Experience",
			Problem:      "Issue " + string(rune('A'+i)),
			WhyItMatters: "Why",
			Suggestion:   "Do something",
		})
	}
	input := Input{
		Issues: issues,
	}

	recs := GenerateRecommendations(input)
	if len(recs) != 7 {
		t.Fatalf("expected 7 recommendations, got %d", len(recs))
	}
	for i, rec := range recs {
		if rec.Order != i+1 {
			t.Fatalf("expected order %d, got %d", i+1, rec.Order)
		}
	}
}

func TestGenerateRecommendationsMissingJDKeywords(t *testing.T) {
	input := Input{
		MissingJDKeywords: []string{
			"k1", "k2", "k3", "k4", "k5", "k6", "k7", "k8", "k9", "k10",
			"k11", "k12", "k13", "k14", "k15",
		},
	}

	recs := GenerateRecommendations(input)
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
	if recs[0].Title != "Add missing job keywords" {
		t.Fatalf("expected title %q, got %q", "Add missing job keywords", recs[0].Title)
	}
}
