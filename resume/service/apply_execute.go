package service

import (
	"context"
	"strings"

	"resume-backend/resume/model"
	"resume-backend/resume/render"
)

const (
	ApplyResultDraft = "DRAFT"
	ApplyResultFinal = "FINAL"
)

// ApplyHeaderInputs captures user-supplied header fields.
type ApplyHeaderInputs struct {
	Name     string
	Title    string
	Email    string
	Phone    string
	Location string
	Links    []string
}

// ApplyExecutionResult represents the outcome of an apply execution.
type ApplyExecutionResult struct {
	DocxBytes             []byte
	AutoFixesApplied      int
	SafeRewritesApplied   int
	PlaceholdersRemaining int
	Status                string
	Plan                  ApplyPlan
}

// ExecuteApply regenerates a resume with fixes and rewrites applied.
func ExecuteApply(ctx context.Context, resumeText string, analysis AnalysisResultV2_3, headerInputs ApplyHeaderInputs) (ApplyExecutionResult, error) {
	plan := BuildApplyPlan(analysis)

	resumeModel, err := BuildResumeModel(ctx, resumeText)
	if err != nil {
		return ApplyExecutionResult{}, err
	}

	autoFixesApplied := applyAutoFixes(&resumeModel, plan.AutoFixes)
	safeRewritesApplied := applySafeRewrites(&resumeModel, plan.SafeRewrites)
	applyHeaderInputs(&resumeModel, headerInputs)

	if err := resumeModel.Validate(); err != nil {
		return ApplyExecutionResult{}, err
	}

	docxBytes, err := render.RenderResume(resumeModel)
	if err != nil {
		return ApplyExecutionResult{}, err
	}

	placeholdersRemaining := countPlaceholders(plan.BlockedRewrites)
	status := ApplyResultFinal
	if placeholdersRemaining > 0 {
		status = ApplyResultDraft
	}

	return ApplyExecutionResult{
		DocxBytes:             docxBytes,
		AutoFixesApplied:      autoFixesApplied,
		SafeRewritesApplied:   safeRewritesApplied,
		PlaceholdersRemaining: placeholdersRemaining,
		Status:                status,
		Plan:                  plan,
	}, nil
}

func applyHeaderInputs(resumeModel *model.ResumeModel, inputs ApplyHeaderInputs) {
	if inputs.Name != "" {
		resumeModel.Header.Name = inputs.Name
	}
	if inputs.Title != "" {
		resumeModel.Header.Title = inputs.Title
	}
	if inputs.Email != "" {
		resumeModel.Header.Email = inputs.Email
	}
	if inputs.Phone != "" {
		resumeModel.Header.Phone = inputs.Phone
	}
	if inputs.Location != "" {
		resumeModel.Header.Location = inputs.Location
	}
	if len(inputs.Links) > 0 {
		resumeModel.Header.Links = inputs.Links
	}
}

func applyAutoFixes(resumeModel *model.ResumeModel, autoFixes []AnalysisIssue) int {
	applied := 0
	for _, issue := range autoFixes {
		if applySensitiveHeaderFix(resumeModel, issue) {
			applied++
		}
	}
	return applied
}

func applySensitiveHeaderFix(resumeModel *model.ResumeModel, issue AnalysisIssue) bool {
	section := strings.ToLower(issue.Section)
	problem := strings.ToLower(issue.Problem)
	if !strings.Contains(section, "personal") &&
		!strings.Contains(problem, "nationality") &&
		!strings.Contains(problem, "marital") {
		return false
	}

	changed := false
	if resumeModel.Header.Nationality != "" {
		resumeModel.Header.Nationality = ""
		changed = true
	}
	if resumeModel.Header.MaritalStatus != "" {
		resumeModel.Header.MaritalStatus = ""
		changed = true
	}

	if len(resumeModel.Summary) > 0 {
		filtered := resumeModel.Summary[:0]
		for _, line := range resumeModel.Summary {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "nationality") || strings.Contains(lower, "marital") {
				changed = true
				continue
			}
			filtered = append(filtered, line)
		}
		resumeModel.Summary = filtered
	}
	return changed
}

func applySafeRewrites(resumeModel *model.ResumeModel, rewrites []BulletRewrite) int {
	applied := 0
	for _, rewrite := range rewrites {
		if rewrite.Before == "" || rewrite.After == "" {
			continue
		}
		if applyRewriteToHighlights(resumeModel, rewrite.Before, rewrite.After) {
			applied++
		}
	}
	return applied
}

func applyRewriteToHighlights(resumeModel *model.ResumeModel, before, after string) bool {
	for expIndex := range resumeModel.Experience {
		highlights := resumeModel.Experience[expIndex].Highlights
		for i, highlight := range highlights {
			if highlight == before {
				highlights[i] = after
				resumeModel.Experience[expIndex].Highlights = highlights
				return true
			}
		}
	}
	return false
}

func countPlaceholders(rewrites []BulletRewrite) int {
	count := 0
	for _, rewrite := range rewrites {
		count += len(rewrite.PlaceholdersNeeded)
	}
	return count
}
