package analyses

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"resume-backend/internal/llm"
)

type mockLLM struct {
	responses []json.RawMessage
	calls     int
	lastCtx   context.Context
}

func (m *mockLLM) AnalyzeResume(ctx context.Context, input llm.AnalyzeInput) (json.RawMessage, error) {
	m.calls++
	m.lastCtx = ctx
	if len(m.responses) == 0 {
		return nil, nil
	}
	resp := m.responses[0]
	if len(m.responses) > 1 {
		m.responses = m.responses[1:]
	}
	return resp, nil
}

func TestValidateV2WithRetry(t *testing.T) {
	invalid := json.RawMessage(`{
  "meta": {"promptVersion":"v2","model":"gpt-4o-mini","jobDescriptionProvided":false,"confidence":0.5,"assumptions":[],"limitations":[]},
  "summary": {"overallAssessment":"ok","strengths":[],"weaknesses":[]},
  "ats": {"score":50,"scoreBreakdown":{"skills":50,"experience":40,"impact":30,"formatting":0,"roleFit":20},"missingKeywords":{"fromJobDescription":[],"industryCommon":[]},"formattingIssues":[]},
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins":[],"mediumEffort":[],"deepFixes":[]}
}`)
	valid := json.RawMessage(`{
  "meta": {"promptVersion":"v2","model":"gpt-4o-mini","jobDescriptionProvided":false,"confidence":0.5,"assumptions":[],"limitations":[]},
  "summary": {"overallAssessment":"ok","strengths":[],"weaknesses":[]},
  "ats": {"score":50,"scoreBreakdown":{"skills":20,"experience":20,"impact":20,"formatting":20,"roleFit":20},"missingKeywords":{"fromJobDescription":[],"industryCommon":[]},"formattingIssues":[]},
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins":[],"mediumEffort":[],"deepFixes":[]}
}`)

	mock := &mockLLM{responses: []json.RawMessage{invalid, valid}}
	_, err := ValidateV2WithRetry(context.Background(), mock, llm.AnalyzeInput{PromptVersion: "v2"})
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if mock.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.calls)
	}
	if msg, ok := llm.ExtraSystemMessageFromContext(mock.lastCtx); !ok || !strings.Contains(msg, "Fix the JSON") {
		t.Fatalf("expected retry context to include extra system message")
	}
}

func TestValidateV2WithRetryFails(t *testing.T) {
	invalid := json.RawMessage(`{
  "meta": {"promptVersion":"v2","model":"gpt-4o-mini","jobDescriptionProvided":false,"confidence":0.5,"assumptions":[],"limitations":[]},
  "summary": {"overallAssessment":"ok","strengths":[],"weaknesses":[]},
  "ats": {"score":50,"scoreBreakdown":{"skills":50,"experience":40,"impact":30,"formatting":0,"roleFit":20},"missingKeywords":{"fromJobDescription":[],"industryCommon":[]},"formattingIssues":[]},
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins":[],"mediumEffort":[],"deepFixes":[]}
}`)

	mock := &mockLLM{responses: []json.RawMessage{invalid, invalid}}
	_, err := ValidateV2WithRetry(context.Background(), mock, llm.AnalyzeInput{PromptVersion: "v2"})
	if err == nil {
		t.Fatalf("expected retry to fail with validation error")
	}
	if mock.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.calls)
	}
}

func TestNormalizeScoreBreakdownAdjustsUp(t *testing.T) {
	r := AnalysisResultV2{
		Meta:    MetaV2{PromptVersion: "v2", Model: "gpt-4o-mini"},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2{
			Score: 50,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills:     10,
				Experience: 10,
				Impact:     10,
				Formatting: 10,
				RoleFit:    28,
			},
		},
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("expected normalization to succeed, got error: %v", err)
	}
	if total := r.ATS.ScoreBreakdown.Skills + r.ATS.ScoreBreakdown.Experience + r.ATS.ScoreBreakdown.Impact +
		r.ATS.ScoreBreakdown.Formatting + r.ATS.ScoreBreakdown.RoleFit; total != 100 {
		t.Fatalf("expected total 100, got %.0f", total)
	}
}

func TestNormalizeScoreBreakdownAdjustsDown(t *testing.T) {
	r := AnalysisResultV2{
		Meta:    MetaV2{PromptVersion: "v2", Model: "gpt-4o-mini"},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2{
			Score: 50,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills:     30,
				Experience: 30,
				Impact:     30,
				Formatting: 10,
				RoleFit:    10,
			},
		},
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("expected normalization to succeed, got error: %v", err)
	}
	if total := r.ATS.ScoreBreakdown.Skills + r.ATS.ScoreBreakdown.Experience + r.ATS.ScoreBreakdown.Impact +
		r.ATS.ScoreBreakdown.Formatting + r.ATS.ScoreBreakdown.RoleFit; total != 100 {
		t.Fatalf("expected total 100, got %.0f", total)
	}
}

func TestNormalizeScoreBreakdownNegativeAdjustment(t *testing.T) {
	r := AnalysisResultV2{
		Meta:    MetaV2{PromptVersion: "v2", Model: "gpt-4o-mini"},
		Summary: SummaryV1{OverallAssessment: "ok"},
		ATS: ATSV2{
			Score: 50,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills:     60,
				Experience: 30,
				Impact:     20,
				Formatting: 5,
				RoleFit:    5,
			},
		},
	}
	if err := r.Validate(); err == nil {
		t.Fatalf("expected normalization to fail due to negative adjustment")
	}
}
