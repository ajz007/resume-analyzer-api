package analyses

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestValidOutputUnmarshal(t *testing.T) {
	payload := loadFixture(t, "testdata/valid_output_v1.json")

	var out AnalysisResultV1
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected valid JSON to unmarshal, got error: %v", err)
	}
}

func TestInvalidOutputFails(t *testing.T) {
	payload := loadFixture(t, "testdata/invalid_output_v1.json")

	var out AnalysisResultV1
	if err := json.Unmarshal(payload, &out); err == nil {
		t.Fatalf("expected invalid JSON to fail unmarshal")
	}
}

func TestSanitizeError(t *testing.T) {
	long := strings.Repeat("a", 600)
	msg := sanitizeError(errorsWithNewlines(long))

	if strings.Contains(msg, "\n") || strings.Contains(msg, "\r") {
		t.Fatalf("expected newlines to be stripped, got %q", msg)
	}
	if len(msg) != 500 {
		t.Fatalf("expected length 500, got %d", len(msg))
	}
}

func TestV1GoodUnmarshalAndValidate(t *testing.T) {
	payload := loadFixture(t, "testdata/v1_good.json")

	var out AnalysisResultV1
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v1 JSON to unmarshal, got error: %v", err)
	}
}

func TestV2GoodUnmarshalAndValidate(t *testing.T) {
	payload := loadFixture(t, "testdata/v2_good.json")

	var out AnalysisResultV2
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2 JSON to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("expected v2 JSON to validate, got error: %v", err)
	}
}

func TestSchemaRequiredKeys(t *testing.T) {
	payload := loadFixture(t, "testdata/v1_good.json")

	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("unmarshal into map: %v", err)
	}

	required := []string{
		"summary",
		"ats",
		"issues",
		"bulletRewrites",
		"missingInformation",
		"actionPlan",
	}
	for _, key := range required {
		if _, ok := raw[key]; !ok {
			t.Fatalf("missing required key: %s", key)
		}
	}
}

func loadFixture(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func errorsWithNewlines(msg string) error {
	return &testError{msg: "bad\n" + msg + "\r\nend"}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
