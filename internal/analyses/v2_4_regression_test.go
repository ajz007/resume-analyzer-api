package analyses

import (
	"encoding/json"
	"testing"
)

func TestNormalizeToFinalV2_4JobMatchGolden(t *testing.T) {
	raw := loadFixture(t, "testdata/v2_4_job_match_good.json")

	out, err := normalizeToFinal(raw, Analysis{PromptVersion: "v2_4", Model: "test-model", Mode: ModeJobMatch})
	if err != nil {
		t.Fatalf("normalize v2_4 job match: %v", err)
	}

	if out.JobMatchScoring.Score != 78 {
		t.Fatalf("expected recomputed jobMatchScoring.score 78, got %.0f", out.JobMatchScoring.Score)
	}
	if out.FinalScore != 78 {
		t.Fatalf("expected finalScore 78, got %.0f", out.FinalScore)
	}
	if out.MatchScore != 78 {
		t.Fatalf("expected matchScore 78, got %.0f", out.MatchScore)
	}
	if out.AIScreening.Score != 80 {
		t.Fatalf("expected recomputed aiScreening.score 80, got %.0f", out.AIScreening.Score)
	}
	if out.AIScreening.Verdict.Tier != "GOOD" {
		t.Fatalf("expected recomputed tier GOOD, got %q", out.AIScreening.Verdict.Tier)
	}
	if len(out.JobRequirementProfile.TopPriorities) != 3 {
		t.Fatalf("expected 3 top priorities, got %d", len(out.JobRequirementProfile.TopPriorities))
	}
	if out.JobRequirementProfile.TopPriorities[0].ID != "req_backend_go" {
		t.Fatalf("expected top priority preserved, got %q", out.JobRequirementProfile.TopPriorities[0].ID)
	}
	if len(out.FixThisFirst) == 0 {
		t.Fatalf("expected fixThisFirst to be present")
	}
}

func TestNormalizeToFinalV2_4ATSGolden(t *testing.T) {
	raw := loadFixture(t, "testdata/v2_4_ats_good.json")

	out, err := normalizeToFinal(raw, Analysis{PromptVersion: "v2_4", Model: "test-model", Mode: ModeATS})
	if err != nil {
		t.Fatalf("normalize v2_4 ats: %v", err)
	}

	if out.FinalScore != 82 {
		t.Fatalf("expected finalScore 82, got %.0f", out.FinalScore)
	}
	if out.MatchScore != 0 {
		t.Fatalf("expected matchScore 0, got %.0f", out.MatchScore)
	}
	if out.JobRequirementProfile.IsApplicable {
		t.Fatalf("expected jobRequirementProfile.isApplicable=false")
	}
}

func TestAnalysisResultV2_4BadTopPriorityWeightsFixture(t *testing.T) {
	raw := loadFixture(t, "testdata/v2_4_bad_weights.json")

	var out AnalysisResultV2_4
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal bad weights fixture: %v", err)
	}
	if err := out.Validate(); err == nil {
		t.Fatalf("expected schema validation to reject bad top priority weights")
	}
}

func TestAnalysisResultV2_4BadAIScreeningBreakdownFixture(t *testing.T) {
	raw := loadFixture(t, "testdata/v2_4_bad_ai_breakdown.json")

	var out AnalysisResultV2_4
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal bad ai breakdown fixture: %v", err)
	}
	if err := out.Validate(); err == nil {
		t.Fatalf("expected schema validation to reject bad aiScreening breakdown")
	}
}

func TestValidateContentV2_4BadHardGuaranteeFixture(t *testing.T) {
	raw := loadFixture(t, "testdata/v2_4_bad_hard_guarantee.json")

	var out AnalysisResultV2_4
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal bad hard guarantee fixture: %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("expected hard guarantee fixture to be schema-valid, got %v", err)
	}
	if err := ValidateContentV2_4(&out); err == nil {
		t.Fatalf("expected content validation to reject hard guarantee phrase")
	}
}

func TestNormalizedAnalysisResultV2_4MarshalIncludesFrontendFields(t *testing.T) {
	raw := loadFixture(t, "testdata/v2_4_job_match_good.json")

	out, err := normalizeToFinal(raw, Analysis{PromptVersion: "v2_4", Model: "test-model", Mode: ModeJobMatch})
	if err != nil {
		t.Fatalf("normalize v2_4 job match: %v", err)
	}
	payload, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal normalized result: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal normalized result: %v", err)
	}

	for _, key := range []string{"jobRequirementProfile", "jobMatchScoring", "aiScreening", "fixThisFirst"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("expected marshaled normalized result to include %s", key)
		}
	}
}
