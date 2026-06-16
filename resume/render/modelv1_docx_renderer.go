package render

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	modelv1 "resume-backend/resume/modelv1"
)

var defaultModelV1SectionOrder = []string{
	string(modelv1.SectionSummary),
	string(modelv1.SectionSkills),
	string(modelv1.SectionExperience),
	string(modelv1.SectionProjects),
	string(modelv1.SectionEducation),
	string(modelv1.SectionCertifications),
	string(modelv1.SectionAchievements),
	string(modelv1.SectionCustomSections),
}

// RenderResumeModelV1 renders a ResumeModel v1 document as a simple ATS-safe DOCX.
func RenderResumeModelV1(resume modelv1.ResumeModel) ([]byte, error) {
	if errs := modelv1.ValidateStructure(resume); len(errs) > 0 {
		return nil, fmt.Errorf("resume model is structurally invalid")
	}

	documentXML, err := renderModelV1DocumentXML(resume)
	if err != nil {
		return nil, err
	}
	if err := validateDocumentXMLStrict(documentXML); err != nil {
		return nil, err
	}
	if err := validateDocumentXMLStructure(documentXML); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	files := map[string]string{
		"[Content_Types].xml":          contentTypesXML,
		"_rels/.rels":                  packageRelsXML,
		"word/_rels/document.xml.rels": documentRelsXML,
		"word/document.xml":            documentXML,
		"word/numbering.xml":           numberingXML,
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/_rels/document.xml.rels", "word/document.xml", "word/numbering.xml"} {
		file, err := writer.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write([]byte(files[name])); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func renderModelV1DocumentXML(resume modelv1.ResumeModel) (string, error) {
	var body strings.Builder

	addParagraph(&body, resume.Basics.FullName, paragraphOptions{Bold: true, Size: 32})
	addParagraph(&body, resume.Basics.Headline, paragraphOptions{Italic: true, Size: 22})
	addParagraph(&body, contactLine(resume.Basics), paragraphOptions{})
	addParagraph(&body, linksLine(resume.Basics.Links), paragraphOptions{})

	for _, section := range sectionOrder(resume.SectionOrder) {
		switch section {
		case string(modelv1.SectionSummary):
			renderSummary(&body, resume.Summary)
		case string(modelv1.SectionSkills):
			renderSkills(&body, resume.Skills)
		case string(modelv1.SectionExperience):
			renderExperience(&body, resume.Experience)
		case string(modelv1.SectionProjects):
			renderProjects(&body, resume.Projects)
		case string(modelv1.SectionEducation):
			renderEducation(&body, resume.Education)
		case string(modelv1.SectionCertifications):
			renderCertifications(&body, resume.Certifications)
		case string(modelv1.SectionAchievements):
			renderAchievements(&body, resume.Achievements)
		case string(modelv1.SectionCustomSections):
			renderCustomSections(&body, resume.CustomSections)
		}
	}

	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="` + wmlNamespace + `">` +
		`<w:body>` +
		body.String() +
		`<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="720" w:right="720" w:bottom="720" w:left="720" w:header="360" w:footer="360" w:gutter="0"/></w:sectPr>` +
		`</w:body></w:document>`, nil
}

type paragraphOptions struct {
	Bold   bool
	Italic bool
	Size   int
	Bullet bool
}

func addHeading(body *strings.Builder, text string) {
	addParagraph(body, text, paragraphOptions{Bold: true, Size: 24})
}

func addBullet(body *strings.Builder, text string) {
	addParagraph(body, text, paragraphOptions{Bullet: true})
}

func addParagraph(body *strings.Builder, text string, opts paragraphOptions) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	body.WriteString("<w:p>")
	if opts.Bullet {
		body.WriteString(`<w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>`)
	}
	body.WriteString("<w:r>")
	if opts.Bold || opts.Italic || opts.Size > 0 {
		body.WriteString("<w:rPr>")
		if opts.Bold {
			body.WriteString("<w:b/>")
		}
		if opts.Italic {
			body.WriteString("<w:i/>")
		}
		if opts.Size > 0 {
			body.WriteString(fmt.Sprintf(`<w:sz w:val="%d"/>`, opts.Size))
		}
		body.WriteString("</w:rPr>")
	}
	body.WriteString("<w:t")
	if strings.TrimSpace(text) != text {
		body.WriteString(` xml:space="preserve"`)
	}
	body.WriteString(">")
	body.WriteString(xmlEscape(text))
	body.WriteString("</w:t></w:r></w:p>")
}

func renderSummary(body *strings.Builder, summary modelv1.Summary) {
	if strings.TrimSpace(summary.Text) == "" {
		return
	}
	addHeading(body, "Summary")
	addParagraph(body, summary.Text, paragraphOptions{})
}

func renderSkills(body *strings.Builder, skills []modelv1.SkillCategory) {
	if len(skills) == 0 {
		return
	}
	wroteHeading := false
	for _, category := range skills {
		names := skillNames(category.Items)
		if strings.TrimSpace(category.Category) == "" || len(names) == 0 {
			continue
		}
		if !wroteHeading {
			addHeading(body, "Skills")
			wroteHeading = true
		}
		addParagraph(body, category.Category+": "+strings.Join(names, ", "), paragraphOptions{})
	}
}

func renderExperience(body *strings.Builder, items []modelv1.Experience) {
	if len(items) == 0 {
		return
	}
	addHeading(body, "Experience")
	for _, item := range items {
		addParagraph(body, joinNonEmpty(" | ",
			joinNonEmpty(", ", item.Title, item.Company),
			item.Location,
			dateRange(item.StartDate, item.EndDate, item.IsCurrent),
		), paragraphOptions{Bold: true})
		addParagraph(body, item.Summary, paragraphOptions{})
		for _, highlight := range item.Highlights {
			addBullet(body, highlight.Text)
		}
		if len(nonEmpty(item.Technologies)) > 0 {
			addParagraph(body, "Technologies: "+strings.Join(nonEmpty(item.Technologies), ", "), paragraphOptions{})
		}
	}
}

func renderProjects(body *strings.Builder, items []modelv1.Project) {
	if len(items) == 0 {
		return
	}
	addHeading(body, "Projects")
	for _, item := range items {
		addParagraph(body, joinNonEmpty(" | ", item.Name, item.Role), paragraphOptions{Bold: true})
		addParagraph(body, item.Description, paragraphOptions{})
		for _, highlight := range item.Highlights {
			addBullet(body, highlight.Text)
		}
		if len(nonEmpty(item.Technologies)) > 0 {
			addParagraph(body, "Technologies: "+strings.Join(nonEmpty(item.Technologies), ", "), paragraphOptions{})
		}
		for _, link := range item.Links {
			addParagraph(body, formatLink(link), paragraphOptions{})
		}
	}
}

func renderEducation(body *strings.Builder, items []modelv1.Education) {
	if len(items) == 0 {
		return
	}
	addHeading(body, "Education")
	for _, item := range items {
		addParagraph(body, joinNonEmpty(" | ",
			item.Institution,
			joinNonEmpty(", ", item.Degree, item.FieldOfStudy),
			formatLocation(item.Location),
			dateRange(item.StartDate, item.EndDate, false),
		), paragraphOptions{})
	}
}

func renderCertifications(body *strings.Builder, items []modelv1.Certification) {
	if len(items) == 0 {
		return
	}
	addHeading(body, "Certifications")
	for _, item := range items {
		addParagraph(body, joinNonEmpty(" | ",
			joinNonEmpty(", ", item.Name, item.Issuer),
			dateRange(item.IssueDate, item.ExpiryDate, false),
			item.CredentialURL,
		), paragraphOptions{})
	}
}

func renderAchievements(body *strings.Builder, items []modelv1.Achievement) {
	if len(items) == 0 {
		return
	}
	addHeading(body, "Achievements")
	for _, item := range items {
		addParagraph(body, joinNonEmpty(": ", item.Title, item.Description), paragraphOptions{})
	}
}

func renderCustomSections(body *strings.Builder, sections []modelv1.CustomSection) {
	for _, section := range sections {
		if strings.TrimSpace(section.Title) == "" || len(section.Items) == 0 {
			continue
		}
		addHeading(body, section.Title)
		for _, item := range section.Items {
			addBullet(body, item.Text)
		}
	}
}

func sectionOrder(order []string) []string {
	if len(order) == 0 {
		return defaultModelV1SectionOrder
	}
	return order
}

func contactLine(basics modelv1.Basics) string {
	return joinNonEmpty(" | ", basics.Email, basics.Phone, formatLocation(basics.Location))
}

func linksLine(links []modelv1.Link) string {
	formatted := make([]string, 0, len(links))
	for _, link := range links {
		if value := formatLink(link); value != "" {
			formatted = append(formatted, value)
		}
	}
	return strings.Join(formatted, " | ")
}

func formatLink(link modelv1.Link) string {
	url := strings.TrimSpace(link.URL)
	if url == "" {
		return ""
	}
	label := strings.TrimSpace(link.Label)
	if label == "" {
		label = strings.TrimSpace(link.Type)
	}
	if label == "" {
		return url
	}
	return label + ": " + url
}

func skillNames(items []modelv1.SkillItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name := strings.TrimSpace(item.Name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func dateRange(start, end string, current bool) string {
	start = formatResumeDate(start)
	if current {
		return joinNonEmpty(" - ", start, "Present")
	}
	return joinNonEmpty(" - ", start, formatResumeDate(end))
}

func formatResumeDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := time.Parse("2006-01", value)
	if err != nil {
		return value
	}
	return parsed.Format("Jan 2006")
}

func formatLocation(location modelv1.Location) string {
	return joinNonEmpty(", ", location.City, location.State, location.Country)
}

func joinNonEmpty(sep string, values ...string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, sep)
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func xmlEscape(value string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>
</Types>`

const packageRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const documentRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>
</Relationships>`

const numberingXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:abstractNum w:abstractNumId="0">
<w:lvl w:ilvl="0">
<w:start w:val="1"/>
<w:numFmt w:val="bullet"/>
<w:lvlText w:val="&#8226;"/>
<w:lvlJc w:val="left"/>
<w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr>
</w:lvl>
</w:abstractNum>
<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
</w:numbering>`
