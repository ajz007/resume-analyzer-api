package skills

import (
	"reflect"
	"testing"

	"resume-backend/resume/model"
)

func TestBuildSkillListDedupesAndPreservesCase(t *testing.T) {
	resumeSkills := model.ResumeSkills{
		Languages: []string{"CRM"},
	}
	missing := []string{"crm", "Crm", "hubspot"}

	got := BuildSkillList(resumeSkills, missing, 12, 8)
	want := []string{"CRM", "Hubspot"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildSkillListHonorsMaxLimit(t *testing.T) {
	resumeSkills := model.ResumeSkills{
		Languages: []string{"Go", "Python", "Java", "Ruby", "C#", "C++"},
		Tools:     []string{"Docker", "Kubernetes", "AWS", "GCP", "Azure"},
	}
	missing := []string{"Terraform", "SQL", "Redis", "PostgreSQL"}

	got := BuildSkillList(resumeSkills, missing, 12, 8)
	if len(got) != 12 {
		t.Fatalf("expected 12 skills, got %d", len(got))
	}
}

func TestBuildSkillListOrdering(t *testing.T) {
	resumeSkills := model.ResumeSkills{
		Languages: []string{"Go", "Python"},
		Tools:     []string{"Docker"},
	}
	missing := []string{"Kubernetes", "Terraform"}

	got := BuildSkillList(resumeSkills, missing, 12, 8)
	want := []string{"Go", "Python", "Docker", "Kubernetes", "Terraform"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildSkillLinesSplitsIntoTwoLines(t *testing.T) {
	resumeSkills := model.ResumeSkills{
		Languages: []string{"Go", "Python", "Java", "Ruby"},
	}

	got := BuildSkillLines(resumeSkills, nil, 12, 8, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if got[0] == "" || got[1] == "" {
		t.Fatalf("expected non-empty lines, got %v", got)
	}
}
