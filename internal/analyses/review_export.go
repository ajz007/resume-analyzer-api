package analyses

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"resume-backend/internal/extract"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"resume-backend/internal/shared/server/middleware"
)

// RenderReviewedResume preserves all unedited extracted source text. It does not
// reconstruct candidate facts with another model or retain the uploaded layout.
func RenderReviewedResume(source string, result map[string]any, selected *[]string, layout ...string) ([]byte, error) {
	state, err := reviewState(result)
	if err != nil {
		return nil, err
	}
	report, requirements, err := mergedReview(result, state)
	if err != nil {
		return nil, err
	}
	if err = report.Validate(source, requirements, state.Answers); err != nil {
		return nil, err
	}
	if selected != nil {
		available := map[string]bool{}
		for _, e := range report.Edits {
			available[e.ID] = true
		}
		chosen := map[string]bool{}
		for _, id := range *selected {
			if !available[id] || chosen[id] {
				return nil, fmt.Errorf("unknown or duplicated selected edit")
			}
			chosen[id] = true
		}
		filtered := []ReviewEdit{}
		for _, e := range report.Edits {
			if chosen[e.ID] {
				filtered = append(filtered, e)
			}
		}
		report.Edits = filtered
	}
	layoutText := source
	if len(layout) > 0 {
		layoutText = layout[0]
	}
	revised, err := report.ApplyToLayout(source, layoutText)
	if err != nil {
		return nil, err
	}
	return renderTextDOCX(revised)
}

func renderTextDOCX(text string) ([]byte, error) {
	text = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || isReviewBullet(r) || (r >= 0x20 && unicode.IsGraphic(r)) {
			return r
		}
		return -1
	}, text)
	var document strings.Builder
	document.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	paragraphs := reviewParagraphs(text)
	for i, p := range paragraphs {
		style := "Normal"
		if p.Heading {
			style = "Heading1"
		} else if i == 0 && len(p.Text) < 80 {
			style = "Title"
		}
		var escaped bytes.Buffer
		if err := xml.EscapeText(&escaped, []byte(p.Text)); err != nil {
			return nil, err
		}
		document.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + style + `"/>`)
		if !p.Bullet && (reviewRoleLine.MatchString(p.Text) || reviewDateLine.MatchString(p.Text) || (i+1 < len(paragraphs) && reviewRoleLine.MatchString(paragraphs[i+1].Text))) {
			document.WriteString(`<w:keepNext/><w:keepLines/>`)
		}
		if p.Bullet {
			document.WriteString(`<w:keepLines/>`)
			document.WriteString(`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr>`)
		}
		document.WriteString(`</w:pPr><w:r><w:t xml:space="preserve">` + escaped.String() + `</w:t></w:r></w:p>`)
	}
	document.WriteString(`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="850" w:right="850" w:bottom="850" w:left="850"/></w:sectPr></w:body></w:document>`)
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	files := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rIdNumbering" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/></Relationships>`},
		{"word/styles.xml", `<?xml version="1.0" encoding="UTF-8"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Arial" w:hAnsi="Arial"/><w:color w:val="000000"/><w:sz w:val="21"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="60" w:line="240" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style><w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="0" w:after="120"/></w:pPr><w:rPr><w:b/><w:color w:val="000000"/><w:sz w:val="34"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="180" w:after="90"/></w:pPr><w:rPr><w:b/><w:color w:val="000000"/><w:sz w:val="25"/></w:rPr></w:style></w:styles>`},
		{"word/numbering.xml", `<?xml version="1.0" encoding="UTF-8"?><w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="singleLevel"/><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/><w:lvlJc w:val="left"/><w:pPr><w:tabs><w:tab w:val="num" w:pos="280"/></w:tabs><w:ind w:left="280" w:hanging="220"/></w:pPr></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num></w:numbering>`},
		{"word/document.xml", document.String()},
	}
	for _, f := range files {
		entry, err := writer.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err = entry.Write([]byte(f.body)); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (h *Handler) downloadReviewedResume(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	analysis, err := h.Svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil || analysis.UserID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
		return
	}
	if analysis.Status != StatusCompleted {
		c.JSON(http.StatusConflict, gin.H{"error": "report is not complete"})
		return
	}
	_, locked, _, err := h.applyPaywall(c.Request.Context(), userID, analysis.ID, analysis.Result)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify report access"})
		return
	}
	if locked {
		c.JSON(http.StatusForbidden, gin.H{"error": "unlock the report before exporting"})
		return
	}
	var request struct {
		EditIDs       *[]string `json:"editIds"`
		ReviewVersion *int      `json:"reviewVersion"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid edit selection"})
		return
	}
	state, stateErr := reviewState(analysis.Result)
	if stateErr != nil || (state.Version > 0 && (request.ReviewVersion == nil || *request.ReviewVersion != state.Version)) {
		c.JSON(http.StatusConflict, gin.H{"error": "review changed; reload before downloading"})
		return
	}
	doc, err := h.DocRepo.GetByID(c.Request.Context(), userID, analysis.DocumentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source document not found"})
		return
	}
	source, err := loadText(c.Request.Context(), h.Svc.Store, doc.ExtractedTextKey)
	if err != nil || strings.TrimSpace(source) == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "original extracted text is unavailable"})
		return
	}
	layoutText := source
	if doc.StorageKey != "" {
		file, openErr := h.Svc.Store.Open(c.Request.Context(), doc.StorageKey)
		if openErr != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "original document is unavailable for rebuilding the resume"})
			return
		}
		fileBytes, readErr := io.ReadAll(io.LimitReader(file, 25*1024*1024+1))
		file.Close()
		if readErr != nil || len(fileBytes) > 25*1024*1024 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "original document could not be read"})
			return
		}
		originalText, originalErr := extract.ExtractTextFromBytes(c.Request.Context(), fileBytes, doc.MimeType, doc.FileName)
		if originalErr != nil || canonicalReviewSource(originalText).Text != canonicalReviewSource(source).Text {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "original document no longer matches the analyzed resume"})
			return
		}
		layoutText, err = extract.ExtractReviewLayout(c.Request.Context(), fileBytes, doc.MimeType, doc.FileName)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "original document layout could not be rebuilt safely"})
			return
		}
	}
	data, err := RenderReviewedResume(source, analysis.Result, request.EditIDs, layoutText)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "edits could not be safely applied to the original text"})
		return
	}
	c.Header("Content-Disposition", `attachment; filename="reviewed-resume.docx"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", data)
}
