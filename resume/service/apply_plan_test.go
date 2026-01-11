package service

import "testing"

func TestBuildApplyPlanFiltersAndOrders(t *testing.T) {
	analysis := AnalysisResultV2_3{
		Issues: []AnalysisIssue{
			{
				Priority:          3,
				AutoFixable:       false,
				RequiresUserInput: []string{"metrics", "team_size"},
			},
			{
				Priority:          1,
				AutoFixable:       true,
				RequiresUserInput: []string{"email", "phone"},
			},
			{
				Priority:          2,
				AutoFixable:       true,
				RequiresUserInput: []string{"email"},
			},
		},
		BulletRewrites: []BulletRewrite{
			{
				Section:            "A",
				MetricsSource:      "resume",
				ClaimSupport:       "supported",
				PlaceholdersNeeded: nil,
			},
			{
				Section:            "B",
				MetricsSource:      "placeholder",
				ClaimSupport:       "supported",
				PlaceholdersNeeded: []string{"X"},
			},
			{
				Section:            "C",
				MetricsSource:      "resume",
				ClaimSupport:       "supported",
				PlaceholdersNeeded: []string{},
			},
			{
				Section:            "D",
				MetricsSource:      "resume",
				ClaimSupport:       "unsupported",
				PlaceholdersNeeded: []string{},
			},
		},
	}

	plan := BuildApplyPlan(analysis)

	if len(plan.AutoFixes) != 2 {
		t.Fatalf("expected 2 autoFixes, got %d", len(plan.AutoFixes))
	}
	if plan.AutoFixes[0].Priority != 1 || plan.AutoFixes[1].Priority != 2 {
		t.Fatalf("expected autoFixes ordered by priority")
	}

	expectedInputs := []string{"email", "phone", "metrics", "team_size"}
	if len(plan.NeedsInput) != len(expectedInputs) {
		t.Fatalf("expected %d needsInput, got %d", len(expectedInputs), len(plan.NeedsInput))
	}
	for i, val := range expectedInputs {
		if plan.NeedsInput[i] != val {
			t.Fatalf("expected needsInput[%d]=%q, got %q", i, val, plan.NeedsInput[i])
		}
	}

	if len(plan.SafeRewrites) != 2 {
		t.Fatalf("expected 2 safeRewrites, got %d", len(plan.SafeRewrites))
	}
	if plan.SafeRewrites[0].Section != "A" || plan.SafeRewrites[1].Section != "C" {
		t.Fatalf("expected safeRewrites in input order")
	}

	if len(plan.BlockedRewrites) != 1 {
		t.Fatalf("expected 1 blockedRewrite, got %d", len(plan.BlockedRewrites))
	}
	if plan.BlockedRewrites[0].Section != "B" {
		t.Fatalf("expected blockedRewrite section B, got %q", plan.BlockedRewrites[0].Section)
	}
}
