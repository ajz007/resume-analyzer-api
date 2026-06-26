package resumes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"resume-backend/internal/bootstrap"
	resumespkg "resume-backend/internal/resumes"
	"resume-backend/internal/shared/auth"
	"resume-backend/internal/shared/config"
	modelv1 "resume-backend/resume/modelv1"
)

type mockGenerationLLM struct {
	response string
	err      error
}

func (m mockGenerationLLM) Complete(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestResumesCreateSuccess(t *testing.T) {
	router := newTestRouter(t)
	ownerID := "google:12345"

	resp := performJSON(t, router, http.MethodPost, "/api/v1/resumes", ownerID, map[string]any{
		"title":  "Backend Engineer Resume",
		"resume": validResume(),
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	assertNoLegacyResumeAliases(t, resp)
	var body resumeBody
	decodeJSON(t, resp, &body)
	if body.ID == "" {
		t.Fatal("expected id")
	}
	if body.CurrentVersionID == "" {
		t.Fatal("expected currentVersionId")
	}
	if body.Status != resumespkg.StatusDraft {
		t.Fatalf("expected draft status, got %q", body.Status)
	}
	if body.Title != "Backend Engineer Resume" {
		t.Fatalf("expected title, got %q", body.Title)
	}
}

func TestResumesCreateGuestRejected(t *testing.T) {
	router := newTestRouter(t)

	resp := performGuestJSON(t, router, http.MethodPost, "/api/v1/resumes", "guest-123", map[string]any{
		"title":  "Guest Resume",
		"resume": validResume(),
	})

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesCreateWithoutAuthRejected(t *testing.T) {
	router := newTestRouter(t)

	resp := performWithoutAuthJSON(t, router, http.MethodPost, "/api/v1/resumes", map[string]any{
		"title":  "Anonymous Resume",
		"resume": validResume(),
	})

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesCreateStructuralValidationFailure(t *testing.T) {
	router := newTestRouter(t)
	resume := validResume()
	resume.SchemaVersion = "resume.v2"

	resp := performJSON(t, router, http.MethodPost, "/api/v1/resumes", uuid.NewString(), map[string]any{
		"title":  "Invalid Resume",
		"resume": resume,
	})

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesCreateRejectsLongTitle(t *testing.T) {
	router := newTestRouter(t)

	resp := performJSON(t, router, http.MethodPost, "/api/v1/resumes", uuid.NewString(), map[string]any{
		"title":  strings.Repeat("x", 161),
		"resume": validResume(),
	})

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesCreateInvalidBodyReturnsBindDetails(t *testing.T) {
	router := newTestRouter(t)

	resp := performJSON(t, router, http.MethodPost, "/api/v1/resumes", uuid.NewString(), map[string]any{
		"title": "Backend Engineer Resume",
		"resume": map[string]any{
			"schemaVersion": "resume.v1",
			"summary":       "",
		},
	})

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
	var body errorResponseBody
	decodeJSON(t, resp, &body)
	if body.Error.Message != "invalid request body" {
		t.Fatalf("expected invalid request body message, got %#v", body)
	}
	if len(body.Error.Details) == 0 || body.Error.Details[0].Field != "resume.summary" {
		t.Fatalf("expected bind details for resume.summary, got %#v", body.Error.Details)
	}
}

func TestResumesUpdateMissingTitleReturnsFieldDetails(t *testing.T) {
	router := newTestRouter(t)
	created := createResume(t, router, "google:12345", "Backend Engineer Resume", validResume())

	resp := performJSON(t, router, http.MethodPut, "/api/v1/resumes/"+created.ID, "google:12345", map[string]any{
		"resume": validResume(),
	})

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
	var body errorResponseBody
	decodeJSON(t, resp, &body)
	if body.Error.Message != "invalid resume request" {
		t.Fatalf("expected invalid resume request message, got %#v", body)
	}
	if len(body.Error.Details) == 0 || body.Error.Details[0].Field != "title" {
		t.Fatalf("expected title validation details, got %#v", body.Error.Details)
	}
}

func TestResumesIncompleteDraftCreatesWithWarnings(t *testing.T) {
	router := newTestRouter(t)

	resp := performJSON(t, router, http.MethodPost, "/api/v1/resumes", uuid.NewString(), map[string]any{
		"title":  "Draft Resume",
		"resume": modelv1.ResumeModel{SchemaVersion: modelv1.SchemaVersion},
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var body resumeBody
	decodeJSON(t, resp, &body)
	if len(body.ReadinessWarnings) == 0 {
		t.Fatal("expected readiness warnings")
	}
}

func TestResumesGenerateValidResponseCreatesResume(t *testing.T) {
	app := newTestApp(t)
	app.ResumesService.LLM = mockGenerationLLM{response: generationResponseJSON(t, modelv1.ResumeGenerationResponse{
		Resume: validResume(),
		RequiresUserInput: []modelv1.RequiresUserInput{{
			Field:    "basics.links.github.url",
			Message:  "GitHub URL was not provided.",
			Severity: "optional",
		}},
		Assumptions: []modelv1.Assumption{{
			Message: "Used the provided target role as resume target.",
		}},
		Warnings: []modelv1.ResponseWarning{{
			Message: "Measurable impact was missing for one bullet, so no metric was fabricated.",
		}},
	})}
	ownerID := "google:12345"

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", ownerID, generateRequestBody())

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	assertNoLegacyResumeAliases(t, resp)
	var body generateBody
	decodeJSON(t, resp, &body)
	if body.ID == "" {
		t.Fatal("expected id")
	}
	if body.CurrentVersionID == "" {
		t.Fatal("expected currentVersionId")
	}
	if len(body.RequiresUserInput) != 1 {
		t.Fatalf("expected requiresUserInput from LLM response, got %#v", body.RequiresUserInput)
	}
	if len(body.Warnings) != 1 {
		t.Fatalf("expected warnings from LLM response, got %#v", body.Warnings)
	}

	versionsResp := performJSON(t, app.Router, http.MethodGet, "/api/v1/resumes/"+body.ID+"/versions", ownerID, nil)
	if versionsResp.Code != http.StatusOK {
		t.Fatalf("expected versions status 200, got %d: %s", versionsResp.Code, versionsResp.Body.String())
	}
	var versions []versionBody
	decodeJSON(t, versionsResp, &versions)
	if len(versions) != 1 {
		t.Fatalf("expected one version, got %d", len(versions))
	}
	if versions[0].SourceType != resumespkg.SourceAIGenerated {
		t.Fatalf("expected source type %q, got %q", resumespkg.SourceAIGenerated, versions[0].SourceType)
	}
}

func TestResumesGenerateLogsAndHandlesValidSmallJD(t *testing.T) {
	app := newTestApp(t)
	app.ResumesService.LLM = mockGenerationLLM{response: generationResponseJSON(t, modelv1.ResumeGenerationResponse{
		Resume: validResume(),
	})}
	jobDescription := "Need Go APIs and PostgreSQL."

	resp, logs := performJSONWithLogs(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", map[string]any{
		"title":          "Backend Engineer Resume",
		"targetRole":     "Backend Engineer",
		"generationMode": resumespkg.GenerationModeFromJobDescription,
		"jobDescription": jobDescription,
		"experienceText": "Alex worked at Acme on Go APIs.",
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	for _, needle := range []string{
		`"msg":"resume.generate.request.start"`,
		`"msg":"resume.generate.request.end"`,
		`"msg":"resume.generate.llm.start"`,
		`"msg":"resume.generate.llm.finish"`,
		`"generation_mode":"from_job_description"`,
		`"job_description_length":28`,
	} {
		if !strings.Contains(logs, needle) {
			t.Fatalf("expected logs to contain %s, logs=%s", needle, logs)
		}
	}
	if strings.Contains(logs, jobDescription) {
		t.Fatalf("expected logs not to contain raw job description, logs=%s", logs)
	}
}

func TestResumesGenerateOversizedJobDescriptionReturnsValidationError(t *testing.T) {
	app := newTestApp(t)

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", map[string]any{
		"title":          "Backend Engineer Resume",
		"generationMode": resumespkg.GenerationModeFromJobDescription,
		"jobDescription": strings.Repeat("a", 30001),
	})

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
	var body errorResponseBody
	decodeJSON(t, resp, &body)
	if body.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error, got %#v", body.Error)
	}
	if len(body.Error.Details) == 0 || body.Error.Details[0].Field != "jobDescription" {
		t.Fatalf("expected jobDescription validation details, got %#v", body.Error.Details)
	}
}

func TestResumesGenerateTimeoutReturnsStructuredError(t *testing.T) {
	app := newTestApp(t)
	app.ResumesService.LLM = mockGenerationLLM{err: context.DeadlineExceeded}

	resp, logs := performJSONWithLogs(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", generateRequestBody())

	if resp.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected status 504, got %d: %s", resp.Code, resp.Body.String())
	}
	var body errorResponseBody
	decodeJSON(t, resp, &body)
	if body.Error.Code != "RESUME_GENERATION_TIMEOUT" {
		t.Fatalf("expected RESUME_GENERATION_TIMEOUT, got %#v", body.Error)
	}
	if !strings.Contains(logs, `"msg":"resume.generate.llm.timeout"`) {
		t.Fatalf("expected timeout log, got %s", logs)
	}
}

func TestResumesGenerateGuestRejected(t *testing.T) {
	app := newTestApp(t)

	resp := performGuestJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "guest-123", generateRequestBody())

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGenerateWithoutAuthRejected(t *testing.T) {
	app := newTestApp(t)

	resp := performWithoutAuthJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", generateRequestBody())

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGenerateFromNotesModeStillWorks(t *testing.T) {
	app := newTestApp(t)
	app.ResumesService.LLM = mockGenerationLLM{response: generationResponseJSON(t, modelv1.ResumeGenerationResponse{
		Resume: validResume(),
	})}
	req := generateRequestBody()
	req["generationMode"] = resumespkg.GenerationModeFromNotes

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var body generateBody
	decodeJSON(t, resp, &body)
	if body.ID == "" || body.CurrentVersionID == "" {
		t.Fatalf("expected generated resume ids, got %#v", body)
	}
}

func TestResumesGenerateFromJobDescriptionOnlyCreatesTemplateDraft(t *testing.T) {
	app := newTestApp(t)
	template := jdOnlyTemplateResume()
	app.ResumesService.LLM = mockGenerationLLM{response: generationResponseJSON(t, modelv1.ResumeGenerationResponse{
		Resume: template,
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", map[string]any{
		"title":          "Backend Engineer Template",
		"targetRole":     "Backend Engineer",
		"seniority":      "Senior",
		"generationMode": resumespkg.GenerationModeFromJobDescription,
		"jobDescription": "We need Go, PostgreSQL, APIs, Kubernetes, and observability experience.",
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var body generateBody
	decodeJSON(t, resp, &body)
	if body.Resume.Target.RoleTitle != "Backend Engineer" {
		t.Fatalf("expected target role, got %#v", body.Resume.Target)
	}
	if len(body.RequiresUserInput) == 0 {
		t.Fatal("expected requiresUserInput for JD-only template")
	}
	if !hasTestWarning(body.Warnings, jdTemplateWarningForTest) {
		t.Fatalf("expected JD template warning, got %#v", body.Warnings)
	}
	for _, exp := range body.Resume.Experience {
		if exp.Company != "" && exp.Company != "[Your Company]" {
			t.Fatalf("expected no invented company, got %q", exp.Company)
		}
		if exp.StartDate != "" || exp.EndDate != "" {
			t.Fatalf("expected no invented dates, got %q-%q", exp.StartDate, exp.EndDate)
		}
		for _, highlight := range exp.Highlights {
			if strings.Contains(highlight.Text, "35%") || strings.Contains(highlight.Text, "90%") {
				t.Fatalf("expected no invented metric in highlight %q", highlight.Text)
			}
		}
	}
}

func TestResumesGenerateFromJobDescriptionOnlyRejectsInventedFacts(t *testing.T) {
	app := newTestApp(t)
	invented := jdOnlyTemplateResume()
	invented.Experience[0].Company = "Acme"
	invented.Experience[0].StartDate = "2021-01"
	invented.Experience[0].Highlights[0].Text = "Reduced latency by 40%."
	app.ResumesService.LLM = mockGenerationLLM{response: generationResponseJSON(t, modelv1.ResumeGenerationResponse{
		Resume: invented,
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", map[string]any{
		"title":          "Backend Engineer Template",
		"targetRole":     "Backend Engineer",
		"generationMode": resumespkg.GenerationModeFromJobDescription,
		"jobDescription": "We need Go and PostgreSQL.",
	})

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGenerateFromJobDescriptionWithExperienceUsesProvidedExperienceOnly(t *testing.T) {
	app := newTestApp(t)
	resume := validResume()
	resume.Skills[0].Items = []modelv1.SkillItem{{Name: "Go", Source: "user_provided"}}
	app.ResumesService.LLM = mockGenerationLLM{response: generationResponseJSON(t, modelv1.ResumeGenerationResponse{
		Resume: resume,
		Warnings: []modelv1.ResponseWarning{{
			Message: "Kubernetes was requested in the job description but was not present in the user's experience notes.",
		}},
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", map[string]any{
		"title":          "Backend Engineer Resume",
		"targetRole":     "Backend Engineer",
		"generationMode": resumespkg.GenerationModeFromJobDescription,
		"jobDescription": "We need Go, PostgreSQL, APIs, and Kubernetes.",
		"experienceText": "Alex worked at Acme as a Backend Engineer from 2021-01 to 2024-12 and used Go.",
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var body generateBody
	decodeJSON(t, resp, &body)
	if body.Resume.Experience[0].Company != "Acme" {
		t.Fatalf("expected provided company, got %q", body.Resume.Experience[0].Company)
	}
	for _, category := range body.Resume.Skills {
		for _, item := range category.Items {
			if item.Name == "Kubernetes" {
				t.Fatal("expected unsupported JD skill not to be silently added")
			}
		}
	}
	if len(body.Warnings) == 0 {
		t.Fatal("expected warning for unsupported JD requirement")
	}
}

func TestResumesGenerateBlankCreatesMinimalDraftWithoutLLM(t *testing.T) {
	app := newTestApp(t)
	app.ResumesService.LLM = nil

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", map[string]any{
		"title":          "Blank Resume",
		"targetRole":     "Backend Engineer",
		"seniority":      "Senior",
		"generationMode": resumespkg.GenerationModeBlank,
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var body generateBody
	decodeJSON(t, resp, &body)
	if body.Resume.SchemaVersion != modelv1.SchemaVersion {
		t.Fatalf("expected schema version %q, got %q", modelv1.SchemaVersion, body.Resume.SchemaVersion)
	}
	if body.Resume.Target.RoleTitle != "Backend Engineer" {
		t.Fatalf("expected target role to be preserved, got %#v", body.Resume.Target)
	}
	if len(body.Resume.Experience) != 0 {
		t.Fatalf("expected blank experience, got %#v", body.Resume.Experience)
	}
	if len(body.ReadinessWarnings) == 0 {
		t.Fatal("expected readiness warnings for blank draft")
	}
}

func TestResumesGenerateInvalidGenerationModeRejected(t *testing.T) {
	app := newTestApp(t)

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", map[string]any{
		"title":          "Invalid Mode",
		"generationMode": "from_linkedin",
		"experienceText": "Some notes.",
	})

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGenerateInvalidAIResponseIsRejected(t *testing.T) {
	app := newTestApp(t)
	invalid := validResume()
	invalid.SchemaVersion = "resume.v2"
	app.ResumesService.LLM = mockGenerationLLM{response: generationResponseJSON(t, modelv1.ResumeGenerationResponse{Resume: invalid})}
	ownerID := "google:12345"

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", ownerID, generateRequestBody())

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", resp.Code, resp.Body.String())
	}
	var body errorResponseBody
	decodeJSON(t, resp, &body)
	if body.Error.Code != "RESUME_GENERATION_INVALID_OUTPUT" {
		t.Fatalf("expected RESUME_GENERATION_INVALID_OUTPUT, got %#v", body.Error)
	}
	listResp := performJSON(t, app.Router, http.MethodGet, "/api/v1/resumes", ownerID, nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var list []map[string]any
	decodeJSON(t, listResp, &list)
	if len(list) != 0 {
		t.Fatalf("expected invalid AI response not to save a resume, got %d", len(list))
	}
}

func TestResumesGenerateMissingIDsAreSanitized(t *testing.T) {
	app := newTestApp(t)
	resume := validResume()
	resume.Experience[0].ID = ""
	resume.Experience[0].Highlights[0].ID = ""
	app.ResumesService.LLM = mockGenerationLLM{response: generationResponseJSON(t, modelv1.ResumeGenerationResponse{Resume: resume})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", generateRequestBody())

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	assertNoLegacyResumeAliases(t, resp)
	var body generateBody
	decodeJSON(t, resp, &body)
	if body.Resume.Experience[0].ID == "" {
		t.Fatal("expected sanitized experience id")
	}
	if body.Resume.Experience[0].Highlights[0].ID == "" {
		t.Fatal("expected sanitized highlight id")
	}
}

func TestResumesGenerateRejectsMalformedAIResponse(t *testing.T) {
	app := newTestApp(t)
	app.ResumesService.LLM = mockGenerationLLM{response: `{"resume":`}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", generateRequestBody())

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGenerateRejectsWrappedAIResponse(t *testing.T) {
	app := newTestApp(t)
	app.ResumesService.LLM = mockGenerationLLM{response: "```json\n" + generationResponseJSON(t, modelv1.ResumeGenerationResponse{Resume: validResume()}) + "\n```"}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", generateRequestBody())

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGenerateRejectsMissingWrapperKeys(t *testing.T) {
	app := newTestApp(t)
	data, err := json.Marshal(map[string]any{
		"resume": validResume(),
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	app.ResumesService.LLM = mockGenerationLLM{response: string(data)}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", generateRequestBody())

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGenerateIncompleteStructurallyValidResumeSavesWithReadinessWarnings(t *testing.T) {
	app := newTestApp(t)
	app.ResumesService.LLM = mockGenerationLLM{response: generationResponseJSON(t, modelv1.ResumeGenerationResponse{
		Resume: modelv1.ResumeModel{SchemaVersion: modelv1.SchemaVersion},
		RequiresUserInput: []modelv1.RequiresUserInput{{
			Field:    "basics.fullName",
			Message:  "Full name was not provided.",
			Severity: "required",
		}},
		Warnings: []modelv1.ResponseWarning{{
			Message: "Resume is incomplete because the source notes were sparse.",
		}},
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/generate", "google:12345", generateRequestBody())

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	assertNoLegacyResumeAliases(t, resp)
	var body generateBody
	decodeJSON(t, resp, &body)
	if body.ID == "" || body.CurrentVersionID == "" {
		t.Fatalf("expected saved resume ids, got %#v", body)
	}
	if len(body.ReadinessWarnings) == 0 {
		t.Fatal("expected readiness warnings")
	}
	if len(body.RequiresUserInput) != 1 {
		t.Fatalf("expected requiresUserInput, got %#v", body.RequiresUserInput)
	}
	if len(body.Warnings) != 1 {
		t.Fatalf("expected anti-fabrication warning, got %#v", body.Warnings)
	}
}

func TestResumesTailorValidResponseCreatesTailoredResume(t *testing.T) {
	app := newTestApp(t)
	ownerID := "google:12345"
	created := createResume(t, app.Router, ownerID, "Backend Engineer Resume", validResume())
	tailored := validResume()
	tailored.Summary.Text = "Backend engineer focused on Go APIs and PostgreSQL systems for platform teams."
	app.ResumesService.LLM = mockGenerationLLM{response: tailoringResponseJSON(t, modelv1.ResumeTailoringResponse{
		TailoredResume: tailored,
		Changes: []modelv1.TailoringChange{{
			Section:    "summary",
			ItemID:     "summary",
			ChangeType: "rewrite",
			Before:     "Backend engineer with measurable platform delivery impact.",
			After:      tailored.Summary.Text,
			Reason:     "Matched the summary to backend platform requirements already supported by the resume.",
			Risk:       "safe",
		}},
		MissingRequirements: []modelv1.MissingRequirement{{
			Requirement:    "Kubernetes",
			Recommendation: "Add only if the candidate has real Kubernetes experience.",
		}},
		Warnings: []modelv1.ResponseWarning{{
			Message: "No unsupported tools were added.",
		}},
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", ownerID, tailorRequestBody())

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	assertNoLegacyTailorAliases(t, resp)
	var body tailorBody
	decodeJSON(t, resp, &body)
	if body.SourceResumeID != created.ID {
		t.Fatalf("expected source resume %s, got %s", created.ID, body.SourceResumeID)
	}
	if body.SourceVersionID != created.CurrentVersionID {
		t.Fatalf("expected source version %s, got %s", created.CurrentVersionID, body.SourceVersionID)
	}
	if body.ID == "" || body.ID == created.ID {
		t.Fatalf("expected new tailored resume id, got %q", body.ID)
	}
	if body.CurrentVersionID == "" {
		t.Fatal("expected currentVersionId")
	}
	if body.Resume.Summary.Text != tailored.Summary.Text {
		t.Fatalf("expected public resume payload, got %#v", body.Resume.Summary)
	}
	if len(body.Changes) != 1 {
		t.Fatalf("expected one change, got %#v", body.Changes)
	}
	if len(body.MissingRequirements) != 1 || body.MissingRequirements[0].Requirement != "Kubernetes" {
		t.Fatalf("expected missing Kubernetes requirement, got %#v", body.MissingRequirements)
	}
	if len(body.Warnings) != 1 {
		t.Fatalf("expected warning, got %#v", body.Warnings)
	}
	sourceResp := performJSON(t, app.Router, http.MethodGet, "/api/v1/resumes/"+created.ID, ownerID, nil)
	if sourceResp.Code != http.StatusOK {
		t.Fatalf("expected source get status 200, got %d: %s", sourceResp.Code, sourceResp.Body.String())
	}
	var sourceBody resumeBody
	decodeJSON(t, sourceResp, &sourceBody)
	if sourceBody.CurrentVersionID != created.CurrentVersionID {
		t.Fatalf("expected source version unchanged, got %s", sourceBody.CurrentVersionID)
	}
	if sourceBody.Resume.Summary.Text == tailored.Summary.Text {
		t.Fatal("expected source resume summary not to be overwritten by tailored resume")
	}

	versionsResp := performJSON(t, app.Router, http.MethodGet, "/api/v1/resumes/"+body.ID+"/versions", ownerID, nil)
	if versionsResp.Code != http.StatusOK {
		t.Fatalf("expected versions status 200, got %d: %s", versionsResp.Code, versionsResp.Body.String())
	}
	var versions []versionBody
	decodeJSON(t, versionsResp, &versions)
	if len(versions) != 1 {
		t.Fatalf("expected one tailored version, got %d", len(versions))
	}
	if versions[0].SourceType != resumespkg.SourceAITailored {
		t.Fatalf("expected source type %q, got %q", resumespkg.SourceAITailored, versions[0].SourceType)
	}
	if versions[0].SourceVersionID != created.CurrentVersionID {
		t.Fatalf("expected tailored version sourceVersionId %s, got %s", created.CurrentVersionID, versions[0].SourceVersionID)
	}

	listResp := performJSON(t, app.Router, http.MethodGet, "/api/v1/resumes", ownerID, nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var list []map[string]any
	decodeJSON(t, listResp, &list)
	var tailoredItem map[string]any
	for _, item := range list {
		if item["id"] == body.ID {
			tailoredItem = item
			break
		}
	}
	if tailoredItem == nil {
		t.Fatalf("expected tailored resume in list, got %#v", list)
	}
	if tailoredItem["originType"] != resumespkg.OriginAITailored {
		t.Fatalf("expected tailored origin type, got %#v", tailoredItem["originType"])
	}
	if tailoredItem["sourceResumeId"] != created.ID {
		t.Fatalf("expected sourceResumeId %s, got %#v", created.ID, tailoredItem["sourceResumeId"])
	}
}

func TestResumesTailorInvalidEmbeddedResumeRejected(t *testing.T) {
	app := newTestApp(t)
	ownerID := "google:12345"
	created := createResume(t, app.Router, ownerID, "Backend Engineer Resume", validResume())
	invalid := validResume()
	invalid.SchemaVersion = "resume.v2"
	app.ResumesService.LLM = mockGenerationLLM{response: tailoringResponseJSON(t, modelv1.ResumeTailoringResponse{
		TailoredResume: invalid,
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", ownerID, tailorRequestBody())

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesTailorInvalidChangeTypeRejected(t *testing.T) {
	app := newTestApp(t)
	ownerID := "google:12345"
	created := createResume(t, app.Router, ownerID, "Backend Engineer Resume", validResume())
	app.ResumesService.LLM = mockGenerationLLM{response: tailoringResponseJSON(t, modelv1.ResumeTailoringResponse{
		TailoredResume: validResume(),
		Changes: []modelv1.TailoringChange{{
			Section:    "summary",
			ItemID:     "summary",
			ChangeType: "transform",
			Before:     "old",
			After:      "new",
			Reason:     "invalid change type",
			Risk:       "safe",
		}},
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", ownerID, tailorRequestBody())

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesTailorInvalidRiskRejected(t *testing.T) {
	app := newTestApp(t)
	ownerID := "google:12345"
	created := createResume(t, app.Router, ownerID, "Backend Engineer Resume", validResume())
	app.ResumesService.LLM = mockGenerationLLM{response: tailoringResponseJSON(t, modelv1.ResumeTailoringResponse{
		TailoredResume: validResume(),
		Changes: []modelv1.TailoringChange{{
			Section:    "summary",
			ItemID:     "summary",
			ChangeType: "rewrite",
			Before:     "old",
			After:      "new",
			Reason:     "invalid risk",
			Risk:       "low",
		}},
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", ownerID, tailorRequestBody())

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesTailorUnsafeChangeRejected(t *testing.T) {
	app := newTestApp(t)
	ownerID := "google:12345"
	created := createResume(t, app.Router, ownerID, "Backend Engineer Resume", validResume())
	response := validTailoringResponseForTest()
	response.Changes[0].Risk = "unsafe"
	app.ResumesService.LLM = mockGenerationLLM{response: tailoringResponseJSON(t, response)}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", ownerID, tailorRequestBody())

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesTailorUnsupportedSkillRejected(t *testing.T) {
	app := newTestApp(t)
	ownerID := "google:12345"
	created := createResume(t, app.Router, ownerID, "Backend Engineer Resume", validResume())
	response := validTailoringResponseForTest()
	response.TailoredResume.Skills[0].Items = append(response.TailoredResume.Skills[0].Items, modelv1.SkillItem{
		Name:   "Kubernetes",
		Source: "ai_tailored",
	})
	app.ResumesService.LLM = mockGenerationLLM{response: tailoringResponseJSON(t, response)}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", ownerID, tailorRequestBody())

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesTailorUnsupportedMetricRejected(t *testing.T) {
	app := newTestApp(t)
	ownerID := "google:12345"
	created := createResume(t, app.Router, ownerID, "Backend Engineer Resume", validResume())
	response := validTailoringResponseForTest()
	response.TailoredResume.Experience[0].Highlights[0].Text = "Reduced API latency by 90% through query and cache improvements."
	app.ResumesService.LLM = mockGenerationLLM{response: tailoringResponseJSON(t, response)}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", ownerID, tailorRequestBody())

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesTailorSavesTailoredResume(t *testing.T) {
	app := newTestApp(t)
	ownerID := "google:12345"
	created := createResume(t, app.Router, ownerID, "Backend Engineer Resume", validResume())
	app.ResumesService.LLM = mockGenerationLLM{response: tailoringResponseJSON(t, modelv1.ResumeTailoringResponse{
		TailoredResume: validResume(),
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", ownerID, tailorRequestBody())

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var body tailorBody
	decodeJSON(t, resp, &body)
	getResp := performJSON(t, app.Router, http.MethodGet, "/api/v1/resumes/"+body.ID, ownerID, nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected tailored resume get status 200, got %d: %s", getResp.Code, getResp.Body.String())
	}
}

func TestResumesTailorForbiddenForDifferentOwner(t *testing.T) {
	app := newTestApp(t)
	created := createResume(t, app.Router, "google:owner", "Backend Engineer Resume", validResume())
	app.ResumesService.LLM = mockGenerationLLM{response: tailoringResponseJSON(t, modelv1.ResumeTailoringResponse{
		TailoredResume: validResume(),
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", "google:other", tailorRequestBody())

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesTailorGuestRejected(t *testing.T) {
	app := newTestApp(t)
	created := createResume(t, app.Router, "google:owner", "Backend Engineer Resume", validResume())

	resp := performGuestJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", "guest-123", tailorRequestBody())

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesTailorWithoutAuthRejected(t *testing.T) {
	app := newTestApp(t)
	created := createResume(t, app.Router, "google:owner", "Backend Engineer Resume", validResume())

	resp := performWithoutAuthJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", tailorRequestBody())

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesTailorReturnsReadinessWarnings(t *testing.T) {
	app := newTestApp(t)
	ownerID := "google:12345"
	created := createResume(t, app.Router, ownerID, "Backend Engineer Resume", validResume())
	app.ResumesService.LLM = mockGenerationLLM{response: tailoringResponseJSON(t, modelv1.ResumeTailoringResponse{
		TailoredResume: modelv1.ResumeModel{SchemaVersion: modelv1.SchemaVersion},
	})}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/tailor", ownerID, tailorRequestBody())

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var body tailorBody
	decodeJSON(t, resp, &body)
	if len(body.ReadinessWarnings) == 0 {
		t.Fatal("expected readiness warnings")
	}
}

func TestResumesUpdateCreatesNewVersion(t *testing.T) {
	router := newTestRouter(t)
	ownerID := "google:12345"
	created := createResume(t, router, ownerID, "Backend Engineer Resume", validResume())

	updatedResume := validResume()
	updatedResume.Summary.Text = "Updated summary with 45% measurable impact."
	resp := performJSON(t, router, http.MethodPut, "/api/v1/resumes/"+created.ID, ownerID, map[string]any{
		"title":  "Backend Engineer Resume",
		"resume": updatedResume,
		"changeSummary": map[string]any{
			"message": "Updated summary and skills",
		},
	})

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var updated resumeBody
	decodeJSON(t, resp, &updated)
	if updated.CurrentVersionID == created.CurrentVersionID {
		t.Fatal("expected a new currentVersionId")
	}

	versionsResp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ID+"/versions", ownerID, nil)
	if versionsResp.Code != http.StatusOK {
		t.Fatalf("expected versions status 200, got %d: %s", versionsResp.Code, versionsResp.Body.String())
	}
	var versions []versionBody
	decodeJSON(t, versionsResp, &versions)
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].VersionNumber != 2 {
		t.Fatalf("expected latest version number 2, got %d", versions[0].VersionNumber)
	}
}

func TestResumesListOnlyReturnsOwnerResumes(t *testing.T) {
	router := newTestRouter(t)
	ownerID := "google:12345"
	otherOwnerID := "google:67890"
	createResume(t, router, ownerID, "Owner Resume", validResume())
	createResume(t, router, otherOwnerID, "Other Resume", validResume())

	resp := performJSON(t, router, http.MethodGet, "/api/v1/resumes", ownerID, nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var list []map[string]any
	decodeJSON(t, resp, &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 owner resume, got %d", len(list))
	}
	if list[0]["title"] != "Owner Resume" {
		t.Fatalf("expected owner resume, got %q", list[0]["title"])
	}
	if _, ok := list[0]["resume"]; ok {
		t.Fatal("expected list response to omit full resume payload")
	}
	if _, ok := list[0]["currentVersionId"]; ok {
		t.Fatal("expected list response to omit currentVersionId")
	}
	if list[0]["id"] == "" {
		t.Fatalf("expected id in list item, got %#v", list[0])
	}
	if list[0]["originType"] != resumespkg.OriginManual {
		t.Fatalf("expected manual origin type, got %#v", list[0]["originType"])
	}
}

func TestResumesListGuestRejected(t *testing.T) {
	router := newTestRouter(t)

	resp := performGuestJSON(t, router, http.MethodGet, "/api/v1/resumes", "guest-123", nil)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesListWithoutAuthRejected(t *testing.T) {
	router := newTestRouter(t)

	resp := performWithoutAuthJSON(t, router, http.MethodGet, "/api/v1/resumes", nil)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGetForbiddenForDifferentOwner(t *testing.T) {
	router := newTestRouter(t)
	created := createResume(t, router, uuid.NewString(), "Owner Resume", validResume())

	resp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ID, uuid.NewString(), nil)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesUpdateForbiddenForDifferentOwner(t *testing.T) {
	router := newTestRouter(t)
	created := createResume(t, router, "google:owner", "Owner Resume", validResume())

	resp := performJSON(t, router, http.MethodPut, "/api/v1/resumes/"+created.ID, "google:other", map[string]any{
		"title":  "Other Update",
		"resume": validResume(),
	})

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGetVersionsAndVersion(t *testing.T) {
	router := newTestRouter(t)
	ownerID := "google:12345"
	created := createResume(t, router, ownerID, "Backend Engineer Resume", validResume())

	versionsResp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ID+"/versions", ownerID, nil)
	if versionsResp.Code != http.StatusOK {
		t.Fatalf("expected versions status 200, got %d: %s", versionsResp.Code, versionsResp.Body.String())
	}
	var versions []versionBody
	decodeJSON(t, versionsResp, &versions)
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}

	versionResp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ID+"/versions/"+versions[0].ID, ownerID, nil)
	if versionResp.Code != http.StatusOK {
		t.Fatalf("expected version status 200, got %d: %s", versionResp.Code, versionResp.Body.String())
	}
	var version versionBody
	decodeJSON(t, versionResp, &version)
	if version.ID != versions[0].ID {
		t.Fatalf("expected version %s, got %s", versions[0].ID, version.ID)
	}
}

func TestResumesVersionsForbiddenForDifferentOwner(t *testing.T) {
	router := newTestRouter(t)
	created := createResume(t, router, "google:owner", "Owner Resume", validResume())

	resp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ID+"/versions", "google:other", nil)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGetVersionForbiddenForDifferentOwner(t *testing.T) {
	router := newTestRouter(t)
	created := createResume(t, router, "google:owner", "Owner Resume", validResume())
	versionsResp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ID+"/versions", "google:owner", nil)
	if versionsResp.Code != http.StatusOK {
		t.Fatalf("expected versions status 200, got %d: %s", versionsResp.Code, versionsResp.Body.String())
	}
	var versions []versionBody
	decodeJSON(t, versionsResp, &versions)
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}

	resp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ID+"/versions/"+versions[0].ID, "google:other", nil)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesExportDOCXSuccess(t *testing.T) {
	router := newTestRouter(t)
	ownerID := "google:12345"
	created := createResume(t, router, ownerID, "Backend Engineer Resume", validResume())

	resp := performJSON(t, router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/export/docx", ownerID, nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if ct := resp.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("unexpected content type: %s", ct)
	}
	if cd := resp.Header().Get("Content-Disposition"); cd != `attachment; filename="backend_engineer_resume.docx"` {
		t.Fatalf("unexpected content disposition: %s", cd)
	}
	if resp.Body.Len() == 0 {
		t.Fatal("expected docx body")
	}
}

func TestResumesExportDOCXForbiddenForDifferentOwner(t *testing.T) {
	router := newTestRouter(t)
	created := createResume(t, router, "google:owner", "Owner Resume", validResume())

	resp := performJSON(t, router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/export/docx", "google:other", nil)

	if resp.Code != http.StatusForbidden {
		// Existing resume service convention is 403 for cross-owner access.
		t.Fatalf("expected status 403, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesExportDOCXGuestRejected(t *testing.T) {
	router := newTestRouter(t)
	created := createResume(t, router, "google:owner", "Owner Resume", validResume())

	resp := performGuestJSON(t, router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/export/docx", "guest-123", nil)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesExportDOCXWithoutAuthRejected(t *testing.T) {
	router := newTestRouter(t)
	created := createResume(t, router, "google:owner", "Owner Resume", validResume())

	resp := performWithoutAuthJSON(t, router, http.MethodPost, "/api/v1/resumes/"+created.ID+"/export/docx", nil)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesExportDOCXStructuralValidationFailure(t *testing.T) {
	app := newTestApp(t)
	ownerID := "google:12345"
	resumeID := uuid.NewString()
	versionID := uuid.NewString()
	now := time.Now().UTC()
	invalid := modelv1.ResumeModel{SchemaVersion: "resume.v2"}

	_, err := app.ResumesRepo.Create(context.Background(), resumespkg.Resume{
		ID:               resumeID,
		OwnerID:          ownerID,
		Title:            "Invalid Resume",
		Status:           resumespkg.StatusDraft,
		CurrentVersionID: versionID,
		CurrentResume:    invalid,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, resumespkg.ResumeVersion{
		ID:            versionID,
		ResumeID:      resumeID,
		VersionNumber: 1,
		SourceType:    resumespkg.SourceManual,
		Resume:        invalid,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("seed invalid resume: %v", err)
	}

	resp := performJSON(t, app.Router, http.MethodPost, "/api/v1/resumes/"+resumeID+"/export/docx", ownerID, nil)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if cd := resp.Header().Get("Content-Disposition"); cd != "" {
		t.Fatalf("expected no download header, got %s", cd)
	}
}

func TestResumesInvalidResumeIDHandling(t *testing.T) {
	router := newTestRouter(t)

	resp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/not-a-uuid", uuid.NewString(), nil)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

type resumeBody struct {
	ID                string                      `json:"id"`
	Title             string                      `json:"title"`
	Status            string                      `json:"status"`
	CurrentVersionID  string                      `json:"currentVersionId"`
	Resume            modelv1.ResumeModel         `json:"resume"`
	ReadinessWarnings []modelv1.ValidationWarning `json:"readinessWarnings"`
}

type errorResponseBody struct {
	Error struct {
		Code    string                    `json:"code"`
		Message string                    `json:"message"`
		Details []modelv1.ValidationError `json:"details"`
	} `json:"error"`
}

type generateBody struct {
	ID                string                      `json:"id"`
	Title             string                      `json:"title"`
	Status            string                      `json:"status"`
	CurrentVersionID  string                      `json:"currentVersionId"`
	Resume            modelv1.ResumeModel         `json:"resume"`
	RequiresUserInput []modelv1.RequiresUserInput `json:"requiresUserInput"`
	Assumptions       []modelv1.Assumption        `json:"assumptions"`
	Warnings          []modelv1.ResponseWarning   `json:"warnings"`
	ReadinessWarnings []modelv1.ValidationWarning `json:"readinessWarnings"`
}

type tailorBody struct {
	SourceResumeID      string                       `json:"sourceResumeId"`
	SourceVersionID     string                       `json:"sourceVersionId"`
	ID                  string                       `json:"id"`
	Title               string                       `json:"title"`
	Status              string                       `json:"status"`
	CurrentVersionID    string                       `json:"currentVersionId"`
	Resume              modelv1.ResumeModel          `json:"resume"`
	Changes             []modelv1.TailoringChange    `json:"changes"`
	MissingRequirements []modelv1.MissingRequirement `json:"missingRequirements"`
	Warnings            []modelv1.ResponseWarning    `json:"warnings"`
	ReadinessWarnings   []modelv1.ValidationWarning  `json:"readinessWarnings"`
}

type versionBody struct {
	ID              string `json:"id"`
	VersionNumber   int    `json:"versionNumber"`
	SourceType      string `json:"sourceType"`
	SourceVersionID string `json:"sourceVersionId"`
}

func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	return newTestApp(t).Router
}

func newTestApp(t *testing.T) *bootstrap.App {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.Config{
		Port:            "0",
		CORSAllowOrigin: []string{"http://localhost:5173"},
		LocalStoreDir:   t.TempDir(),
		Env:             "dev",
		ObjectStoreType: "local",
	}
	app, err := bootstrap.Build(cfg)
	if err != nil {
		t.Fatalf("bootstrap build: %v", err)
	}
	return app
}

func createResume(t *testing.T, router *gin.Engine, ownerID, title string, resume modelv1.ResumeModel) resumeBody {
	t.Helper()
	resp := performJSON(t, router, http.MethodPost, "/api/v1/resumes", ownerID, map[string]any{
		"title":  title,
		"resume": resume,
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var body resumeBody
	decodeJSON(t, resp, &body)
	return body
}

func performJSON(t *testing.T, router *gin.Engine, method, path, ownerID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	token, err := auth.SignJWT(auth.Claims{Sub: ownerID})
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func performJSONWithLogs(t *testing.T, router *gin.Engine, method, path, ownerID string, body any) (*httptest.ResponseRecorder, string) {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
	}()

	resp := performJSON(t, router, method, path, ownerID, body)

	_ = w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read log output: %v", err)
	}
	return resp, buf.String()
}

func performGuestJSON(t *testing.T, router *gin.Engine, method, path, guestID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Guest-Id", guestID)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func performWithoutAuthJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func decodeJSON(t *testing.T, resp *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, resp.Body.String())
	}
}

func assertNoLegacyResumeAliases(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response map: %v", err)
	}
	for _, key := range []string{"resumeId", "versionId", "model", "data", "resumeModel", "tailoredResume"} {
		if _, ok := body[key]; ok {
			t.Fatalf("unexpected legacy alias %q in response %#v", key, body)
		}
	}
	if _, ok := body["id"]; !ok {
		t.Fatalf("expected id in response %#v", body)
	}
	if _, ok := body["resume"]; !ok {
		t.Fatalf("expected resume in response %#v", body)
	}
}

func assertNoLegacyTailorAliases(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	assertNoLegacyResumeAliases(t, resp)
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response map: %v", err)
	}
	for _, key := range []string{"tailoredResumeId"} {
		if _, ok := body[key]; ok {
			t.Fatalf("unexpected legacy tailoring alias %q in response %#v", key, body)
		}
	}
	if _, ok := body["sourceVersionId"]; !ok {
		t.Fatalf("expected sourceVersionId in response %#v", body)
	}
}

func generationResponseJSON(t *testing.T, response modelv1.ResumeGenerationResponse) string {
	t.Helper()
	if response.RequiresUserInput == nil {
		response.RequiresUserInput = []modelv1.RequiresUserInput{}
	}
	if response.Assumptions == nil {
		response.Assumptions = []modelv1.Assumption{}
	}
	if response.Warnings == nil {
		response.Warnings = []modelv1.ResponseWarning{}
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal generation response: %v", err)
	}
	return string(data)
}

func tailoringResponseJSON(t *testing.T, response modelv1.ResumeTailoringResponse) string {
	t.Helper()
	if response.Changes == nil {
		response.Changes = []modelv1.TailoringChange{}
	}
	if response.MissingRequirements == nil {
		response.MissingRequirements = []modelv1.MissingRequirement{}
	}
	if response.Warnings == nil {
		response.Warnings = []modelv1.ResponseWarning{}
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal tailoring response: %v", err)
	}
	return string(data)
}

func generateRequestBody() map[string]any {
	return map[string]any{
		"title":                  "Backend Engineer Resume",
		"targetRole":             "Backend Engineer",
		"seniority":              "Senior",
		"experienceText":         "Alex worked at Acme as a Backend Engineer from 2021-01 to 2024-12.",
		"skillsText":             "Go, PostgreSQL, APIs",
		"educationText":          "",
		"additionalInstructions": "Keep it ATS friendly.",
	}
}

func tailorRequestBody() map[string]any {
	return map[string]any{
		"jobDescription":         "We need a backend engineer with Go, PostgreSQL, APIs, and Kubernetes.",
		"targetRole":             "Backend Engineer",
		"additionalInstructions": "Keep it truthful and ATS friendly.",
	}
}

func validTailoringResponseForTest() modelv1.ResumeTailoringResponse {
	resume := validResume()
	resume.Summary.Text = "Backend engineer focused on Go APIs and reliable platform delivery."
	return modelv1.ResumeTailoringResponse{
		TailoredResume: resume,
		Changes: []modelv1.TailoringChange{{
			Section:    "summary",
			ItemID:     "summary",
			ChangeType: "rewrite",
			Before:     "Backend engineer with measurable platform delivery impact.",
			After:      resume.Summary.Text,
			Reason:     "Aligned summary with job description language.",
			Risk:       "safe",
		}},
		MissingRequirements: []modelv1.MissingRequirement{{
			Requirement:    "Kubernetes",
			Recommendation: "Ask the user whether they have Kubernetes experience before adding it.",
		}},
		Warnings: []modelv1.ResponseWarning{{
			Message: "Unsupported JD requirements were not added to the resume.",
		}},
	}
}

const jdTemplateWarningForTest = "This is a role-targeted template generated from the job description, not a complete resume. Add your real experience before applying."

func hasTestWarning(warnings []modelv1.ResponseWarning, message string) bool {
	for _, warning := range warnings {
		if warning.Message == message {
			return true
		}
	}
	return false
}

func jdOnlyTemplateResume() modelv1.ResumeModel {
	return modelv1.ResumeModel{
		SchemaVersion: modelv1.SchemaVersion,
		Target: modelv1.Target{
			RoleTitle: "Backend Engineer",
			Seniority: "Senior",
		},
		Summary: modelv1.Summary{Text: "Role-targeted backend engineering template. Replace placeholders with your real experience before applying."},
		Experience: []modelv1.Experience{{
			ID:        "exp-template-1",
			Company:   "[Your Company]",
			Title:     "[Your Role]",
			StartDate: "",
			EndDate:   "",
			Highlights: []modelv1.Highlight{{
				ID:     "exp-template-1-highlight-1",
				Text:   "Describe a backend API, database, or reliability contribution supported by your real work.",
				Source: "ai_generated",
			}},
		}},
		Projects:       []modelv1.Project{},
		Skills:         []modelv1.SkillCategory{},
		Education:      []modelv1.Education{},
		Certifications: []modelv1.Certification{},
		Achievements:   []modelv1.Achievement{},
		CustomSections: []modelv1.CustomSection{},
		SectionOrder:   []string{"summary", "experience"},
	}
}

func validResume() modelv1.ResumeModel {
	years := 6
	return modelv1.ResumeModel{
		SchemaVersion: modelv1.SchemaVersion,
		Basics: modelv1.Basics{
			FullName: "Alex Rivera",
			Headline: "Backend Engineer",
			Email:    "alex@example.com",
		},
		Summary: modelv1.Summary{Text: "Backend engineer with measurable platform delivery impact."},
		Skills: []modelv1.SkillCategory{{
			Category: "Backend",
			Items: []modelv1.SkillItem{{
				Name:   "Go",
				Level:  "advanced",
				Years:  &years,
				Source: "user_provided",
			}},
		}},
		Experience: []modelv1.Experience{{
			ID:             "exp-1",
			Company:        "Acme",
			Title:          "Backend Engineer",
			EmploymentType: "full_time",
			StartDate:      "2021-01",
			EndDate:        "2024-12",
			Highlights: []modelv1.Highlight{{
				ID:     "exp-1-highlight-1",
				Text:   "Reduced API latency by 35% through query and cache improvements.",
				Source: "user_provided",
			}},
		}},
		SectionOrder: []string{"summary", "skills", "experience"},
	}
}
