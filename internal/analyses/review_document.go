package analyses

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type reviewParagraph struct {
	Text            string
	Bullet, Heading bool
}

var reviewDateLine = regexp.MustCompile(`(?i)(?:Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:tember)?|Sept(?:ember)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\s+20[0-9]{2}\b`)
var reviewFieldLine = regexp.MustCompile(`^[A-Za-z][A-Za-z &/+.-]{0,45}:`)
var reviewGradeLocation = regexp.MustCompile(`([0-9])([A-Z][a-z]{2,})`)
var reviewRoleLine = regexp.MustCompile(`^(?:(?:Senior|Lead|Staff|Principal|Associate|Module|Full Stack|Solutions)\s+)?(?:Software|Application|Backend|Frontend|Systems?|Solutions|Full Stack|Module)\s+(?:Engineer|Developer|Architect|Analyst|Lead)`)
var reviewEmailPhone = regexp.MustCompile(`([[:alnum:]._%+-]+@[[:alnum:].-]+\.[[:alpha:]]{2,})(\+[0-9])`)
var reviewContactLabel = regexp.MustCompile(`([0-9])((?:LinkedIn|GitHub|Github|Coding Profile))`)
var reviewAdjacentLabels = regexp.MustCompile(`(LinkedIn|GitHub|Github)(GitHub|Github|Coding Profile)`)

func reviewMetadataSpacing(line string) string {
	line = reviewEmailPhone.ReplaceAllString(line, "$1 $2")
	line = reviewContactLabel.ReplaceAllString(line, "$1 $2")
	for i := 0; i < 2; i++ {
		line = reviewAdjacentLabels.ReplaceAllString(line, "$1 $2")
	}
	if strings.Contains(line, "GPA:") {
		line = reviewGradeLocation.ReplaceAllString(line, "$1 $2")
		line = strings.ReplaceAll(line, ";GPA:", "; GPA:")
		line = strings.ReplaceAll(line, ";CGPA:", "; CGPA:")
	}
	if date := reviewDateLine.FindStringIndex(line); date != nil && date[0] > 0 && line[date[0]-1] != ' ' {
		line = line[:date[0]] + " " + line[date[0]:]
	}
	return line
}
func isReviewBullet(r rune) bool { return r == '•' || r == '\uf0b7' || r == '\x7f' || r == '◦' }
func reviewHeading(s string) bool {
	key := strings.ToLower(strings.Join(strings.Fields(s), " "))
	if strings.HasPrefix(key, "selected projects") {
		return true
	}
	for _, h := range []string{"professional summary", "summary", "profile", "professional experience", "work experience", "experience", "technical skills", "core competencies", "skills", "education", "projects", "key achievements", "achievements", "achievements & certification", "certifications & continuous learning", "continuous learning & technical engagement", "personal summary", "certifications", "publications"} {
		if key == h {
			return true
		}
	}
	return false
}

// Physical PDF rows already contain readable words. Reflow only wrapped lines;
// keep blank-line, list and heading boundaries instead of guessing new facts.
func reviewParagraphs(text string) []reviewParagraph {
	result := []reviewParagraph{}
	separate := true
	previousLength := 0
	pendingBullet := false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = reviewMetadataSpacing(strings.TrimSpace(line))
		if line == "" {
			separate = true
			continue
		}
		first, _ := utf8.DecodeRuneInString(line)
		bullet := isReviewBullet(first)
		heading := reviewHeading(line)
		if bullet {
			line = strings.TrimSpace(strings.TrimPrefix(line, string(first)))
			if line == "" {
				pendingBullet = true
				separate = true
				continue
			}
		}
		if pendingBullet {
			bullet = true
			pendingBullet = false
		}
		if strings.Trim(line, ".,;:!?% ") == "" && len(result) > 0 {
			result[len(result)-1].Text += line
			continue
		}
		boundary := heading || reviewDateLine.MatchString(line) || reviewRoleLine.MatchString(line) || strings.Contains(line, "@") || strings.Contains(line, "|") || reviewFieldLine.MatchString(line)
		if !separate && !bullet && !boundary && len(result) > 0 && !result[len(result)-1].Heading && (result[len(result)-1].Bullet || previousLength >= 85) {
			result[len(result)-1].Text += " " + line
		} else {
			result = append(result, reviewParagraph{line, bullet, heading})
		}
		separate = false
		previousLength = utf8.RuneCountInString(line)
	}
	return result
}
func reflowReviewText(layout string) string {
	var blocks []string
	for _, p := range reviewParagraphs(layout) {
		text := p.Text
		if p.Bullet {
			text = "• " + text
		}
		blocks = append(blocks, text)
	}
	return strings.Join(blocks, "\n\n")
}

// A full-document character inventory verifies that a physical-layout view did
// not drop, add or change source content. It may reorder PDF drawing objects.
func reviewCharacters(s string) map[rune]int {
	result := map[rune]int{}
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		if isReviewBullet(r) {
			r = '•'
		}
		result[r]++
	}
	return result
}
func sameReviewCharacters(a, b string) bool {
	aa, bb := reviewCharacters(a), reviewCharacters(b)
	if len(aa) != len(bb) {
		return false
	}
	for r, n := range aa {
		if bb[r] != n {
			return false
		}
	}
	return true
}
func compactReviewSource(s string) reviewSourceView {
	var b strings.Builder
	v := reviewSourceView{}
	for i, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		end := i + utf8.RuneLen(r)
		if isReviewBullet(r) {
			r = '•'
		}
		c := string(r)
		b.WriteString(c)
		for range []byte(c) {
			v.Starts = append(v.Starts, i)
			v.Ends = append(v.Ends, end)
		}
	}
	v.Text = b.String()
	return v
}

// This translation is used only AFTER the source quote has been validated.
// No fuzzy lexical matching: the exact nonspace character sequence must occur
// exactly once in the independent layout view of the same uploaded file.
func layoutSpan(layout, validatedSource string) (int, int, error) {
	view := compactReviewSource(layout)
	q := compactReviewSource(validatedSource).Text
	if q == "" || strings.Count(view.Text, q) != 1 {
		return 0, 0, fmt.Errorf("validated passage has no unique layout location")
	}
	a := strings.Index(view.Text, q)
	return view.Starts[a], view.Ends[a+len(q)-1], nil
}
func paragraphBounds(text string, a, b int) (int, int) {
	start := strings.LastIndex(text[:a], "\n\n")
	if start < 0 {
		start = 0
	} else {
		start += 2
	}
	end := strings.Index(text[b:], "\n\n")
	if end < 0 {
		end = len(text)
	} else {
		end += b
	}
	return start, end
}

// ApplyToLayout preserves the uploaded file's content while rebuilding its
// whitespace/list structure. The original source remains the anchor authority.
func (r ReviewReport) ApplyToLayout(source, layout string) (string, error) {
	if !sameReviewCharacters(source, layout) {
		return "", fmt.Errorf("original file layout differs from the stored source")
	}
	text := reflowReviewText(layout)
	// Reflow may normalize list glyphs, but must not change candidate text.
	if !sameReviewCharacters(source, text) {
		return "", fmt.Errorf("document reflow would change source content")
	}
	type patch struct {
		a, b int
		text string
	}
	patches := []patch{}
	for _, e := range r.Edits {
		if e.Operation == "retain" {
			continue
		}
		originalBullet := false
		if e.Operation != "add" {
			sa, sb, err := sourceSpan(source, e.Before)
			if err != nil {
				return "", err
			}
			a, b, err := layoutSpan(text, source[sa:sb])
			if err != nil {
				return "", err
			}
			ps, pe := paragraphBounds(text, a, b)
			prefix := strings.TrimSpace(text[ps:a])
			first, _ := utf8.DecodeRuneInString(text[ps:pe])
			originalBullet = isReviewBullet(first)
			complete := strings.TrimSpace(text[b:pe]) == "" && (prefix == "" || prefix == "•")
			replacement := e.After
			if e.Operation == "remove" || e.Operation == "move" {
				replacement = ""
				if complete {
					a = ps
					b = pe
				}
			}
			if complete && originalBullet && replacement != "" {
				a = ps
				replacement = "• " + strings.TrimSpace(strings.TrimPrefix(replacement, "•"))
			}
			patches = append(patches, patch{a, b, replacement})
		}
		if e.Operation == "add" || e.Operation == "move" {
			sa, sb, err := sourceSpan(source, e.InsertAfter)
			if err != nil {
				return "", err
			}
			a, b, err := layoutSpan(text, source[sa:sb])
			if err != nil {
				return "", err
			}
			ps, pe := paragraphBounds(text, a, b)
			if strings.TrimSpace(text[b:pe]) != "" {
				return "", fmt.Errorf("insertion anchor must end at a paragraph boundary")
			}
			first, _ := utf8.DecodeRuneInString(text[ps:pe])
			after := e.After
			if originalBullet || (e.Operation == "add" && isReviewBullet(first)) {
				after = "• " + strings.TrimSpace(strings.TrimPrefix(after, "•"))
			}
			patches = append(patches, patch{b, b, "\n\n" + after})
		}
	}
	sort.SliceStable(patches, func(i, j int) bool {
		if patches[i].a == patches[j].a {
			return patches[i].b < patches[j].b
		}
		return patches[i].a < patches[j].a
	})
	var out strings.Builder
	cursor := 0
	for _, p := range patches {
		if p.a < cursor {
			return "", fmt.Errorf("layout edits overlap")
		}
		out.WriteString(text[cursor:p.a])
		out.WriteString(p.text)
		cursor = p.b
	}
	out.WriteString(text[cursor:])
	return out.String(), nil
}
