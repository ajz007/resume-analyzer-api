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

	"github.com/ledongthuc/pdf"

	"resume-backend/internal/shared/storage/object"
)

const (
	mimePDF  = "application/pdf"
	mimeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

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
		return extractPDF(data)
	case mimeDOCX:
		return extractDOCX(data)
	default:
		return "", fmt.Errorf("unsupported mime type: %s", normalized)
	}
}

type keySaver interface {
	SaveWithKey(ctx context.Context, storageKey string, contentType string, r io.Reader) (int64, error)
}

func saveExtracted(ctx context.Context, store object.ObjectStore, key string, text string) error {
	saver, ok := store.(keySaver)
	if !ok {
		return errors.New("object store does not support SaveWithKey")
	}
	reader := strings.NewReader(text)
	_, err := saver.SaveWithKey(ctx, key, "text/plain; charset=utf-8", reader)
	return err
}

func extractPDF(data []byte) (string, error) {
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
