package analyses

import (
	"math"
	"sort"
	"strings"
)

type WeightedScoreItem struct {
	Score  float64
	Weight float64
}

func CalculateWeightedScore(items []WeightedScoreItem) float64 {
	total := 0.0
	totalWeight := 0.0
	for _, item := range items {
		if item.Weight <= 0 {
			continue
		}
		total += item.Score * item.Weight
		totalWeight += item.Weight
	}
	if totalWeight <= 0 {
		return 0
	}
	return roundClampedScore(total / totalWeight)
}

func CalculateJobMatchScore(scoring JobMatchScoringV1) float64 {
	total := 0.0
	for _, item := range scoring.RequirementScores {
		total += item.Score * item.Weight
	}
	return roundClampedScore(total / 100)
}

func CalculateAIScreeningScore(screening AIScreeningV1) float64 {
	items := make([]WeightedScoreItem, 0, len(screening.ScoreBreakdown))
	for _, item := range screening.ScoreBreakdown {
		items = append(items, WeightedScoreItem{
			Score:  item.Score,
			Weight: item.Weight,
		})
	}
	return CalculateWeightedScore(items)
}

func ResolveAIScreeningTier(score float64) string {
	score = roundClampedScore(score)
	switch {
	case score >= 85:
		return "STRONG"
	case score >= 70:
		return "GOOD"
	case score >= 55:
		return "BORDERLINE"
	default:
		return "WEAK"
	}
}

func ResolveAIScreeningRisk(score float64) string {
	score = roundClampedScore(score)
	switch {
	case score >= 70:
		return "LOW"
	case score >= 55:
		return "MEDIUM"
	default:
		return "HIGH"
	}
}

func BuildFixThisFirst(profile JobRequirementProfileV1, scoring JobMatchScoringV1) []FixThisFirstItemV1 {
	type candidate struct {
		requirement RequirementScoreV1
		priority    JobPriorityV1
	}

	prioritiesByID := make(map[string]JobPriorityV1, len(profile.TopPriorities))
	for _, item := range profile.TopPriorities {
		prioritiesByID[item.ID] = item
	}

	candidates := make([]candidate, 0, len(scoring.RequirementScores))
	for _, item := range scoring.RequirementScores {
		if item.Weight < 15 || item.Score >= 70 {
			continue
		}
		candidates = append(candidates, candidate{
			requirement: item,
			priority:    prioritiesByID[item.RequirementID],
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i].requirement
		right := candidates[j].requirement
		if left.Weight != right.Weight {
			return left.Weight > right.Weight
		}
		return left.Score < right.Score
	})

	if len(candidates) > 3 {
		candidates = candidates[:3]
	}

	out := make([]FixThisFirstItemV1, 0, len(candidates))
	for i, candidate := range candidates {
		req := candidate.requirement
		priority := candidate.priority
		title := strings.TrimSpace(req.Requirement)
		if title == "" {
			title = strings.TrimSpace(priority.Priority)
		}
		if title == "" {
			title = strings.TrimSpace(req.RequirementID)
		}

		expectedImpact := "MEDIUM"
		if req.Weight >= 20 {
			expectedImpact = "HIGH"
		}

		why := strings.TrimSpace(req.Gap)
		if why == "" {
			why = strings.TrimSpace(priority.WhyItMatters)
		}
		if why == "" {
			why = "This is a high-weight requirement with weak resume evidence."
		}

		action := strings.TrimSpace(priority.EvidenceExpected)
		if action == "" {
			action = "Add specific resume evidence for this requirement."
		}

		status := strings.ToUpper(strings.TrimSpace(req.MatchStatus))
		requiresUserInput := req.Score < 60 || status == "MISSING" || status == "WEAK"

		out = append(out, FixThisFirstItemV1{
			Priority:            i + 1,
			Title:               title,
			Why:                 why,
			LinkedRequirementID: req.RequirementID,
			ExpectedImpact:      expectedImpact,
			Effort:              "MEDIUM",
			Action:              action,
			RequiresUserInput:   requiresUserInput,
		})
	}
	return out
}

func roundClampedScore(score float64) float64 {
	return math.Round(clampScore(score))
}
