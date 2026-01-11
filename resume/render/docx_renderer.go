package render

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"resume-backend/resume/model"
)

const defaultTemplatePath = "assets/templates/resume_modern_ats_v1.docx"

// RenderResume renders a ResumeModel into a DOCX byte slice.
func RenderResume(resume model.ResumeModel) ([]byte, error) {
	if strings.TrimSpace(resume.Header.Name) == "" {
		return nil, errors.New("full name is required")
	}
	if strings.TrimSpace(resume.Header.Email) == "" && strings.TrimSpace(resume.Header.Phone) == "" {
		return nil, errors.New("email or phone is required")
	}
	return renderResumeFromTemplate(defaultTemplatePath, resume)
}

func renderResumeFromTemplate(templatePath string, resume model.ResumeModel) ([]byte, error) {
	templateBytes, err := os.ReadFile(filepath.Clean(templatePath))
	if err != nil {
		return nil, err
	}

	reader, err := zip.NewReader(bytes.NewReader(templateBytes), int64(len(templateBytes)))
	if err != nil {
		return nil, err
	}

	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	defer writer.Close()

	for _, file := range reader.File {
		if normalizeZipName(file.Name) == "word/document.xml" {
			updated, err := renderDocumentXML(file, resume)
			if err != nil {
				return nil, err
			}
			if err := writeZipFile(writer, file, updated); err != nil {
				return nil, err
			}
			continue
		}

		content, err := readZipFile(file)
		if err != nil {
			return nil, err
		}
		if err := writeZipFile(writer, file, content); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return output.Bytes(), nil
}

func renderDocumentXML(file *zip.File, resume model.ResumeModel) ([]byte, error) {
	content, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	xmlText := string(content)

	xmlText, err = expandSummary(xmlText, resume.Summary)
	if err != nil {
		return nil, err
	}

	xmlText, err = expandExperience(xmlText, resume.Experience)
	if err != nil {
		return nil, err
	}

	links := formatLinks(resume.Header.Links)

	replacements := map[string]string{
		"{{FULL_NAME}}": escapeXML(resume.Header.Name),
		"{{TITLE}}":     escapeXML(resume.Header.Title),
		"{{EMAIL}}":     escapeXML(resume.Header.Email),
		"{{PHONE}}":     escapeXML(resume.Header.Phone),
		"{{LOCATION}}":  escapeXML(resume.Header.Location),
		"{{LINKS}}":     escapeXML(links),
	}

	for token, value := range replacements {
		xmlText = strings.ReplaceAll(xmlText, token, value)
	}

	return []byte(xmlText), nil
}

func expandSummary(xmlText string, items []string) (string, error) {
	return expandLoop(xmlText, "SUMMARY", items, func(block string, item string, _ int) string {
		return strings.ReplaceAll(block, "{{SUMMARY_ITEM}}", escapeXML(item))
	})
}

func expandExperience(xmlText string, items []model.ResumeExperience) (string, error) {
	startTag := "{{#EXPERIENCE}}"
	endTag := "{{/EXPERIENCE}}"

	start := strings.Index(xmlText, startTag)
	end := strings.Index(xmlText, endTag)
	if start == -1 || end == -1 || end < start {
		return xmlText, nil
	}

	block := xmlText[start+len(startTag) : end]
	if len(items) == 0 {
		return strings.ReplaceAll(xmlText, xmlText[start:end+len(endTag)], ""), nil
	}

	var rendered strings.Builder
	for _, exp := range items {
		itemBlock := block

		itemBlock, _ = expandHighlights(itemBlock, exp.Highlights)

		replacements := map[string]string{
			"{{EXP_COMPANY}}":  escapeXML(exp.Company),
			"{{EXP_ROLE}}":     escapeXML(exp.Role),
			"{{EXP_LOCATION}}": escapeXML(exp.Location),
			"{{EXP_START}}":    escapeXML(exp.Start),
			"{{EXP_END}}":      escapeXML(exp.End),
		}
		for token, value := range replacements {
			itemBlock = strings.ReplaceAll(itemBlock, token, value)
		}
		rendered.WriteString(itemBlock)
	}

	return xmlText[:start] + rendered.String() + xmlText[end+len(endTag):], nil
}

func expandHighlights(xmlText string, items []string) (string, error) {
	return expandLoop(xmlText, "HIGHLIGHTS", items, func(block string, item string, _ int) string {
		return strings.ReplaceAll(block, "{{HIGHLIGHT_ITEM}}", escapeXML(item))
	})
}

func expandLoop(xmlText, name string, items []string, render func(string, string, int) string) (string, error) {
	startTag := "{{#" + name + "}}"
	endTag := "{{/" + name + "}}"

	start := strings.Index(xmlText, startTag)
	end := strings.Index(xmlText, endTag)
	if start == -1 || end == -1 || end < start {
		return xmlText, nil
	}

	block := xmlText[start+len(startTag) : end]
	if len(items) == 0 {
		return strings.ReplaceAll(xmlText, xmlText[start:end+len(endTag)], ""), nil
	}

	var rendered strings.Builder
	for i, item := range items {
		rendered.WriteString(render(block, item, i))
	}

	return xmlText[:start] + rendered.String() + xmlText[end+len(endTag):], nil
}

func escapeXML(value string) string {
	if value == "" {
		return ""
	}
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func writeZipFile(writer *zip.Writer, source *zip.File, content []byte) error {
	header := &zip.FileHeader{
		Name:   normalizeZipName(source.Name),
		Method: source.Method,
	}
	header.SetModTime(source.Modified)

	dst, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	if _, err := dst.Write(content); err != nil {
		return err
	}
	return nil
}

func normalizeZipName(name string) string {
	return strings.ReplaceAll(name, "\\", "/")
}

func formatLinks(links any) string {
	switch v := links.(type) {
	case []string:
		return strings.Join(v, " | ")
	default:
		return formatLinkStructs(v)
	}
}

func formatLinkStructs(links any) string {
	rv := reflect.ValueOf(links)
	if rv.Kind() != reflect.Slice {
		return ""
	}

	out := make([]string, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i)
		if item.Kind() == reflect.Pointer {
			item = item.Elem()
		}
		if item.Kind() != reflect.Struct {
			return ""
		}

		labelField := item.FieldByName("Label")
		urlField := item.FieldByName("URL")
		if !labelField.IsValid() || !urlField.IsValid() {
			return ""
		}
		if labelField.Kind() != reflect.String || urlField.Kind() != reflect.String {
			return ""
		}

		label := labelField.String()
		url := urlField.String()
		if url == "" {
			continue
		}
		if label != "" {
			out = append(out, label+": "+url)
		} else {
			out = append(out, url)
		}
	}

	return strings.Join(out, " | ")
}
