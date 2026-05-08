package analyses

import "testing"

func TestAnalysisResultV2_4AllowsGithubRequiresUserInput(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.Issues[0].RequiresUserInput = []string{"github"}

	SanitizeV2_4(&out)
	if err := out.Validate(); err != nil {
		t.Fatalf("expected github requiresUserInput to validate, got %v", err)
	}
	if got := out.Issues[0].RequiresUserInput; len(got) != 1 || got[0] != "github" {
		t.Fatalf("expected github requiresUserInput to be preserved, got %#v", got)
	}
}

func TestAnalysisResultV2_4NormalizesGithubURLRequiresUserInput(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.Issues[0].RequiresUserInput = []string{" github_url ", "github_profile"}

	SanitizeV2_4(&out)
	if err := out.Validate(); err != nil {
		t.Fatalf("expected normalized github requiresUserInput to validate, got %v", err)
	}
	if got := out.Issues[0].RequiresUserInput; len(got) != 1 || got[0] != "github" {
		t.Fatalf("expected github_url/github_profile to normalize and dedupe to github, got %#v", got)
	}
}

func TestAnalysisResultV2_4NormalizesGraduationYearRequiresUserInput(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.Issues[0].RequiresUserInput = []string{"graduation_year"}

	SanitizeV2_4(&out)
	if err := out.Validate(); err != nil {
		t.Fatalf("expected normalized education_dates requiresUserInput to validate, got %v", err)
	}
	if got := out.Issues[0].RequiresUserInput; len(got) != 1 || got[0] != "education_dates" {
		t.Fatalf("expected graduation_year to normalize to education_dates, got %#v", got)
	}
}

func TestAnalysisResultV2_4DropsUnknownRequiresUserInputKeys(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.Issues[0].RequiresUserInput = []string{"github", "favorite_color", "repositories", "github"}

	SanitizeV2_4(&out)
	if err := out.Validate(); err != nil {
		t.Fatalf("expected unknown requiresUserInput keys to be dropped before validation, got %v", err)
	}
	want := []string{"github", "project_links"}
	if got := out.Issues[0].RequiresUserInput; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected sanitized requiresUserInput %#v, got %#v", want, got)
	}
}

func TestAnalysisResultV2_4AutoFixableClearsRequiresUserInput(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.Issues[0].AutoFixable = true
	out.Issues[0].RequiresUserInput = []string{"github", "metrics"}

	SanitizeV2_4(&out)
	if err := out.Validate(); err != nil {
		t.Fatalf("expected auto-fixable issue to validate after clearing requiresUserInput, got %v", err)
	}
	if got := out.Issues[0].RequiresUserInput; len(got) != 0 {
		t.Fatalf("expected auto-fixable requiresUserInput to be empty, got %#v", got)
	}
}

func TestAnalysisResultV2_4ValidateStillRejectsUnsanitizedInvalidRequiresUserInput(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.Issues[0].RequiresUserInput = []string{"favorite_color"}

	if err := out.Validate(); err == nil {
		t.Fatalf("expected strict validation to reject unsanitized invalid requiresUserInput")
	}
}
