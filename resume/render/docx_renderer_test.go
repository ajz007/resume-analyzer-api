package render

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"resume-backend/resume/model"
)

func TestRenderResumeExpandsLoops(t *testing.T) {
	resume := model.ResumeModel{
		Header: model.ResumeHeader{
			Name: "Ada Lovelace",
		},
		Summary: []string{"First summary.", "Second summary."},
		Experience: []model.ResumeExperience{
			{
				Company:    "Example Corp",
				Highlights: []string{"Did the thing.", "Did another thing."},
			},
		},
	}

	docxBytes, err := renderResumeFromTemplate("testdata/template.docx", resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	documentXML, err := readDocumentXML(docxBytes)
	if err != nil {
		t.Fatalf("read document.xml failed: %v", err)
	}

	assertContains(t, documentXML, "Ada Lovelace")
	assertContains(t, documentXML, "First summary.")
	assertContains(t, documentXML, "Second summary.")
	assertContains(t, documentXML, "Example Corp")
	assertContains(t, documentXML, "Did the thing.")
	assertContains(t, documentXML, "Did another thing.")
	assertNotContains(t, documentXML, "{{#SUMMARY}}")
	assertNotContains(t, documentXML, "{{/SUMMARY}}")
	assertNotContains(t, documentXML, "{{#EXPERIENCE}}")
	assertNotContains(t, documentXML, "{{/EXPERIENCE}}")
	assertNotContains(t, documentXML, "{{#HIGHLIGHTS}}")
	assertNotContains(t, documentXML, "{{/HIGHLIGHTS}}")
}

func TestRenderResumeRemovesEmptyLoops(t *testing.T) {
	resume := model.ResumeModel{
		Header: model.ResumeHeader{
			Name: "Grace Hopper",
		},
	}

	docxBytes, err := renderResumeFromTemplate("testdata/template.docx", resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	documentXML, err := readDocumentXML(docxBytes)
	if err != nil {
		t.Fatalf("read document.xml failed: %v", err)
	}

	assertContains(t, documentXML, "Grace Hopper")
	assertNotContains(t, documentXML, "{{#SUMMARY}}")
	assertNotContains(t, documentXML, "{{/SUMMARY}}")
	assertNotContains(t, documentXML, "{{#EXPERIENCE}}")
	assertNotContains(t, documentXML, "{{/EXPERIENCE}}")
}

func readDocumentXML(docxBytes []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		return "", err
	}
	for _, file := range reader.File {
		if normalizeZipName(file.Name) == "word/document.xml" {
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
