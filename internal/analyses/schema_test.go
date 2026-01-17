package analyses

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

func TestATSV1MissingKeywordsArray(t *testing.T) {
	payload := []byte(`{"ats":{"missingKeywords":["golang","aws"]}}`)

	var result AnalysisResultV1
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := []string(result.ATS.MissingKeywords)
	want := []string{"golang", "aws"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestATSV1MissingKeywordsObject(t *testing.T) {
	payload := []byte(`{
		"ats": {
			"missingKeywords": {
				"fromJobDescription": ["sql", "docker"],
				"industryCommon": "kubernetes",
				"other": {"nested": ["terraform"]}
			}
		}
	}`)

	var result AnalysisResultV1
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := []string(result.ATS.MissingKeywords)
	sort.Strings(got)
	want := []string{"docker", "kubernetes", "sql", "terraform"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
