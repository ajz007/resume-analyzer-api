package extract

import (
	"archive/zip"
	"bytes"
	"context"
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
