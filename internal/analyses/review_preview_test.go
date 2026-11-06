package analyses

import (
	"resume-backend/internal/documents"
	"testing"
)

func TestReviewedPreviewUsesWordStructure(t *testing.T) {
	data, err := renderTextDOCX("Example Name\n\nProfessional Summary\n\nBuilt services.\n\n• Improved latency.")
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := previewDOCXBlocks(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []reviewPreviewBlock{{"title", "Example Name"}, {"heading", "Professional Summary"}, {"paragraph", "Built services."}, {"bullet", "Improved latency."}}
	if len(blocks) != len(want) {
		t.Fatal(blocks)
	}
	for i, block := range blocks {
		if block != want[i] {
			t.Fatalf("block %d: %+v", i, block)
		}
	}
}

func TestTailoredStorageKeyUsesSourceDirectory(t *testing.T) {
	source := documents.Document{StorageKey: "documents/owner/original/file.pdf"}
	if got := tailoredStorageKey(source, "id"); got != "documents/owner/original/tailored-id.docx" {
		t.Fatal(got)
	}
}
