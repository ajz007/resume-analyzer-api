package modelv1

import "testing"

func TestValidateResumeGenerationResponseValid(t *testing.T) {
	response := ResumeGenerationResponse{
		Resume: completeResume(),
		RequiresUserInput: []RequiresUserInput{{
			Field:    "basics.links.github.url",
			Message:  "GitHub URL was not provided.",
			Severity: "optional",
		}},
		Assumptions: []Assumption{{
			Message: "Used the most recent role as the target role.",
		}},
		Warnings: []ResponseWarning{{
			Message: "Some dates were unavailable in the source resume.",
		}},
	}

	if errs := ValidateResumeGenerationResponse(response); len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %#v", errs)
	}
}

func TestValidateResumeGenerationResponseInvalidEmbeddedResume(t *testing.T) {
	response := ResumeGenerationResponse{
		Resume: ResumeModel{SchemaVersion: "resume.v2"},
	}

	assertStructuralError(t, ValidateResumeGenerationResponse(response), "resume.schemaVersion")
}

func TestValidateResumeGenerationResponseInvalidRequiresUserInputSeverity(t *testing.T) {
	response := ResumeGenerationResponse{
		Resume: completeResume(),
		RequiresUserInput: []RequiresUserInput{{
			Field:    "basics.links.github.url",
			Message:  "GitHub URL was not provided.",
			Severity: "nice_to_have",
		}},
	}

	assertStructuralError(t, ValidateResumeGenerationResponse(response), "requiresUserInput[0].severity")
}

func TestValidateResumeTailoringResponseValid(t *testing.T) {
	response := ResumeTailoringResponse{
		TailoredResume: completeResume(),
		Changes: []TailoringChange{{
			Section:    "experience",
			ItemID:     "exp-1-highlight-1",
			ChangeType: "rewrite",
			Before:     "Increased qualified pipeline by 35% through account-based outbound campaigns.",
			After:      "Increased qualified enterprise pipeline by 35% with targeted account-based outbound campaigns.",
			Reason:     "Aligned the bullet with enterprise sales language from the job description.",
			Risk:       "safe",
		}},
		MissingRequirements: []MissingRequirement{{
			Requirement:    "Healthcare sales experience",
			Recommendation: "Ask the user whether they have healthcare sales experience before adding it.",
		}},
		Warnings: []ResponseWarning{{
			Message: "One job requirement was not supported by the source resume.",
		}},
	}

	if errs := ValidateResumeTailoringResponse(response); len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %#v", errs)
	}
}

func TestValidateResumeTailoringResponseInvalidChangeType(t *testing.T) {
	response := validTailoringResponse()
	response.Changes[0].ChangeType = "transform"

	assertStructuralError(t, ValidateResumeTailoringResponse(response), "changes[0].changeType")
}

func TestValidateResumeTailoringResponseInvalidRisk(t *testing.T) {
	response := validTailoringResponse()
	response.Changes[0].Risk = "risky"

	assertStructuralError(t, ValidateResumeTailoringResponse(response), "changes[0].risk")
}

func TestValidateResumeTailoringResponseInvalidSection(t *testing.T) {
	response := validTailoringResponse()
	response.Changes[0].Section = "references"

	assertStructuralError(t, ValidateResumeTailoringResponse(response), "changes[0].section")
}

func TestValidateResumeTailoringResponseInvalidEmbeddedResume(t *testing.T) {
	response := validTailoringResponse()
	response.TailoredResume.SchemaVersion = ""

	assertStructuralError(t, ValidateResumeTailoringResponse(response), "tailoredResume.schemaVersion")
}

func TestValidateResumeTailoringResponseChangeTypeContentRules(t *testing.T) {
	response := ResumeTailoringResponse{
		TailoredResume: completeResume(),
		Changes: []TailoringChange{
			{
				Section:    "skills",
				ItemID:     "skills",
				ChangeType: "add",
				After:      "Added Go to Backend skills.",
				Reason:     "Skill was present in source material.",
				Risk:       "safe",
			},
			{
				Section:    "summary",
				ItemID:     "summary",
				ChangeType: "remove",
				Before:     "Removed unsupported claim.",
				Reason:     "Claim was not supported by source material.",
				Risk:       "needs_user_confirmation",
			},
			{
				Section:    "experience",
				ItemID:     "exp-1",
				ChangeType: "reorder",
				Reason:     "Moved the most relevant role first.",
				Risk:       "safe",
			},
			{
				Section:    "education",
				ItemID:     "education-1",
				ChangeType: "no_change",
				Reason:     "Education already matches the target role.",
				Risk:       "safe",
			},
		},
	}

	if errs := ValidateResumeTailoringResponse(response); len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %#v", errs)
	}
}

func TestValidateResumeTailoringResponseAddRequiresAfter(t *testing.T) {
	response := validTailoringResponse()
	response.Changes[0].ChangeType = "add"
	response.Changes[0].Before = ""
	response.Changes[0].After = ""

	assertStructuralError(t, ValidateResumeTailoringResponse(response), "changes[0].after")
}

func validTailoringResponse() ResumeTailoringResponse {
	return ResumeTailoringResponse{
		TailoredResume: completeResume(),
		Changes: []TailoringChange{{
			Section:    "summary",
			ItemID:     "summary",
			ChangeType: "rewrite",
			Before:     "Business development leader with a record of building pipeline and closing enterprise partnerships.",
			After:      "Business development leader focused on SaaS pipeline generation and enterprise partnerships.",
			Reason:     "Made the summary more specific to the target role.",
			Risk:       "safe",
		}},
	}
}
