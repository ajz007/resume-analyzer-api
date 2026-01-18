package render

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"strings"
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

func TestRenderResumeExpandsSkills(t *testing.T) {
	resume := model.ResumeModel{
		Header: model.ResumeHeader{
			Name: "Katherine Johnson",
		},
		Skills: model.ResumeSkills{
			Languages:     []string{"Go", "Python"},
			Frameworks:    []string{"Gin"},
			Databases:     []string{"PostgreSQL"},
			CloudDevOps:   []string{"AWS"},
			Observability: []string{"Prometheus"},
			Tools:         []string{"Docker"},
		},
	}

	docxBytes, err := renderResumeFromTemplate("../../assets/templates/resume_modern_ats_v1.docx", resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	documentXML, err := readDocumentXML(docxBytes)
	if err != nil {
		t.Fatalf("read document.xml failed: %v", err)
	}

	assertNotContains(t, documentXML, "{{#SKILLS}}")
	assertNotContains(t, documentXML, "{{SKILL_ITEM}}")
	assertContains(t, documentXML, "Go")
	assertContains(t, documentXML, "Python")
	assertContains(t, documentXML, "Gin")
	assertContains(t, documentXML, "PostgreSQL")
	assertContains(t, documentXML, "AWS")
}

func TestRenderResumeStyleSmoke(t *testing.T) {
	resume := model.ResumeModel{
		Header: model.ResumeHeader{
			Name:     "Taylor Otwell",
			Email:    "taylor@example.com",
			Phone:    "555-555-5555",
			Location: "Austin, TX",
			Links:    []string{"https://linkedin.com/in/taylor"},
		},
		Summary: []string{"Summary line one.", "Summary line two."},
		Skills: model.ResumeSkills{
			Tools: []string{"Go, SQL"},
		},
		Experience: []model.ResumeExperience{
			{
				Company:    "Acme",
				Role:       "Engineer",
				Location:   "Remote",
				Start:      "2020-01",
				End:        "Present",
				Highlights: []string{"Shipped features."},
			},
		},
		Education: []model.ResumeEducation{
			{
				Institution: "State University",
				Degree:      "BS",
				Field:       "Computer Science",
				Location:    "Austin, TX",
				Start:       "2016-08",
				End:         "2020-05",
			},
		},
		Certifications: []model.ResumeCertification{
			{
				Name:   "AWS SA",
				Issuer: "AWS",
				Date:   "2022-06",
			},
		},
		Achievements: []model.ResumeAchievement{
			{
				Title: "Award A",
				Date:  "2021-12",
			},
		},
	}

	docxBytes, err := renderResumeFromTemplate("../../assets/templates/resume_modern_ats_v1.docx", resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	documentXML, err := readDocumentXML(docxBytes)
	if err != nil {
		t.Fatalf("read document.xml failed: %v", err)
	}

	if strings.Contains(documentXML, "{{") {
		t.Fatalf("expected no template placeholders, found token markers")
	}

	assertHeadingStyled(t, documentXML, "Summary")
	assertHeadingStyled(t, documentXML, "Skills")
	assertHeadingStyled(t, documentXML, "Experience")
	assertHeadingStyled(t, documentXML, "Education")

	if !strings.Contains(documentXML, "<w:pStyle w:val=\"ListParagraph\"") {
		t.Fatalf("expected bullet list paragraph style")
	}
}

func TestRenderResumeHasNoTemplateTokens(t *testing.T) {
	resume := model.ResumeModel{
		Header: model.ResumeHeader{
			Name:  "Ada Lovelace",
			Email: "ada@example.com",
		},
	}

	docxBytes, err := renderResumeFromTemplate("../../assets/templates/resume_modern_ats_v1.docx", resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	documentXML, err := readDocumentXML(docxBytes)
	if err != nil {
		t.Fatalf("read document.xml failed: %v", err)
	}

	if strings.Contains(documentXML, "{{") || strings.Contains(documentXML, "}}") {
		t.Fatalf("expected no template tokens, found %q", findRemainingToken(documentXML))
	}
}

func TestRenderResumeProducesValidDocx(t *testing.T) {
	resume := model.ResumeModel{
		Header: model.ResumeHeader{
			Name:  "Ada Lovelace",
			Email: "ada@example.com",
		},
	}

	docxBytes, err := renderResumeFromTemplate("../../assets/templates/resume_modern_ats_v1.docx", resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		t.Fatalf("zip reader failed: %v", err)
	}

	required := map[string]bool{
		"[Content_Types].xml": false,
		"_rels/.rels":         false,
		"word/document.xml":   false,
	}
	for _, file := range reader.File {
		name := normalizeZipName(file.Name)
		if _, ok := required[name]; ok {
			required[name] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Fatalf("expected docx to contain %s", name)
		}
	}

	documentXML, err := readDocumentXML(docxBytes)
	if err != nil {
		t.Fatalf("read document.xml failed: %v", err)
	}

	var doc struct {
		XMLName xml.Name `xml:"document"`
	}
	if err := xml.Unmarshal([]byte(documentXML), &doc); err != nil {
		t.Fatalf("document.xml parse failed: %v", err)
	}
	if doc.XMLName.Local != "document" {
		t.Fatalf("expected document.xml root <document>, got %q", doc.XMLName.Local)
	}
}

func TestRenderDocumentXMLSplitTokens(t *testing.T) {
	content, err := os.ReadFile("testdata/split_tokens_document.xml")
	if err != nil {
		t.Fatalf("read fixture failed: %v", err)
	}

	resume := model.ResumeModel{
		Header: model.ResumeHeader{
			Name:  "Ada Lovelace",
			Email: "ada@example.com",
		},
		Summary: []string{"Summary line."},
		Skills: model.ResumeSkills{
			Languages: []string{"Go", "Python"},
		},
		Experience: []model.ResumeExperience{
			{
				Role:       "Engineer",
				Highlights: []string{"Shipped a feature."},
			},
		},
	}

	rendered, err := renderDocumentXMLText(string(content), resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	assertContains(t, rendered, "Summary line.")
	assertContains(t, rendered, "Go")
	assertContains(t, rendered, "Python")
	assertContains(t, rendered, "Engineer")
	assertContains(t, rendered, "Shipped a feature.")

	if strings.Contains(rendered, "{{") || strings.Contains(rendered, "}}") {
		t.Fatalf("expected no template tokens, found %q", findRemainingToken(rendered))
	}
}

func TestRenderDocumentXMLSplitHeaderTokens(t *testing.T) {
	content, err := os.ReadFile("testdata/split_header_tokens_document.xml")
	if err != nil {
		t.Fatalf("read fixture failed: %v", err)
	}

	resume := model.ResumeModel{
		Header: model.ResumeHeader{
			Name:     "Ada Lovelace",
			Title:    "Engineer",
			Email:    "ada@example.com",
			Phone:    "555-555-5555",
			Location: "London, UK",
			Links:    []string{"https://example.com"},
		},
	}

	rendered, err := renderDocumentXMLText(string(content), resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	assertContains(t, rendered, "Ada Lovelace")
	assertContains(t, rendered, "Engineer")
	assertContains(t, rendered, "ada@example.com")
	assertContains(t, rendered, "555-555-5555")
	assertContains(t, rendered, "London, UK")
	assertContains(t, rendered, "https://example.com")

	if strings.Contains(rendered, "{{") || strings.Contains(rendered, "}}") {
		t.Fatalf("expected no template tokens, found %q", findRemainingToken(rendered))
	}
}

func TestRenderDocumentXMLSplitExperienceTokens(t *testing.T) {
	content, err := os.ReadFile("testdata/split_experience_tokens_document.xml")
	if err != nil {
		t.Fatalf("read fixture failed: %v", err)
	}

	resume := model.ResumeModel{
		Header: model.ResumeHeader{
			Name:  "Ada Lovelace",
			Email: "ada@example.com",
		},
		Experience: []model.ResumeExperience{
			{
				Company:    "Example Corp",
				Role:       "Engineer",
				Location:   "Remote",
				Start:      "2020-01",
				End:        "2023-01",
				Highlights: []string{"Shipped a feature."},
			},
		},
	}

	rendered, err := renderDocumentXMLText(string(content), resume)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	assertContains(t, rendered, "Engineer")
	assertContains(t, rendered, "Example Corp")
	assertContains(t, rendered, "Remote")
	assertContains(t, rendered, "2020-01")
	assertContains(t, rendered, "2023-01")
	assertContains(t, rendered, "Shipped a feature.")

	if strings.Contains(rendered, "{{") || strings.Contains(rendered, "}}") {
		t.Fatalf("expected no template tokens, found %q", findRemainingToken(rendered))
	}
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

func assertHeadingStyled(t *testing.T, xmlText, heading string) {
	t.Helper()
	tag := "<w:t>" + heading + "</w:t>"
	idx := strings.Index(xmlText, tag)
	if idx == -1 {
		t.Fatalf("expected heading %q", heading)
	}
	windowStart := idx - 300
	if windowStart < 0 {
		windowStart = 0
	}
	window := xmlText[windowStart:idx]

	style := StyleMap["sectionHeading"]
	if style.Bold && !strings.Contains(window, "<w:b") {
		t.Fatalf("expected %q heading to be bold", heading)
	}
	if style.Size > 0 {
		sizeTag := "w:sz w:val=\"" + itoa(style.Size) + "\""
		if !strings.Contains(window, sizeTag) {
			t.Fatalf("expected %q heading size %d", heading, style.Size)
		}
	}
	if style.Color != "" {
		colorTag := "w:color w:val=\"" + style.Color + "\""
		if !strings.Contains(window, colorTag) {
			t.Fatalf("expected %q heading color %s", heading, style.Color)
		}
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + (value % 10))
		value /= 10
	}
	return string(buf[i:])
}
