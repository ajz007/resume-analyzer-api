package analyses

import "context"

// UnlockChecker decides whether a user can see a full report. payments.Service implements it.
type UnlockChecker interface {
	PaywallActive() bool
	IsUnlocked(ctx context.Context, userID, analysisID string) (bool, error)
}

// Locked section identifiers returned to clients in lockedSections.
const (
	LockedReport             = "report"
	LockedIssues             = "issues"
	LockedBulletRewrites     = "bulletRewrites"
	LockedRecommendations    = "recommendations"
	LockedActionPlan         = "actionPlan"
	LockedMissingInformation = "missingInformation"
	LockedJobRequirements    = "jobRequirements"
)

// freeTierIssueLimit is how many issues a locked report still shows.
const freeTierIssueLimit = 3

// RedactForFreeTier returns a copy of a normalized result with paid sections removed,
// plus the sections that were actually locked. Scores, summary, ATS details, AI
// screening and fixThisFirst stay free. Removed lists are replaced with empty values so
// clients can render the report shape unchanged.
func RedactForFreeTier(result map[string]any) (map[string]any, []string) {
	locked := []string{}
	if result == nil {
		return nil, locked
	}
	out := make(map[string]any, len(result))
	for k, v := range result {
		if k != reviewSessionKey {
			out[k] = v
		}
	}

	if issues, ok := out["issues"].([]any); ok && len(issues) > freeTierIssueLimit {
		out["issues"] = issues[:freeTierIssueLimit]
		locked = append(locked, LockedIssues)
	}
	for _, key := range []string{LockedBulletRewrites, LockedRecommendations, LockedMissingInformation} {
		if list, ok := out[key].([]any); ok && len(list) > 0 {
			out[key] = []any{}
			locked = append(locked, key)
		}
	}
	if plan, ok := out["actionPlan"].(map[string]any); ok && hasNonEmptyList(plan) {
		out["actionPlan"] = map[string]any{"quickWins": []any{}, "mediumEffort": []any{}, "deepFixes": []any{}}
		locked = append(locked, LockedActionPlan)
	}

	jobLocked := false
	if profile, ok := out["jobRequirementProfile"].(map[string]any); ok {
		if list, ok := profile["topPriorities"].([]any); ok && len(list) > 0 {
			out["jobRequirementProfile"] = withEmptyList(profile, "topPriorities")
			jobLocked = true
		}
	}
	if scoring, ok := out["jobMatchScoring"].(map[string]any); ok {
		if list, ok := scoring["requirementScores"].([]any); ok && len(list) > 0 {
			out["jobMatchScoring"] = withEmptyList(scoring, "requirementScores")
			jobLocked = true
		}
	}
	if jobLocked {
		locked = append(locked, LockedJobRequirements)
	}

	if report, ok := out["report"].(map[string]any); ok {
		// Allowlist only non-paid navigation. Never shallow-copy future report fields.
		preview := map[string]any{"schemaVersion": report["schemaVersion"], "scope": report["scope"], "priorities": report["priorities"]}
		out["report"] = preview
		locked = append(locked, LockedReport)
	}
	return out, locked
}

func hasNonEmptyList(m map[string]any) bool {
	for _, v := range m {
		if list, ok := v.([]any); ok && len(list) > 0 {
			return true
		}
	}
	return false
}

func withEmptyList(m map[string]any, key string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	out[key] = []any{}
	return out
}

// applyPaywall redacts result for ownerID unless the report is unlocked. It returns the
// result to send, whether it is locked, and the locked sections.
func (h *Handler) applyPaywall(ctx context.Context, ownerID, analysisID string, result map[string]any) (map[string]any, bool, []string, error) {
	// Answers and generation locks are owner-only through the dedicated review API.
	// Never embed them in public shares, legacy result responses or paywall previews.
	if result != nil {
		clean := make(map[string]any, len(result))
		for k, v := range result {
			if k != reviewSessionKey {
				clean[k] = v
			}
		}
		result = clean
	}

	if h.Unlocks == nil || !h.Unlocks.PaywallActive() || result == nil {
		return result, false, []string{}, nil
	}
	unlocked, err := h.Unlocks.IsUnlocked(ctx, ownerID, analysisID)
	if err != nil {
		return nil, false, nil, err
	}
	if unlocked {
		return result, false, []string{}, nil
	}
	redacted, sections := RedactForFreeTier(result)
	return redacted, true, sections, nil
}
