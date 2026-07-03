package resumes

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"resume-backend/internal/shared/telemetry"
	modelv1 "resume-backend/resume/modelv1"
)

const (
	draftTypeResumeDraft    = "resume_draft"
	draftTypeSampleTemplate = "sample_template"
)

type generationPlan struct {
	Response       modelv1.ResumeGenerationResponse
	FallbackUsed   bool
	FallbackReason string
	DraftType      string
}

func (s *Service) generatePlan(ctx context.Context, req GenerateRequest) (generationPlan, error) {
	raw, err := s.completeGenerationWithRetry(ctx, req, buildGenerationPrompt(req), "initial")
	if err != nil {
		return generationPlan{}, err
	}

	plan, errs, repaired := validateGenerationCandidate(raw, req)
	if len(errs) == 0 {
		plan.DraftType = inferDraftType(req)
		logGenerationValidation(ctx, req, "resume.generate.validation.repaired", len(errs), repaired, false, false, "", "")
		return plan, nil
	}

	logGenerationValidation(ctx, req, "resume.generate.validation.failed", len(errs), repaired, false, false, "", validationSummary(errs))

	retryRaw, retryErr := s.completeGenerationWithRetry(ctx, req, buildGenerationRetryPrompt(req, raw, errs), "retry")
	if retryErr == nil {
		retryPlan, retryErrs, retryRepaired := validateGenerationCandidate(retryRaw, req)
		if len(retryErrs) == 0 {
			retryPlan.DraftType = inferDraftType(req)
			logGenerationValidation(ctx, req, "resume.generate.validation.retry_completed", len(retryErrs), retryRepaired, true, false, "", "")
			return retryPlan, nil
		}
		logGenerationValidation(ctx, req, "resume.generate.validation.retry_failed", len(retryErrs), retryRepaired, true, false, "", validationSummary(retryErrs))
	} else if err := maybeTimeoutError(retryErr); err != nil {
		return generationPlan{}, err
	}

	fallback, fallbackErr := buildFallbackGenerationResponse(req)
	if fallbackErr != nil {
		return generationPlan{}, fallbackErr
	}
	logGenerationValidation(ctx, req, "resume.generate.validation.fallback_used", 0, true, true, true, fallback.FallbackReason, "")
	return fallback, nil
}

func (s *Service) completeGenerationWithRetry(ctx context.Context, req GenerateRequest, prompt string, phase string) (string, error) {
	raw, err := s.completeGenerationOnce(ctx, req, prompt, phase)
	if err == nil {
		return raw, nil
	}
	if !errors.Is(err, ErrGenerationTimeout) {
		return "", err
	}
	raw, retryErr := s.completeGenerationOnce(ctx, req, prompt, phase+"_retry")
	if retryErr != nil {
		return "", maybeTimeoutError(retryErr)
	}
	return raw, nil
}

func maybeTimeoutError(err error) error {
	if err == nil {
		return nil
	}
	if isGenerationTimeoutError(err) {
		return ErrGenerationTimeout
	}
	return err
}

func (s *Service) completeGenerationOnce(ctx context.Context, req GenerateRequest, prompt string, phase string) (string, error) {
	llmStart := time.Now()
	telemetry.Info("resume.generate.llm.start", generationLogFields(ctx, req, map[string]any{
		"duration_ms": 0.0,
		"phase":       phase,
	}))
	raw, err := s.LLM.Complete(ctx, prompt)
	if err != nil {
		fields := generationLogFields(ctx, req, map[string]any{
			"duration_ms": durationMilliseconds(time.Since(llmStart)),
			"phase":       phase,
		})
		if isGenerationTimeoutError(err) {
			telemetry.Error("resume.generate.llm.timeout", fields)
			return "", ErrGenerationTimeout
		}
		fields["error"] = sanitizeGenerationError(err)
		telemetry.Error("resume.generate.llm.error", fields)
		return "", err
	}
	telemetry.Info("resume.generate.llm.finish", generationLogFields(ctx, req, map[string]any{
		"duration_ms": durationMilliseconds(time.Since(llmStart)),
		"phase":       phase,
	}))
	return raw, nil
}

func validateGenerationCandidate(raw string, req GenerateRequest) (generationPlan, []modelv1.ValidationError, bool) {
	var response modelv1.ResumeGenerationResponse
	if err := decodeGenerationResponse(raw, &response); err != nil {
		return generationPlan{}, []modelv1.ValidationError{{Field: "body", Message: err.Error()}}, false
	}
	repaired := repairGenerationResponse(&response, req)
	errs := modelv1.ValidateResumeGenerationResponse(response)
	if len(errs) == 0 {
		errs = validateGenerationSafety(response, req)
	}
	return generationPlan{Response: response}, errs, repaired
}

func repairGenerationResponse(response *modelv1.ResumeGenerationResponse, req GenerateRequest) bool {
	if response == nil {
		return false
	}
	changed := false
	normalizeGenerationResponse(response)
	if repairResumeModel(&response.Resume, req, &response.RequiresUserInput) {
		changed = true
	}
	if normalizeWrapperFields(response) {
		changed = true
	}
	normalizeGenerationModeResponse(response, req)
	return changed
}

func normalizeWrapperFields(response *modelv1.ResumeGenerationResponse) bool {
	changed := false
	filteredInput := make([]modelv1.RequiresUserInput, 0, len(response.RequiresUserInput))
	for _, item := range response.RequiresUserInput {
		item.Field = truncateRunes(strings.TrimSpace(item.Field), 200)
		item.Message = truncateRunes(strings.TrimSpace(item.Message), 500)
		switch item.Severity {
		case "required", "optional":
		default:
			item.Severity = "optional"
			changed = true
		}
		if item.Field == "" || item.Message == "" {
			changed = true
			continue
		}
		filteredInput = append(filteredInput, item)
	}
	if len(filteredInput) != len(response.RequiresUserInput) {
		changed = true
	}
	response.RequiresUserInput = filteredInput

	filteredAssumptions := make([]modelv1.Assumption, 0, len(response.Assumptions))
	for _, assumption := range response.Assumptions {
		assumption.Message = truncateRunes(strings.TrimSpace(assumption.Message), 500)
		if assumption.Message == "" {
			changed = true
			continue
		}
		filteredAssumptions = append(filteredAssumptions, assumption)
	}
	if len(filteredAssumptions) != len(response.Assumptions) {
		changed = true
	}
	response.Assumptions = filteredAssumptions

	filteredWarnings := make([]modelv1.ResponseWarning, 0, len(response.Warnings))
	for _, warning := range response.Warnings {
		warning.Message = truncateRunes(strings.TrimSpace(warning.Message), 500)
		if warning.Message == "" {
			changed = true
			continue
		}
		filteredWarnings = append(filteredWarnings, warning)
	}
	if len(filteredWarnings) != len(response.Warnings) {
		changed = true
	}
	response.Warnings = filteredWarnings
	return changed
}

func repairResumeModel(model *modelv1.ResumeModel, req GenerateRequest, requiresUserInput *[]modelv1.RequiresUserInput) bool {
	if model == nil {
		return false
	}
	changed := false
	if strings.TrimSpace(model.SchemaVersion) != modelv1.SchemaVersion {
		model.SchemaVersion = modelv1.SchemaVersion
		changed = true
	}
	model.Summary.Text = truncateRunes(strings.TrimSpace(model.Summary.Text), 1000)
	model.Target.RoleTitle = truncateRunes(strings.TrimSpace(model.Target.RoleTitle), 140)
	model.Target.Seniority = truncateRunes(strings.TrimSpace(model.Target.Seniority), 80)
	model.Target.Persona = truncateRunes(strings.TrimSpace(model.Target.Persona), 140)
	model.Target.Industry = truncateRunes(strings.TrimSpace(model.Target.Industry), 120)
	if strings.TrimSpace(model.Target.RoleTitle) == "" && strings.TrimSpace(req.TargetRole) != "" {
		model.Target.RoleTitle = truncateRunes(req.TargetRole, 140)
		changed = true
	}
	if strings.TrimSpace(model.Target.Seniority) == "" && strings.TrimSpace(req.Seniority) != "" {
		model.Target.Seniority = truncateRunes(req.Seniority, 80)
		changed = true
	}

	model.Basics.FullName = truncateRunes(strings.TrimSpace(model.Basics.FullName), 120)
	model.Basics.Headline = truncateRunes(strings.TrimSpace(model.Basics.Headline), 180)
	model.Basics.Email = truncateRunes(strings.TrimSpace(model.Basics.Email), 160)
	model.Basics.Phone = truncateRunes(strings.TrimSpace(model.Basics.Phone), 40)
	model.Basics.Location.City = truncateRunes(strings.TrimSpace(model.Basics.Location.City), 120)
	model.Basics.Location.State = truncateRunes(strings.TrimSpace(model.Basics.Location.State), 120)
	model.Basics.Location.Country = truncateRunes(strings.TrimSpace(model.Basics.Location.Country), 120)
	for i := range model.Basics.Links {
		model.Basics.Links[i].Label = truncateRunes(strings.TrimSpace(model.Basics.Links[i].Label), 80)
		model.Basics.Links[i].URL = truncateRunes(strings.TrimSpace(model.Basics.Links[i].URL), 500)
		if !isAllowedLinkType(model.Basics.Links[i].Type) && strings.TrimSpace(model.Basics.Links[i].Type) != "" {
			model.Basics.Links[i].Type = "other"
			changed = true
		}
	}

	for i := range model.Skills {
		model.Skills[i].Category = truncateRunes(strings.TrimSpace(model.Skills[i].Category), 80)
		for j := range model.Skills[i].Items {
			item := &model.Skills[i].Items[j]
			item.Name = truncateRunes(strings.TrimSpace(item.Name), 80)
			if !isAllowedSkillLevel(item.Level) {
				if strings.TrimSpace(item.Level) != "" {
					changed = true
				}
				item.Level = ""
			}
			if !isAllowedSource(item.Source) {
				item.Source = "ai_generated"
				changed = true
			}
			if item.Years != nil && *item.Years < 0 {
				item.Years = nil
				changed = true
			}
		}
	}

	seenIDs := map[string]bool{}
	for i := range model.Experience {
		exp := &model.Experience[i]
		exp.ID, changed = normalizeUniqueID(exp.ID, stableID("exp", i, exp.Company, exp.Title, exp.StartDate), seenIDs, changed)
		exp.Company = truncateRunes(strings.TrimSpace(exp.Company), 140)
		exp.Title = truncateRunes(strings.TrimSpace(exp.Title), 140)
		exp.Location = truncateRunes(strings.TrimSpace(exp.Location), 140)
		exp.Summary = truncateRunes(strings.TrimSpace(exp.Summary), 700)
		if normalizeDateField(&exp.StartDate, fmt.Sprintf("experience[%d].startDate", i), requiresUserInput) {
			changed = true
		}
		if normalizeDateField(&exp.EndDate, fmt.Sprintf("experience[%d].endDate", i), requiresUserInput) {
			changed = true
		}
		if !isAllowedEmploymentType(exp.EmploymentType) {
			if strings.TrimSpace(exp.EmploymentType) != "" {
				changed = true
			}
			exp.EmploymentType = ""
		}
		for j := range exp.Highlights {
			hl := &exp.Highlights[j]
			hl.ID, changed = normalizeUniqueID(hl.ID, stableID(exp.ID+"-highlight", j, hl.Text), seenIDs, changed)
			hl.Text = truncateRunes(strings.TrimSpace(hl.Text), 350)
			if !isAllowedSource(hl.Source) {
				hl.Source = "ai_generated"
				changed = true
			}
			for k := range hl.Tags {
				hl.Tags[k] = truncateRunes(strings.TrimSpace(hl.Tags[k]), 80)
			}
		}
		for j := range exp.Technologies {
			exp.Technologies[j] = truncateRunes(strings.TrimSpace(exp.Technologies[j]), 80)
		}
	}

	for i := range model.Projects {
		project := &model.Projects[i]
		project.ID, changed = normalizeUniqueID(project.ID, stableID("project", i, project.Name, project.Role), seenIDs, changed)
		project.Name = truncateRunes(strings.TrimSpace(project.Name), 140)
		project.Description = truncateRunes(strings.TrimSpace(project.Description), 700)
		project.Role = truncateRunes(strings.TrimSpace(project.Role), 120)
		for j := range project.Highlights {
			hl := &project.Highlights[j]
			hl.ID, changed = normalizeUniqueID(hl.ID, stableID(project.ID+"-highlight", j, hl.Text), seenIDs, changed)
			hl.Text = truncateRunes(strings.TrimSpace(hl.Text), 350)
			if !isAllowedSource(hl.Source) {
				hl.Source = "ai_generated"
				changed = true
			}
			for k := range hl.Tags {
				hl.Tags[k] = truncateRunes(strings.TrimSpace(hl.Tags[k]), 80)
			}
		}
		for j := range project.Technologies {
			project.Technologies[j] = truncateRunes(strings.TrimSpace(project.Technologies[j]), 80)
		}
		for j := range project.Links {
			project.Links[j].Label = truncateRunes(strings.TrimSpace(project.Links[j].Label), 80)
			project.Links[j].URL = truncateRunes(strings.TrimSpace(project.Links[j].URL), 500)
			if !isAllowedLinkType(project.Links[j].Type) && strings.TrimSpace(project.Links[j].Type) != "" {
				project.Links[j].Type = "other"
				changed = true
			}
		}
	}

	for i := range model.Education {
		edu := &model.Education[i]
		edu.ID, changed = normalizeUniqueID(edu.ID, stableID("education", i, edu.Institution, edu.Degree, edu.FieldOfStudy), seenIDs, changed)
		edu.Institution = truncateRunes(strings.TrimSpace(edu.Institution), 180)
		edu.Degree = truncateRunes(strings.TrimSpace(edu.Degree), 140)
		edu.FieldOfStudy = truncateRunes(strings.TrimSpace(edu.FieldOfStudy), 140)
		edu.Location.City = truncateRunes(strings.TrimSpace(edu.Location.City), 120)
		edu.Location.State = truncateRunes(strings.TrimSpace(edu.Location.State), 120)
		edu.Location.Country = truncateRunes(strings.TrimSpace(edu.Location.Country), 120)
		if normalizeDateField(&edu.StartDate, fmt.Sprintf("education[%d].startDate", i), requiresUserInput) {
			changed = true
		}
		if normalizeDateField(&edu.EndDate, fmt.Sprintf("education[%d].endDate", i), requiresUserInput) {
			changed = true
		}
	}

	for i := range model.Certifications {
		cert := &model.Certifications[i]
		cert.ID, changed = normalizeUniqueID(cert.ID, stableID("certification", i, cert.Name, cert.Issuer), seenIDs, changed)
		cert.Name = truncateRunes(strings.TrimSpace(cert.Name), 180)
		cert.Issuer = truncateRunes(strings.TrimSpace(cert.Issuer), 140)
		cert.CredentialURL = truncateRunes(strings.TrimSpace(cert.CredentialURL), 500)
		if normalizeDateField(&cert.IssueDate, fmt.Sprintf("certifications[%d].issueDate", i), requiresUserInput) {
			changed = true
		}
		if normalizeDateField(&cert.ExpiryDate, fmt.Sprintf("certifications[%d].expiryDate", i), requiresUserInput) {
			changed = true
		}
	}

	for i := range model.Achievements {
		ach := &model.Achievements[i]
		ach.ID, changed = normalizeUniqueID(ach.ID, stableID("achievement", i, ach.Title), seenIDs, changed)
		ach.Title = truncateRunes(strings.TrimSpace(ach.Title), 180)
		ach.Description = truncateRunes(strings.TrimSpace(ach.Description), 500)
	}

	for i := range model.CustomSections {
		section := &model.CustomSections[i]
		section.ID, changed = normalizeUniqueID(section.ID, stableID("custom-section", i, section.Title), seenIDs, changed)
		section.Title = truncateRunes(strings.TrimSpace(section.Title), 120)
		for j := range section.Items {
			item := &section.Items[j]
			item.ID, changed = normalizeUniqueID(item.ID, stableID(section.ID+"-item", j, item.Text), seenIDs, changed)
			item.Text = truncateRunes(strings.TrimSpace(item.Text), 500)
		}
	}

	filteredOrder := make([]string, 0, len(model.SectionOrder))
	seenSections := map[string]bool{}
	for _, key := range model.SectionOrder {
		if !isAllowedSectionKey(key) || seenSections[key] {
			changed = true
			continue
		}
		seenSections[key] = true
		filteredOrder = append(filteredOrder, key)
	}
	model.SectionOrder = filteredOrder
	return changed
}

func buildGenerationRetryPrompt(req GenerateRequest, raw string, errs []modelv1.ValidationError) string {
	var builder strings.Builder
	builder.WriteString(buildGenerationPrompt(req))
	builder.WriteString("\n\nThe previous response was invalid. Return corrected JSON only.\n")
	builder.WriteString("Validation errors:\n")
	for _, err := range errs {
		builder.WriteString("- ")
		builder.WriteString(err.Field)
		builder.WriteString(": ")
		builder.WriteString(err.Message)
		builder.WriteString("\n")
	}
	builder.WriteString("\nPrevious invalid JSON:\n")
	builder.WriteString(raw)
	return builder.String()
}

func buildFallbackGenerationResponse(req GenerateRequest) (generationPlan, error) {
	req = normalizeGenerateRequest(req)
	switch {
	case req.GenerationMode == GenerationModeSampleFromJobDescription:
		return buildJDTemplateFallback(req), nil
	case req.GenerationMode == GenerationModeFromExperience:
		return buildNotesFallback(req), nil
	default:
		return generationPlan{}, ErrInvalidLLMOutput
	}
}

func buildJDTemplateFallback(req GenerateRequest) generationPlan {
	skills := extractSuggestedSkills(req.JobDescription)
	response := modelv1.ResumeGenerationResponse{
		Resume: modelv1.ResumeModel{
			SchemaVersion: modelv1.SchemaVersion,
			Target: modelv1.Target{
				RoleTitle: truncateRunes(req.TargetRole, 140),
				Seniority: truncateRunes(req.Seniority, 80),
			},
			Summary: modelv1.Summary{
				Text: "Example summary: Replace this sample with a summary based on your real experience before applying.",
			},
			Skills:         skills,
			Experience:     jdTemplateExperience(),
			Projects:       []modelv1.Project{},
			Education:      []modelv1.Education{},
			Certifications: []modelv1.Certification{},
			Achievements:   []modelv1.Achievement{},
			CustomSections: []modelv1.CustomSection{},
			SectionOrder:   jdTemplateSectionOrder(skills),
		},
		RequiresUserInput: []modelv1.RequiresUserInput{
			{Field: "basics.fullName", Message: "Add your full name and contact details.", Severity: "required"},
			{Field: "summary.text", Message: "Replace the sample summary with a summary based on your real experience.", Severity: "required"},
			{Field: "experience", Message: "Add your real work history, employers, titles, and verified accomplishments.", Severity: "required"},
			{Field: "projects", Message: "Add real projects or portfolio items that support this target role.", Severity: "required"},
			{Field: "metrics", Message: "Replace sample metrics with verified results from your real work.", Severity: "required"},
			{Field: "education", Message: "Add your real education details if relevant.", Severity: "optional"},
			{Field: "certifications", Message: "Add only certifications you actually hold.", Severity: "optional"},
		},
		Warnings: []modelv1.ResponseWarning{
			{Message: jdTemplateWarning},
		},
	}
	return generationPlan{
		Response:       response,
		FallbackUsed:   true,
		FallbackReason: "template_fallback_after_invalid_output",
		DraftType:      draftTypeSampleTemplate,
	}
}

func buildNotesFallback(req GenerateRequest) generationPlan {
	summary := truncateRunes(strings.TrimSpace(req.ExperienceText), 1000)
	response := modelv1.ResumeGenerationResponse{
		Resume: modelv1.ResumeModel{
			SchemaVersion: modelv1.SchemaVersion,
			Target: modelv1.Target{
				RoleTitle: truncateRunes(req.TargetRole, 140),
				Seniority: truncateRunes(req.Seniority, 80),
			},
			Summary:        modelv1.Summary{Text: summary},
			Skills:         buildUserProvidedSkills(req.SkillsText),
			Experience:     buildNotesFallbackExperience(req.ExperienceText),
			Projects:       []modelv1.Project{},
			Education:      []modelv1.Education{},
			Certifications: []modelv1.Certification{},
			Achievements:   []modelv1.Achievement{},
			CustomSections: buildNotesFallbackSections(req),
			SectionOrder:   buildNotesFallbackSectionOrder(req),
		},
		RequiresUserInput: buildNotesFallbackRequiresUserInput(req),
		Warnings: []modelv1.ResponseWarning{
			{Message: "This draft was created from limited notes. Replace placeholders and verify all details before using it."},
		},
	}
	if req.JobDescription != "" {
		response.Warnings = append(response.Warnings, modelv1.ResponseWarning{
			Message: "The job description was used to guide structure and wording. Unsupported job requirements were not added as user claims.",
		})
	}
	return generationPlan{
		Response:       response,
		FallbackUsed:   true,
		FallbackReason: "partial_draft_fallback_after_invalid_output",
		DraftType:      draftTypeResumeDraft,
	}
}

func jdTemplateExperience() []modelv1.Experience {
	return []modelv1.Experience{{
		ID:      stableID("exp-template", 0, "template"),
		Company: "[Your Company]",
		Title:   "[Your Role]",
		Summary: "Example: Replace this section with your real experience relevant to the target role.",
		Highlights: []modelv1.Highlight{{
			ID:     stableID("exp-template-highlight", 0, "template"),
			Text:   "Example: Add a real accomplishment with verified scope, tools, and measurable impact.",
			Source: "ai_generated",
		}},
	}}
}

func jdTemplateSectionOrder(skills []modelv1.SkillCategory) []string {
	order := []string{"summary"}
	if len(skills) > 0 {
		order = append(order, "skills")
	}
	return append(order, "experience")
}

func buildNotesFallbackExperience(experienceText string) []modelv1.Experience {
	experienceText = strings.TrimSpace(experienceText)
	if experienceText == "" {
		return []modelv1.Experience{}
	}
	return []modelv1.Experience{{
		ID:         stableID("exp-notes", 0, experienceText),
		Summary:    truncateRunes(experienceText, 700),
		Highlights: []modelv1.Highlight{},
	}}
}

func buildNotesFallbackSections(req GenerateRequest) []modelv1.CustomSection {
	var sections []modelv1.CustomSection
	if trimmed := strings.TrimSpace(req.EducationText); trimmed != "" {
		sections = append(sections, modelv1.CustomSection{
			ID:    stableID("custom-section", len(sections), "education-notes"),
			Title: "Education Notes",
			Items: []modelv1.CustomSectionItem{{
				ID:   stableID("custom-section-item", 0, trimmed),
				Text: truncateRunes(trimmed, 500),
			}},
		})
	}
	return sections
}

func buildNotesFallbackSectionOrder(req GenerateRequest) []string {
	var order []string
	if strings.TrimSpace(req.ExperienceText) != "" {
		order = append(order, "summary", "experience")
	}
	if strings.TrimSpace(req.SkillsText) != "" {
		if len(order) == 0 || order[len(order)-1] != "skills" {
			order = append(order, "skills")
		}
	}
	if strings.TrimSpace(req.EducationText) != "" {
		order = append(order, "customSections")
	}
	return dedupeSectionOrder(order)
}

func buildNotesFallbackRequiresUserInput(req GenerateRequest) []modelv1.RequiresUserInput {
	items := []modelv1.RequiresUserInput{
		{Field: "basics.fullName", Message: "Add your full name and contact details.", Severity: "required"},
	}
	if strings.TrimSpace(req.ExperienceText) != "" {
		items = append(items,
			modelv1.RequiresUserInput{Field: "experience[0].company", Message: "Add the employer name for the experience note.", Severity: "required"},
			modelv1.RequiresUserInput{Field: "experience[0].title", Message: "Add the job title for the experience note.", Severity: "required"},
			modelv1.RequiresUserInput{Field: "experience[0].startDate", Message: "Add the start date for the experience note.", Severity: "optional"},
			modelv1.RequiresUserInput{Field: "experience[0].endDate", Message: "Add the end date for the experience note or mark it current.", Severity: "optional"},
		)
	}
	if strings.TrimSpace(req.EducationText) != "" {
		items = append(items, modelv1.RequiresUserInput{
			Field:    "education",
			Message:  "Convert the education notes into structured education details.",
			Severity: "optional",
		})
	}
	return items
}

func buildUserProvidedSkills(skillsText string) []modelv1.SkillCategory {
	parts := splitSuggestions(skillsText)
	if len(parts) == 0 {
		return []modelv1.SkillCategory{}
	}
	items := make([]modelv1.SkillItem, 0, len(parts))
	for _, skill := range parts {
		items = append(items, modelv1.SkillItem{Name: skill, Source: "user_provided"})
	}
	return []modelv1.SkillCategory{{
		Category: "Skills",
		Items:    items,
	}}
}

func extractSuggestedSkills(jobDescription string) []modelv1.SkillCategory {
	candidates := []string{
		"Go", "Java", "Spring Boot", "PostgreSQL", "APIs", "Kubernetes", "Salesforce", "HubSpot",
		"CRM", "LinkedIn", "ZoomInfo", "Apollo", "Lead Generation", "Market Analysis",
		"Pipeline Management", "Relationship Management", "Contract Negotiation", "Product Demonstrations",
	}
	lower := strings.ToLower(jobDescription)
	var matches []string
	for _, candidate := range candidates {
		if strings.Contains(lower, strings.ToLower(candidate)) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return []modelv1.SkillCategory{}
	}
	items := make([]modelv1.SkillItem, 0, len(matches))
	for _, match := range matches {
		items = append(items, modelv1.SkillItem{Name: match, Source: "ai_generated"})
	}
	return []modelv1.SkillCategory{{
		Category: "Suggested Skills",
		Items:    items,
	}}
}

func splitSuggestions(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = truncateRunes(strings.TrimSpace(field), 80)
		if field == "" || seen[strings.ToLower(field)] {
			continue
		}
		seen[strings.ToLower(field)] = true
		out = append(out, field)
	}
	return out
}

func dedupeSectionOrder(keys []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func inferDraftType(req GenerateRequest) string {
	if req.GenerationMode == GenerationModeSampleFromJobDescription {
		return draftTypeSampleTemplate
	}
	return draftTypeResumeDraft
}

func normalizeUniqueID(current, fallback string, seen map[string]bool, changed bool) (string, bool) {
	current = strings.TrimSpace(current)
	if current == "" || seen[current] {
		current = fallback
		changed = true
	}
	for seen[current] {
		current = current + "-dup"
		changed = true
	}
	seen[current] = true
	return current, changed
}

func normalizeDateField(value *string, field string, requiresUserInput *[]modelv1.RequiresUserInput) bool {
	if value == nil {
		return false
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		*value = ""
		return false
	}
	if validYYYYMM(trimmed) {
		*value = trimmed
		return false
	}
	*value = ""
	appendRequiresUserInput(requiresUserInput, field, "Add a valid date in YYYY-MM format.", "optional")
	return true
}

func appendRequiresUserInput(items *[]modelv1.RequiresUserInput, field, message, severity string) {
	for _, item := range *items {
		if item.Field == field && item.Message == message {
			return
		}
	}
	*items = append(*items, modelv1.RequiresUserInput{
		Field:    field,
		Message:  message,
		Severity: severity,
	})
}

func validYYYYMM(value string) bool {
	return regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`).MatchString(value)
}

func truncateRunes(value string, max int) string {
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}

func isAllowedSource(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "user_provided", "parsed_from_resume", "ai_generated", "ai_rewritten", "ai_tailored":
		return true
	default:
		return false
	}
}

func isAllowedEmploymentType(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "full_time", "part_time", "contract", "internship", "freelance", "self_employed", "other":
		return true
	default:
		return false
	}
}

func isAllowedLinkType(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "linkedin", "github", "portfolio", "website", "email", "phone", "other":
		return true
	default:
		return false
	}
}

func isAllowedSkillLevel(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "beginner", "intermediate", "advanced", "expert":
		return true
	default:
		return false
	}
}

func isAllowedSectionKey(value string) bool {
	switch strings.TrimSpace(value) {
	case "summary", "skills", "experience", "projects", "education", "certifications", "achievements", "customSections":
		return true
	default:
		return false
	}
}

func validationSummary(errs []modelv1.ValidationError) string {
	if len(errs) == 0 {
		return ""
	}
	parts := make([]string, 0, minInt(3, len(errs)))
	for i := 0; i < len(errs) && i < 3; i++ {
		parts = append(parts, fmt.Sprintf("%s: %s", errs[i].Field, errs[i].Message))
	}
	return strings.Join(parts, "; ")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func logGenerationValidation(ctx context.Context, req GenerateRequest, msg string, issueCount int, repairAttempted, retryAttempted, fallbackUsed bool, fallbackReason, validationSummary string) {
	fields := generationLogFields(ctx, req, map[string]any{
		"issue_count":       issueCount,
		"repair_attempted":  repairAttempted,
		"retry_attempted":   retryAttempted,
		"fallback_used":     fallbackUsed,
		"fallback_reason":   fallbackReason,
		"validation_reason": validationSummary,
	})
	telemetry.Info(msg, fields)
}
