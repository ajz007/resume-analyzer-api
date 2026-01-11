package service

import "sort"

// AnalysisResultV2_3 captures the analysis output needed for apply plan generation.
type AnalysisResultV2_3 struct {
	Issues         []AnalysisIssue `json:"issues"`
	BulletRewrites []BulletRewrite `json:"bulletRewrites"`
}

// AnalysisIssue represents a detected issue in the resume.
type AnalysisIssue struct {
	Section           string   `json:"section"`
	Problem           string   `json:"problem"`
	Priority          int      `json:"priority"`
	AutoFixable       bool     `json:"autoFixable"`
	RequiresUserInput []string `json:"requiresUserInput"`
}

// BulletRewrite represents a suggested rewrite for a resume bullet.
type BulletRewrite struct {
	Section            string   `json:"section"`
	Before             string   `json:"before"`
	After              string   `json:"after"`
	MetricsSource      string   `json:"metricsSource"`
	PlaceholdersNeeded []string `json:"placeholdersNeeded"`
	ClaimSupport       string   `json:"claimSupport"`
}

// ApplyPlan is the actionable output derived from AnalysisResultV2_3.
type ApplyPlan struct {
	AutoFixes       []AnalysisIssue `json:"autoFixes"`
	SafeRewrites    []BulletRewrite `json:"safeRewrites"`
	NeedsInput      []string        `json:"needsInput"`
	BlockedRewrites []BulletRewrite `json:"blockedRewrites"`
}

// BuildApplyPlan derives an ApplyPlan from the v2_3 analysis output.
func BuildApplyPlan(analysis AnalysisResultV2_3) ApplyPlan {
	issues := append([]AnalysisIssue(nil), analysis.Issues...)
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].Priority < issues[j].Priority
	})

	autoFixes := make([]AnalysisIssue, 0, len(issues))
	needsInput := make([]string, 0)
	seenInputs := make(map[string]struct{})
	for _, issue := range issues {
		if issue.AutoFixable {
			autoFixes = append(autoFixes, issue)
		}
		for _, input := range issue.RequiresUserInput {
			if _, ok := seenInputs[input]; ok {
				continue
			}
			seenInputs[input] = struct{}{}
			needsInput = append(needsInput, input)
		}
	}

	safeRewrites := make([]BulletRewrite, 0)
	blockedRewrites := make([]BulletRewrite, 0)
	for _, rewrite := range analysis.BulletRewrites {
		if isSafeRewrite(rewrite) {
			safeRewrites = append(safeRewrites, rewrite)
		}
		if len(rewrite.PlaceholdersNeeded) > 0 {
			blockedRewrites = append(blockedRewrites, rewrite)
		}
	}

	return ApplyPlan{
		AutoFixes:       autoFixes,
		SafeRewrites:    safeRewrites,
		NeedsInput:      needsInput,
		BlockedRewrites: blockedRewrites,
	}
}

func isSafeRewrite(rewrite BulletRewrite) bool {
	return rewrite.MetricsSource == "resume" &&
		rewrite.ClaimSupport == "supported" &&
		len(rewrite.PlaceholdersNeeded) == 0
}
