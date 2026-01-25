package recommendations

import (
	"sort"
	"strings"
)

type actionPlanCandidate struct {
	title    string
	impact   string
	severity string
}

func fromIssues(issues []Issue) []Recommendation {
	out := make([]Recommendation, 0, len(issues))
	for _, issue := range issues {
		title := strings.TrimSpace(issue.Problem)
		if title == "" {
			title = strings.TrimSpace(issue.Section)
		}
		if title == "" {
			title = "Issue found"
		}
		severity, impact := mapIssueSeverity(issue.Severity)
		action := strings.TrimSpace(issue.Suggestion)
		if action == "" {
			action = "Fix: " + title
		}
		why := strings.TrimSpace(issue.WhyItMatters)
		if why == "" {
			why = "Improves clarity and relevance for recruiters."
		}
		out = append(out, Recommendation{
			ID:       "ISSUE_" + slugify(title),
			Category: inferCategory(issue.Section, title),
			Severity: severity,
			Title:    title,
			Why:      why,
			Action:   action,
			Impact:   impact,
		})
	}
	return out
}

func fromMissingJDKeywords(k []string) []Recommendation {
	keywords := uniqueSortedStrings(k)
	if len(keywords) == 0 {
		return nil
	}
	action := "Add 5–10 missing keywords naturally into Skills + Experience bullets to mirror the job description."
	if len(keywords) > 0 {
		action = action + " Focus on: " + strings.Join(keywords, ", ")
	}
	return []Recommendation{
		{
			ID:       "ATS_MISSING_JD_KEYWORDS",
			Category: "ATS",
			Severity: "warning",
			Title:    "Add missing job keywords",
			Why:      "Improves ATS match and helps recruiters quickly spot relevant skills.",
			Action:   action,
			Impact:   "high",
		},
	}
}

func fromFormattingIssues(fi []string) []Recommendation {
	items := uniqueSortedStrings(fi)
	if len(items) == 0 {
		return nil
	}
	groups := map[string][]string{
		"bullets":  {},
		"headers":  {},
		"sections": {},
		"other":    {},
	}
	for _, item := range items {
		lower := strings.ToLower(item)
		switch {
		case strings.Contains(lower, "bullet"):
			groups["bullets"] = append(groups["bullets"], item)
		case strings.Contains(lower, "header") || strings.Contains(lower, "heading"):
			groups["headers"] = append(groups["headers"], item)
		case strings.Contains(lower, "section"):
			groups["sections"] = append(groups["sections"], item)
		default:
			groups["other"] = append(groups["other"], item)
		}
	}

	type grouped struct {
		key   string
		items []string
	}
	orderedGroups := []grouped{
		{key: "bullets", items: groups["bullets"]},
		{key: "headers", items: groups["headers"]},
		{key: "sections", items: groups["sections"]},
		{key: "other", items: groups["other"]},
	}

	out := make([]Recommendation, 0, 2)
	for _, group := range orderedGroups {
		if len(group.items) == 0 {
			continue
		}
		title := "Fix ATS formatting issues"
		action := "Fix formatting issues: " + strings.Join(group.items, "; ")
		id := "ATS_FORMATTING_" + strings.ToUpper(group.key)
		switch group.key {
		case "bullets":
			title = "Standardize bullet formatting"
			action = "Standardize bullet formatting by fixing: " + strings.Join(group.items, "; ")
			id = "ATS_FORMATTING_BULLETS"
		case "headers":
			title = "Fix heading consistency"
			action = "Fix heading consistency by correcting: " + strings.Join(group.items, "; ")
			id = "ATS_FORMATTING_HEADERS"
		case "sections":
			title = "Fix section structure"
			action = "Fix section structure by correcting: " + strings.Join(group.items, "; ")
			id = "ATS_FORMATTING_SECTIONS"
		}
		out = append(out, Recommendation{
			ID:       id,
			Category: "FORMATTING",
			Severity: "warning",
			Title:    title,
			Why:      "Formatting issues reduce ATS readability and can hide key details.",
			Action:   action,
			Impact:   "medium",
		})
		if len(out) == 2 {
			break
		}
	}
	return out
}

func fromActionPlan(ap ActionPlan) []Recommendation {
	items := make([]actionPlanCandidate, 0, 8)
	for _, item := range ap.DeepFixes {
		text := strings.TrimSpace(item)
		if text == "" {
			continue
		}
		items = append(items, actionPlanCandidate{title: text, impact: "high", severity: "warning"})
	}
	for _, item := range ap.MediumEffort {
		text := strings.TrimSpace(item)
		if text == "" {
			continue
		}
		items = append(items, actionPlanCandidate{title: text, impact: "medium", severity: "warning"})
	}
	for _, item := range ap.QuickWins {
		text := strings.TrimSpace(item)
		if text == "" {
			continue
		}
		items = append(items, actionPlanCandidate{title: text, impact: "low", severity: "info"})
	}
	if len(items) == 0 {
		return nil
	}
	sortByImpactThenTitle(items)
	if len(items) > 2 {
		items = items[:2]
	}
	out := make([]Recommendation, 0, len(items))
	for _, item := range items {
		title := item.title
		out = append(out, Recommendation{
			ID:       "ACTION_PLAN_" + slugify(title),
			Category: inferCategory("", title),
			Severity: item.severity,
			Title:    title,
			Why:      "High-impact action from the plan.",
			Action:   title,
			Impact:   item.impact,
		})
	}
	return out
}

func fromMissingInformation(items []string) []Recommendation {
	out := make([]Recommendation, 0, len(items))
	for _, item := range uniqueSortedStrings(items) {
		title := "Add missing information: " + item
		out = append(out, Recommendation{
			ID:       "MISSING_INFO_" + slugify(item),
			Category: "STRUCTURE",
			Severity: "warning",
			Title:    title,
			Why:      "Recruiters expect this detail to evaluate fit quickly.",
			Action:   "Add the missing information: " + item,
			Impact:   "medium",
		})
	}
	return out
}

func sortByImpactThenTitle(items []actionPlanCandidate) {
	sort.Slice(items, func(i, j int) bool {
		if impactRank(items[i].impact) != impactRank(items[j].impact) {
			return impactRank(items[i].impact) > impactRank(items[j].impact)
		}
		return strings.ToLower(items[i].title) < strings.ToLower(items[j].title)
	})
}
