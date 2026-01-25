package analyses

import "resume-backend/internal/analyses/recommendations"

// Recommendation is an alias of the recommendations module type.
type Recommendation = recommendations.Recommendation

func normalizeRecommendations(value []Recommendation) []Recommendation {
	if value == nil {
		return []Recommendation{}
	}
	return value
}
