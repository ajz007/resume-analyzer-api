package recommendations

import (
	"sort"
	"strings"
	"unicode"
)

// GenerateRecommendations builds deterministic recommendations from a normalized analysis result.
func GenerateRecommendations(input Input) []Recommendation {
	candidates := make([]Recommendation, 0, 16)
	mappers := []func(Input) []Recommendation{
		func(in Input) []Recommendation {
			return fromIssues(in.Issues)
		},
		func(in Input) []Recommendation {
			return fromMissingJDKeywords(in.MissingJDKeywords)
		},
		func(in Input) []Recommendation {
			return fromFormattingIssues(in.FormattingIssues)
		},
		func(in Input) []Recommendation {
			return fromActionPlan(in.ActionPlan)
		},
		func(in Input) []Recommendation {
			return fromMissingInformation(in.MissingInformation)
		},
	}
	for _, mapper := range mappers {
		candidates = append(candidates, mapper(input)...)
	}

	deduped := dedupe(candidates)
	sortRecommendations(deduped)
	if len(deduped) > 7 {
		deduped = deduped[:7]
	}
	for i := range deduped {
		deduped[i].Order = i + 1
	}
	return deduped
}

func severityRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func impactRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}

func categoryRank(value string) int {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "ATS":
		return 5
	case "SKILLS":
		return 4
	case "EXPERIENCE":
		return 3
	case "STRUCTURE":
		return 2
	case "FORMATTING":
		return 1
	default:
		return 0
	}
}

func slugify(input string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(input)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "item"
	}
	return out
}

func dedupe(items []Recommendation) []Recommendation {
	seen := make(map[string]Recommendation, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if existing, ok := seen[id]; ok {
			merged := mergeRecommendation(existing, item)
			seen[id] = merged
			continue
		}
		seen[id] = item
		order = append(order, id)
	}
	out := make([]Recommendation, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	return out
}

func mergeRecommendation(a, b Recommendation) Recommendation {
	if strings.TrimSpace(a.Title) == "" {
		a.Title = b.Title
	}
	if strings.TrimSpace(a.Why) == "" {
		a.Why = b.Why
	}
	if strings.TrimSpace(a.Action) == "" {
		a.Action = b.Action
	}
	if strings.TrimSpace(a.Category) == "" {
		a.Category = b.Category
	}
	if strings.TrimSpace(a.Severity) == "" {
		a.Severity = b.Severity
	}
	if strings.TrimSpace(a.Impact) == "" {
		a.Impact = b.Impact
	}
	return a
}

func sortRecommendations(items []Recommendation) {
	sort.Slice(items, func(i, j int) bool {
		a := items[i]
		b := items[j]
		if severityRank(a.Severity) != severityRank(b.Severity) {
			return severityRank(a.Severity) > severityRank(b.Severity)
		}
		if impactRank(a.Impact) != impactRank(b.Impact) {
			return impactRank(a.Impact) > impactRank(b.Impact)
		}
		if categoryRank(a.Category) != categoryRank(b.Category) {
			return categoryRank(a.Category) > categoryRank(b.Category)
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
}

func mapIssueSeverity(value string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return "critical", "high"
	case "high":
		return "warning", "high"
	case "medium":
		return "warning", "medium"
	default:
		return "info", "low"
	}
}

func inferCategory(section string, title string) string {
	combined := strings.ToLower(strings.TrimSpace(section + " " + title))
	switch {
	case strings.Contains(combined, "skill") || strings.Contains(combined, "keyword"):
		return "SKILLS"
	case strings.Contains(combined, "format") || strings.Contains(combined, "bullet") || strings.Contains(combined, "font") || strings.Contains(combined, "layout"):
		return "FORMATTING"
	case strings.Contains(combined, "experience") || strings.Contains(combined, "role") || strings.Contains(combined, "project"):
		return "EXPERIENCE"
	case strings.Contains(combined, "structure") || strings.Contains(combined, "section") || strings.Contains(combined, "summary") || strings.Contains(combined, "header") || strings.Contains(combined, "order"):
		return "STRUCTURE"
	case strings.Contains(combined, "ats"):
		return "ATS"
	default:
		return "ATS"
	}
}

func uniqueSortedStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}
