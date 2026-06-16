package modelv1

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationWarning struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

var datePattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

var knownSectionKeys = map[string]bool{
	string(SectionSummary):        true,
	string(SectionSkills):         true,
	string(SectionExperience):     true,
	string(SectionProjects):       true,
	string(SectionEducation):      true,
	string(SectionCertifications): true,
	string(SectionAchievements):   true,
	string(SectionCustomSections): true,
}

var linkTypes = map[string]bool{
	"linkedin":  true,
	"github":    true,
	"portfolio": true,
	"website":   true,
	"email":     true,
	"phone":     true,
	"other":     true,
}

var skillLevels = map[string]bool{
	"beginner":     true,
	"intermediate": true,
	"advanced":     true,
	"expert":       true,
}

var sources = map[string]bool{
	"user_provided":      true,
	"parsed_from_resume": true,
	"ai_generated":       true,
	"ai_rewritten":       true,
	"ai_tailored":        true,
}

var employmentTypes = map[string]bool{
	"full_time":     true,
	"part_time":     true,
	"contract":      true,
	"internship":    true,
	"freelance":     true,
	"self_employed": true,
	"other":         true,
}

// ValidateStructure returns hard validation errors for malformed model data.
func ValidateStructure(model ResumeModel) []ValidationError {
	var errs []ValidationError

	if strings.TrimSpace(model.SchemaVersion) != SchemaVersion {
		errs = append(errs, ValidationError{
			Field:   "schemaVersion",
			Message: `schemaVersion must be "resume.v1"`,
		})
	}

	addLengthError := func(field, value string, max int) {
		if utf8.RuneCountInString(value) > max {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("must be %d characters or fewer", max),
			})
		}
	}
	addDateError := func(field, value string) {
		if strings.TrimSpace(value) != "" && !datePattern.MatchString(value) {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: "must be empty or use YYYY-MM format",
			})
		}
	}
	addEnumError := func(field, value string, allowed map[string]bool) {
		if strings.TrimSpace(value) != "" && !allowed[value] {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: "has an invalid value",
			})
		}
	}

	addLengthError("basics.fullName", model.Basics.FullName, 120)
	addLengthError("basics.headline", model.Basics.Headline, 180)
	addLengthError("basics.email", model.Basics.Email, 160)
	addLengthError("basics.phone", model.Basics.Phone, 40)
	addLengthError("basics.location.city", model.Basics.Location.City, 120)
	addLengthError("basics.location.state", model.Basics.Location.State, 120)
	addLengthError("basics.location.country", model.Basics.Location.Country, 120)
	for i, link := range model.Basics.Links {
		addEnumError(fmt.Sprintf("basics.links[%d].type", i), link.Type, linkTypes)
		addLengthError(fmt.Sprintf("basics.links[%d].label", i), link.Label, 80)
		addLengthError(fmt.Sprintf("basics.links[%d].url", i), link.URL, 500)
	}

	addLengthError("target.roleTitle", model.Target.RoleTitle, 140)
	addLengthError("target.seniority", model.Target.Seniority, 80)
	addLengthError("target.persona", model.Target.Persona, 140)
	addLengthError("target.industry", model.Target.Industry, 120)
	addLengthError("summary.text", model.Summary.Text, 1000)

	for i, category := range model.Skills {
		categoryField := fmt.Sprintf("skills[%d].category", i)
		if strings.TrimSpace(category.Category) == "" {
			errs = append(errs, ValidationError{
				Field:   categoryField,
				Message: "skill category is required",
			})
		}
		addLengthError(categoryField, category.Category, 80)
		for j, item := range category.Items {
			nameField := fmt.Sprintf("skills[%d].items[%d].name", i, j)
			if strings.TrimSpace(item.Name) == "" {
				errs = append(errs, ValidationError{
					Field:   nameField,
					Message: "skill name is required",
				})
			}
			addLengthError(nameField, item.Name, 80)
			addEnumError(fmt.Sprintf("skills[%d].items[%d].level", i, j), item.Level, skillLevels)
			addEnumError(fmt.Sprintf("skills[%d].items[%d].source", i, j), item.Source, sources)
			if item.Years != nil && *item.Years < 0 {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("skills[%d].items[%d].years", i, j),
					Message: "must not be negative",
				})
			}
		}
	}

	ids := map[string]string{}
	addIDError := func(field, id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			errs = append(errs, ValidationError{Field: field, Message: "id is required"})
			return
		}
		if previous, ok := ids[id]; ok {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("duplicate id also used at %s", previous),
			})
			return
		}
		ids[id] = field
	}

	for i, exp := range model.Experience {
		prefix := fmt.Sprintf("experience[%d]", i)
		addIDError(prefix+".id", exp.ID)
		addLengthError(prefix+".company", exp.Company, 140)
		addLengthError(prefix+".title", exp.Title, 140)
		addLengthError(prefix+".location", exp.Location, 140)
		addLengthError(prefix+".summary", exp.Summary, 700)
		addDateError(prefix+".startDate", exp.StartDate)
		addDateError(prefix+".endDate", exp.EndDate)
		addEnumError(prefix+".employmentType", exp.EmploymentType, employmentTypes)
		for j, highlight := range exp.Highlights {
			highlightPrefix := fmt.Sprintf("%s.highlights[%d]", prefix, j)
			addIDError(highlightPrefix+".id", highlight.ID)
			addLengthError(highlightPrefix+".text", highlight.Text, 350)
			addEnumError(highlightPrefix+".source", highlight.Source, sources)
			for k, tag := range highlight.Tags {
				addLengthError(fmt.Sprintf("%s.tags[%d]", highlightPrefix, k), tag, 80)
			}
		}
		for j, technology := range exp.Technologies {
			addLengthError(fmt.Sprintf("%s.technologies[%d]", prefix, j), technology, 80)
		}
	}

	for i, project := range model.Projects {
		prefix := fmt.Sprintf("projects[%d]", i)
		addIDError(prefix+".id", project.ID)
		addLengthError(prefix+".name", project.Name, 140)
		addLengthError(prefix+".description", project.Description, 700)
		addLengthError(prefix+".role", project.Role, 120)
		for j, highlight := range project.Highlights {
			highlightPrefix := fmt.Sprintf("%s.highlights[%d]", prefix, j)
			addIDError(highlightPrefix+".id", highlight.ID)
			addLengthError(highlightPrefix+".text", highlight.Text, 350)
			addEnumError(highlightPrefix+".source", highlight.Source, sources)
			for k, tag := range highlight.Tags {
				addLengthError(fmt.Sprintf("%s.tags[%d]", highlightPrefix, k), tag, 80)
			}
		}
		for j, technology := range project.Technologies {
			addLengthError(fmt.Sprintf("%s.technologies[%d]", prefix, j), technology, 80)
		}
		for j, link := range project.Links {
			addEnumError(fmt.Sprintf("%s.links[%d].type", prefix, j), link.Type, linkTypes)
			addLengthError(fmt.Sprintf("%s.links[%d].label", prefix, j), link.Label, 80)
			addLengthError(fmt.Sprintf("%s.links[%d].url", prefix, j), link.URL, 500)
		}
	}

	for i, education := range model.Education {
		prefix := fmt.Sprintf("education[%d]", i)
		addIDError(prefix+".id", education.ID)
		addLengthError(prefix+".institution", education.Institution, 180)
		addLengthError(prefix+".degree", education.Degree, 140)
		addLengthError(prefix+".fieldOfStudy", education.FieldOfStudy, 140)
		addDateError(prefix+".startDate", education.StartDate)
		addDateError(prefix+".endDate", education.EndDate)
		addLengthError(prefix+".location.city", education.Location.City, 120)
		addLengthError(prefix+".location.state", education.Location.State, 120)
		addLengthError(prefix+".location.country", education.Location.Country, 120)
	}

	for i, certification := range model.Certifications {
		prefix := fmt.Sprintf("certifications[%d]", i)
		addIDError(prefix+".id", certification.ID)
		addLengthError(prefix+".name", certification.Name, 180)
		addLengthError(prefix+".issuer", certification.Issuer, 140)
		addDateError(prefix+".issueDate", certification.IssueDate)
		addDateError(prefix+".expiryDate", certification.ExpiryDate)
		addLengthError(prefix+".credentialUrl", certification.CredentialURL, 500)
	}

	for i, achievement := range model.Achievements {
		prefix := fmt.Sprintf("achievements[%d]", i)
		addIDError(prefix+".id", achievement.ID)
		addLengthError(prefix+".title", achievement.Title, 180)
		addLengthError(prefix+".description", achievement.Description, 500)
	}

	for i, section := range model.CustomSections {
		prefix := fmt.Sprintf("customSections[%d]", i)
		addIDError(prefix+".id", section.ID)
		addLengthError(prefix+".title", section.Title, 120)
		for j, item := range section.Items {
			itemPrefix := fmt.Sprintf("%s.items[%d]", prefix, j)
			addIDError(itemPrefix+".id", item.ID)
			addLengthError(itemPrefix+".text", item.Text, 500)
		}
	}

	for i, key := range model.SectionOrder {
		if !knownSectionKeys[key] {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("sectionOrder[%d]", i),
				Message: "unknown section key",
			})
		}
	}

	return errs
}

// ValidateReadiness returns soft warnings for resumes that may be incomplete or weak.
func ValidateReadiness(model ResumeModel) []ValidationWarning {
	var warnings []ValidationWarning

	addWarning := func(field, message string) {
		warnings = append(warnings, ValidationWarning{Field: field, Message: message})
	}

	if strings.TrimSpace(model.Basics.FullName) == "" {
		addWarning("basics.fullName", "missing full name")
	}
	if strings.TrimSpace(model.Basics.Email) == "" && strings.TrimSpace(model.Basics.Phone) == "" {
		addWarning("basics.email", "missing email or phone")
	}
	if strings.TrimSpace(model.Summary.Text) == "" {
		addWarning("summary.text", "missing summary")
	} else if utf8.RuneCountInString(model.Summary.Text) > 700 {
		addWarning("summary.text", "summary may be too long for a resume")
	}
	if len(model.Skills) == 0 {
		addWarning("skills", "no skills listed")
	}
	if len(model.Experience) == 0 && len(model.Projects) == 0 {
		addWarning("experience", "no experience or projects listed")
	}

	for i, exp := range model.Experience {
		prefix := fmt.Sprintf("experience[%d]", i)
		if strings.TrimSpace(exp.Company) == "" {
			addWarning(prefix+".company", "experience is missing company")
		}
		if strings.TrimSpace(exp.Title) == "" {
			addWarning(prefix+".title", "experience is missing title")
		}
		if exp.IsCurrent && strings.TrimSpace(exp.StartDate) == "" {
			addWarning(prefix+".startDate", "current job has empty start date")
		}
		if !exp.IsCurrent && strings.TrimSpace(exp.EndDate) == "" {
			addWarning(prefix+".endDate", "non-current job has empty end date")
		}
		if len(exp.Highlights) > 8 {
			addWarning(prefix+".highlights", "too many bullets in one experience")
		}
		for j, highlight := range exp.Highlights {
			if strings.TrimSpace(highlight.Text) != "" && !hasMeasurableImpact(highlight.Text) {
				addWarning(fmt.Sprintf("%s.highlights[%d].text", prefix, j), "bullet lacks measurable impact")
			}
		}
	}

	return warnings
}

func hasMeasurableImpact(text string) bool {
	lower := strings.ToLower(text)
	if strings.ContainsAny(lower, "0123456789") {
		return true
	}
	impactTerms := []string{
		"%", "$", "increased", "decreased", "reduced", "improved", "grew",
		"saved", "accelerated", "optimized", "revenue", "cost", "users",
	}
	for _, term := range impactTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}
