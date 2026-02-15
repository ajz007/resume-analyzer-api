package analyses

import (
	"errors"
	"fmt"
	"math"
)

// AnalysisResultV2 represents the v2 analysis output schema.
type AnalysisResultV2 struct {
	Meta               MetaV2            `json:"meta"`
	Summary            SummaryV1         `json:"summary"`
	ATS                ATSV2             `json:"ats"`
	Issues             []IssueV2         `json:"issues"`
	BulletRewrites     []BulletRewriteV1 `json:"bulletRewrites"`
	MissingInformation []string          `json:"missingInformation"`
	ActionPlan         ActionPlanV1      `json:"actionPlan"`
}

type MetaV2 struct {
	PromptVersion          string   `json:"promptVersion"`
	Model                  string   `json:"model"`
	JobDescriptionProvided bool     `json:"jobDescriptionProvided"`
	Confidence             float64  `json:"confidence"`
	Assumptions            []string `json:"assumptions"`
	Limitations            []string `json:"limitations"`
	Mode                   string   `json:"mode,omitempty"`
	PrimaryScoreType       string   `json:"primaryScoreType,omitempty"`
}

type ATSV2 struct {
	Score            float64           `json:"score"`
	ScoreBreakdown   ScoreBreakdownV2  `json:"scoreBreakdown"`
	MissingKeywords  MissingKeywordsV2 `json:"missingKeywords"`
	FormattingIssues []string          `json:"formattingIssues"`
}

type ScoreBreakdownV2 struct {
	Skills     float64 `json:"skills"`
	Experience float64 `json:"experience"`
	Impact     float64 `json:"impact"`
	Formatting float64 `json:"formatting"`
	RoleFit    float64 `json:"roleFit"`
}

type MissingKeywordsV2 struct {
	FromJobDescription []string `json:"fromJobDescription"`
	IndustryCommon     []string `json:"industryCommon"`
}

type IssueV2 struct {
	Severity     IssueSeverityV1 `json:"severity"`
	Section      string          `json:"section"`
	Problem      string          `json:"problem"`
	WhyItMatters string          `json:"whyItMatters"`
	Suggestion   string          `json:"suggestion"`
	Evidence     string          `json:"evidence"`
	FixEffort    string          `json:"fixEffort"`
}

// Validate checks basic schema constraints.
func (r *AnalysisResultV2) Validate() error {
	if r == nil {
		return errors.New("analysis result is nil")
	}
	if r.Meta.PromptVersion == "" || r.Meta.Model == "" {
		return errors.New("meta.promptVersion and meta.model are required")
	}
	if r.Summary.OverallAssessment == "" {
		return errors.New("summary.overallAssessment is required")
	}
	if r.ATS.Score < 0 || r.ATS.Score > 100 {
		return errors.New("ats.score must be between 0 and 100")
	}
	if err := normalizeScoreBreakdown(&r.ATS.ScoreBreakdown); err != nil {
		return err
	}
	return nil
}

func normalizeScoreBreakdown(b *ScoreBreakdownV2) error {
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
	for _, v := range values {
		if v.value < 0 || v.value > 100 {
			return fmt.Errorf("ats.scoreBreakdown.%s must be between 0 and 100", v.name)
		}
		if math.Abs(v.value-math.Round(v.value)) > 0.000001 {
			return fmt.Errorf("ats.scoreBreakdown.%s must be an integer", v.name)
		}
	}

	total := b.Skills + b.Experience + b.Impact + b.Formatting + b.RoleFit
	if math.Abs(total-100) <= 0.000001 {
		return nil
	}

	delta := 100 - total
	b.Formatting += delta
	if b.Formatting < 0 || b.Formatting > 100 {
		return fmt.Errorf("ats.scoreBreakdown.formatting adjustment out of range: %.3f", b.Formatting)
	}

	total = b.Skills + b.Experience + b.Impact + b.Formatting + b.RoleFit
	if math.Abs(total-100) > 0.000001 {
		return fmt.Errorf("ats.scoreBreakdown must total 100, got %.3f", total)
	}
	return nil
}
