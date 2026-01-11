package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"resume-backend/resume/model"
	"resume-backend/resume/render"
)

func main() {
	outPath := flag.String("out", "./out/sample_resume.docx", "output path for generated DOCX")
	flag.Parse()

	resumeModel := sampleResumeModel()

	docxBytes, err := render.RenderResume(resumeModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render failed: %v\n", err)
		os.Exit(1)
	}

	if err := writeOutputs(*outPath, resumeModel, docxBytes); err != nil {
		fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		os.Exit(1)
	}

	if err := validateRenderedDocx(*outPath); err != nil {
		fmt.Fprintf(os.Stderr, "render validation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OK: wrote %s\n", *outPath)
}

func writeOutputs(outPath string, resumeModel model.ResumeModel, docxBytes []byte) error {
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(outPath, docxBytes, 0o644); err != nil {
		return err
	}

	modelPath := filepath.Join(dir, "sample_resume_model.json")
	payload, err := json.MarshalIndent(resumeModel, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(modelPath, payload, 0o644); err != nil {
		return err
	}

	return nil
}

func sampleResumeModel() model.ResumeModel {
	return model.ResumeModel{
		Header: model.ResumeHeader{
			Name:     "Jordan Lee",
			Title:    "Senior Backend Engineer",
			Email:    "jordan.lee@example.com",
			Phone:    "+1-555-0102",
			Location: "Austin, TX",
			Links: []string{
				"https://www.linkedin.com/in/jordanlee",
				"https://github.com/jordanlee",
			},
		},
		Summary: []string{
			"Backend engineer with 8+ years of experience building resilient APIs and data services.",
			"Led platform modernization initiatives spanning cloud migration and observability adoption.",
		},
		Skills: model.ResumeSkills{
			Languages:     []string{"Go", "Java"},
			Frameworks:    []string{"Gin", "Spring Boot"},
			Databases:     []string{"PostgreSQL", "Redis"},
			CloudDevOps:   []string{"AWS", "Docker", "Kubernetes"},
			Observability: []string{"OpenTelemetry", "Datadog"},
			Tools:         []string{"GitHub Actions", "Terraform"},
		},
		Experience: []model.ResumeExperience{
			{
				ID:       "exp_1",
				Company:  "Acme Logistics",
				Role:     "Senior Backend Engineer",
				Location: "Austin, TX",
				Start:    "2021-04",
				End:      "Present",
				Highlights: []string{
					"Designed a routing service that reduced shipment latency by 18%.",
					"Implemented distributed tracing to cut incident triage time by 35%.",
				},
			},
			{
				ID:       "exp_2",
				Company:  "Blue Harbor Systems",
				Role:     "Backend Engineer",
				Location: "Seattle, WA",
				Start:    "2018-01",
				End:      "2021-03",
				Highlights: []string{
					"Built event-driven ingestion pipelines for compliance data feeds.",
				},
			},
		},
		Projects:       []model.ResumeProject{},
		Education:      []model.ResumeEducation{},
		Achievements:   []model.ResumeAchievement{},
		Certifications: []model.ResumeCertification{},
	}
}

func validateRenderedDocx(path string) error {
	docxBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	reader, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		return err
	}

	for _, file := range reader.File {
		if normalizeZipName(file.Name) != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		content, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		text := string(content)
		pos := tokenIndex(text)
		if pos != -1 {
			snippet := snippetAround(text, pos, 200)
			return fmt.Errorf("unresolved template tokens near: %s", snippet)
		}
		return nil
	}

	return fmt.Errorf("document.xml not found in docx")
}

func normalizeZipName(name string) string {
	return strings.ReplaceAll(name, "\\", "/")
}

func tokenIndex(text string) int {
	if idx := strings.Index(text, "{{"); idx != -1 {
		return idx
	}
	if idx := strings.Index(text, "}}"); idx != -1 {
		return idx
	}
	return -1
}

func snippetAround(text string, pos, maxLen int) string {
	if pos < 0 {
		return ""
	}
	start := pos - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
	}
	return text[start:end]
}
