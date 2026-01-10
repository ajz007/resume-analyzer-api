package extract

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/nguyenthenguyen/docx"

	"resume-backend/internal/shared/storage/object"
)

const (
	mimePDF  = "application/pdf"
	mimeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

// ExtractText pulls text from a stored object and persists a derived .extracted.txt copy.
// Libraries used: github.com/ledongthuc/pdf (PDF) and github.com/nguyenthenguyen/docx (DOCX).
func ExtractText(ctx context.Context, store object.ObjectStore, fileKey string, mimeType string) (string, error) {
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

	var text string
	switch mimeType {
	case mimePDF:
		text, err = extractPDF(raw)
	case mimeDOCX:
		text, err = extractDOCX(raw)
	default:
		return "", fmt.Errorf("extract text key=%s mime=%s: unsupported mime type", fileKey, mimeType)
	}
	if err != nil {
		return "", fmt.Errorf("extract text key=%s mime=%s: %w", fileKey, mimeType, err)
	}

	extractedKey := fileKey + ".extracted.txt"
	if err := saveExtracted(ctx, store, extractedKey, text); err != nil {
		return "", fmt.Errorf("extract text key=%s mime=%s: %w", fileKey, mimeType, err)
	}

	return text, nil
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
	reader := bytes.NewReader(data)
	doc, err := docx.ReadDocxFromMemory(reader, int64(len(data)))
	if err != nil {
		return "", err
	}
	defer doc.Close()

	raw := doc.Editable().GetContent()
	return stripDocxXML(raw), nil
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
