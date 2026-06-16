package render

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	modelv1 "resume-backend/resume/modelv1"
)

func TestRenderResumeModelV1Success(t *testing.T) {
	docxBytes, err := RenderResumeModelV1(validModelV1Resume())
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(docxBytes) == 0 {
		t.Fatal("expected docx bytes")
	}

	documentXML := readModelV1DocumentXML(t, docxBytes)
	text := extractModelV1DocumentText(t, documentXML)

	assertModelV1Contains(t, text, "Alex Rivera")
	assertModelV1Contains(t, text, "Backend Engineer")
	assertModelV1Contains(t, text, "alex@example.com")
	assertModelV1Contains(t, text, "Reduced API latency by 35%")
	assertModelV1Contains(t, text, "Jan 2021 - Dec 2024")
	assertModelV1Contains(t, text, "LinkedIn: https://linkedin.com/in/alex")
}

func TestRenderResumeModelV1FailsForStructuralValidationError(t *testing.T) {
	resume := validModelV1Resume()
	resume.SchemaVersion = "resume.v2"

	if _, err := RenderResumeModelV1(resume); err == nil {
		t.Fatal("expected structural validation error")
	}
}

func TestRenderResumeModelV1RespectsSectionOrder(t *testing.T) {
	resume := validModelV1Resume()
	resume.SectionOrder = []string{"projects", "summary", "skills", "experience"}

	docxBytes, err := RenderResumeModelV1(resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	text := extractModelV1DocumentText(t, readModelV1DocumentXML(t, docxBytes))

	assertBefore(t, text, "Projects", "Summary")
	assertBefore(t, text, "Summary", "Skills")
	assertBefore(t, text, "Skills", "Experience")
}

func TestRenderResumeModelV1UsesDefaultSectionOrder(t *testing.T) {
	resume := validModelV1Resume()
	resume.SectionOrder = nil

	docxBytes, err := RenderResumeModelV1(resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	text := extractModelV1DocumentText(t, readModelV1DocumentXML(t, docxBytes))

	assertBefore(t, text, "Summary", "Skills")
	assertBefore(t, text, "Skills", "Experience")
	assertBefore(t, text, "Experience", "Projects")
}

func TestRenderResumeModelV1CurrentJobRendersPresent(t *testing.T) {
	resume := validModelV1Resume()
	resume.Experience[0].IsCurrent = true
	resume.Experience[0].EndDate = ""

	docxBytes, err := RenderResumeModelV1(resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	text := extractModelV1DocumentText(t, readModelV1DocumentXML(t, docxBytes))

	assertModelV1Contains(t, text, "Jan 2021 - Present")
}

func TestRenderResumeModelV1UsesStandardBulletMarker(t *testing.T) {
	docxBytes, err := RenderResumeModelV1(validModelV1Resume())
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	numberingXML := readModelV1DocxPart(t, docxBytes, "word/numbering.xml")

	assertModelV1Contains(t, numberingXML, `w:val="&#8226;"`)
	if strings.Contains(numberingXML, "â€¢") {
		t.Fatalf("expected no mojibake bullet marker in numbering.xml: %s", numberingXML)
	}
}

func TestRenderResumeModelV1IncompleteDraftExports(t *testing.T) {
	docxBytes, err := RenderResumeModelV1(modelv1.ResumeModel{SchemaVersion: modelv1.SchemaVersion})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(docxBytes) == 0 {
		t.Fatal("expected docx bytes")
	}
	if text := extractModelV1DocumentText(t, readModelV1DocumentXML(t, docxBytes)); strings.Contains(text, "{{") {
		t.Fatalf("expected no template tokens, got %q", text)
	}
}

func validModelV1Resume() modelv1.ResumeModel {
	years := 6
	return modelv1.ResumeModel{
		SchemaVersion: modelv1.SchemaVersion,
		Basics: modelv1.Basics{
			FullName: "Alex Rivera",
			Headline: "Backend Engineer",
			Email:    "alex@example.com",
			Phone:    "+1 555 0100",
			Location: modelv1.Location{City: "Austin", State: "TX", Country: "USA"},
			Links: []modelv1.Link{{
				Type:  "linkedin",
				Label: "LinkedIn",
				URL:   "https://linkedin.com/in/alex",
			}},
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
			Location:       "Austin, TX",
			EmploymentType: "full_time",
			StartDate:      "2021-01",
			EndDate:        "2024-12",
			Highlights: []modelv1.Highlight{{
				ID:     "exp-1-highlight-1",
				Text:   "Reduced API latency by 35% through query and cache improvements.",
				Source: "user_provided",
			}},
		}},
		Projects: []modelv1.Project{{
			ID:          "project-1",
			Name:        "Partner API",
			Description: "Built a partner integration API.",
			Role:        "Lead engineer",
			Highlights: []modelv1.Highlight{{
				ID:     "project-1-highlight-1",
				Text:   "Launched integration for 4 partners.",
				Source: "user_provided",
			}},
		}},
		SectionOrder: []string{"summary", "skills", "experience", "projects"},
	}
}

func readModelV1DocumentXML(t *testing.T, docxBytes []byte) string {
	t.Helper()
	return readModelV1DocxPart(t, docxBytes, "word/document.xml")
}

func readModelV1DocxPart(t *testing.T, docxBytes []byte, partName string) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		t.Fatalf("zip reader failed: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != partName {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", partName, err)
		}
		defer rc.Close()
		content, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", partName, err)
		}
		return string(content)
	}
	t.Fatalf("%s not found", partName)
	return ""
}

func extractModelV1DocumentText(t *testing.T, documentXML string) string {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(documentXML))
	var parts []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode document.xml: %v", err)
		}
		if chars, ok := token.(xml.CharData); ok {
			text := strings.TrimSpace(string(chars))
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func assertModelV1Contains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected %q to contain %q", haystack, needle)
	}
}

func assertBefore(t *testing.T, text, before, after string) {
	t.Helper()
	beforeIndex := strings.Index(text, before)
	afterIndex := strings.Index(text, after)
	if beforeIndex == -1 {
		t.Fatalf("expected text to contain %q", before)
	}
	if afterIndex == -1 {
		t.Fatalf("expected text to contain %q", after)
	}
	if beforeIndex >= afterIndex {
		t.Fatalf("expected %q before %q in text:\n%s", before, after, text)
	}
}
