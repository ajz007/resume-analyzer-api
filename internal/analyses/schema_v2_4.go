package analyses

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// AnalysisResultV2_4 represents the v2_4 analysis output schema.
type AnalysisResultV2_4 struct {
	Meta                  MetaV2                  `json:"meta"`
	Summary               SummaryV1               `json:"summary"`
	ATS                   ATSV2_3                 `json:"ats"`
	Issues                []IssueV2_2             `json:"issues"`
	BulletRewrites        []BulletRewriteV2_3     `json:"bulletRewrites"`
	MissingInformation    []string                `json:"missingInformation"`
	ActionPlan            ActionPlanV1            `json:"actionPlan"`
	JobRequirementProfile JobRequirementProfileV1 `json:"jobRequirementProfile"`
	JobMatchScoring       JobMatchScoringV1       `json:"jobMatchScoring"`
	AIScreening           AIScreeningV1           `json:"aiScreening"`
	FixThisFirst          []FixThisFirstItemV1    `json:"fixThisFirst"`
}

type JobRequirementProfileV1 struct {
	IsApplicable           bool                  `json:"isApplicable"`
	PrimaryRole            string                `json:"primaryRole"`
	Seniority              string                `json:"seniority"`
	RoleType               string                `json:"roleType"`
	RecruiterIntentSummary string                `json:"recruiterIntentSummary"`
	TopPriorities          []JobPriorityV1       `json:"topPriorities"`
	HiddenExpectations     []HiddenExpectationV1 `json:"hiddenExpectations"`
	NiceToHaveSignals      []NiceToHaveSignalV1  `json:"niceToHaveSignals"`
}

type JobPriorityV1 struct {
	ID                string  `json:"id"`
	Priority          string  `json:"priority"`
	Importance        string  `json:"importance"`
	Weight            float64 `json:"weight"`
	EvidenceExpected  string  `json:"evidenceExpected"`
	ResumeMatchStatus string  `json:"resumeMatchStatus"`
	WhyItMatters      string  `json:"whyItMatters"`
}

type HiddenExpectationV1 struct {
	ID                string `json:"id"`
	Expectation       string `json:"expectation"`
	ResumeMatchStatus string `json:"resumeMatchStatus"`
	WhyItMatters      string `json:"whyItMatters"`
}

type NiceToHaveSignalV1 struct {
	ID                string `json:"id"`
	Signal            string `json:"signal"`
	ResumeMatchStatus string `json:"resumeMatchStatus"`
	WhyItMatters      string `json:"whyItMatters"`
}

type JobMatchScoringV1 struct {
	Score             float64              `json:"score"`
	ScoringStrategy   string               `json:"scoringStrategy"`
	RequirementScores []RequirementScoreV1 `json:"requirementScores"`
	Explanation       string               `json:"explanation"`
}

type RequirementScoreV1 struct {
	RequirementID        string  `json:"requirementId"`
	Requirement          string  `json:"requirement"`
	Weight               float64 `json:"weight"`
	Score                float64 `json:"score"`
	WeightedContribution float64 `json:"weightedContribution"`
	MatchStatus          string  `json:"matchStatus"`
	Evidence             string  `json:"evidence"`
	Gap                  string  `json:"gap"`
}

type AIScreeningV1 struct {
	Score              float64                      `json:"score"`
	Verdict            AIScreeningVerdictV1         `json:"verdict"`
	ScoreBreakdown     []AIScreeningBreakdownItemV1 `json:"scoreBreakdown"`
	AIRecruiterVerdict AIRecruiterVerdictV1         `json:"aiRecruiterVerdict"`
}

type AIScreeningVerdictV1 struct {
	Tier          string `json:"tier"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
	ScreeningRisk string `json:"screeningRisk"`
}

type AIScreeningBreakdownItemV1 struct {
	ID               string  `json:"id"`
	Label            string  `json:"label"`
	Score            float64 `json:"score"`
	Weight           float64 `json:"weight"`
	Status           string  `json:"status"`
	Explanation      string  `json:"explanation"`
	ImprovementFocus string  `json:"improvementFocus"`
}

type AIRecruiterVerdictV1 struct {
	OneLineVerdict  string `json:"oneLineVerdict"`
	MainConcern     string `json:"mainConcern"`
	StrongestSignal string `json:"strongestSignal"`
	WeakestSignal   string `json:"weakestSignal"`
}

type FixThisFirstItemV1 struct {
	Priority            int    `json:"priority"`
	Title               string `json:"title"`
	Why                 string `json:"why"`
	LinkedRequirementID string `json:"linkedRequirementId"`
	ExpectedImpact      string `json:"expectedImpact"`
	Effort              string `json:"effort"`
	Action              string `json:"action"`
	RequiresUserInput   bool   `json:"requiresUserInput"`
}

// Validate checks basic schema constraints for v2_4.
func (r *AnalysisResultV2_4) Validate() error {
	if r == nil {
		return errors.New("analysis result is nil")
	}
	base := AnalysisResultV2_3{
		Meta:               r.Meta,
		Summary:            r.Summary,
		ATS:                r.ATS,
		Issues:             r.Issues,
		BulletRewrites:     r.BulletRewrites,
		MissingInformation: r.MissingInformation,
		ActionPlan:         r.ActionPlan,
	}
	if err := base.Validate(); err != nil {
		return err
	}
	if err := validateJobRequirementProfileV1(r); err != nil {
		return err
	}
	if err := validateJobMatchScoringV1(r); err != nil {
		return err
	}
	if err := validateAIScreeningV1(&r.AIScreening); err != nil {
		return err
	}
	if err := validateFixThisFirstV1(r.FixThisFirst); err != nil {
		return err
	}
	return nil
}

func validateJobRequirementProfileV1(r *AnalysisResultV2_4) error {
	profile := r.JobRequirementProfile
	if !isAllowedString(profile.Seniority, "ENTRY", "MID", "SENIOR", "STAFF", "MANAGER", "DIRECTOR", "UNCLEAR") {
		return errors.New("jobRequirementProfile.seniority is invalid")
	}
	if !isAllowedString(profile.RoleType, "BACKEND", "FRONTEND", "FULLSTACK", "DATA_ENGINEERING", "DEVOPS_SRE", "ENGINEERING_MANAGER", "PRODUCT", "DESIGN", "OTHER", "UNCLEAR") {
		return errors.New("jobRequirementProfile.roleType is invalid")
	}
	if isJobMatchV2_4(r.Meta) {
		if !profile.IsApplicable {
			return errors.New("jobRequirementProfile.isApplicable must be true when jobDescriptionProvided=true")
		}
		if len(profile.TopPriorities) < 3 || len(profile.TopPriorities) > 7 {
			return errors.New("jobRequirementProfile.topPriorities must contain 3-7 items")
		}
	}
	totalWeight := 0.0
	for i, item := range profile.TopPriorities {
		if !isAllowedString(item.Importance, "CRITICAL", "HIGH", "MEDIUM", "LOW") {
			return fmt.Errorf("jobRequirementProfile.topPriorities[%d].importance is invalid", i)
		}
		if !isAllowedMatchStatusV1(item.ResumeMatchStatus) {
			return fmt.Errorf("jobRequirementProfile.topPriorities[%d].resumeMatchStatus is invalid", i)
		}
		if err := validateWeightV2_4(item.Weight, fmt.Sprintf("jobRequirementProfile.topPriorities[%d].weight", i)); err != nil {
			return err
		}
		if trimmedLen(item.EvidenceExpected) > 200 {
			return fmt.Errorf("jobRequirementProfile.topPriorities[%d].evidenceExpected must be <= 200 chars", i)
		}
		totalWeight += item.Weight
	}
	if isJobMatchV2_4(r.Meta) && math.Abs(totalWeight-100) > 0.000001 {
		return fmt.Errorf("jobRequirementProfile.topPriorities weights must total 100, got %.3f", totalWeight)
	}
	for i, item := range profile.HiddenExpectations {
		if item.ResumeMatchStatus != "" && !isAllowedMatchStatusV1(item.ResumeMatchStatus) {
			return fmt.Errorf("jobRequirementProfile.hiddenExpectations[%d].resumeMatchStatus is invalid", i)
		}
	}
	for i, item := range profile.NiceToHaveSignals {
		if item.ResumeMatchStatus != "" && !isAllowedMatchStatusV1(item.ResumeMatchStatus) {
			return fmt.Errorf("jobRequirementProfile.niceToHaveSignals[%d].resumeMatchStatus is invalid", i)
		}
	}
	return nil
}

func validateJobMatchScoringV1(r *AnalysisResultV2_4) error {
	scoring := r.JobMatchScoring
	if isZeroJobMatchScoringV1(scoring) && !isJobMatchV2_4(r.Meta) {
		return nil
	}
	if err := validateScoreFieldV2_4(scoring.Score, "jobMatchScoring.score"); err != nil {
		return err
	}
	if !isAllowedString(scoring.ScoringStrategy, "JD_WEIGHTED", "GENERIC") {
		return errors.New("jobMatchScoring.scoringStrategy is invalid")
	}
	if isJobMatchV2_4(r.Meta) {
		if scoring.ScoringStrategy != "JD_WEIGHTED" {
			return errors.New("jobMatchScoring.scoringStrategy must be JD_WEIGHTED when jobDescriptionProvided=true")
		}
		if len(scoring.RequirementScores) < 3 || len(scoring.RequirementScores) > 7 {
			return errors.New("jobMatchScoring.requirementScores must contain 3-7 items")
		}
	}
	totalWeight := 0.0
	for i, item := range scoring.RequirementScores {
		if err := validateWeightV2_4(item.Weight, fmt.Sprintf("jobMatchScoring.requirementScores[%d].weight", i)); err != nil {
			return err
		}
		if err := validateScoreFieldV2_4(item.Score, fmt.Sprintf("jobMatchScoring.requirementScores[%d].score", i)); err != nil {
			return err
		}
		if item.WeightedContribution < 0 || item.WeightedContribution > 100 {
			return fmt.Errorf("jobMatchScoring.requirementScores[%d].weightedContribution must be between 0 and 100", i)
		}
		if !isAllowedMatchStatusV1(item.MatchStatus) {
			return fmt.Errorf("jobMatchScoring.requirementScores[%d].matchStatus is invalid", i)
		}
		if trimmedLen(item.Evidence) > 200 {
			return fmt.Errorf("jobMatchScoring.requirementScores[%d].evidence must be <= 200 chars", i)
		}
		totalWeight += item.Weight
	}
	if isJobMatchV2_4(r.Meta) && math.Abs(totalWeight-100) > 0.000001 {
		return fmt.Errorf("jobMatchScoring.requirementScores weights must total 100, got %.3f", totalWeight)
	}
	return nil
}

func validateAIScreeningV1(screening *AIScreeningV1) error {
	if screening == nil {
		return errors.New("aiScreening is required")
	}
	if err := validateScoreFieldV2_4(screening.Score, "aiScreening.score"); err != nil {
		return err
	}
	if !isAllowedString(screening.Verdict.Tier, "STRONG", "GOOD", "BORDERLINE", "WEAK") {
		return errors.New("aiScreening.verdict.tier is invalid")
	}
	if !isAllowedString(screening.Verdict.ScreeningRisk, "LOW", "MEDIUM", "HIGH") {
		return errors.New("aiScreening.verdict.screeningRisk is invalid")
	}
	expected := map[string]float64{
		"semantic_relevance":    25,
		"role_intent_alignment": 20,
		"evidence_strength":     20,
		"impact_strength":       15,
		"signal_density":        10,
		"clarity_specificity":   10,
	}
	if len(screening.ScoreBreakdown) != len(expected) {
		return errors.New("aiScreening.scoreBreakdown must contain exactly six items")
	}
	seen := map[string]bool{}
	totalWeight := 0.0
	for i, item := range screening.ScoreBreakdown {
		expectedWeight, ok := expected[item.ID]
		if !ok {
			return fmt.Errorf("aiScreening.scoreBreakdown[%d].id is invalid", i)
		}
		if seen[item.ID] {
			return fmt.Errorf("aiScreening.scoreBreakdown[%d].id must be unique", i)
		}
		seen[item.ID] = true
		if err := validateScoreFieldV2_4(item.Score, fmt.Sprintf("aiScreening.scoreBreakdown[%d].score", i)); err != nil {
			return err
		}
		if item.Weight != expectedWeight {
			return fmt.Errorf("aiScreening.scoreBreakdown[%d].weight must be %.0f", i, expectedWeight)
		}
		if !isAllowedString(item.Status, "STRONG", "OK", "WEAK") {
			return fmt.Errorf("aiScreening.scoreBreakdown[%d].status is invalid", i)
		}
		totalWeight += item.Weight
	}
	if len(seen) != len(expected) {
		return errors.New("aiScreening.scoreBreakdown must include all required ids")
	}
	if math.Abs(totalWeight-100) > 0.000001 {
		return fmt.Errorf("aiScreening.scoreBreakdown weights must total 100, got %.3f", totalWeight)
	}
	return nil
}

func validateFixThisFirstV1(items []FixThisFirstItemV1) error {
	if len(items) > 5 {
		return errors.New("fixThisFirst must have at most 5 items")
	}
	for i, item := range items {
		if item.Priority < 1 || item.Priority > 5 {
			return fmt.Errorf("fixThisFirst[%d].priority must be between 1 and 5", i)
		}
		if !isAllowedString(item.ExpectedImpact, "HIGH", "MEDIUM", "LOW") {
			return fmt.Errorf("fixThisFirst[%d].expectedImpact is invalid", i)
		}
		if !isAllowedString(item.Effort, "LOW", "MEDIUM", "HIGH") {
			return fmt.Errorf("fixThisFirst[%d].effort is invalid", i)
		}
	}
	return nil
}

func isJobMatchV2_4(meta MetaV2) bool {
	return meta.JobDescriptionProvided || strings.EqualFold(strings.TrimSpace(meta.Mode), string(ModeJobMatch))
}

func isZeroJobMatchScoringV1(scoring JobMatchScoringV1) bool {
	return scoring.Score == 0 &&
		strings.TrimSpace(scoring.ScoringStrategy) == "" &&
		len(scoring.RequirementScores) == 0 &&
		strings.TrimSpace(scoring.Explanation) == ""
}

func validateScoreFieldV2_4(value float64, name string) error {
	if value < 0 || value > 100 {
		return fmt.Errorf("%s must be between 0 and 100", name)
	}
	if !isInteger(value) {
		return fmt.Errorf("%s must be an integer", name)
	}
	return nil
}

func validateWeightV2_4(value float64, name string) error {
	if value < 0 || value > 100 {
		return fmt.Errorf("%s must be between 0 and 100", name)
	}
	return nil
}

func isAllowedMatchStatusV1(value string) bool {
	return isAllowedString(value, "STRONG", "PARTIAL", "WEAK", "MISSING", "UNKNOWN")
}

func isAllowedString(value string, allowed ...string) bool {
	trimmed := strings.TrimSpace(value)
	for _, item := range allowed {
		if trimmed == item {
			return true
		}
	}
	return false
}

func trimmedLen(value string) int {
	return utf8.RuneCountInString(strings.TrimSpace(value))
}
