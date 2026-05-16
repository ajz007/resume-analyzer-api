package analyses

import (
	"context"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"
)

const maxRecoveredEvidenceRunes = 160

type validationTelemetryAnalysisIDKey struct{}

// Recovery is intentionally limited to presentation-quality fields and arrays.
// Scores, required sections, and enum-like business meaning still flow through
// fatal schema validation after this pass.
func withValidationTelemetryAnalysisID(ctx context.Context, analysisID string) context.Context {
	if analysisID == "" {
		return ctx
	}
	return context.WithValue(ctx, validationTelemetryAnalysisIDKey{}, analysisID)
}

func recoverAnalysisV2_3(r *AnalysisResultV2_3) []ValidationIssue {
	if r == nil {
		return []ValidationIssue{{
			Path:     "$",
			Message:  "analysis result is nil",
			Severity: ValidationFatal,
		}}
	}

	var issues []ValidationIssue
	issues = append(issues, recoverSummary(&r.Summary)...)
	issues = append(issues, recoverATSV2_3(&r.ATS)...)
	issues = append(issues, recoverStringSlice(&r.MissingInformation, "missingInformation")...)
	issues = append(issues, recoverActionPlan(&r.ActionPlan)...)
	for i := range r.Issues {
		path := fmt.Sprintf("issues[%d]", i)
		issues = append(issues, recoverStringSlice(&r.Issues[i].RequiresUserInput, path+".requiresUserInput")...)
		before := r.Issues[i].Evidence
		r.Issues[i].Evidence = sanitizeEvidence(r.Issues[i].Evidence, maxRecoveredEvidenceRunes)
		if before != r.Issues[i].Evidence {
			issues = append(issues, ValidationIssue{
				Path:     path + ".evidence",
				Message:  "evidence was capped to validation length",
				Severity: ValidationRecoverable,
			})
		}
	}
	for i := range r.BulletRewrites {
		path := fmt.Sprintf("bulletRewrites[%d]", i)
		issues = append(issues, recoverStringSlice(&r.BulletRewrites[i].PlaceholdersNeeded, path+".placeholdersNeeded")...)
		before := r.BulletRewrites[i].Evidence
		r.BulletRewrites[i].Evidence = sanitizeEvidence(r.BulletRewrites[i].Evidence, maxRecoveredEvidenceRunes)
		if before != r.BulletRewrites[i].Evidence {
			issues = append(issues, ValidationIssue{
				Path:     path + ".evidence",
				Message:  "evidence was capped to validation length",
				Severity: ValidationRecoverable,
			})
		}
	}
	SanitizeV2_3(r)
	return issues
}

func recoverAnalysisV2_4(r *AnalysisResultV2_4) []ValidationIssue {
	if r == nil {
		return []ValidationIssue{{
			Path:     "$",
			Message:  "analysis result is nil",
			Severity: ValidationFatal,
		}}
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
	issues := recoverAnalysisV2_3(&base)
	r.Summary = base.Summary
	r.ATS = base.ATS
	r.Issues = base.Issues
	r.BulletRewrites = base.BulletRewrites
	r.MissingInformation = base.MissingInformation
	r.ActionPlan = base.ActionPlan

	issues = append(issues, recoverStringSlice(&r.Meta.Assumptions, "meta.assumptions")...)
	issues = append(issues, recoverStringSlice(&r.Meta.Limitations, "meta.limitations")...)
	issues = append(issues, recoverJobRequirementProfile(&r.JobRequirementProfile)...)
	issues = append(issues, recoverJobMatchScoring(&r.JobMatchScoring)...)
	if r.AIScreening.ScoreBreakdown == nil {
		r.AIScreening.ScoreBreakdown = []AIScreeningBreakdownItemV1{}
		issues = append(issues, ValidationIssue{
			Path:     "aiScreening.scoreBreakdown",
			Message:  "null array normalized to empty array",
			Severity: ValidationRecoverable,
		})
	}
	if r.FixThisFirst == nil {
		r.FixThisFirst = []FixThisFirstItemV1{}
		issues = append(issues, ValidationIssue{
			Path:     "fixThisFirst",
			Message:  "null array normalized to empty array",
			Severity: ValidationRecoverable,
		})
	}
	SanitizeV2_4(r)
	return issues
}

func validateAnalysisV2_3(r *AnalysisResultV2_3) []ValidationIssue {
	var issues []ValidationIssue
	if r == nil {
		return appendFatalValidationIssue(issues, "$", fmt.Errorf("analysis result is nil"))
	}
	issues = append(issues, validateScoreExplanationSoft(&r.ATS.ScoreExplanation)...)
	if err := r.Validate(); err != nil {
		issues = appendFatalValidationIssue(issues, "$", err)
	}
	return issues
}

func validateAnalysisV2_4(r *AnalysisResultV2_4) []ValidationIssue {
	var issues []ValidationIssue
	if r == nil {
		return appendFatalValidationIssue(issues, "$", fmt.Errorf("analysis result is nil"))
	}
	issues = append(issues, validateScoreExplanationSoft(&r.ATS.ScoreExplanation)...)
	if err := r.Validate(); err != nil {
		issues = appendFatalValidationIssue(issues, "$", err)
	}
	return issues
}

func logValidationTelemetry(ctx context.Context, event, promptVersion string, issues []ValidationIssue) {
	if len(issues) == 0 {
		return
	}
	counts := countValidationIssuesBySeverity(issues)
	analysisID, _ := ctx.Value(validationTelemetryAnalysisIDKey{}).(string)
	log.Printf("%s analysis_id=%s prompt_version=%s issue_count=%d fatal_count=%d recoverable_count=%d warning_count=%d",
		event,
		analysisID,
		promptVersion,
		len(issues),
		counts[ValidationFatal],
		counts[ValidationRecoverable],
		counts[ValidationWarning],
	)
}

func recoverATSV2_3(ats *ATSV2_3) []ValidationIssue {
	var issues []ValidationIssue
	issues = append(issues, recoverStringSlice(&ats.ScoreReasoning, "ats.scoreReasoning")...)
	issues = append(issues, recoverScoreExplanation(&ats.ScoreExplanation)...)
	issues = append(issues, recoverStringSlice(&ats.MissingKeywords.FromJobDescription, "ats.missingKeywords.fromJobDescription")...)
	issues = append(issues, recoverStringSlice(&ats.MissingKeywords.IndustryCommon, "ats.missingKeywords.industryCommon")...)
	issues = append(issues, recoverStringSlice(&ats.FormattingIssues, "ats.formattingIssues")...)
	return issues
}

func recoverScoreExplanation(e *ScoreExplanationV1) []ValidationIssue {
	if e == nil {
		return nil
	}
	var issues []ValidationIssue
	if e.Components == nil {
		e.Components = []ScoreComponentV1{}
		issues = append(issues, ValidationIssue{
			Path:     "ats.scoreExplanation.components",
			Message:  "null array normalized to empty array",
			Severity: ValidationRecoverable,
		})
	}
	for i := range e.Components {
		path := fmt.Sprintf("ats.scoreExplanation.components[%d]", i)
		e.Components[i].Key = strings.TrimSpace(e.Components[i].Key)
		e.Components[i].Label = strings.TrimSpace(e.Components[i].Label)
		e.Components[i].Explanation = strings.TrimSpace(e.Components[i].Explanation)
		issues = append(issues, recoverStringSlice(&e.Components[i].Helped, path+".helped")...)
		issues = append(issues, recoverStringSlice(&e.Components[i].Dragged, path+".dragged")...)
	}
	return issues
}

func validateScoreExplanationSoft(e *ScoreExplanationV1) []ValidationIssue {
	if e == nil {
		return nil
	}
	var issues []ValidationIssue
	for i, c := range e.Components {
		path := fmt.Sprintf("ats.scoreExplanation.components[%d]", i)
		if len(c.Helped) == 0 {
			issues = append(issues, ValidationIssue{
				Path:     path + ".helped",
				Message:  "helped array is empty",
				Severity: ValidationWarning,
			})
		}
		if len(c.Dragged) == 0 {
			issues = append(issues, ValidationIssue{
				Path:     path + ".dragged",
				Message:  "dragged array is empty",
				Severity: ValidationWarning,
			})
		}
	}
	return issues
}

func recoverSummary(summary *SummaryV1) []ValidationIssue {
	var issues []ValidationIssue
	issues = append(issues, recoverStringSlice(&summary.Strengths, "summary.strengths")...)
	issues = append(issues, recoverStringSlice(&summary.Weaknesses, "summary.weaknesses")...)
	return issues
}

func recoverActionPlan(plan *ActionPlanV1) []ValidationIssue {
	var issues []ValidationIssue
	issues = append(issues, recoverStringSlice(&plan.QuickWins, "actionPlan.quickWins")...)
	issues = append(issues, recoverStringSlice(&plan.MediumEffort, "actionPlan.mediumEffort")...)
	issues = append(issues, recoverStringSlice(&plan.DeepFixes, "actionPlan.deepFixes")...)
	return issues
}

func recoverJobRequirementProfile(profile *JobRequirementProfileV1) []ValidationIssue {
	var issues []ValidationIssue
	if profile.TopPriorities == nil {
		profile.TopPriorities = []JobPriorityV1{}
		issues = append(issues, ValidationIssue{Path: "jobRequirementProfile.topPriorities", Message: "null array normalized to empty array", Severity: ValidationRecoverable})
	}
	if profile.HiddenExpectations == nil {
		profile.HiddenExpectations = []HiddenExpectationV1{}
		issues = append(issues, ValidationIssue{Path: "jobRequirementProfile.hiddenExpectations", Message: "null array normalized to empty array", Severity: ValidationRecoverable})
	}
	if profile.NiceToHaveSignals == nil {
		profile.NiceToHaveSignals = []NiceToHaveSignalV1{}
		issues = append(issues, ValidationIssue{Path: "jobRequirementProfile.niceToHaveSignals", Message: "null array normalized to empty array", Severity: ValidationRecoverable})
	}
	return issues
}

func recoverJobMatchScoring(scoring *JobMatchScoringV1) []ValidationIssue {
	var issues []ValidationIssue
	if scoring.RequirementScores == nil {
		scoring.RequirementScores = []RequirementScoreV1{}
		issues = append(issues, ValidationIssue{Path: "jobMatchScoring.requirementScores", Message: "null array normalized to empty array", Severity: ValidationRecoverable})
	}
	for i := range scoring.RequirementScores {
		path := fmt.Sprintf("jobMatchScoring.requirementScores[%d].evidence", i)
		before := scoring.RequirementScores[i].Evidence
		scoring.RequirementScores[i].Evidence = sanitizeEvidence(scoring.RequirementScores[i].Evidence, 200)
		if before != scoring.RequirementScores[i].Evidence {
			issues = append(issues, ValidationIssue{Path: path, Message: "evidence was capped to validation length", Severity: ValidationRecoverable})
		}
	}
	return issues
}

func recoverStringSlice(values *[]string, path string) []ValidationIssue {
	var issues []ValidationIssue
	if *values == nil {
		*values = []string{}
		issues = append(issues, ValidationIssue{
			Path:     path,
			Message:  "null array normalized to empty array",
			Severity: ValidationRecoverable,
		})
	}

	seen := make(map[string]bool, len(*values))
	out := make([]string, 0, len(*values))
	changed := false
	for _, value := range *values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			changed = true
			issues = append(issues, ValidationIssue{
				Path:     path,
				Message:  "empty array item removed",
				Severity: ValidationRecoverable,
			})
			continue
		}
		if utf8.RuneCountInString(trimmed) > 500 {
			trimmed = truncateRunes(trimmed, 500)
			changed = true
			issues = append(issues, ValidationIssue{
				Path:     path,
				Message:  "overlong array item was truncated",
				Severity: ValidationRecoverable,
			})
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			changed = true
			issues = append(issues, ValidationIssue{
				Path:     path,
				Message:  "duplicate array item removed",
				Severity: ValidationWarning,
			})
			continue
		}
		if trimmed != value {
			changed = true
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	if changed {
		*values = out
	}
	return issues
}

func truncateRunes(value string, max int) string {
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}
