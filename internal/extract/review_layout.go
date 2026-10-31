package extract

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// ExtractReviewLayout reads physical rows for document rebuilding only. It does
// not change the cached source used by scoring or existing report anchors.
func ExtractReviewLayout(ctx context.Context, data []byte, mimeType, fileName string) (text string, err error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if normalizeMimeType(mimeType, fileName, data) != mimePDF {
		return ExtractTextFromBytes(ctx, data, mimeType, fileName)
	}
	defer func() {
		if r := recover(); r != nil {
			text = ""
			err = fmt.Errorf("PDF layout could not be read")
		}
	}()
	reader, e := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return "", e
	}
	plain, e := extractPDFWithLibrary(data)
	if e != nil {
		return "", e
	}
	var out strings.Builder
	for page := 1; page <= reader.NumPage(); page++ {
		if e := ctx.Err(); e != nil {
			return "", e
		}
		// Content applies the page's coordinate transform. GetTextByRow uses text
		// matrix coordinates, which can reverse rows on PDFs with a flipped CTM.
		type row struct {
			y, size float64
			glyphs  []pdf.Text
		}
		rows := []row{}
		glyphs := reader.Page(page).Content().Text
		widths := reviewFontWidths(reader.Page(page))
		sort.SliceStable(glyphs, func(i, j int) bool { return glyphs[i].Y > glyphs[j].Y })
		for _, g := range glyphs {
			// The library calls its decoder with a synthetic newline after TJ arrays;
			// some fonts decode that sentinel as U+FFFD. Remove it only when the plain
			// extraction has no such character. Export independently checks inventory.
			if g.S == "\ufffd" && !strings.ContainsRune(plain, '\ufffd') {
				continue
			}
			// Preserve real space glyphs. Estimating every word boundary from glyph
			// widths can insert spaces inside words when embedded-font widths differ.
			if strings.ContainsAny(g.S, "\n\r\t") || g.S == "" {
				continue
			}
			index := -1
			for i := len(rows) - 1; i >= 0; i-- {
				if math.Abs(rows[i].y-g.Y) <= 1.8 {
					index = i
					break
				}
				if rows[i].y-g.Y > 3 {
					break
				}
			}
			if index < 0 {
				rows = append(rows, row{y: g.Y, size: math.Abs(g.FontSize)})
				index = len(rows) - 1
			}
			rows[index].glyphs = append(rows[index].glyphs, g)
		}
		for i, row := range rows {
			if i > 0 && rows[i-1].y-row.y > math.Max(18, rows[i-1].size*1.6) {
				out.WriteString("\n")
			}
			sort.SliceStable(row.glyphs, func(i, j int) bool { return row.glyphs[i].X < row.glyphs[j].X })
			end := 0.0
			previousX := math.Inf(-1)
			knownEnd := false
			for j, g := range row.glyphs {
				// Some embedded fonts expose zero glyph widths. A jump between text
				// fragments then says nothing about a word boundary; trust real spaces.
				if j > 0 && knownEnd && g.X > previousX+.01 && g.X-end > math.Max(1.2, math.Abs(g.FontSize)*.18) && !strings.HasSuffix(row.glyphs[j-1].S, " ") && !strings.HasPrefix(g.S, " ") {
					out.WriteString(" ")
				}
				out.WriteString(g.S)
				width := math.Abs(g.W)
				known := width > 0
				if !known {
					r, _ := utf8.DecodeRuneInString(g.S)
					if w, ok := widths[g.Font][r]; ok {
						width = w / 1000 * math.Abs(g.FontSize)
						known = true
					}
				}
				if math.Abs(g.X-previousX) < .01 {
					end += width
					knownEnd = knownEnd && known
				} else {
					end = g.X + width
					knownEnd = known
				}
				previousX = g.X
			}
			out.WriteString("\n")
		}
		out.WriteString("\n")
	}
	return out.String(), nil
}
