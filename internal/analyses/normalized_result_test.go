package analyses

import (
	"encoding/json"
	"testing"
)

func TestNormalizeMissingFieldsSchemaMismatch(t *testing.T) {
	raw := []byte(`{
  "ats": {"score": 80, "missingKeywords": [], "formattingIssues": []},
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins": [], "mediumEffort": [], "deepFixes": []}
}`)
	analysis := Analysis{PromptVersion: "v1", Model: "test-model"}
	_, err := normalizeAnalysisResult(raw, analysis)
	if err == nil {
		t.Fatalf("expected error for missing summary field")
	}
}

func TestNormalizeClampsScore(t *testing.T) {
	raw := []byte(`{
  "summary": {"overallAssessment": "ok", "strengths": [], "weaknesses": []},
  "ats": {"score": 150, "missingKeywords": [], "formattingIssues": []},
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins": [], "mediumEffort": [], "deepFixes": []}
}`)
	analysis := Analysis{PromptVersion: "v1", Model: "test-model"}
	result, err := normalizeAnalysisResult(raw, analysis)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ats, ok := result["ats"].(map[string]any)
	if !ok {
		t.Fatalf("expected ats in normalized result")
	}
	score, ok := ats["score"].(float64)
	if !ok {
		t.Fatalf("expected score to be a number")
	}
	if score != 100 {
		payload, _ := json.Marshal(result)
		t.Fatalf("expected score to clamp to 100, got %v (%s)", score, payload)
	}
}
