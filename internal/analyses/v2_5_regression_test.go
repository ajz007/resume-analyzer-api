package analyses

import (
	"encoding/json"
	"testing"
)

func TestNormalizeToFinalV2_5JobMatchGolden(t *testing.T) {
	raw := loadFixture(t, "testdata/v2_5_job_match_good.json")

	out, err := normalizeToFinal(raw, Analysis{PromptVersion: "v2_5", Model: "test-model", Mode: ModeJobMatch})
	if err != nil {
		t.Fatalf("normalize v2_5 job match: %v", err)
	}

	if out.Meta.PromptVersion != "v2_5" {
		t.Fatalf("expected meta.promptVersion v2_5, got %q", out.Meta.PromptVersion)
	}
	if out.JobMatchScoring.Score != 78 {
		t.Fatalf("expected recomputed jobMatchScoring.score 78, got %.0f", out.JobMatchScoring.Score)
	}
	if out.AIScreening.Score != 80 {
		t.Fatalf("expected recomputed aiScreening.score 80, got %.0f", out.AIScreening.Score)
	}
	if len(out.JobRequirementProfile.TopPriorities) != 3 {
		t.Fatalf("expected 3 top priorities, got %d", len(out.JobRequirementProfile.TopPriorities))
	}
}

func TestNormalizeToFinalV2_5MatchesV2_4ForSameOutput(t *testing.T) {
	v24, err := normalizeToFinal(loadFixture(t, "testdata/v2_4_job_match_good.json"), Analysis{PromptVersion: "v2_4", Model: "test-model", Mode: ModeJobMatch})
	if err != nil {
		t.Fatalf("normalize v2_4: %v", err)
	}
	v25, err := normalizeToFinal(loadFixture(t, "testdata/v2_5_job_match_good.json"), Analysis{PromptVersion: "v2_5", Model: "test-model", Mode: ModeJobMatch})
	if err != nil {
		t.Fatalf("normalize v2_5: %v", err)
	}

	if v25.ATS.Score != v24.ATS.Score || v25.FinalScore != v24.FinalScore || v25.AIScreening.Score != v24.AIScreening.Score {
		t.Fatalf("expected v2_5 scores to match v2_4: ats %.0f/%.0f final %.0f/%.0f ai %.0f/%.0f",
			v25.ATS.Score, v24.ATS.Score, v25.FinalScore, v24.FinalScore, v25.AIScreening.Score, v24.AIScreening.Score)
	}
}

func TestNormalizedAnalysisResultV2_5MarshalIncludesFrontendFields(t *testing.T) {
	raw := loadFixture(t, "testdata/v2_5_job_match_good.json")

	out, err := normalizeToFinal(raw, Analysis{PromptVersion: "v2_5", Model: "test-model", Mode: ModeJobMatch})
	if err != nil {
		t.Fatalf("normalize v2_5 job match: %v", err)
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
