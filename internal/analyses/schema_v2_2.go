package analyses

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// AnalysisResultV2_2 represents the v2_2 analysis output schema.
type AnalysisResultV2_2 struct {
	Meta               MetaV2              `json:"meta"`
	Summary            SummaryV1           `json:"summary"`
	ATS                ATSV2_2             `json:"ats"`
	Issues             []IssueV2_2         `json:"issues"`
	BulletRewrites     []BulletRewriteV2_1 `json:"bulletRewrites"`
	MissingInformation []string            `json:"missingInformation"`
	ActionPlan         ActionPlanV1        `json:"actionPlan"`
}

type ATSV2_2 struct {
	Score            float64           `json:"score"`
	ScoreBreakdown   ScoreBreakdownV2  `json:"scoreBreakdown"`
	ScoreReasoning   []string          `json:"scoreReasoning"`
	MissingKeywords  MissingKeywordsV2 `json:"missingKeywords"`
	FormattingIssues []string          `json:"formattingIssues"`
}

type IssueV2_2 struct {
	Severity          IssueSeverityV1 `json:"severity"`
	Section           string          `json:"section"`
	Problem           string          `json:"problem"`
	WhyItMatters      string          `json:"whyItMatters"`
	Suggestion        string          `json:"suggestion"`
	Evidence          string          `json:"evidence"`
	FixEffort         string          `json:"fixEffort"`
	Priority          int             `json:"priority"`
	AutoFixable       bool            `json:"autoFixable"`
	RequiresUserInput []string        `json:"requiresUserInput"`
}

// Validate checks basic schema constraints for v2_2.
func (r *AnalysisResultV2_2) Validate() error {
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
	if err := validateScoreBreakdownV2_2(&r.ATS.ScoreBreakdown); err != nil {
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
	}

	return nil
}

func validateScoreBreakdownV2_2(b *ScoreBreakdownV2) error {
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

func isAllowedUserInputKey(key string) bool {
	switch key {
	case "email", "phone", "linkedin", "crm_tools", "metrics", "team_size", "award_dates", "target_role":
		return true
	default:
		return false
	}
}
