package extract

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func layoutPDF(stream string) []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream)}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, s := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, s)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(offsets), xref)
	return b.Bytes()
}
func TestReviewLayoutUsesTransformedCoordinates(t *testing.T) {
	for _, stream := range []string{
		"BT /F1 12 Tf 1 0 0 1 40 720 Tm (Header) Tj 1 0 0 1 40 700 Tm (Body) Tj ET",
		"1 0 0 -1 0 800 cm BT /F1 12 Tf 1 0 0 1 40 80 Tm (Header) Tj 1 0 0 1 40 100 Tm (Body) Tj ET",
	} {
		text, err := ExtractReviewLayout(context.Background(), layoutPDF(stream), "application/pdf", "source.pdf")
		if err != nil || strings.Index(text, "Header") < 0 || strings.Index(text, "Header") > strings.Index(text, "Body") {
			t.Fatal(text, err)
		}
	}
}

func TestReviewLayoutDoesNotGuessWordBreaksFromMissingWidths(t *testing.T) {
	stream := "BT /F1 12 Tf 1 0 0 1 40 720 Tm [(Microser) -2000 (vices demo)] TJ ET"
	text, err := ExtractReviewLayout(context.Background(), layoutPDF(stream), "application/pdf", "source.pdf")
	if err != nil || !strings.Contains(text, "Microservices demo") {
		t.Fatal(text, err)
	}
}
