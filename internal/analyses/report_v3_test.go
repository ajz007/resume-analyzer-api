package analyses

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"resume-backend/internal/documents"
	"resume-backend/internal/llm"
	"resume-backend/internal/shared/storage/object/local"
)

const reviewSource = "Engineer\nTradingCo\nImplemented automated failover, significantly reducing downtime.\nMigrated services, reducing deployment times by 30%.\nA separate initiative reduced costs by 20%.\nEducation: Example University."

func reviewFixture(t *testing.T) AnalysisResultV3 {
	t.Helper()
	var result AnalysisResultV3
	if err := json.Unmarshal(loadFixture(t, "testdata/v2_4_job_match_good.json"), &result); err != nil {
		t.Fatal(err)
	}
	result.Meta.PromptVersion = "v3"
	result.Issues = []IssueV2_2{}
	result.BulletRewrites = []BulletRewriteV2_3{}
	result.MissingInformation = []string{}
	result.ActionPlan = ActionPlanV1{QuickWins: []string{}, MediumEffort: []string{}, DeepFixes: []string{}}
	result.FixThisFirst = []FixThisFirstItemV1{}
	result.Report = ReviewReport{
		SchemaVersion: "3", Scope: "SELECTED_PASSAGES",
		Priorities: []ReviewPriority{{Title: "Clarify resilience", Reason: "Preserve the actual work without a missing metric.", EditIDs: []string{"failover"}}},
		Edits:      []ReviewEdit{{ID: "failover", Operation: "replace", Section: "TradingCo", Before: "Implemented automated failover, significantly reducing downtime.", After: "Implemented automated failover, reducing downtime.", Reason: "Retain the reported outcome without vague amplification.", SourceQuotes: []string{"Implemented automated failover, significantly reducing downtime."}, RequirementIDs: []string{}}},
		Sections:   []ReviewSection{{Heading: "TradingCo", EditIDs: []string{"failover"}, Retained: []ReviewRetained{}}},
		Questions:  []ReviewQuestion{{ID: "recovery", Question: "How did you validate recovery?", Why: "Could make the resilience evidence more concrete.", EditIDs: []string{"failover"}}},
		Outcomes:   ReviewOutcomes{ClearerNow: []string{"Resilience implementation."}, NeedsConfirmation: []string{"Recovery validation."}, StillUnresolved: []string{"No direct payments claim is established."}},
		Cleanup:    []ReviewNote{}, InterviewPreparation: []string{"Explain the recovery behavior."},
		Coverage: []ReviewCoverage{{SourceQuote: "A separate initiative reduced costs by 20%.", Disposition: "not_selected", Reason: "Preserve the unique cost result in the original document."}},
	}
	return result
}

func TestV3ReportSourceValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(*ReviewReport)
	}{
		{"placeholder", func(r *ReviewReport) { r.Edits[0].After = "Reduced downtime by [amount]%." }},
		{"invented number", func(r *ReviewReport) { r.Edits[0].After = "Reduced downtime by 90%." }},
		{"ownership inflation", func(r *ReviewReport) { r.Edits[0].After = "Led automated failover implementation." }},
		{"unknown reference", func(r *ReviewReport) { r.Priorities[0].EditIDs = []string{"missing"} }},
		{"unassembled edit", func(r *ReviewReport) { r.Sections = []ReviewSection{} }},
		{"duplicate assembly", func(r *ReviewReport) { r.Sections[0].EditIDs = []string{"failover", "failover"} }},
		{"invented evidence", func(r *ReviewReport) { r.Edits[0].SourceQuotes = []string{"Built instant payment rails."} }},
		{"missing coverage evidence", func(r *ReviewReport) { r.Coverage[0].SourceQuote = "Does not exist." }},
		{"altered retained text", func(r *ReviewReport) {
			r.Sections[0].Retained = []ReviewRetained{{Text: "Invented achievement", SourceQuote: "Engineer"}}
		}},
		{"unknown requirement", func(r *ReviewReport) { r.Edits[0].RequirementIDs = []string{"not-a-requirement"} }},
		{"numeric substring is not evidence", func(r *ReviewReport) {
			r.Edits[0].After = "Reduced downtime by 3%."
			r.Edits[0].SourceQuotes = []string{"Migrated services, reducing deployment times by 30%."}
		}},
		{"null outcomes", func(r *ReviewReport) { r.Outcomes.ClearerNow = nil }},
		{"invented retained original", func(r *ReviewReport) {
			r.Edits[0].Operation = "retain"
			r.Edits[0].Before = "Built another system."
			r.Edits[0].After = r.Edits[0].Before
		}},
	}
	valid := reviewFixture(t)
	if err := valid.Report.Validate(reviewSource, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := reviewFixture(t)
			tc.change(&r.Report)
			if err := r.Report.Validate(reviewSource, nil); err == nil {
				t.Fatal("expected unsafe report to fail")
			}
		})
	}
}

func TestReviewPatchPreservesUniqueUnselectedAchievements(t *testing.T) {
	r := reviewFixture(t)
	result, err := r.Report.ApplyToSource(reviewSource)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "significantly") || !strings.Contains(result, "Implemented automated failover, reducing downtime.") {
		t.Fatal(result)
	}
	for _, text := range []string{"30%", "20%", "Education: Example University."} {
		if !strings.Contains(result, text) {
			t.Fatalf("lost unchanged source %s", text)
		}
	}
	if _, err = r.Report.ApplyToSource(reviewSource + "\n" + r.Report.Edits[0].Before); err == nil {
		t.Fatal("ambiguous source must fail")
	}
	r.Report.Edits = append(r.Report.Edits, ReviewEdit{ID: "overlap", Operation: "remove", Before: "significantly reducing downtime."})
	if _, err = r.Report.ApplyToSource(reviewSource); err == nil {
		t.Fatal("overlap must fail")
	}
}

func TestReviewPatchOperations(t *testing.T) {
	source := "Header\nFirst bullet.\nSecond bullet.\nFooter"
	tests := []struct {
		name string
		edit ReviewEdit
		want string
	}{
		{"add", ReviewEdit{Operation: "add", InsertAfter: "Header", After: "Summary"}, "Header\nSummary\nFirst bullet.\nSecond bullet.\nFooter"},
		{"remove", ReviewEdit{Operation: "remove", Before: "First bullet."}, "Header\n\nSecond bullet.\nFooter"},
		{"move", ReviewEdit{Operation: "move", Before: "Second bullet.", After: "Second bullet.", InsertAfter: "Header"}, "Header\nSecond bullet.\nFirst bullet.\n\nFooter"},
		{"merge", ReviewEdit{Operation: "merge", Before: "First bullet.\nSecond bullet.", After: "Merged bullet."}, "Header\nMerged bullet.\nFooter"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (ReviewReport{Edits: []ReviewEdit{tc.edit}}).ApplyToSource(source)
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestV3PaywallIsAnAllowlistAndDoesNotMutate(t *testing.T) {
	r := reviewFixture(t)
	r.Report.Questions[0].Question = "PRIVATE QUESTION"
	raw, _ := json.Marshal(r)
	var original map[string]any
	_ = json.Unmarshal(raw, &original)
	original["report"].(map[string]any)["futurePaidField"] = "PRIVATE FUTURE FIELD"
	redacted, locked := RedactForFreeTier(original)
	payload, _ := json.Marshal(redacted)
	if strings.Contains(string(payload), "PRIVATE") || strings.Contains(string(payload), "reducing downtime.") {
		t.Fatalf("paid content leaked: %s", payload)
	}
	if !strings.Contains(strings.Join(locked, ","), LockedReport) {
		t.Fatal("missing report lock")
	}
	if _, ok := original["report"].(map[string]any)["questions"]; !ok {
		t.Fatal("stored report was mutated")
	}
}

type schemaCaptureLLM struct {
	responses    []json.RawMessage
	calls        int
	input        llm.AnalyzeInput
	repairSource string
}

func (m *schemaCaptureLLM) AnalyzeResume(_ context.Context, input llm.AnalyzeInput) (json.RawMessage, error) {
	m.input = input
	m.repairSource = input.ResumeText
	i := m.calls
	m.calls++
	if i >= len(m.responses) {
		i = len(m.responses) - 1
	}
	return m.responses[i], nil
}

func TestV3ValidationUsesStrictSchemaAndSourceOnBoundedRepair(t *testing.T) {
	r := reviewFixture(t)
	valid, _ := json.Marshal(r)
	client := &schemaCaptureLLM{responses: []json.RawMessage{json.RawMessage(`{"bad":true}`), valid}}
	raw, err := ValidateV3WithRetry(context.Background(), client, llm.AnalyzeInput{ResumeText: reviewSource, JobDescription: "backend role", PromptVersion: "v3"})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 || client.repairSource != canonicalReviewSource(reviewSource).Text || len(client.input.ResponseSchema) == 0 {
		t.Fatal("repair lost source/schema")
	}
	normalized, err := normalizeAnalysisResult(raw, Analysis{PromptVersion: "v3", Mode: ModeJobMatch, Model: "gpt-6.1-sol"})
	if err != nil {
		t.Fatal(err)
	}
	if normalized["report"] == nil || normalized["jobMatchScoring"] == nil {
		t.Fatal("normalization discarded report/diagnostics")
	}
	if len(normalized["bulletRewrites"].([]any)) != 0 {
		t.Fatal("duplicate legacy advice")
	}
}

func TestV3StrictSchemaMatchesTypes(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal(AnalysisV3JSONSchema(), &root); err != nil {
		t.Fatal(err)
	}
	var walk func(map[string]any)
	walk = func(s map[string]any) {
		if s["type"] == "object" {
			if s["additionalProperties"] != false {
				t.Fatal("object not strict")
			}
			props := s["properties"].(map[string]any)
			if len(props) != len(s["required"].([]any)) {
				t.Fatal("schema has optional properties")
			}
			for _, child := range props {
				walk(child.(map[string]any))
			}
		}
		if s["type"] == "array" {
			walk(s["items"].(map[string]any))
		}
	}
	walk(root)
	if _, ok := root["properties"].(map[string]any)["report"]; !ok {
		t.Fatal("missing report schema")
	}
}

func docxText(t *testing.T, data []byte) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			f, _ := file.Open()
			defer f.Close()
			raw, _ := io.ReadAll(f)
			decoder := xml.NewDecoder(bytes.NewReader(raw))
			var text strings.Builder
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if value, ok := token.(xml.CharData); ok {
					text.Write(value)
				}
			}
			return text.String()
		}
	}
	t.Fatal("missing document.xml")
	return ""
}

func TestReviewedExportHonorsSelectionAndPreservesSource(t *testing.T) {
	r := reviewFixture(t)
	raw, _ := json.Marshal(r)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	data, err := RenderReviewedResume(reviewSource, result, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := docxText(t, data)
	if strings.Contains(got, "significantly") || !strings.Contains(got, "20%") || !strings.Contains(got, "Example University") {
		t.Fatal(got)
	}
	none := []string{}
	data, err = RenderReviewedResume(reviewSource, result, &none)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(docxText(t, data), "significantly") {
		t.Fatal("kept original was not exported")
	}
	unknown := []string{"invented"}
	if _, err = RenderReviewedResume(reviewSource, result, &unknown); err == nil {
		t.Fatal("unknown selected edit accepted")
	}
}

func TestV3ReviewedDownloadChecksOwnerAndUnlockBeforeSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := reviewFixture(t)
	raw, _ := json.Marshal(r)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	repo := NewMemoryRepo()
	_ = repo.Create(context.Background(), Analysis{ID: "review", UserID: "owner", Status: StatusCompleted, Result: result})
	handler := &Handler{Svc: &Service{Repo: repo}, Unlocks: fakeUnlocks{active: true}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", c.GetHeader("X-Test-Owner")); c.Next() })
	router.POST("/analyses/:id/reviewed-resume", handler.downloadReviewedResume)
	for _, tc := range []struct {
		owner  string
		status int
	}{{"owner", http.StatusForbidden}, {"another-user", http.StatusNotFound}} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/analyses/review/reviewed-resume", strings.NewReader(`{"editIds":[]}`))
		request.Header.Set("X-Test-Owner", tc.owner)
		router.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("owner=%s: expected %d, got %d", tc.owner, tc.status, response.Code)
		}
	}
}

func TestReportCompatibilityIncludesModelAndPrompt(t *testing.T) {
	base := Analysis{PromptVersion: "v3", Model: "gpt-6.1-sol", Provider: "openai", AnalysisVersion: "test"}
	if !compatibleAnalysis(base, base) {
		t.Fatal("identical configuration did not match")
	}
	changed := base
	changed.Model = "another-model"
	if compatibleAnalysis(base, changed) {
		t.Fatal("model change reused old report")
	}
	changed = base
	changed.PromptVersion = "v2_8"
	if compatibleAnalysis(base, changed) {
		t.Fatal("legacy report reused")
	}
}

func TestReviewedDownloadReturnsChosenEditsWithoutAnotherModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	store := local.New(t.TempDir())
	docs := documents.NewMemoryRepo()
	repo := NewMemoryRepo()
	key, _, _, err := store.Save(ctx, "owner", "source.txt", strings.NewReader(reviewSource))
	if err != nil {
		t.Fatal(err)
	}
	if err = docs.Create(ctx, documents.Document{ID: "source", UserID: "owner", ExtractedTextKey: key}); err != nil {
		t.Fatal(err)
	}
	fixture := reviewFixture(t)
	raw, _ := json.Marshal(fixture)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	if err = repo.Create(ctx, Analysis{ID: "review", DocumentID: "source", UserID: "owner", Status: StatusCompleted, Result: result}); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{Svc: &Service{Repo: repo, Store: store}, DocRepo: docs, Unlocks: fakeUnlocks{active: true, unlocked: map[string]bool{"owner|review": true}}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", "owner"); c.Next() })
	router.POST("/analyses/:id/reviewed-resume", handler.downloadReviewedResume)
	for _, tc := range []struct {
		body     string
		status   int
		original bool
	}{
		{`{"editIds":["failover"]}`, 200, false},
		{`{"editIds":[]}`, 200, true},
		{`{"editIds":["unknown"]}`, 422, false},
		{`{"editIds":["failover","failover"]}`, 422, false},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("POST", "/analyses/review/reviewed-resume", strings.NewReader(tc.body)))
		if response.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.body, response.Code, response.Body.String())
		}
		if response.Code == 200 {
			text := docxText(t, response.Body.Bytes())
			if strings.Contains(text, "significantly") != tc.original || !strings.Contains(text, "Education: Example University.") {
				t.Fatal(text)
			}
		}
	}
}

func TestV3ReferenceScenariosAcrossModes(t *testing.T) {
	var cases []struct {
		ID     string           `json:"id"`
		Source string           `json:"source"`
		Result AnalysisResultV3 `json:"result"`
	}
	if err := json.Unmarshal(loadFixture(t, "testdata/report_v3_cases.json"), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 6 {
		t.Fatalf("expected six scenarios, got %d", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			if err := tc.Result.AnalysisResultV2_4.Validate(); err != nil {
				t.Fatal(err)
			}
			if err := tc.Result.Report.Validate(tc.Source, tc.Result.JobRequirementProfile.TopPriorities); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(tc.Result)
			mode := ModeJobMatch
			if !tc.Result.Meta.JobDescriptionProvided {
				mode = ModeATS
			}
			normalized, err := normalizeAnalysisResult(raw, Analysis{PromptVersion: "v3", Mode: mode, Model: "gpt-6.1-sol"})
			if err != nil {
				t.Fatal(err)
			}
			data, err := RenderReviewedResume(tc.Source, normalized, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(docxText(t, data), "Education: Example University.") {
				t.Fatal("source coverage lost in export")
			}
		})
	}
}
