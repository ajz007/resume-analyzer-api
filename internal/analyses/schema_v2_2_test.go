package analyses

import (
	"encoding/json"
	"testing"
)

func TestAnalysisResultV2_2GoodFixture(t *testing.T) {
	payload := loadFixture(t, "testdata/v2_2_good.json")

	var out AnalysisResultV2_2
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2_2 good fixture to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("expected v2_2 good fixture to validate, got error: %v", err)
	}
}

func TestAnalysisResultV2_2BadSumFixture(t *testing.T) {
	payload := loadFixture(t, "testdata/v2_2_bad_sum.json")

	var out AnalysisResultV2_2
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2_2 bad sum fixture to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err == nil {
		t.Fatalf("expected validation error for scoreBreakdown sum")
	}
}

func TestAnalysisResultV2_2BadPlaceholdersFixture(t *testing.T) {
	payload := loadFixture(t, "testdata/v2_2_bad_placeholders.json")

	var out AnalysisResultV2_2
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2_2 bad placeholders fixture to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err == nil {
		t.Fatalf("expected validation error for autoFixable/placeholder rules")
	}
}
