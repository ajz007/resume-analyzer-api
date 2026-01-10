package analyses

import (
	"encoding/json"
	"testing"
)

func TestAnalysisResultV2_3GoodFixture(t *testing.T) {
	payload := loadFixture(t, "testdata/v2_3_good.json")

	var out AnalysisResultV2_3
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2_3 good fixture to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("expected v2_3 good fixture to validate, got error: %v", err)
	}
}

func TestAnalysisResultV2_3BadClaimSupportFixture(t *testing.T) {
	payload := loadFixture(t, "testdata/v2_3_bad_claimsupport.json")

	var out AnalysisResultV2_3
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2_3 bad claimsupport fixture to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err == nil {
		t.Fatalf("expected validation error for claimsupport rules")
	}
}

func TestAnalysisResultV2_3BadEvidenceFixture(t *testing.T) {
	payload := loadFixture(t, "testdata/v2_3_bad_evidence.json")

	var out AnalysisResultV2_3
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2_3 bad evidence fixture to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err == nil {
		t.Fatalf("expected validation error for evidence rules")
	}
}
