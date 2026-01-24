package render

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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

	xmlText, err := renderDocumentXMLText(string(content), resume)
	if err != nil {
		return nil, err
	}

	return []byte(xmlText), nil
}

func renderDocumentXMLText(xmlText string, resume model.ResumeModel) (string, error) {
	rootStart, rootEnd, err := extractRootTags(xmlText)
	if err != nil {
		return "", err
	}
	root, header, err := parseXMLDocument(xmlText)
	if err != nil {
		return "", err
	}

	body := findBodyNode(root)
	if err := expandLoopInContainer(body, "SUMMARY", resume.Summary, "{{SUMMARY_ITEM}}"); err != nil {
		return "", err
	}

	if err := expandLoopInContainer(body, "SKILLS", flattenSkills(resume.Skills), "{{SKILL_ITEM}}"); err != nil {
		return "", err
	}

	if err := expandExperienceInContainer(body, resume.Experience); err != nil {
		return "", err
	}

	if err := expandEducationInContainer(body, resume.Education); err != nil {
		return "", err
	}

	if err := expandCertificationsInContainer(body, resume.Certifications); err != nil {
		return "", err
	}

	if err := expandAwardsInContainer(body, resume.Achievements); err != nil {
		return "", err
	}

	links := formatLinks(resume.Header.Links)

	replacements := map[string]string{
		"{{FULL_NAME}}": resume.Header.Name,
		"{{TITLE}}":     resume.Header.Title,
		"{{EMAIL}}":     resume.Header.Email,
		"{{PHONE}}":     resume.Header.Phone,
		"{{LOCATION}}":  resume.Header.Location,
		"{{LINKS}}":     links,
	}

	replaceTokensInNode(root, replacements)
	replaceTokensInNode(root, map[string]string{
		"{{#HIGHLIGHTS}}":    "",
		"{{/HIGHLIGHTS}}":    "",
		"{{HIGHLIGHT_ITEM}}": "",
	})
	applyContactPlaceholders(root, resume)
	removeEmptySections(root, resume)
	normalizeParagraphNesting(root)
	if err := validateNoPlaceholders(root); err != nil {
		return "", err
	}
	enforceHeadingBold(root, []string{"Summary", "Skills", "Experience", "Education"})

	xmlText, err = encodeXMLDocument(header, root, rootStart, rootEnd)
	if err != nil {
		return "", err
	}

	if err := validateDocumentXMLStrict(xmlText); err != nil {
		return "", err
	}
	if err := validateDocumentXMLStructure(xmlText); err != nil {
		return "", err
	}

	if token := findRemainingToken(xmlText); token != "" {
		return "", fmt.Errorf("template token remains in document.xml: %s", token)
	}

	return xmlText, nil
}

func expandExperienceInContainer(container *xmlNode, items []model.ResumeExperience) error {
	return expandLoopInContainerWithRenderer(container, "EXPERIENCE", len(items), func(template []*xmlNode, idx int) ([]*xmlNode, error) {
		item := items[idx]
		nodes := cloneNodes(template)
		tmp := &xmlNode{Name: xml.Name{Local: "root"}, Children: nodes}

		if err := expandLoopInContainer(tmp, "HIGHLIGHTS", item.Highlights, "{{HIGHLIGHT_ITEM}}"); err != nil {
			return nil, err
		}
		expandHighlightsFallback(tmp, item.Highlights)

		replaceTokensInNode(tmp, map[string]string{
			"{{EXP_COMPANY}}":  item.Company,
			"{{EXP_ROLE}}":     item.Role,
			"{{EXP_LOCATION}}": item.Location,
			"{{EXP_START}}":    item.Start,
			"{{EXP_END}}":      item.End,
		})

		return tmp.Children, nil
	})
}

func expandEducationInContainer(container *xmlNode, items []model.ResumeEducation) error {
	return expandLoopInContainerWithRenderer(container, "EDUCATION", len(items), func(template []*xmlNode, idx int) ([]*xmlNode, error) {
		item := items[idx]
		nodes := cloneNodes(template)
		tmp := &xmlNode{Name: xml.Name{Local: "root"}, Children: nodes}

		replaceTokensInNode(tmp, map[string]string{
			"{{EDU_INSTITUTION}}": item.Institution,
			"{{EDU_DEGREE}}":      item.Degree,
			"{{EDU_FIELD}}":       item.Field,
			"{{EDU_LOCATION}}":    item.Location,
			"{{EDU_START}}":       item.Start,
			"{{EDU_END}}":         item.End,
		})

		return tmp.Children, nil
	})
}

func expandCertificationsInContainer(container *xmlNode, items []model.ResumeCertification) error {
	return expandLoopInContainerWithRenderer(container, "CERTIFICATIONS", len(items), func(template []*xmlNode, idx int) ([]*xmlNode, error) {
		item := items[idx]
		nodes := cloneNodes(template)
		tmp := &xmlNode{Name: xml.Name{Local: "root"}, Children: nodes}

		replaceTokensInNode(tmp, map[string]string{
			"{{CERT_NAME}}":    item.Name,
			"{{CERT_ISSUER}}":  item.Issuer,
			"{{CERT_DATE}}":    item.Date,
			"{{CERT_EXPIRES}}": item.Expires,
		})

		return tmp.Children, nil
	})
}

func expandAwardsInContainer(container *xmlNode, items []model.ResumeAchievement) error {
	return expandLoopInContainerWithRenderer(container, "AWARDS", len(items), func(template []*xmlNode, idx int) ([]*xmlNode, error) {
		item := items[idx]
		nodes := cloneNodes(template)
		tmp := &xmlNode{Name: xml.Name{Local: "root"}, Children: nodes}

		replaceTokensInNode(tmp, map[string]string{
			"{{AWARD_TITLE}}": item.Title,
			"{{AWARD_DATE}}":  item.Date,
		})

		return tmp.Children, nil
	})
}

func flattenSkills(skills model.ResumeSkills) []string {
	out := make([]string, 0, len(skills.Languages)+len(skills.Frameworks)+len(skills.Databases)+len(skills.CloudDevOps)+len(skills.Observability)+len(skills.Tools))
	seen := make(map[string]struct{})

	add := func(values []string) {
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, trimmed)
		}
	}

	add(skills.Languages)
	add(skills.Frameworks)
	add(skills.Databases)
	add(skills.CloudDevOps)
	add(skills.Observability)
	add(skills.Tools)

	return out
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
	header := source.FileHeader
	header.Name = normalizeZipName(source.Name)

	dst, err := writer.CreateHeader(&header)
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

var tokenPattern = regexp.MustCompile(`{{[^}]+}}`)
var placeholderPattern = regexp.MustCompile(`(?i)\[(email|phone|handle)\]`)
var todoPattern = regexp.MustCompile(`(?i)\bTODO\b`)

func findRemainingToken(xmlText string) string {
	if match := tokenPattern.FindString(xmlText); match != "" {
		return match
	}
	if idx := strings.Index(xmlText, "{{"); idx != -1 {
		end := idx + 40
		if end > len(xmlText) {
			end = len(xmlText)
		}
		return xmlText[idx:end]
	}
	if idx := strings.Index(xmlText, "}}"); idx != -1 {
		start := idx - 40
		if start < 0 {
			start = 0
		}
		return xmlText[start : idx+2]
	}
	return ""
}

func applyContactPlaceholders(root *xmlNode, resume model.ResumeModel) {
	replaceOrRemovePlaceholder(root, placeholderRegex("email"), "[email]", resume.Header.Email)
	replaceOrRemovePlaceholder(root, placeholderRegex("phone"), "[phone]", resume.Header.Phone)
	replaceOrRemovePlaceholder(root, placeholderRegex("handle"), "[handle]", contactHandle(resume.Header.Links))
}

func replaceOrRemovePlaceholder(root *xmlNode, pattern *regexp.Regexp, placeholder, value string) {
	if root == nil {
		return
	}
	placeholderLower := strings.ToLower(placeholder)
	if strings.TrimSpace(value) == "" {
		removeParagraphs(root, func(p *xmlNode) bool {
			text := strings.ToLower(paragraphText(p))
			return strings.Contains(text, placeholderLower)
		})
		return
	}
	walkXML(root, func(n *xmlNode) bool {
		if !isElement(n, "p") {
			return true
		}
		text := paragraphText(n)
		if !pattern.MatchString(text) || !strings.Contains(strings.ToLower(text), placeholderLower) {
			return true
		}
		replacePatternInParagraph(n, pattern, value)
		return true
	})
}

func placeholderRegex(name string) *regexp.Regexp {
	escaped := regexp.QuoteMeta(name)
	return regexp.MustCompile(`(?i)\[` + escaped + `\]`)
}

func replacePatternInParagraph(p *xmlNode, pattern *regexp.Regexp, value string) {
	textNodes := collectTextElements(p)
	if len(textNodes) == 0 {
		return
	}
	combined := ""
	for _, node := range textNodes {
		combined += nodeText(node)
	}
	updated := pattern.ReplaceAllString(combined, value)
	if updated == combined {
		return
	}
	setNodeText(textNodes[0], updated)
	for i := 1; i < len(textNodes); i++ {
		setNodeText(textNodes[i], "")
	}
}

func contactHandle(links any) string {
	switch v := links.(type) {
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

func removeEmptySections(root *xmlNode, resume model.ResumeModel) {
	sections := []struct {
		heading string
		empty   bool
	}{
		{"Summary", len(resume.Summary) == 0},
		{"Skills", len(flattenSkills(resume.Skills)) == 0},
		{"Experience", len(resume.Experience) == 0},
		{"Education", len(resume.Education) == 0},
		{"Certifications", len(resume.Certifications) == 0},
		{"Awards", len(resume.Achievements) == 0},
		{"Projects", len(resume.Projects) == 0},
	}
	for _, section := range sections {
		if !section.empty {
			continue
		}
		removeParagraphs(root, func(p *xmlNode) bool {
			return strings.EqualFold(strings.TrimSpace(paragraphText(p)), section.heading)
		})
	}
}

func validateNoPlaceholders(root *xmlNode) error {
	if root == nil {
		return nil
	}
	var failure error
	walkXML(root, func(n *xmlNode) bool {
		if !isElement(n, "p") {
			return true
		}
		text := paragraphText(n)
		switch {
		case tokenPattern.MatchString(text):
			failure = fmt.Errorf("document.xml contains unresolved template tokens")
			return false
		case placeholderPattern.MatchString(text):
			failure = fmt.Errorf("document.xml contains placeholder contact values")
			return false
		case todoPattern.MatchString(text):
			failure = fmt.Errorf("document.xml contains TODO placeholders")
			return false
		}
		return true
	})
	return failure
}

func validateDocumentXMLStrict(xmlText string) error {
	rootStart, _, err := extractRootTags(xmlText)
	if err != nil {
		return err
	}
	declared := namespacesFromRootStart(rootStart)
	decoder := xml.NewDecoder(strings.NewReader(xmlText))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("document.xml parse failed: %w\n%s", err, firstLines(xmlText, 5))
		}
		switch t := token.(type) {
		case xml.StartElement:
			if err := checkDeclaredNamespace(t.Name.Space, t.Name.Local, declared, "element", xmlText); err != nil {
				return err
			}
			for _, attr := range t.Attr {
				if err := checkDeclaredNamespace(attr.Name.Space, attr.Name.Local, declared, "attribute", xmlText); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateDocumentXMLStructure(xmlText string) error {
	decoder := xml.NewDecoder(strings.NewReader(xmlText))
	var stack []xml.Name
	type runState struct {
		seenText bool
	}
	var runs []runState

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("document.xml parse failed: %w\n%s", err, firstLines(xmlText, 5))
		}
		switch t := token.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name)
			if isWmlElement(t.Name, "p") {
				for i := len(stack) - 2; i >= 0; i-- {
					if isWmlElement(stack[i], "p") {
						return fmt.Errorf("document.xml has nested <w:p>\n%s", firstLines(xmlText, 5))
					}
				}
			}
			if isWmlElement(t.Name, "r") {
				runs = append(runs, runState{})
			}
			if isWmlElement(t.Name, "t") && len(runs) > 0 {
				runs[len(runs)-1].seenText = true
			}
			if isWmlElement(t.Name, "rPr") && len(runs) > 0 && runs[len(runs)-1].seenText {
				return fmt.Errorf("document.xml has <w:rPr> after <w:t> in a run\n%s", firstLines(xmlText, 5))
			}
		case xml.EndElement:
			if isWmlElement(t.Name, "r") && len(runs) > 0 {
				runs = runs[:len(runs)-1]
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return nil
}

func isWmlElement(name xml.Name, local string) bool {
	return name.Local == local && name.Space == wmlNamespace
}

func checkDeclaredNamespace(space, local string, declared map[string]string, kind string, xmlText string) error {
	if space == "" {
		return nil
	}
	prefix, ok := knownNamespacePrefixes[space]
	if !ok {
		return nil
	}
	if uri, ok := declared[prefix]; ok && uri == space {
		return nil
	}
	name := local
	if prefix != "" {
		name = prefix + ":" + local
	}
	return fmt.Errorf("document.xml missing root namespace for %s %s\n%s", kind, name, firstLines(xmlText, 5))
}

var knownNamespacePrefixes = map[string]string{
	wmlNamespace: "w",
	relNamespace: "r",
	"http://schemas.openxmlformats.org/drawingml/2006/main":                 "a",
	"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing": "wp",
	"http://schemas.openxmlformats.org/drawingml/2006/picture":              "pic",
	"http://schemas.openxmlformats.org/markup-compatibility/2006":           "mc",
	"http://schemas.microsoft.com/office/word/2010/wordml":                  "w14",
	"http://schemas.microsoft.com/office/word/2012/wordml":                  "w15",
}

func firstLines(text string, count int) string {
	if count <= 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > count {
		lines = lines[:count]
	}
	return strings.Join(lines, "\n")
}
