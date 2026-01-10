package analyses

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// AnalysisResultV2_1 represents the v2_1 analysis output schema.
type AnalysisResultV2_1 struct {
	Meta               MetaV2              `json:"meta"`
	Summary            SummaryV1           `json:"summary"`
	ATS                ATSV2               `json:"ats"`
	Issues             []IssueV2_1         `json:"issues"`
	BulletRewrites     []BulletRewriteV2_1 `json:"bulletRewrites"`
	MissingInformation []string            `json:"missingInformation"`
	ActionPlan         ActionPlanV1        `json:"actionPlan"`
}

type IssueV2_1 struct {
	Severity     IssueSeverityV1 `json:"severity"`
	Section      string          `json:"section"`
	Problem      string          `json:"problem"`
	WhyItMatters string          `json:"whyItMatters"`
	Suggestion   string          `json:"suggestion"`
	Evidence     string          `json:"evidence"`
	FixEffort    string          `json:"fixEffort"`
	Priority     int             `json:"priority"`
}

type BulletRewriteV2_1 struct {
	Section            string   `json:"section"`
	Before             string   `json:"before"`
	After              string   `json:"after"`
	Rationale          string   `json:"rationale"`
	MetricsSource      string   `json:"metricsSource"`
	PlaceholdersNeeded []string `json:"placeholdersNeeded"`
}

// Validate checks basic schema constraints for v2_1.
func (r *AnalysisResultV2_1) Validate() error {
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
	if err := validateScoreBreakdown(&r.ATS.ScoreBreakdown); err != nil {
		return err
	}

	for i, issue := range r.Issues {
		if issue.Priority < 1 || issue.Priority > 10 {
			return fmt.Errorf("issues[%d].priority must be between 1 and 10", i)
		}
		if issue.Evidence != "notFound" && utf8.RuneCountInString(issue.Evidence) > 160 {
			return fmt.Errorf("issues[%d].evidence must be <= 160 chars", i)
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

func validateScoreBreakdown(b *ScoreBreakdownV2) error {
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

func isInteger(v float64) bool {
	return math.Abs(v-math.Round(v)) <= 0.000001
}
