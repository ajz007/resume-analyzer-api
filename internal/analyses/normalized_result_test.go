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

func TestNormalizeFinalAndMatchScoreFromTopLevel(t *testing.T) {
	raw := []byte(`{
  "matchScore": 88,
  "meta": {
    "promptVersion": "v2_3",
    "model": "test-model",
    "jobDescriptionProvided": true,
    "confidence": 0.5,
    "assumptions": [],
    "limitations": []
  },
  "summary": {"overallAssessment": "ok", "strengths": [], "weaknesses": []},
  "ats": {
    "score": 74,
    "scoreBreakdown": {"skills": 20, "experience": 20, "impact": 20, "formatting": 20, "roleFit": 20},
    "scoreReasoning": ["a", "b", "c"],
    "scoreExplanation": {
      "components": [
        {"key": "atsReadability", "label": "ATS Readability", "score": 75, "weight": 25, "explanation": "x", "helped": ["a"], "dragged": ["b"]},
        {"key": "skillMatch", "label": "Skill Match", "score": 70, "weight": 30, "explanation": "x", "helped": ["a"], "dragged": ["b"]},
        {"key": "experienceRelevance", "label": "Experience Relevance", "score": 80, "weight": 30, "explanation": "x", "helped": ["a"], "dragged": ["b"]},
        {"key": "resumeStructure", "label": "Resume Structure", "score": 78, "weight": 15, "explanation": "x", "helped": ["a"], "dragged": ["b"]}
      ]
    },
    "missingKeywords": {"fromJobDescription": ["a", "b", "c"], "industryCommon": []},
    "formattingIssues": []
  },
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins": [], "mediumEffort": [], "deepFixes": []}
}`)
	analysis := Analysis{PromptVersion: "v2_3", Model: "test-model", Mode: ModeJobMatch}
	result, err := normalizeAnalysisResult(raw, analysis)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := result["finalScore"].(float64); !ok || got != 88 {
		t.Fatalf("expected finalScore 88, got %v", result["finalScore"])
	}
	if got, ok := result["matchScore"].(float64); !ok || got != 88 {
		t.Fatalf("expected matchScore 88, got %v", result["matchScore"])
	}
	meta, ok := result["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta in normalized result")
	}
	if meta["mode"] != "JOB_MATCH" {
		t.Fatalf("expected meta.mode JOB_MATCH, got %v", meta["mode"])
	}
	if meta["primaryScoreType"] != "JOB_MATCH" {
		t.Fatalf("expected meta.primaryScoreType JOB_MATCH, got %v", meta["primaryScoreType"])
	}
}

func TestNormalizeMatchScoreFromMissingKeywords(t *testing.T) {
	raw := []byte(`{
  "meta": {
    "promptVersion": "v2_3",
    "model": "test-model",
    "jobDescriptionProvided": true,
    "confidence": 0.5,
    "assumptions": [],
    "limitations": []
  },
  "summary": {"overallAssessment": "ok", "strengths": [], "weaknesses": []},
  "ats": {
    "score": 70,
    "scoreBreakdown": {"skills": 20, "experience": 20, "impact": 20, "formatting": 20, "roleFit": 20},
    "scoreReasoning": ["a", "b", "c"],
    "scoreExplanation": {
      "components": [
        {"key": "atsReadability", "label": "ATS Readability", "score": 75, "weight": 25, "explanation": "x", "helped": ["a"], "dragged": ["b"]},
        {"key": "skillMatch", "label": "Skill Match", "score": 70, "weight": 30, "explanation": "x", "helped": ["a"], "dragged": ["b"]},
        {"key": "experienceRelevance", "label": "Experience Relevance", "score": 80, "weight": 30, "explanation": "x", "helped": ["a"], "dragged": ["b"]},
        {"key": "resumeStructure", "label": "Resume Structure", "score": 78, "weight": 15, "explanation": "x", "helped": ["a"], "dragged": ["b"]}
      ]
    },
    "missingKeywords": {"fromJobDescription": ["a", "b", "c"], "industryCommon": []},
    "formattingIssues": []
  },
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins": [], "mediumEffort": [], "deepFixes": []}
}`)
	analysis := Analysis{PromptVersion: "v2_3", Model: "test-model", Mode: ModeJobMatch}
	result, err := normalizeAnalysisResult(raw, analysis)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := result["matchScore"].(float64); !ok || got != 85 {
		t.Fatalf("expected matchScore 85, got %v", result["matchScore"])
	}
	if got, ok := result["finalScore"].(float64); !ok || got != 85 {
		t.Fatalf("expected finalScore 85, got %v", result["finalScore"])
	}
}

func TestNormalizeFinalScoreATSModeUsesATSScore(t *testing.T) {
	raw := []byte(`{
  "matchScore": 88,
  "meta": {
    "promptVersion": "v2_3",
    "model": "test-model",
    "jobDescriptionProvided": true,
    "confidence": 0.5,
    "assumptions": [],
    "limitations": []
  },
  "summary": {"overallAssessment": "ok", "strengths": [], "weaknesses": []},
  "ats": {
    "score": 74,
    "scoreBreakdown": {"skills": 20, "experience": 20, "impact": 20, "formatting": 20, "roleFit": 20},
    "scoreReasoning": ["a", "b", "c"],
    "scoreExplanation": {
      "components": [
        {"key": "atsReadability", "label": "ATS Readability", "score": 75, "weight": 25, "explanation": "x", "helped": ["a"], "dragged": ["b"]},
        {"key": "skillMatch", "label": "Skill Match", "score": 70, "weight": 30, "explanation": "x", "helped": ["a"], "dragged": ["b"]},
        {"key": "experienceRelevance", "label": "Experience Relevance", "score": 80, "weight": 30, "explanation": "x", "helped": ["a"], "dragged": ["b"]},
        {"key": "resumeStructure", "label": "Resume Structure", "score": 78, "weight": 15, "explanation": "x", "helped": ["a"], "dragged": ["b"]}
      ]
    },
    "missingKeywords": {"fromJobDescription": ["a", "b", "c"], "industryCommon": []},
    "formattingIssues": []
  },
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins": [], "mediumEffort": [], "deepFixes": []}
}`)
	analysis := Analysis{PromptVersion: "v2_3", Model: "test-model", Mode: ModeATS}
	result, err := normalizeAnalysisResult(raw, analysis)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := result["finalScore"].(float64); !ok || got != 74 {
		t.Fatalf("expected finalScore 74, got %v", result["finalScore"])
	}
	meta, ok := result["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta in normalized result")
	}
	if meta["mode"] != "ATS" {
		t.Fatalf("expected meta.mode ATS, got %v", meta["mode"])
	}
	if meta["primaryScoreType"] != "ATS" {
		t.Fatalf("expected meta.primaryScoreType ATS, got %v", meta["primaryScoreType"])
	}
}
