package contract

import (
	"strings"
)

import "resume-backend/resume/model"

const (
	PlaceholderPrefix      = "TO-FILL:"
	PlaceholderEmail       = "TO-FILL: Email"
	PlaceholderPhone       = "TO-FILL: Phone"
	PlaceholderLinkedIn    = "TO-FILL: LinkedIn"
	PlaceholderSkills      = "TO-FILL: Skills"
	PlaceholderUniversity  = "TO-FILL: University"
	PlaceholderDegree      = "TO-FILL: Degree"
	PlaceholderField       = "TO-FILL: Field"
	PlaceholderEduLocation = "TO-FILL: Location"
	PlaceholderEduStart    = "TO-FILL: Start"
	PlaceholderEduEnd      = "TO-FILL: End"
)

type MissingFieldsError struct {
	Fields []string
}

func (e MissingFieldsError) Error() string {
	return "missing required fields: " + strings.Join(e.Fields, ", ")
}

// Enforce ensures required sections and header fields are present.
// When strict is true, missing fields are reported without applying placeholders.
func Enforce(resume *model.ResumeModel, strict bool) error {
	missing := collectMissing(resume)
	if strict && len(missing) > 0 {
		return MissingFieldsError{Fields: missing}
	}

	applyPlaceholders(resume, missing)
	return nil
}

func collectMissing(resume *model.ResumeModel) []string {
	missing := make([]string, 0, 4)
	if !hasValue(resume.Header.Email) {
		missing = append(missing, "header.email")
	}
	if !hasValue(resume.Header.Phone) {
		missing = append(missing, "header.phone")
	}
	if !hasLinkedIn(resume.Header.Links) {
		missing = append(missing, "header.linkedin")
	}
	if !hasSkills(resume.Skills) {
		missing = append(missing, "skills")
	}
	if !hasEducation(resume.Education) {
		missing = append(missing, "education")
	}
	return missing
}

func applyPlaceholders(resume *model.ResumeModel, missing []string) {
	missingSet := make(map[string]struct{}, len(missing))
	for _, field := range missing {
		missingSet[field] = struct{}{}
	}

	if _, ok := missingSet["header.email"]; ok {
		resume.Header.Email = PlaceholderEmail
	}
	if _, ok := missingSet["header.phone"]; ok {
		resume.Header.Phone = PlaceholderPhone
	}
	if _, ok := missingSet["header.linkedin"]; ok {
		resume.Header.Links = append(resume.Header.Links, PlaceholderLinkedIn)
	}
	if _, ok := missingSet["skills"]; ok {
		resume.Skills = model.ResumeSkills{Tools: []string{PlaceholderSkills}}
	}
	if _, ok := missingSet["education"]; ok {
		resume.Education = []model.ResumeEducation{
			{
				Institution: PlaceholderUniversity,
				Degree:      PlaceholderDegree,
				Field:       PlaceholderField,
				Location:    PlaceholderEduLocation,
				Start:       PlaceholderEduStart,
				End:         PlaceholderEduEnd,
			},
		}
	}
}

func hasValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	return !strings.HasPrefix(strings.ToUpper(trimmed), PlaceholderPrefix)
}

func hasLinkedIn(links []string) bool {
	for _, link := range links {
		trimmed := strings.TrimSpace(link)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(trimmed), PlaceholderPrefix) {
			continue
		}
		if strings.Contains(strings.ToLower(trimmed), "linkedin.com") {
			return true
		}
	}
	return false
}

func hasSkills(skills model.ResumeSkills) bool {
	for _, value := range flattenSkills(skills) {
		if hasValue(value) {
			return true
		}
	}
	return false
}

func hasEducation(items []model.ResumeEducation) bool {
	for _, edu := range items {
		if hasValue(edu.Institution) || hasValue(edu.Degree) || hasValue(edu.Field) || hasValue(edu.Location) || hasValue(edu.Start) || hasValue(edu.End) {
			return true
		}
	}
	return false
}

func flattenSkills(skills model.ResumeSkills) []string {
	out := make([]string, 0, len(skills.Languages)+len(skills.Frameworks)+len(skills.Databases)+len(skills.CloudDevOps)+len(skills.Observability)+len(skills.Tools))
	out = append(out, skills.Languages...)
	out = append(out, skills.Frameworks...)
	out = append(out, skills.Databases...)
	out = append(out, skills.CloudDevOps...)
	out = append(out, skills.Observability...)
	out = append(out, skills.Tools...)
	return out
}
