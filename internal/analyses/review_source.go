package analyses

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// reviewSourceView permits only whitespace normalization and whitespace around
// an existing intra-word hyphen. Case, punctuation and every nonspace character
// stay intact. Each normalized byte maps back to the original byte span.
type reviewSourceView struct {
	Text         string
	Starts, Ends []int
}
type sourceRune struct {
	r          rune
	start, end int
}

func canonicalReviewSource(source string) reviewSourceView {
	runes := []sourceRune{}
	for i, r := range source {
		end := i + utf8.RuneLen(r)
		if unicode.IsSpace(r) {
			if len(runes) > 0 && runes[len(runes)-1].r == ' ' {
				runes[len(runes)-1].end = end
			} else {
				runes = append(runes, sourceRune{' ', i, end})
			}
		} else {
			runes = append(runes, sourceRune{r, i, end})
		}
	}
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	remove := map[int]bool{}
	for i, t := range runes {
		if t.r != '-' {
			continue
		}
		a, b := i-1, i+1
		for a >= 0 && runes[a].r == ' ' {
			a--
		}
		for b < len(runes) && runes[b].r == ' ' {
			b++
		}
		if a >= 0 && b < len(runes) && isWord(runes[a].r) && isWord(runes[b].r) {
			for j := a + 1; j < b; j++ {
				if runes[j].r == ' ' {
					remove[j] = true
				}
			}
		}
	}
	var out strings.Builder
	v := reviewSourceView{}
	for i, t := range runes {
		if remove[i] {
			continue
		}
		s := string(t.r)
		out.WriteString(s)
		for range []byte(s) {
			v.Starts = append(v.Starts, t.start)
			v.Ends = append(v.Ends, t.end)
		}
	}
	v.Text = out.String()
	return v
}

func sourceSpan(source, quote string) (int, int, error) {
	s, q := canonicalReviewSource(source), strings.TrimSpace(canonicalReviewSource(quote).Text)
	if q == "" {
		return 0, 0, fmt.Errorf("empty source quote")
	}
	matches := [][2]int{}
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
	first, _ := utf8.DecodeRuneInString(q)
	last, _ := utf8.DecodeLastRuneInString(q)
	for cursor := 0; cursor <= len(s.Text)-len(q); {
		rel := strings.Index(s.Text[cursor:], q)
		if rel < 0 {
			break
		}
		a := cursor + rel
		b := a + len(q)
		valid := true
		if a > 0 && word(first) {
			r, _ := utf8.DecodeLastRuneInString(s.Text[:a])
			valid = !word(r)
		}
		if valid && b < len(s.Text) && word(last) {
			r, _ := utf8.DecodeRuneInString(s.Text[b:])
			valid = !word(r)
		}
		if valid {
			matches = append(matches, [2]int{s.Starts[a], s.Ends[b-1]})
		}
		_, step := utf8.DecodeRuneInString(s.Text[a:])
		cursor = a + step
	}
	if len(matches) != 1 {
		return 0, 0, fmt.Errorf("source quote must identify exactly one passage (found %d)", len(matches))
	}
	return matches[0][0], matches[0][1], nil
}

// PreparePresentation resolves trusted positions on the server, never from
// model-supplied offsets. The UI uses these same patch positions for ordering.
func (r *ReviewReport) PreparePresentation(source string) error {
	edits := map[string]ReviewEdit{}
	ordinals := map[string]int{}
	for i, e := range r.Edits {
		edits[e.ID] = e
		ordinals[e.ID] = i
	}
	for i := range r.Sections {
		section := &r.Sections[i]
		section.Items = []ReviewSectionItem{}
		for _, id := range section.EditIDs {
			e, ok := edits[id]
			if !ok {
				return fmt.Errorf("unknown edit %s", id)
			}
			a, b, err := sourceSpan(source, e.Before)
			if e.Operation == "add" || e.Operation == "move" {
				_, a, err = sourceSpan(source, e.InsertAfter)
				b = a
			}
			if err != nil {
				return err
			}
			section.Items = append(section.Items, ReviewSectionItem{EditID: id, RetainedIndex: -1, SourceStart: a, SourceEnd: b, Ordinal: ordinals[id]})
		}
		for j, item := range section.Retained {
			a, b, err := sourceSpan(source, item.SourceQuote)
			if err != nil {
				return err
			}
			section.Items = append(section.Items, ReviewSectionItem{RetainedIndex: j, SourceStart: a, SourceEnd: b})
		}
		// Stable ordering also matches multiple insertions after a shared anchor.
		sortReviewItems(section.Items)
	}
	return nil
}

// Applied only when generating a new report; old stored reports remain usable.
func validateFocusedReviewQuestions(questions []ReviewQuestion) error {
	for _, q := range questions {
		if utf8.RuneCountInString(q.Question) > 260 || len(strings.Fields(q.Question)) > 45 || strings.Count(q.Question, "?") > 1 || strings.Contains(q.Question, ";") {
			return fmt.Errorf("question %s must ask one short question (up to 45 words/260 characters, one question mark); put optional details in why", q.ID)
		}
	}
	return nil
}
