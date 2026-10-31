package analyses

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"resume-backend/internal/llm"
)

// AnalysisResultV3 preserves the established diagnostic/scoring contract while
// storing customer-facing edits once. The legacy advice arrays must be empty.
type AnalysisResultV3 struct {
	AnalysisResultV2_4
	Report ReviewReport `json:"report"`
}

type ReviewReport struct {
	SchemaVersion        string           `json:"schemaVersion"`
	Scope                string           `json:"scope"`
	Priorities           []ReviewPriority `json:"priorities"`
	Edits                []ReviewEdit     `json:"edits"`
	Sections             []ReviewSection  `json:"sections"`
	Questions            []ReviewQuestion `json:"questions"`
	Outcomes             ReviewOutcomes   `json:"outcomes"`
	Cleanup              []ReviewNote     `json:"cleanup"`
	InterviewPreparation []string         `json:"interviewPreparation"`
	Coverage             []ReviewCoverage `json:"coverage"`
}
type ReviewPriority struct {
	Title   string   `json:"title"`
	Reason  string   `json:"reason"`
	EditIDs []string `json:"editIds"`
}
type ReviewEdit struct {
	ID                string   `json:"id"`
	Operation         string   `json:"operation"`
	Section           string   `json:"section"`
	Before            string   `json:"before"`
	After             string   `json:"after"`
	InsertAfter       string   `json:"insertAfter"`
	Reason            string   `json:"reason"`
	SourceQuotes      []string `json:"sourceQuotes"`
	RequirementIDs    []string `json:"requirementIds"`
	ConfirmedAnswerID string   `json:"confirmedAnswerId,omitempty" review:"derived"`
}
type ReviewSectionItem struct {
	EditID        string `json:"editId"`
	RetainedIndex int    `json:"retainedIndex"`
	SourceStart   int    `json:"sourceStart"`
	SourceEnd     int    `json:"sourceEnd"`
	Ordinal       int    `json:"-"`
}

func sortReviewItems(items []ReviewSectionItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.SourceStart != b.SourceStart {
			return a.SourceStart < b.SourceStart
		}
		if a.SourceEnd != b.SourceEnd {
			return a.SourceEnd < b.SourceEnd
		}
		return a.Ordinal < b.Ordinal
	})
}

type ReviewSection struct {
	Items    []ReviewSectionItem `json:"items,omitempty" review:"derived"`
	Heading  string              `json:"heading"`
	EditIDs  []string            `json:"editIds"`
	Retained []ReviewRetained    `json:"retained"`
}
type ReviewRetained struct {
	Text        string `json:"text"`
	SourceQuote string `json:"sourceQuote"`
}
type ReviewQuestion struct {
	ID             string   `json:"id"`
	Question       string   `json:"question"`
	Why            string   `json:"why"`
	EditIDs        []string `json:"editIds"`
	RequirementIDs []string `json:"requirementIds,omitempty"`
}
type ReviewOutcomes struct {
	ClearerNow           []string `json:"clearerNow"`
	NeedsConfirmation    []string `json:"needsConfirmation"`
	StillUnresolved      []string `json:"stillUnresolved"`
	OptionalImprovements []string `json:"optionalImprovements,omitempty"`
}
type ReviewNote struct {
	Text        string `json:"text"`
	Severity    string `json:"severity"`
	SourceQuote string `json:"sourceQuote"`
}
type ReviewCoverage struct {
	SourceQuote string `json:"sourceQuote"`
	Disposition string `json:"disposition"`
	Reason      string `json:"reason"`
}

var unfinishedReviewText = regexp.MustCompile(`(?i)\[[^\]]+\]|\bTO-FILL\b|\bX%|\breplace with\b`)
var reviewNumbers = regexp.MustCompile(`[0-9]+(?:[.,][0-9]+)*%?`)

func (r *ReviewReport) Validate(source string, requirements []JobPriorityV1, answerSets ...[]ReviewAnswer) error {
	if r == nil || r.SchemaVersion != "3" || r.Scope != "SELECTED_PASSAGES" {
		return fmt.Errorf("report requires schemaVersion=3 and scope=SELECTED_PASSAGES")
	}
	if len(r.Priorities) > 3 {
		return fmt.Errorf("report has more than three priorities")
	}
	if r.Priorities == nil || r.Edits == nil || r.Sections == nil || r.Questions == nil || r.Coverage == nil || r.Cleanup == nil || r.InterviewPreparation == nil || r.Outcomes.ClearerNow == nil || r.Outcomes.NeedsConfirmation == nil || r.Outcomes.StillUnresolved == nil {
		return fmt.Errorf("report collections must be arrays")
	}
	edits := map[string]ReviewEdit{}
	reqs := map[string]bool{}
	for _, req := range requirements {
		reqs[req.ID] = true
	}
	for _, e := range r.Edits {
		if strings.TrimSpace(e.ID) == "" || edits[e.ID].ID != "" {
			return fmt.Errorf("edit IDs must be nonempty and unique")
		}
		edits[e.ID] = e
		if strings.TrimSpace(e.Section) == "" || strings.TrimSpace(e.Reason) == "" {
			return fmt.Errorf("edit %s requires section and reason", e.ID)
		}
		switch e.Operation {
		case "replace", "remove", "retain", "add", "move", "merge":
		default:
			return fmt.Errorf("edit %s has invalid operation", e.ID)
		}
		if e.Operation != "add" && strings.TrimSpace(e.Before) == "" {
			return fmt.Errorf("edit %s requires before", e.ID)
		}
		if e.Operation == "add" && e.Before != "" {
			return fmt.Errorf("added passage must not replace original text")
		}
		if e.Operation == "remove" {
			if e.After != "" {
				return fmt.Errorf("removed passage must have empty after")
			}
		} else if strings.TrimSpace(e.After) == "" || unfinishedReviewText.MatchString(e.After) {
			return fmt.Errorf("edit %s is not ready to use", e.ID)
		}
		if e.Operation == "retain" && e.After != e.Before {
			return fmt.Errorf("retained passage must be unchanged")
		}
		if (e.Operation == "add" || e.Operation == "move") && strings.TrimSpace(e.InsertAfter) == "" {
			return fmt.Errorf("edit %s requires an original insertion anchor", e.ID)
		}
		if e.Operation != "add" && e.Operation != "move" && e.InsertAfter != "" {
			return fmt.Errorf("insertion anchor only applies to add/move")
		}
		if len(e.SourceQuotes) == 0 {
			return fmt.Errorf("edit %s requires source evidence", e.ID)
		}
		support := strings.Join(e.SourceQuotes, " ")
		if e.ConfirmedAnswerID != "" {
			authorized := false
			for _, answers := range answerSets {
				for _, answer := range answers {
					if answer.ID == e.ConfirmedAnswerID && answer.Kind == "experience" && answer.Edit != nil && answer.Edit.ID == e.ID && answer.Delta.Passage == e.After && strings.Contains(answer.Text, answer.Delta.AnswerQuote) && answer.Delta.AnswerQuote != "" {
						if len(e.RequirementIDs) != 1 || e.RequirementIDs[0] != answer.RequirementID {
							return fmt.Errorf("answer requirement mismatch")
						}
						support += " " + answer.Delta.AnswerQuote
						authorized = true
					}
				}
			}
			if !authorized {
				return fmt.Errorf("edit requires its saved confirmed answer")
			}
		}
		supportedNumbers := map[string]bool{}
		for _, n := range reviewNumbers.FindAllString(support, -1) {
			supportedNumbers[strings.ReplaceAll(n, ",", "")] = true
		}
		for _, n := range reviewNumbers.FindAllString(e.After, -1) {
			if !supportedNumbers[strings.ReplaceAll(n, ",", "")] {
				return fmt.Errorf("edit %s introduces unsupported number %s", e.ID, n)
			}
		}
		for _, verb := range []string{"architected", "owned", "led", "managed"} {
			pattern := regexp.MustCompile(`(?i)\b` + verb + `\b`)
			if pattern.MatchString(e.After) && !pattern.MatchString(support) {
				return fmt.Errorf("edit %s introduces unsupported ownership verb %s", e.ID, verb)
			}
		}
		for _, id := range e.RequirementIDs {
			if !reqs[id] {
				return fmt.Errorf("edit %s references unknown requirement %s", e.ID, id)
			}
		}
		if source != "" {
			for _, q := range e.SourceQuotes {
				if _, _, err := sourceSpan(source, q); err != nil {
					return fmt.Errorf("edit %s evidence: %w", e.ID, err)
				}
			}
			if e.Operation != "add" {
				if _, _, err := sourceSpan(source, e.Before); err != nil {
					return fmt.Errorf("edit %s original: %w", e.ID, err)
				}
			}
		}
	}
	refs := func(ids []string) error {
		for _, id := range ids {
			if _, ok := edits[id]; !ok {
				return fmt.Errorf("unknown edit reference %s", id)
			}
		}
		return nil
	}
	for _, p := range r.Priorities {
		if strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Reason) == "" {
			return fmt.Errorf("priority title/reason required")
		}
		if err := refs(p.EditIDs); err != nil {
			return err
		}
	}
	occurrences := map[string]int{}
	for _, s := range r.Sections {
		if strings.TrimSpace(s.Heading) == "" {
			return fmt.Errorf("section heading required")
		}
		if err := refs(s.EditIDs); err != nil {
			return err
		}
		for _, id := range s.EditIDs {
			if edits[id].Operation == "remove" {
				return fmt.Errorf("removed edit cannot appear in revised passages")
			}
			occurrences[id]++
		}
		for _, retained := range s.Retained {
			if retained.Text == "" || retained.Text != retained.SourceQuote {
				return fmt.Errorf("retained text must equal its original source quote")
			}
			if source != "" {
				if _, _, err := sourceSpan(source, retained.SourceQuote); err != nil {
					return fmt.Errorf("retained passage: %w", err)
				}
			}
		}
	}
	for id, e := range edits {
		if e.Operation != "remove" && occurrences[id] != 1 {
			return fmt.Errorf("edit %s must appear exactly once in revised passages", id)
		}
	}
	questionIDs := map[string]bool{}
	for _, q := range r.Questions {
		if strings.TrimSpace(q.ID) == "" || questionIDs[q.ID] || strings.TrimSpace(q.Question) == "" || strings.TrimSpace(q.Why) == "" {
			return fmt.Errorf("question ID, question and purpose required")
		}
		questionIDs[q.ID] = true
		for _, id := range q.RequirementIDs {
			if !reqs[id] {
				return fmt.Errorf("question references unknown requirement")
			}
		}
		if err := refs(q.EditIDs); err != nil {
			return err
		}
	}
	for _, c := range r.Coverage {
		if !isAllowedString(c.Disposition, "retained", "rewritten", "moved", "merged", "removed", "not_selected") || strings.TrimSpace(c.Reason) == "" {
			return fmt.Errorf("invalid coverage disposition")
		}
		if source != "" {
			if _, _, err := sourceSpan(source, c.SourceQuote); err != nil {
				return fmt.Errorf("coverage: %w", err)
			}
		}
	}
	for _, note := range r.Cleanup {
		if strings.TrimSpace(note.Text) == "" || !isAllowedString(note.Severity, "high", "medium", "low") {
			return fmt.Errorf("invalid remaining concern")
		}
		if source != "" && note.SourceQuote != "" {
			if _, _, err := sourceSpan(source, note.SourceQuote); err != nil {
				return fmt.Errorf("cleanup evidence: %w", err)
			}
		}
	}
	if source != "" {
		if _, err := r.ApplyToSource(source); err != nil {
			return err
		}
		if err := r.PreparePresentation(source); err != nil {
			return err
		}
	}
	return nil
}

// ApplyToSource changes only anchored original passages. Everything else is
// retained verbatim; ambiguous anchors and overlapping changes fail closed.
func (r ReviewReport) ApplyToSource(source string) (string, error) {
	type patch struct {
		start, end int
		text       string
	}
	patches := []patch{}
	for _, e := range r.Edits {
		if e.Operation == "retain" {
			continue
		}
		if e.Operation != "add" {
			a, b, err := sourceSpan(source, e.Before)
			if err != nil {
				return "", fmt.Errorf("edit %s: %w", e.ID, err)
			}
			replacement := e.After
			if e.Operation == "move" {
				replacement = ""
			}
			patches = append(patches, patch{a, b, replacement})
		}
		if e.Operation == "add" || e.Operation == "move" {
			_, b, err := sourceSpan(source, e.InsertAfter)
			if err != nil {
				return "", fmt.Errorf("edit %s anchor: %w", e.ID, err)
			}
			patches = append(patches, patch{b, b, "\n" + e.After})
		}
	}
	sort.SliceStable(patches, func(i, j int) bool {
		if patches[i].start == patches[j].start {
			return patches[i].end < patches[j].end
		}
		return patches[i].start < patches[j].start
	})
	previousEnd := -1
	for _, p := range patches {
		if p.start < previousEnd {
			return "", fmt.Errorf("review edits overlap or insert into a changed passage")
		}
		previousEnd = p.end
	}
	var out strings.Builder
	cursor := 0
	for _, p := range patches {
		out.WriteString(source[cursor:p.start])
		out.WriteString(p.text)
		cursor = p.end
	}
	out.WriteString(source[cursor:])
	return out.String(), nil
}

func ValidateV3WithRetry(ctx context.Context, client llm.Client, input llm.AnalyzeInput) (json.RawMessage, error) {
	input.ResponseSchema = AnalysisV3JSONSchema()
	originalSource := input.ResumeText
	// Give the model readable whitespace, retaining the original for validation.
	input.ResumeText = canonicalReviewSource(originalSource).Text
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := client.AnalyzeResume(ctx, input)
		if err != nil {
			return nil, err
		}
		var parsed AnalysisResultV3
		err = json.Unmarshal(raw, &parsed)
		if err == nil && parsed.Meta.PromptVersion != "v3" {
			err = fmt.Errorf("meta.promptVersion must be v3")
		}
		if err == nil && parsed.Meta.JobDescriptionProvided != (strings.TrimSpace(input.JobDescription) != "") {
			err = fmt.Errorf("job-description mode does not match input")
		}
		if err == nil {
			err = parsed.AnalysisResultV2_4.Validate()
		}
		if err == nil && (len(parsed.Issues) > 0 || len(parsed.BulletRewrites) > 0 || len(parsed.MissingInformation) > 0 || len(parsed.FixThisFirst) > 0 || len(parsed.ActionPlan.QuickWins) > 0 || len(parsed.ActionPlan.MediumEffort) > 0 || len(parsed.ActionPlan.DeepFixes) > 0) {
			err = fmt.Errorf("legacy advice must be empty; use the canonical report")
		}
		if err == nil {
			err = ValidateContentV2_4(&parsed.AnalysisResultV2_4)
		}
		if err == nil {
			err = parsed.Report.Validate(originalSource, parsed.JobRequirementProfile.TopPriorities)
		}
		if err == nil {
			err = validateFocusedReviewQuestions(parsed.Report.Questions)
		}
		if err == nil {
			err = parsed.Report.PreparePresentation(originalSource)
		}
		if err == nil {
			return json.Marshal(parsed)
		}
		last = err
		// Keep the original resume and JD on repair. Never ask a model to repair
		// factual claims without the evidence that constrains those claims.
		ctx = llm.WithExtraSystemMessage(ctx, "The prior response failed validation: "+err.Error()+". Generate the complete corrected response from the original source. Do not invent facts or placeholders. JSON only.")
	}
	return nil, fmt.Errorf("report validation failed after bounded repair: %w", last)
}

// The strict schema is derived from the same Go types consumed by validation,
// avoiding a second hand-maintained copy of the provider contract.
func AnalysisV3JSONSchema() json.RawMessage {
	raw, _ := json.Marshal(strictTypeSchema(reflect.TypeOf(AnalysisResultV3{})))
	return raw
}
func strictTypeSchema(t reflect.Type) map[string]any {
	switch t.Kind() {
	case reflect.Struct:
		properties := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				embedded := strictTypeSchema(f.Type)
				for k, v := range embedded["properties"].(map[string]any) {
					properties[k] = v
					required = append(required, k)
				}
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" || f.Tag.Get("review") == "derived" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			properties[name] = strictTypeSchema(f.Type)
			required = append(required, name)
		}
		sort.Strings(required)
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": strictTypeSchema(t.Elem())}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	default:
		return map[string]any{"type": "string"}
	}
}
