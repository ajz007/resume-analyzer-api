package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractTextFromBytes_ZipDocxNormalizes(t *testing.T) {
	path := filepath.Join("..", "..", "resume", "render", "testdata", "template.docx")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test docx: %v", err)
	}

	if _, err := ExtractTextFromBytes(context.Background(), data, "application/zip", "test.docx"); err != nil {
		t.Fatalf("expected docx to extract from zip mime, got error: %v", err)
	}
}

func TestExtractTextFromBytes_RealZipRejected(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("notes.txt")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	_, err = ExtractTextFromBytes(context.Background(), buf.Bytes(), "application/zip", "notes.zip")
	if err == nil {
		t.Fatal("expected unsupported mime error for zip")
	}
	if !strings.Contains(err.Error(), "unsupported mime type: application/zip") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractTextFromBytes_PDFFallsBackWhenPrimaryBlank(t *testing.T) {
	restore := stubPDFExtractors(
		func(data []byte) (string, error) { return "", nil },
		func(ctx context.Context, data []byte) (string, error) { return "fallback resume text", nil },
	)
	defer restore()

	got, err := ExtractTextFromBytes(context.Background(), []byte("%PDF"), "application/pdf", "resume.pdf")
	if err != nil {
		t.Fatalf("expected fallback extraction to pass, got %v", err)
	}
	if got != "fallback resume text" {
		t.Fatalf("expected fallback text, got %q", got)
	}
}

func TestExtractTextFromBytes_PDFRejectsBlankExtraction(t *testing.T) {
	restore := stubPDFExtractors(
		func(data []byte) (string, error) { return "", nil },
		func(ctx context.Context, data []byte) (string, error) { return " \n\t", nil },
	)
	defer restore()

	_, err := ExtractTextFromBytes(context.Background(), []byte("%PDF"), "application/pdf", "resume.pdf")
	if err == nil {
		t.Fatal("expected blank PDF extraction to fail")
	}
	if !strings.Contains(err.Error(), "produced no text") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractTextFromBytes_PDFFallsBackWhenPrimaryErrors(t *testing.T) {
	restore := stubPDFExtractors(
		func(data []byte) (string, error) { return "", errors.New("primary failed") },
		func(ctx context.Context, data []byte) (string, error) { return "fallback resume text", nil },
	)
	defer restore()

	got, err := ExtractTextFromBytes(context.Background(), []byte("%PDF"), "application/pdf", "resume.pdf")
	if err != nil {
		t.Fatalf("expected fallback extraction to pass, got %v", err)
	}
	if got != "fallback resume text" {
		t.Fatalf("expected fallback text, got %q", got)
	}
}

func TestSaveExtractedRejectsBlankText(t *testing.T) {
	store := &capturingSaver{}
	err := saveExtracted(context.Background(), store, "resume.pdf.extracted.txt", " \n\t")
	if err == nil {
		t.Fatal("expected blank extracted text to be rejected")
	}
	if store.calls != 0 {
		t.Fatalf("expected blank text not to be cached, got %d save calls", store.calls)
	}
}

func TestPDFToTextBinaryUsesConfiguredPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pdftotext")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake pdftotext: %v", err)
	}
	t.Setenv("PDFTOTEXT_PATH", path)

	got, err := pdftotextBinary()
	if err != nil {
		t.Fatalf("expected configured pdftotext path, got %v", err)
	}
	if got != path {
		t.Fatalf("expected configured path %q, got %q", path, got)
	}
}

func TestPDFToTextBinaryRejectsBadConfiguredPath(t *testing.T) {
	t.Setenv("PDFTOTEXT_PATH", filepath.Join(t.TempDir(), "missing-pdftotext"))

	_, err := pdftotextBinary()
	if err == nil {
		t.Fatal("expected invalid configured pdftotext path to fail")
	}
	if !strings.Contains(err.Error(), "PDFTOTEXT_PATH") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stubPDFExtractors(primary func([]byte) (string, error), fallback func(context.Context, []byte) (string, error)) func() {
	oldPrimary := extractPDFPlainText
	oldFallback := extractPDFWithFallback
	extractPDFPlainText = primary
	extractPDFWithFallback = fallback
	return func() {
		extractPDFPlainText = oldPrimary
		extractPDFWithFallback = oldFallback
	}
}

type capturingSaver struct {
	calls int
}

func (s *capturingSaver) Save(ctx context.Context, userId string, fileName string, r io.Reader) (string, int64, string, error) {
	n, err := io.Copy(io.Discard, r)
	return "key", n, "text/plain", err
}

func (s *capturingSaver) Open(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (s *capturingSaver) SaveWithKey(ctx context.Context, storageKey string, contentType string, r io.Reader) (int64, error) {
	s.calls++
	n, err := io.Copy(io.Discard, r)
	return n, err
}
