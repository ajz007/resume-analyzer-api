package analyses

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReviewSourceMapsOnlyWhitespaceAndHyphens(t *testing.T) {
	source := "Start.\nPerformed debugging and root\n-\ncause analysis.\nEnd."
	a, b, err := sourceSpan(source, "Performed debugging and root-cause analysis.")
	if err != nil || source[a:b] != "Performed debugging and root\n-\ncause analysis." {
		t.Fatal(a, b, err)
	}
	for _, bad := range []string{"performed debugging and root-cause analysis.", "Performed debugging and root cause analysis.", "Performed debugging and root/cause analysis."} {
		if _, _, err = sourceSpan(source, bad); err == nil {
			t.Fatal("changed text accepted", bad)
		}
	}
	if _, _, err = sourceSpan(source+"\nPerformed debugging and root-cause analysis.", "Performed debugging and root-cause analysis."); err == nil {
		t.Fatal("normalization made duplicate anchors unsafe")
	}
	if _, _, err = sourceSpan("GitHub uses JavaScript.", "Git"); err == nil {
		t.Fatal("partial token matched")
	}
	if _, _, err = sourceSpan("Costs 30%.", "3"); err == nil {
		t.Fatal("partial number matched")
	}
	source = "Built café APIs.\nUnchanged."
	a, b, err = sourceSpan(source, "café APIs.")
	if err != nil || source[a:b] != "café APIs." {
		t.Fatal("unicode byte mapping", a, b, err)
	}
}

func TestReviewSectionOrdersMovedTextAfterItsRetainedAnchor(t *testing.T) {
	source := "Employer\nFirst achievement.\nRecovery achievement.\nOther section\nCost improvement."
	r := ReviewReport{Edits: []ReviewEdit{{ID: "move", Operation: "move", Before: "Cost improvement.", After: "Cost improvement.", InsertAfter: "Recovery achievement."}}, Sections: []ReviewSection{{Heading: "Employer", EditIDs: []string{"move"}, Retained: []ReviewRetained{{Text: "First achievement.", SourceQuote: "First achievement."}, {Text: "Recovery achievement.", SourceQuote: "Recovery achievement."}}}}}
	if err := r.PreparePresentation(source); err != nil {
		t.Fatal(err)
	}
	items := r.Sections[0].Items
	if len(items) != 3 || items[0].RetainedIndex != 0 || items[1].RetainedIndex != 1 || items[2].EditID != "move" {
		t.Fatal(items)
	}
	var schema map[string]any
	_ = json.Unmarshal(AnalysisV3JSONSchema(), &schema)
	root := schema["properties"].(map[string]any)
	report := root["report"].(map[string]any)["properties"].(map[string]any)
	section := report["sections"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if _, ok := section["items"]; ok {
		t.Fatal("model can supply trusted derived ordering")
	}
}

func TestReviewLayoutKeepsListsAndRefusesChangedContent(t *testing.T) {
	source := "Engineer\nExperience\n•\nFirst achievement.\n•\nSecond achievement.\nEducation\nUniversity"
	layout := "Engineer\n\nExperience\n\n• First achievement.\n• Second achievement.\n\nEducation\nUniversity"
	r := ReviewReport{Edits: []ReviewEdit{{ID: "move", Operation: "move", Before: "Second achievement.", After: "Second achievement.", InsertAfter: "First achievement."}}}
	got, err := r.ApplyToLayout(source, layout)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, "•") != 2 || strings.Count(got, "Second achievement.") != 1 || !strings.Contains(got, "• First achievement.\n\n• Second achievement.") {
		t.Fatal(got)
	}
	if _, err = r.ApplyToLayout(source, strings.ReplaceAll(layout, "University", "Another university")); err == nil {
		t.Fatal("changed original file accepted")
	}
	r.Edits[0] = ReviewEdit{Operation: "remove", Before: "First achievement."}
	got, err = r.ApplyToLayout(source, layout)
	if err != nil || strings.Count(got, "•") != 1 || strings.Contains(got, "First achievement.") {
		t.Fatal(got, err)
	}
	r.Edits[0] = ReviewEdit{Operation: "replace", Before: "First achievement.", After: "Clearer first achievement."}
	got, err = r.ApplyToLayout(source, layout)
	if err != nil || !strings.Contains(got, "• Clearer first achievement.") {
		t.Fatal(got, err)
	}
}

func TestFocusedQuestionsBoundedWithoutBlockingLegacyReports(t *testing.T) {
	if err := validateFocusedReviewQuestions([]ReviewQuestion{{ID: "q", Question: "When did you first use Terraform professionally?"}}); err != nil {
		t.Fatal(err)
	}
	if err := validateFocusedReviewQuestions([]ReviewQuestion{{ID: "q", Question: "What did you build? What dates did you work?"}}); err == nil {
		t.Fatal("compound questions accepted")
	}
	if err := validateFocusedReviewQuestions([]ReviewQuestion{{ID: "q", Question: strings.Repeat("A", 261)}}); err == nil {
		t.Fatal("overlong question accepted")
	}
}

func TestReviewReflowKeepsProjectAndFieldBoundaries(t *testing.T) {
	source := "Experience\n• Built APIs.\nProject | Provisioning\nA project description.\n• Delivered services.\nSelected projects (internal)\n• Built a demo.\nSkills\nBackend: " + strings.Repeat("Spring ", 20) + "\nDatabase: SQL\nUniversityAug 2020 – May 2024\nB.Tech;CGPA: 7.98Chennai"
	got := reflowReviewText(source)
	for _, want := range []string{"Built APIs.\n\nProject | Provisioning", "Delivered services.\n\nSelected projects", "\n\nDatabase: SQL", "University Aug 2020", "CGPA: 7.98 Chennai"} {
		if !strings.Contains(got, want) {
			t.Fatal(want, got)
		}
	}
	if !sameReviewCharacters(source, got) {
		t.Fatal("reflow changed nonspace content")
	}
}
