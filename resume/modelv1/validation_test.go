package modelv1

import "testing"

func TestValidateStructureValidMinimalIncompleteDraft(t *testing.T) {
	model := ResumeModel{SchemaVersion: SchemaVersion}

	if errs := ValidateStructure(model); len(errs) != 0 {
		t.Fatalf("expected no structural errors, got %#v", errs)
	}
}

func TestValidateStructureValidCompleteResume(t *testing.T) {
	model := completeResume()

	if errs := ValidateStructure(model); len(errs) != 0 {
		t.Fatalf("expected no structural errors, got %#v", errs)
	}
}

func TestValidateStructureInvalidSchemaVersion(t *testing.T) {
	model := ResumeModel{SchemaVersion: "resume.v2"}

	assertStructuralError(t, ValidateStructure(model), "schemaVersion")
}

func TestValidateStructureMissingSchemaVersion(t *testing.T) {
	model := ResumeModel{}

	assertStructuralError(t, ValidateStructure(model), "schemaVersion")
}

func TestValidateStructureInvalidDates(t *testing.T) {
	model := completeResume()
	model.Experience[0].StartDate = "Jan 2024"
	model.Experience[0].EndDate = "2024-01-01"
	model.Education[0].StartDate = "2024"
	model.Certifications[0].IssueDate = "2024-1"

	errs := ValidateStructure(model)
	assertStructuralError(t, errs, "experience[0].startDate")
	assertStructuralError(t, errs, "experience[0].endDate")
	assertStructuralError(t, errs, "education[0].startDate")
	assertStructuralError(t, errs, "certifications[0].issueDate")
}

func TestValidateStructureMissingIDs(t *testing.T) {
	model := completeResume()
	model.Experience[0].ID = ""
	model.Experience[0].Highlights[0].ID = ""
	model.Projects[0].ID = ""
	model.Projects[0].Highlights[0].ID = ""
	model.Education[0].ID = ""
	model.Certifications[0].ID = ""
	model.Achievements[0].ID = ""
	model.CustomSections[0].ID = ""
	model.CustomSections[0].Items[0].ID = ""

	errs := ValidateStructure(model)
	assertStructuralError(t, errs, "experience[0].id")
	assertStructuralError(t, errs, "experience[0].highlights[0].id")
	assertStructuralError(t, errs, "projects[0].id")
	assertStructuralError(t, errs, "projects[0].highlights[0].id")
	assertStructuralError(t, errs, "education[0].id")
	assertStructuralError(t, errs, "certifications[0].id")
	assertStructuralError(t, errs, "achievements[0].id")
	assertStructuralError(t, errs, "customSections[0].id")
	assertStructuralError(t, errs, "customSections[0].items[0].id")
}

func TestValidateStructureDuplicateIDs(t *testing.T) {
	model := completeResume()
	model.Projects[0].ID = model.Experience[0].ID

	assertStructuralError(t, ValidateStructure(model), "projects[0].id")
}

func TestValidateStructureInvalidSectionOrderKey(t *testing.T) {
	model := completeResume()
	model.SectionOrder = []string{"summary", "references"}

	assertStructuralError(t, ValidateStructure(model), "sectionOrder[1]")
}

func TestValidateStructureDuplicateSectionOrderKey(t *testing.T) {
	model := completeResume()
	model.SectionOrder = []string{"summary", "skills", "summary"}

	assertStructuralError(t, ValidateStructure(model), "sectionOrder[2]")
}

func TestValidateStructureEmptySkillCategoryAndName(t *testing.T) {
	model := completeResume()
	model.Skills[0].Category = " "
	model.Skills[0].Items[0].Name = ""

	errs := ValidateStructure(model)
	assertStructuralError(t, errs, "skills[0].category")
	assertStructuralError(t, errs, "skills[0].items[0].name")
}

func TestValidateStructureMaxLengths(t *testing.T) {
	model := completeResume()
	model.Target.RoleTitle = repeated("x", 141)
	model.Experience[0].Highlights[0].Text = repeated("x", 351)
	model.Projects[0].Highlights[0].Text = repeated("x", 351)
	model.Certifications[0].Issuer = repeated("x", 141)
	model.CustomSections[0].Items[0].Text = repeated("x", 501)

	errs := ValidateStructure(model)
	assertStructuralError(t, errs, "target.roleTitle")
	assertStructuralError(t, errs, "experience[0].highlights[0].text")
	assertStructuralError(t, errs, "projects[0].highlights[0].text")
	assertStructuralError(t, errs, "certifications[0].issuer")
	assertStructuralError(t, errs, "customSections[0].items[0].text")
}

func TestValidateStructureNegativeSkillYears(t *testing.T) {
	years := -1
	model := completeResume()
	model.Skills[0].Items[0].Years = &years

	assertStructuralError(t, ValidateStructure(model), "skills[0].items[0].years")
}

func TestValidateStructureInvalidSourceEnum(t *testing.T) {
	model := completeResume()
	model.Skills[0].Items[0].Source = "crawler"
	model.Experience[0].Highlights[0].Source = "manual"

	errs := ValidateStructure(model)
	assertStructuralError(t, errs, "skills[0].items[0].source")
	assertStructuralError(t, errs, "experience[0].highlights[0].source")
}

func TestValidateStructureInvalidEmploymentAndLinkEnums(t *testing.T) {
	model := completeResume()
	model.Experience[0].EmploymentType = "permanent"
	model.Basics.Links[0].Type = "social"
	model.Projects[0].Links[0].Type = "demo"

	errs := ValidateStructure(model)
	assertStructuralError(t, errs, "experience[0].employmentType")
	assertStructuralError(t, errs, "basics.links[0].type")
	assertStructuralError(t, errs, "projects[0].links[0].type")
}

func TestValidateStructureInvalidSkillLevelEnum(t *testing.T) {
	model := completeResume()
	model.Skills[0].Items[0].Level = "master"

	assertStructuralError(t, ValidateStructure(model), "skills[0].items[0].level")
}

func TestValidateReadinessWarningsForIncompleteResume(t *testing.T) {
	model := ResumeModel{
		SchemaVersion: SchemaVersion,
		Experience: []Experience{{
			ID:        "exp-1",
			IsCurrent: true,
			Highlights: []Highlight{{
				ID:   "highlight-1",
				Text: "Owned sales outreach and partner coordination",
			}},
		}},
	}

	warnings := ValidateReadiness(model)
	assertReadinessWarning(t, warnings, "basics.fullName")
	assertReadinessWarning(t, warnings, "basics.email")
	assertReadinessWarning(t, warnings, "summary.text")
	assertReadinessWarning(t, warnings, "skills")
	assertReadinessWarning(t, warnings, "experience[0].company")
	assertReadinessWarning(t, warnings, "experience[0].title")
	assertReadinessWarning(t, warnings, "experience[0].startDate")
	assertReadinessWarning(t, warnings, "experience[0].highlights[0].text")
}

func TestValidateReadinessWarnsForCurrentJobWithEndDate(t *testing.T) {
	model := ResumeModel{
		SchemaVersion: SchemaVersion,
		Summary:       Summary{Text: "Summary with 20% measurable impact."},
		Skills: []SkillCategory{{
			Category: "Backend",
			Items:    []SkillItem{{Name: "Go"}},
		}},
		Experience: []Experience{{
			ID:        "exp-1",
			Company:   "Acme",
			Title:     "Engineer",
			StartDate: "2023-01",
			EndDate:   "2024-01",
			IsCurrent: true,
		}},
	}

	assertReadinessWarning(t, ValidateReadiness(model), "experience[0].endDate")
}

func TestReadinessWarningsDoNotAppearAsStructuralErrors(t *testing.T) {
	model := ResumeModel{SchemaVersion: SchemaVersion}

	if warnings := ValidateReadiness(model); len(warnings) == 0 {
		t.Fatal("expected readiness warnings")
	}
	if errs := ValidateStructure(model); len(errs) != 0 {
		t.Fatalf("expected no structural errors, got %#v", errs)
	}
}

func completeResume() ResumeModel {
	years := 5
	return ResumeModel{
		SchemaVersion: SchemaVersion,
		Basics: Basics{
			FullName: "Alex Rivera",
			Headline: "Business Development Manager",
			Email:    "alex@example.com",
			Phone:    "+1 555 0100",
			Location: Location{City: "Austin", State: "TX", Country: "USA"},
			Links: []Link{{
				Type:  "linkedin",
				Label: "LinkedIn",
				URL:   "https://linkedin.com/in/alex",
			}},
		},
		Target: Target{
			RoleTitle: "Business Development Manager",
			Seniority: "Manager",
			Persona:   "Revenue growth",
			Industry:  "SaaS",
		},
		Summary: Summary{Text: "Business development leader with a record of building pipeline and closing enterprise partnerships."},
		Skills: []SkillCategory{{
			Category: "Sales",
			Items: []SkillItem{{
				Name:   "Pipeline generation",
				Level:  "advanced",
				Years:  &years,
				Source: "user_provided",
			}},
		}},
		Experience: []Experience{{
			ID:             "exp-1",
			Company:        "Acme",
			Title:          "Business Development Manager",
			Location:       "Austin, TX",
			EmploymentType: "full_time",
			StartDate:      "2021-01",
			EndDate:        "2024-12",
			Summary:        "Owned new business pipeline and strategic partner development.",
			Highlights: []Highlight{{
				ID:     "exp-1-highlight-1",
				Text:   "Increased qualified pipeline by 35% through account-based outbound campaigns.",
				Tags:   []string{"pipeline"},
				Source: "user_provided",
			}},
			Technologies: []string{"Salesforce", "LinkedIn Sales Navigator"},
		}},
		Projects: []Project{{
			ID:          "project-1",
			Name:        "Partner expansion",
			Description: "Expanded partner channel for a new market segment.",
			Role:        "Lead",
			Highlights: []Highlight{{
				ID:     "project-1-highlight-1",
				Text:   "Signed 4 strategic partners in 2 quarters.",
				Source: "ai_tailored",
			}},
			Technologies: []string{"HubSpot"},
			Links: []Link{{
				Type:  "website",
				Label: "Case study",
				URL:   "https://example.com/case-study",
			}},
		}},
		Education: []Education{{
			ID:           "education-1",
			Institution:  "State University",
			Degree:       "Bachelor's",
			FieldOfStudy: "Marketing",
			StartDate:    "2014-08",
			EndDate:      "2018-05",
			Location:     Location{City: "Austin", State: "TX", Country: "USA"},
		}},
		Certifications: []Certification{{
			ID:            "certification-1",
			Name:          "Salesforce Administrator",
			Issuer:        "Salesforce",
			IssueDate:     "2020-06",
			ExpiryDate:    "2026-06",
			CredentialURL: "https://example.com/credential",
		}},
		Achievements: []Achievement{{
			ID:          "achievement-1",
			Title:       "President's Club",
			Description: "Recognized for exceeding annual quota.",
		}},
		CustomSections: []CustomSection{{
			ID:    "custom-1",
			Title: "Languages",
			Items: []CustomSectionItem{{
				ID:   "custom-1-item-1",
				Text: "English, Spanish",
			}},
		}},
		SectionOrder: []string{"summary", "skills", "experience", "projects", "education", "certifications", "achievements", "customSections"},
	}
}

func assertStructuralError(t *testing.T, errs []ValidationError, field string) {
	t.Helper()
	for _, err := range errs {
		if err.Field == field {
			return
		}
	}
	t.Fatalf("expected structural error for %s, got %#v", field, errs)
}

func assertReadinessWarning(t *testing.T, warnings []ValidationWarning, field string) {
	t.Helper()
	for _, warning := range warnings {
		if warning.Field == field {
			return
		}
	}
	t.Fatalf("expected readiness warning for %s, got %#v", field, warnings)
}

func repeated(value string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += value
	}
	return result
}
