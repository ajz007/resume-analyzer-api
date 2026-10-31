package extract

import (
	"github.com/ledongthuc/pdf"
	"strings"
	"unicode/utf8"
)

// The upstream library reads simple-font Widths but not Type0 descendant W
// arrays. Recover those metrics so positioned chunks are not mistaken for
// separate words, while still separating real column gaps.
func reviewFontWidths(page pdf.Page) map[string]map[rune]float64 {
	result := map[string]map[rune]float64{}
	for _, name := range page.Fonts() {
		font := page.Font(name)
		desc := font.V.Key("DescendantFonts").Index(0)
		w := desc.Key("W")
		if w.Len() == 0 {
			continue
		}
		widths := map[rune]float64{}
		encoder := font.Encoder()
		add := func(cid int, width float64) {
			if cid < 0 || cid > 65535 {
				return
			}
			decoded := encoder.Decode(string([]byte{byte(cid >> 8), byte(cid)}))
			if utf8.RuneCountInString(decoded) != 1 {
				return
			}
			r, _ := utf8.DecodeRuneInString(decoded)
			if r != '\ufffd' {
				widths[r] = width
			}
		}
		for i := 0; i < w.Len(); {
			cid := int(w.Index(i).Int64())
			next := w.Index(i + 1)
			if next.Kind() == pdf.Array {
				for j := 0; j < next.Len(); j++ {
					add(cid+j, next.Index(j).Float64())
				}
				i += 2
			} else {
				end := int(next.Int64())
				width := w.Index(i + 2).Float64()
				if cid >= 0 && end >= cid && end <= 65535 {
					for j := cid; j <= end; j++ {
						add(j, width)
					}
				}
				i += 3
			}
		}
		base := font.BaseFont()
		if i := strings.Index(base, "+"); i >= 0 {
			base = base[i+1:]
		}
		result[base] = widths
	}
	return result
}
