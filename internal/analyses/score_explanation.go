package analyses

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// ScoreExplanationV1 explains how the ATS score is calculated.
type ScoreExplanationV1 struct {
	Components []ScoreComponentV1 `json:"components"`
}

// ScoreComponentV1 represents a weighted score component.
type ScoreComponentV1 struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Score       float64  `json:"score"`
	Weight      float64  `json:"weight"`
	Explanation string   `json:"explanation"`
	Helped      []string `json:"helped"`
	Dragged     []string `json:"dragged"`
}

var scoreExplanationKeys = map[string]string{
	"atsReadability":      "ATS Readability",
	"skillMatch":          "Skill Match",
	"experienceRelevance": "Experience Relevance",
	"resumeStructure":     "Resume Structure",
}

func validateScoreExplanationV1(e *ScoreExplanationV1) error {
	if e == nil {
		return errors.New("ats.scoreExplanation is required")
	}
	if len(e.Components) != len(scoreExplanationKeys) {
		return fmt.Errorf("ats.scoreExplanation.components must contain %d items", len(scoreExplanationKeys))
	}
	seen := make(map[string]bool, len(scoreExplanationKeys))
	totalWeight := 0.0
	for i, c := range e.Components {
		key := strings.TrimSpace(c.Key)
		if key == "" {
			return fmt.Errorf("ats.scoreExplanation.components[%d].key is required", i)
		}
		if _, ok := scoreExplanationKeys[key]; !ok {
			return fmt.Errorf("ats.scoreExplanation.components[%d].key must be one of: atsReadability, skillMatch, experienceRelevance, resumeStructure", i)
		}
		if seen[key] {
			return fmt.Errorf("ats.scoreExplanation.components[%d].key must be unique", i)
		}
		seen[key] = true
		if strings.TrimSpace(c.Label) == "" {
			return fmt.Errorf("ats.scoreExplanation.components[%d].label is required", i)
		}
		if c.Score < 0 || c.Score > 100 {
			return fmt.Errorf("ats.scoreExplanation.components[%d].score must be between 0 and 100", i)
		}
		if !isInteger(c.Score) {
			return fmt.Errorf("ats.scoreExplanation.components[%d].score must be an integer", i)
		}
		if c.Weight < 0 || c.Weight > 100 {
			return fmt.Errorf("ats.scoreExplanation.components[%d].weight must be between 0 and 100", i)
		}
		if !isInteger(c.Weight) {
			return fmt.Errorf("ats.scoreExplanation.components[%d].weight must be an integer", i)
		}
		totalWeight += c.Weight
		if strings.TrimSpace(c.Explanation) == "" {
			return fmt.Errorf("ats.scoreExplanation.components[%d].explanation is required", i)
		}
		if len(c.Helped) == 0 {
			return fmt.Errorf("ats.scoreExplanation.components[%d].helped must have at least 1 item", i)
		}
		if len(c.Dragged) == 0 {
			return fmt.Errorf("ats.scoreExplanation.components[%d].dragged must have at least 1 item", i)
		}
		for _, item := range c.Helped {
			if strings.TrimSpace(item) == "" {
				return fmt.Errorf("ats.scoreExplanation.components[%d].helped must not include empty items", i)
			}
		}
		for _, item := range c.Dragged {
			if strings.TrimSpace(item) == "" {
				return fmt.Errorf("ats.scoreExplanation.components[%d].dragged must not include empty items", i)
			}
		}
	}
	if len(seen) != len(scoreExplanationKeys) {
		return errors.New("ats.scoreExplanation.components must include all components")
	}
	if math.Abs(totalWeight-100) > 0.000001 {
		return fmt.Errorf("ats.scoreExplanation.components weights must total 100, got %.3f", totalWeight)
	}
	return nil
}

func normalizeScoreExplanation(value ScoreExplanationV1) ScoreExplanationV1 {
	if value.Components == nil {
		value.Components = []ScoreComponentV1{}
	}
	for i := range value.Components {
		if value.Components[i].Helped == nil {
			value.Components[i].Helped = []string{}
		}
		if value.Components[i].Dragged == nil {
			value.Components[i].Dragged = []string{}
		}
	}
	return value
}
