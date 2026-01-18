package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"resume-backend/resume/contract"
)

type mockApplyLLM struct {
	response string
}

func (m *mockApplyLLM) Complete(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return m.response, nil
}

func TestExecuteApplyRewritesAndDraftStatus(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	root := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	llmResponse := `{"header":{"name":"Test User","title":"","email":"","phone":"","location":"","links":[]},` +
		`"summary":["Nationality: India","Experienced developer."],` +
		`"skills":{"languages":[],"frameworks":[],"databases":[],"cloudDevOps":[],"observability":[],"tools":[]},` +
		`"experience":[{"id":"exp_1","company":"Acme","role":"Dev","location":"","start":"2020-01","end":"Present","highlights":["Old bullet"]}],` +
		`"projects":[],"education":[],"achievements":[],"certifications":[]}`

	prevClient := Client
	Client = &mockApplyLLM{response: llmResponse}
	defer func() {
		Client = prevClient
	}()

	analysis := AnalysisResultV2_3{
		Issues: []AnalysisIssue{
			{
				Section:           "Personal Summary",
				Problem:           "Contains nationality details",
				Priority:          1,
				AutoFixable:       true,
				RequiresUserInput: []string{},
			},
		},
		BulletRewrites: []BulletRewrite{
			{
				Section:            "Experience",
				Before:             "Old bullet",
				After:              "New bullet",
				MetricsSource:      "resume",
				PlaceholdersNeeded: []string{},
				ClaimSupport:       "supported",
			},
			{
				Section:            "Experience",
				Before:             "Another bullet",
				After:              "Another rewrite with X",
				MetricsSource:      "resume",
				PlaceholdersNeeded: []string{"X"},
				ClaimSupport:       "supported",
			},
		},
	}

	result, err := ExecuteApply(context.Background(), "sample resume text", analysis, ApplyHeaderInputs{
		Email: "user@example.com",
	}, false)
	if err != nil {
		t.Fatalf("ExecuteApply failed: %v", err)
	}
	if result.Status != ApplyResultDraft {
		t.Fatalf("expected draft status, got %q", result.Status)
	}
	if result.PlaceholdersRemaining != 1 {
		t.Fatalf("expected 1 placeholder remaining, got %d", result.PlaceholdersRemaining)
	}
	if result.SafeRewritesApplied != 1 {
		t.Fatalf("expected 1 safe rewrite applied, got %d", result.SafeRewritesApplied)
	}
	if result.AutoFixesApplied != 1 {
		t.Fatalf("expected 1 auto fix applied, got %d", result.AutoFixesApplied)
	}

	documentXML, err := readDocumentXML(result.DocxBytes)
	if err != nil {
		t.Fatalf("read document.xml failed: %v", err)
	}
	assertContains(t, documentXML, "New bullet")
	assertNotContains(t, documentXML, "Old bullet")
	assertNotContains(t, documentXML, "Nationality: India")
	assertContains(t, documentXML, "user@example.com")
}

func TestExecuteApplyStrictModeMissingContact(t *testing.T) {
	llmResponse := `{"header":{"name":"Test User","title":"","email":"","phone":"","location":"","links":[]},"summary":[],"skills":{"languages":[],"frameworks":[],"databases":[],"cloudDevOps":[],"observability":[],"tools":[]},"experience":[],"projects":[],"education":[],"achievements":[],"certifications":[]}`

	prevClient := Client
	Client = &mockApplyLLM{response: llmResponse}
	defer func() {
		Client = prevClient
	}()

	analysis := AnalysisResultV2_3{}

	_, err := ExecuteApply(context.Background(), "sample resume text", analysis, ApplyHeaderInputs{}, true)
	if err == nil {
		t.Fatalf("expected strict mode error")
	}
	var missing contract.MissingFieldsError
	if !errors.As(err, &missing) {
		t.Fatalf("expected MissingFieldsError, got %T", err)
	}
	if len(missing.Fields) == 0 {
		t.Fatalf("expected missing fields, got none")
	}
}

func readDocumentXML(docxBytes []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		return "", err
	}
	for _, file := range reader.File {
		if normalizeTestZipName(file.Name) == "word/document.xml" {
			rc, err := file.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			content, err := io.ReadAll(rc)
			if err != nil {
				return "", err
			}
			return string(content), nil
		}
	}
	return "", io.EOF
}

func normalizeTestZipName(name string) string {
	return strings.ReplaceAll(name, "\\", "/")
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !bytes.Contains([]byte(haystack), []byte(needle)) {
		t.Fatalf("expected to contain %q", needle)
	}
}

func assertNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if bytes.Contains([]byte(haystack), []byte(needle)) {
		t.Fatalf("expected to not contain %q", needle)
	}
}
