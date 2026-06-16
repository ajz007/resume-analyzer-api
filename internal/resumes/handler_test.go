package resumes_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"resume-backend/internal/bootstrap"
	"resume-backend/internal/shared/auth"
	"resume-backend/internal/shared/config"
	modelv1 "resume-backend/resume/modelv1"
)

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
	var body resumeBody
	decodeJSON(t, resp, &body)
	if body.ResumeID == "" {
		t.Fatal("expected resumeId")
	}
	if body.CurrentVersionID == "" {
		t.Fatal("expected currentVersionId")
	}
	if body.Title != "Backend Engineer Resume" {
		t.Fatalf("expected title, got %q", body.Title)
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

func TestResumesUpdateCreatesNewVersion(t *testing.T) {
	router := newTestRouter(t)
	ownerID := "google:12345"
	created := createResume(t, router, ownerID, "Backend Engineer Resume", validResume())

	updatedResume := validResume()
	updatedResume.Summary.Text = "Updated summary with 45% measurable impact."
	resp := performJSON(t, router, http.MethodPut, "/api/v1/resumes/"+created.ResumeID, ownerID, map[string]any{
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

	versionsResp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ResumeID+"/versions", ownerID, nil)
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
}

func TestResumesGetForbiddenForDifferentOwner(t *testing.T) {
	router := newTestRouter(t)
	created := createResume(t, router, uuid.NewString(), "Owner Resume", validResume())

	resp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ResumeID, uuid.NewString(), nil)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestResumesGetVersionsAndVersion(t *testing.T) {
	router := newTestRouter(t)
	ownerID := "google:12345"
	created := createResume(t, router, ownerID, "Backend Engineer Resume", validResume())

	versionsResp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ResumeID+"/versions", ownerID, nil)
	if versionsResp.Code != http.StatusOK {
		t.Fatalf("expected versions status 200, got %d: %s", versionsResp.Code, versionsResp.Body.String())
	}
	var versions []versionBody
	decodeJSON(t, versionsResp, &versions)
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}

	versionResp := performJSON(t, router, http.MethodGet, "/api/v1/resumes/"+created.ResumeID+"/versions/"+versions[0].ID, ownerID, nil)
	if versionResp.Code != http.StatusOK {
		t.Fatalf("expected version status 200, got %d: %s", versionResp.Code, versionResp.Body.String())
	}
	var version versionBody
	decodeJSON(t, versionResp, &version)
	if version.ID != versions[0].ID {
		t.Fatalf("expected version %s, got %s", versions[0].ID, version.ID)
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
	ResumeID          string                      `json:"resumeId"`
	Title             string                      `json:"title"`
	CurrentVersionID  string                      `json:"currentVersionId"`
	ReadinessWarnings []modelv1.ValidationWarning `json:"readinessWarnings"`
}

type versionBody struct {
	ID            string `json:"id"`
	VersionNumber int    `json:"versionNumber"`
}

func newTestRouter(t *testing.T) *gin.Engine {
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
	return app.Router
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

func decodeJSON(t *testing.T, resp *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, resp.Body.String())
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
