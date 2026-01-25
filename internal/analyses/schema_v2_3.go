package analyses

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// AnalysisResultV2_3 represents the v2_3 analysis output schema.
type AnalysisResultV2_3 struct {
	Meta               MetaV2              `json:"meta"`
	Summary            SummaryV1           `json:"summary"`
	ATS                ATSV2_3             `json:"ats"`
	Issues             []IssueV2_2         `json:"issues"`
	BulletRewrites     []BulletRewriteV2_3 `json:"bulletRewrites"`
	MissingInformation []string            `json:"missingInformation"`
	ActionPlan         ActionPlanV1        `json:"actionPlan"`
}

type ATSV2_3 struct {
	Score            float64            `json:"score"`
	ScoreBreakdown   ScoreBreakdownV2   `json:"scoreBreakdown"`
	ScoreReasoning   []string           `json:"scoreReasoning"`
	ScoreExplanation ScoreExplanationV1 `json:"scoreExplanation"`
	MissingKeywords  MissingKeywordsV2  `json:"missingKeywords"`
	FormattingIssues []string           `json:"formattingIssues"`
}

type BulletRewriteV2_3 struct {
	Section            string   `json:"section"`
	Before             string   `json:"before"`
	After              string   `json:"after"`
	Rationale          string   `json:"rationale"`
	MetricsSource      string   `json:"metricsSource"`
	PlaceholdersNeeded []string `json:"placeholdersNeeded"`
	ClaimSupport       string   `json:"claimSupport"`
	Evidence           string   `json:"evidence"`
}

// Validate checks basic schema constraints for v2_3.
func (r *AnalysisResultV2_3) Validate() error {
	if r == nil {
		return errors.New("analysis result is nil")
	}
	if r.Meta.PromptVersion == "" || r.Meta.Model == "" {
		return errors.New("meta.promptVersion and meta.model are required")
	}
	if r.Summary.OverallAssessment == "" {
		return errors.New("summary.overallAssessment is required")
	}

	if r.Meta.JobDescriptionProvided == false && len(r.ATS.MissingKeywords.FromJobDescription) > 0 {
		return errors.New("missingKeywords.fromJobDescription must be empty when jobDescriptionProvided=false")
	}

	if r.ATS.Score < 0 || r.ATS.Score > 100 {
		return errors.New("ats.score must be between 0 and 100")
	}
	if !isInteger(r.ATS.Score) {
		return errors.New("ats.score must be an integer")
	}
	if len(r.ATS.ScoreReasoning) < 3 || len(r.ATS.ScoreReasoning) > 6 {
		return errors.New("ats.scoreReasoning must have 3-6 items")
	}
	if err := validateScoreBreakdownV2_3(&r.ATS.ScoreBreakdown); err != nil {
		return err
	}
	if err := validateScoreExplanationV1(&r.ATS.ScoreExplanation); err != nil {
		return err
	}

	for i, issue := range r.Issues {
		if issue.Priority < 1 || issue.Priority > 10 {
			return fmt.Errorf("issues[%d].priority must be between 1 and 10", i)
		}
		if issue.Evidence != "notFound" && utf8.RuneCountInString(issue.Evidence) > 160 {
			return fmt.Errorf("issues[%d].evidence must be <= 160 chars", i)
		}
		if issue.AutoFixable && len(issue.RequiresUserInput) > 0 {
			return fmt.Errorf("issues[%d].requiresUserInput must be empty when autoFixable=true", i)
		}
		for _, key := range issue.RequiresUserInput {
			if !isAllowedUserInputKey(key) {
				return fmt.Errorf("issues[%d].requiresUserInput contains invalid key: %s", i, key)
			}
		}
	}

	for i, br := range r.BulletRewrites {
		switch strings.ToLower(strings.TrimSpace(br.MetricsSource)) {
		case "resume":
			// ok
		case "placeholder":
			if len(br.PlaceholdersNeeded) == 0 {
				return fmt.Errorf("bulletRewrites[%d].placeholdersNeeded required when metricsSource=placeholder", i)
			}
		default:
			return fmt.Errorf("bulletRewrites[%d].metricsSource must be resume or placeholder", i)
		}

		switch br.ClaimSupport {
		case "supported", "inferred", "placeholder":
			// ok
		default:
			return fmt.Errorf("bulletRewrites[%d].claimSupport must be supported, inferred, or placeholder", i)
		}
		if br.ClaimSupport == "supported" && br.Evidence == "notFound" {
			return fmt.Errorf("bulletRewrites[%d].evidence required when claimSupport=supported", i)
		}
		if br.MetricsSource == "resume" && br.ClaimSupport == "placeholder" {
			return fmt.Errorf("bulletRewrites[%d].claimSupport cannot be placeholder when metricsSource=resume", i)
		}
		if br.Evidence != "notFound" && utf8.RuneCountInString(br.Evidence) > 160 {
			return fmt.Errorf("bulletRewrites[%d].evidence must be <= 160 chars", i)
		}
	}

	return nil
}

func validateScoreBreakdownV2_3(b *ScoreBreakdownV2) error {
	if b == nil {
		return errors.New("ats.scoreBreakdown is required")
	}
	values := []struct {
		name  string
		value float64
	}{
		{name: "skills", value: b.Skills},
		{name: "experience", value: b.Experience},
		{name: "impact", value: b.Impact},
		{name: "formatting", value: b.Formatting},
		{name: "roleFit", value: b.RoleFit},
	}
	total := 0.0
	for _, v := range values {
		if v.value < 0 || v.value > 100 {
			return fmt.Errorf("ats.scoreBreakdown.%s must be between 0 and 100", v.name)
		}
		if !isInteger(v.value) {
			return fmt.Errorf("ats.scoreBreakdown.%s must be an integer", v.name)
		}
		total += v.value
	}
	if math.Abs(total-100) > 0.000001 {
		return fmt.Errorf("ats.scoreBreakdown must total 100, got %.3f", total)
	}
	return nil
}
