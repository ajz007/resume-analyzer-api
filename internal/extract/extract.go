package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"

	"resume-backend/internal/shared/storage/object"
)

const (
	mimePDF  = "application/pdf"
	mimeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

type ParseStatus string

const (
	ParseSuccess       ParseStatus = "PARSE_SUCCESS"
	ParseLowConfidence ParseStatus = "PARSE_LOW_CONFIDENCE"
	ParseFailed        ParseStatus = "PARSE_FAILED"
)

const (
	ParseCodeUnsupportedResumeFormat = "UNSUPPORTED_RESUME_FORMAT"
)

const (
	// These thresholds intentionally catch only obviously unusable extraction.
	// A short but real one-page resume can be compact, so the service treats
	// borderline output as a parse-status response instead of fabricating text.
	minSuccessfulTextRunes    = 120
	minLowConfidenceTextRunes = 80
)

type ParseIssue struct {
	Status                 ParseStatus
	Code                   string
	Title                  string
	Message                string
	Recommendations        []string
	Parser                 string
	FileType               string
	ExtractedCharCount     int
	ATSInsightTitle        string
	ATSInsightMessage      string
	TechnicalDetailForLogs string
}

func (e *ParseIssue) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.TechnicalDetailForLogs) != "" {
		return e.TechnicalDetailForLogs
	}
	return e.Message
}

// ExtractText pulls text from a stored object and persists a derived .extracted.txt copy.
// Libraries used: github.com/ledongthuc/pdf (PDF) and github.com/nguyenthenguyen/docx (DOCX).
func ExtractText(ctx context.Context, store object.ObjectStore, fileKey string, mimeType string, fileName string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	body, err := store.Open(ctx, fileKey)
	if err != nil {
		return "", fmt.Errorf("extract text key=%s mime=%s: %w", fileKey, mimeType, err)
	}
	defer body.Close()

	raw, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("extract text key=%s mime=%s: read: %w", fileKey, mimeType, err)
	}

	text, err := ExtractTextFromBytes(ctx, raw, mimeType, fileName)
	if err != nil {
		return "", fmt.Errorf("extract text key=%s mime=%s: %w", fileKey, mimeType, err)
	}

	extractedKey := fileKey + ".extracted.txt"
	if err := saveExtracted(ctx, store, extractedKey, text); err != nil {
		return "", fmt.Errorf("extract text key=%s mime=%s: %w", fileKey, mimeType, err)
	}

	return text, nil
}

// ExtractTextFromBytes extracts text from an in-memory payload.
func ExtractTextFromBytes(ctx context.Context, data []byte, mimeType string, fileName string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	normalized := normalizeMimeType(mimeType, fileName, data)
	switch normalized {
	case mimePDF:
		return extractPDF(data, normalized)
	case mimeDOCX:
		text, err := extractDOCX(data)
		if err != nil {
			return "", newParseIssue(ParseFailed, "docx", normalized, 0, err.Error())
		}
		if issue := assessTextQuality(text, "docx", normalized, ""); issue != nil {
			return "", issue
		}
		return text, nil
	default:
		return "", fmt.Errorf("unsupported mime type: %s", normalized)
	}
}

type keySaver interface {
	SaveWithKey(ctx context.Context, storageKey string, contentType string, r io.Reader) (int64, error)
}

func saveExtracted(ctx context.Context, store object.ObjectStore, key string, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("extracted text is empty")
	}
	saver, ok := store.(keySaver)
	if !ok {
		return errors.New("object store does not support SaveWithKey")
	}
	reader := strings.NewReader(text)
	_, err := saver.SaveWithKey(ctx, key, "text/plain; charset=utf-8", reader)
	return err
}

var extractPDFPlainText = extractPDFWithLibrary

func extractPDF(data []byte, fileType string) (string, error) {
	text, err := extractPDFPlainText(data)
	if err != nil {
		return "", newParseIssue(ParseFailed, "github.com/ledongthuc/pdf", fileType, 0, err.Error())
	}
	if issue := assessTextQuality(text, "github.com/ledongthuc/pdf", fileType, ""); issue != nil {
		return "", issue
	}
	return text, nil
}

func extractPDFWithLibrary(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	pdfReader, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", err
	}
	plain, err := pdfReader.GetPlainText()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, plain); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func extractDOCX(data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty docx data")
	}
	readerAt := bytes.NewReader(data)
	zr, err := zip.NewReader(readerAt, int64(len(data)))
	if err != nil {
		return "", err
	}

	var docFile *zip.File
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", errors.New("document.xml file not found")
	}

	rc, err := docFile.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	raw, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	return stripDocxXML(string(raw)), nil
}

func assessTextQuality(text string, parser string, fileType string, technicalDetail string) *ParseIssue {
	charCount := utf8.RuneCountInString(strings.TrimSpace(text))
	switch {
	case charCount == 0:
		return newParseIssue(ParseFailed, parser, fileType, charCount, fallbackString(technicalDetail, "extracted text is empty"))
	case charCount < minLowConfidenceTextRunes:
		return newParseIssue(ParseFailed, parser, fileType, charCount, fallbackString(technicalDetail, "extracted text is too small"))
	case charCount < minSuccessfulTextRunes:
		return newParseIssue(ParseLowConfidence, parser, fileType, charCount, fallbackString(technicalDetail, "extracted text is below confidence threshold"))
	default:
		return nil
	}
}

func newParseIssue(status ParseStatus, parser string, fileType string, charCount int, technicalDetail string) *ParseIssue {
	return &ParseIssue{
		Status:             status,
		Code:               ParseCodeUnsupportedResumeFormat,
		Title:              "Unable to reliably read resume",
		Message:            "Your resume appears to use formatting that may be difficult for ATS systems and resume parsers to read.",
		Recommendations:    ParseRecommendations(),
		Parser:             parser,
		FileType:           fileType,
		ExtractedCharCount: charCount,
		ATSInsightTitle:    "Resume Format Warning",
		ATSInsightMessage:  "Your resume format may not be ATS-friendly. If our parser cannot reliably extract text, some ATS platforms may also struggle to process it.",
		TechnicalDetailForLogs: fmt.Sprintf(
			"resume parse %s: code=%s parser=%s file_type=%s extracted_chars=%d detail=%s",
			status,
			ParseCodeUnsupportedResumeFormat,
			parser,
			fileType,
			charCount,
			technicalDetail,
		),
	}
}

func ParseRecommendations() []string {
	return []string{
		"Upload a DOCX version",
		"Export as a text-based PDF",
		"Avoid Canva-style or image-heavy layouts",
		"Try a simpler ATS-friendly format",
	}
}

func fallbackString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func stripDocxXML(raw string) string {
	decoder := xml.NewDecoder(strings.NewReader(raw))
	var buf strings.Builder
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return raw
		}
		switch t := tok.(type) {
		case xml.CharData:
			buf.WriteString(string(t))
		case xml.EndElement:
			if t.Name.Local == "p" || t.Name.Local == "br" {
				if last := buf.Len(); last > 0 {
					buf.WriteString("\n")
				}
			}
		}
	}
	return strings.TrimSpace(buf.String())
}

func normalizeMimeType(mimeType string, fileName string, data []byte) string {
	clean := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	if clean != "application/zip" {
		return clean
	}

	if mapped := mapOOXMLFromZip(data); mapped != "" {
		return mapped
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".docx":
		return mimeDOCX
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	default:
		return clean
	}
}

func mapOOXMLFromZip(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	readerAt := bytes.NewReader(data)
	zr, err := zip.NewReader(readerAt, int64(len(data)))
	if err != nil {
		return ""
	}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		switch name {
		case "word/document.xml":
			return mimeDOCX
		case "xl/workbook.xml":
			return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		case "ppt/presentation.xml":
			return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
		}
	}
	return ""
}
