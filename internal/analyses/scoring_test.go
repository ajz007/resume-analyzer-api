package analyses

import "testing"

func TestCalculateWeightedScore(t *testing.T) {
	got := CalculateWeightedScore([]WeightedScoreItem{
		{Score: 80, Weight: 25},
		{Score: 60, Weight: 75},
	})
	if got != 65 {
		t.Fatalf("expected weighted score 65, got %.0f", got)
	}
}

func TestCalculateWeightedScoreRounding(t *testing.T) {
	got := CalculateWeightedScore([]WeightedScoreItem{
		{Score: 82.4, Weight: 50},
		{Score: 83.6, Weight: 50},
	})
	if got != 83 {
		t.Fatalf("expected rounded score 83, got %.0f", got)
	}
}

func TestCalculateWeightedScoreClampsAbove100AndBelow0(t *testing.T) {
	above := CalculateWeightedScore([]WeightedScoreItem{{Score: 140, Weight: 100}})
	if above != 100 {
		t.Fatalf("expected score above 100 to clamp to 100, got %.0f", above)
	}

	below := CalculateWeightedScore([]WeightedScoreItem{{Score: -20, Weight: 100}})
	if below != 0 {
		t.Fatalf("expected score below 0 to clamp to 0, got %.0f", below)
	}
}

func TestCalculateWeightedScoreReturnsZeroWithoutValidWeight(t *testing.T) {
	got := CalculateWeightedScore([]WeightedScoreItem{
		{Score: 100, Weight: 0},
		{Score: 100, Weight: -5},
	})
	if got != 0 {
		t.Fatalf("expected zero without valid weight, got %.0f", got)
	}
}

func TestCalculateJobMatchScoreIgnoresLLMFinalScore(t *testing.T) {
	got := CalculateJobMatchScore(JobMatchScoringV1{
		Score: 99,
		RequirementScores: []RequirementScoreV1{
			{RequirementID: "backend", Score: 50, Weight: 50},
			{RequirementID: "aws", Score: 70, Weight: 50},
		},
	})
	if got != 60 {
		t.Fatalf("expected deterministic job match score 60, got %.0f", got)
	}
}

func TestCalculateAIScreeningScoreIgnoresLLMFinalScore(t *testing.T) {
	got := CalculateAIScreeningScore(AIScreeningV1{
		Score: 100,
		ScoreBreakdown: []AIScreeningBreakdownItemV1{
			{ID: "semantic_relevance", Score: 80, Weight: 25},
			{ID: "role_intent_alignment", Score: 70, Weight: 20},
			{ID: "evidence_strength", Score: 60, Weight: 20},
			{ID: "impact_strength", Score: 50, Weight: 15},
			{ID: "signal_density", Score: 40, Weight: 10},
			{ID: "clarity_specificity", Score: 30, Weight: 10},
		},
	})
	if got != 61 {
		t.Fatalf("expected deterministic AI screening score 61, got %.0f", got)
	}
}

func TestResolveAIScreeningTier(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  string
	}{
		{name: "strong lower bound", score: 85, want: "STRONG"},
		{name: "good lower bound", score: 70, want: "GOOD"},
		{name: "borderline lower bound", score: 55, want: "BORDERLINE"},
		{name: "weak upper bound", score: 54, want: "WEAK"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveAIScreeningTier(tt.score); got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestResolveAIScreeningRisk(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  string
	}{
		{name: "strong low risk", score: 85, want: "LOW"},
		{name: "good low risk", score: 70, want: "LOW"},
		{name: "borderline medium risk", score: 55, want: "MEDIUM"},
		{name: "weak high risk", score: 54, want: "HIGH"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveAIScreeningRisk(tt.score); got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestBuildFixThisFirstSortingAndMax3(t *testing.T) {
	profile := JobRequirementProfileV1{
		TopPriorities: []JobPriorityV1{
			{ID: "req_a", Priority: "A", EvidenceExpected: "Add A evidence.", WhyItMatters: "A matters."},
			{ID: "req_b", Priority: "B", EvidenceExpected: "Add B evidence.", WhyItMatters: "B matters."},
			{ID: "req_c", Priority: "C", EvidenceExpected: "Add C evidence.", WhyItMatters: "C matters."},
			{ID: "req_d", Priority: "D", EvidenceExpected: "Add D evidence.", WhyItMatters: "D matters."},
			{ID: "req_e", Priority: "E", EvidenceExpected: "Add E evidence.", WhyItMatters: "E matters."},
		},
	}
	scoring := JobMatchScoringV1{
		RequirementScores: []RequirementScoreV1{
			{RequirementID: "req_a", Requirement: "Requirement A", Weight: 20, Score: 65, MatchStatus: "PARTIAL", Gap: "A gap."},
			{RequirementID: "req_b", Requirement: "Requirement B", Weight: 30, Score: 65, MatchStatus: "PARTIAL", Gap: "B gap."},
			{RequirementID: "req_c", Requirement: "Requirement C", Weight: 30, Score: 55, MatchStatus: "WEAK", Gap: "C gap."},
			{RequirementID: "req_d", Requirement: "Requirement D", Weight: 25, Score: 50, MatchStatus: "MISSING", Gap: "D gap."},
			{RequirementID: "req_e", Requirement: "Requirement E", Weight: 10, Score: 40, MatchStatus: "WEAK", Gap: "E gap."},
		},
	}

	got := BuildFixThisFirst(profile, scoring)
	if len(got) != 3 {
		t.Fatalf("expected max 3 fixThisFirst items, got %d", len(got))
	}
	wantIDs := []string{"req_c", "req_b", "req_d"}
	for i, want := range wantIDs {
		if got[i].LinkedRequirementID != want {
			t.Fatalf("expected item %d to link %s, got %s", i, want, got[i].LinkedRequirementID)
		}
		if got[i].Priority != i+1 {
			t.Fatalf("expected item %d priority %d, got %d", i, i+1, got[i].Priority)
		}
	}
	if !got[0].RequiresUserInput {
		t.Fatalf("expected weak requirement to require user input")
	}
	if got[1].RequiresUserInput {
		t.Fatalf("expected partial score 65 requirement not to require user input")
	}
	if got[0].ExpectedImpact != "HIGH" || got[0].Effort != "MEDIUM" {
		t.Fatalf("expected high impact and medium effort, got impact=%s effort=%s", got[0].ExpectedImpact, got[0].Effort)
	}
}
