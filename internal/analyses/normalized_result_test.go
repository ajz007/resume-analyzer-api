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

func TestNormalizeV2_4ReturnsJobRequirementProfile(t *testing.T) {
	result := normalizeV2_4Fixture(t, func(r *AnalysisResultV2_4) {
		r.JobRequirementProfile.PrimaryRole = " Backend Engineer "
	})

	profile, ok := result["jobRequirementProfile"].(map[string]any)
	if !ok {
		t.Fatalf("expected jobRequirementProfile in normalized result")
	}
	if profile["primaryRole"] != "Backend Engineer" {
		t.Fatalf("expected trimmed primaryRole, got %v", profile["primaryRole"])
	}
	topPriorities, ok := profile["topPriorities"].([]any)
	if !ok || len(topPriorities) == 0 {
		t.Fatalf("expected topPriorities list, got %v", profile["topPriorities"])
	}
}

func TestNormalizeV2_4JobMatchFinalScoreUsesRecomputedScore(t *testing.T) {
	result := normalizeV2_4Fixture(t, func(r *AnalysisResultV2_4) {
		r.JobMatchScoring.Score = 99
	})

	if got, ok := result["matchScore"].(float64); !ok || got != 78 {
		t.Fatalf("expected recomputed matchScore 78, got %v", result["matchScore"])
	}
	if got, ok := result["finalScore"].(float64); !ok || got != 78 {
		t.Fatalf("expected finalScore 78, got %v", result["finalScore"])
	}
	scoring, ok := result["jobMatchScoring"].(map[string]any)
	if !ok {
		t.Fatalf("expected jobMatchScoring in normalized result")
	}
	if got, ok := scoring["score"].(float64); !ok || got != 78 {
		t.Fatalf("expected jobMatchScoring.score 78, got %v", scoring["score"])
	}
}

func TestNormalizeV2_4ATSModeUsesATSScoreAndZeroMatchScore(t *testing.T) {
	raw := validAnalysisResultV2_4()
	raw.Meta.Mode = string(ModeATS)
	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal v2_4 fixture: %v", err)
	}

	result, err := normalizeAnalysisResult(payload, Analysis{PromptVersion: "v2_4", Model: "test-model", Mode: ModeATS})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := result["finalScore"].(float64); !ok || got != 82 {
		t.Fatalf("expected finalScore 82, got %v", result["finalScore"])
	}
	if got, ok := result["matchScore"].(float64); !ok || got != 0 {
		t.Fatalf("expected matchScore 0, got %v", result["matchScore"])
	}
}

func TestNormalizeV2_4AIScreeningScoreIsRecomputed(t *testing.T) {
	result := normalizeV2_4Fixture(t, func(r *AnalysisResultV2_4) {
		r.AIScreening.Score = 100
		r.AIScreening.Verdict.Tier = "STRONG"
		r.AIScreening.Verdict.ScreeningRisk = "LOW"
	})

	screening, ok := result["aiScreening"].(map[string]any)
	if !ok {
		t.Fatalf("expected aiScreening in normalized result")
	}
	if got, ok := screening["score"].(float64); !ok || got != 80 {
		t.Fatalf("expected recomputed aiScreening.score 80, got %v", screening["score"])
	}
	verdict, ok := screening["verdict"].(map[string]any)
	if !ok {
		t.Fatalf("expected aiScreening.verdict")
	}
	if verdict["tier"] != "GOOD" {
		t.Fatalf("expected recomputed tier GOOD, got %v", verdict["tier"])
	}
	if verdict["screeningRisk"] != "LOW" {
		t.Fatalf("expected recomputed screeningRisk LOW, got %v", verdict["screeningRisk"])
	}
}

func TestNormalizeV2_4FixThisFirstFallbackCreatedWhenMissing(t *testing.T) {
	result := normalizeV2_4Fixture(t, func(r *AnalysisResultV2_4) {
		r.FixThisFirst = nil
		r.JobMatchScoring.RequirementScores[0].Score = 55
		r.JobMatchScoring.RequirementScores[0].MatchStatus = "WEAK"
		r.JobMatchScoring.RequirementScores[1].Score = 65
		r.JobMatchScoring.RequirementScores[1].MatchStatus = "PARTIAL"
		r.JobMatchScoring.RequirementScores[2].Score = 80
		r.JobMatchScoring.RequirementScores[2].MatchStatus = "STRONG"
	})

	items, ok := result["fixThisFirst"].([]any)
	if !ok {
		t.Fatalf("expected fixThisFirst list, got %v", result["fixThisFirst"])
	}
	if len(items) == 0 {
		t.Fatalf("expected fallback fixThisFirst items")
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected fixThisFirst item object")
	}
	if first["linkedRequirementId"] != "req_backend_go" {
		t.Fatalf("expected first fallback linked to req_backend_go, got %v", first["linkedRequirementId"])
	}
}

func normalizeV2_4Fixture(t *testing.T, mutate func(*AnalysisResultV2_4)) map[string]any {
	t.Helper()
	raw := validAnalysisResultV2_4()
	if mutate != nil {
		mutate(&raw)
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal v2_4 fixture: %v", err)
	}
	result, err := normalizeAnalysisResult(payload, Analysis{PromptVersion: "v2_4", Model: "test-model", Mode: ModeJobMatch})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}
