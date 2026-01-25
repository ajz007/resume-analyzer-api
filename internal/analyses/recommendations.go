package analyses

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
)

// Recommendation represents a deterministic suggestion derived from analysis results.
type Recommendation struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Why      string `json:"why"`
	Action   string `json:"action"`
	Impact   string `json:"impact"`
	Order    int    `json:"order"`
}

type recommendationCandidate struct {
	Recommendation
	priority int
}

const (
	recommendationSeverityInfo     = "info"
	recommendationSeverityWarning  = "warning"
	recommendationSeverityCritical = "critical"
)

const (
	recommendationImpactLow    = "low"
	recommendationImpactMedium = "medium"
	recommendationImpactHigh   = "high"
)

func buildRecommendations(out NormalizedAnalysisResult) []Recommendation {
	candidates := make([]recommendationCandidate, 0, 16)

	for _, issue := range out.Issues {
		title := strings.TrimSpace(issue.Problem)
		if title == "" {
			title = "Issue in " + strings.TrimSpace(issue.Section)
		}
		category := inferRecommendationCategory(issue.Section, "ATS")
		severity, impact := mapIssueSeverity(issue.Severity)
		priority := issue.Priority
		if priority <= 0 {
			priority = 50
		}
		rec := Recommendation{
			ID:       makeRecommendationID("issue", string(issue.Severity), issue.Section, issue.Problem, issue.Suggestion),
			Category: category,
			Severity: severity,
			Title:    title,
			Why:      strings.TrimSpace(issue.WhyItMatters),
			Action:   strings.TrimSpace(issue.Suggestion),
			Impact:   impact,
		}
		candidates = append(candidates, recommendationCandidate{Recommendation: rec, priority: priority})
	}

	for _, keyword := range out.ATS.MissingKeywords.FromJobDescription {
		kw := strings.TrimSpace(keyword)
		if kw == "" {
			continue
		}
		rec := Recommendation{
			ID:       makeRecommendationID("missing_keyword", kw),
			Category: "SKILLS",
			Severity: recommendationSeverityWarning,
			Title:    "Add keyword: " + kw,
			Why:      "Keyword appears in the job description but not in the resume.",
			Action:   "Include this keyword in a relevant skills or experience bullet.",
			Impact:   recommendationImpactMedium,
		}
		candidates = append(candidates, recommendationCandidate{Recommendation: rec, priority: 40})
	}

	for _, issue := range out.ATS.FormattingIssues {
		item := strings.TrimSpace(issue)
		if item == "" {
			continue
		}
		rec := Recommendation{
			ID:       makeRecommendationID("formatting", item),
			Category: "FORMATTING",
			Severity: recommendationSeverityWarning,
			Title:    "Fix formatting: " + item,
			Why:      "Formatting issues can reduce ATS readability.",
			Action:   "Resolve the following formatting issue: " + item,
			Impact:   recommendationImpactMedium,
		}
		candidates = append(candidates, recommendationCandidate{Recommendation: rec, priority: 30})
	}

	appendActionPlanRecommendations(&candidates, "quickWins", out.ActionPlan.QuickWins)
	appendActionPlanRecommendations(&candidates, "mediumEffort", out.ActionPlan.MediumEffort)
	appendActionPlanRecommendations(&candidates, "deepFixes", out.ActionPlan.DeepFixes)

	deduped := dedupeRecommendations(candidates)
	sortRecommendations(deduped)
	if len(deduped) > 7 {
		deduped = deduped[:7]
	}
	for i := range deduped {
		deduped[i].Order = i + 1
	}
	outList := make([]Recommendation, 0, len(deduped))
	for _, item := range deduped {
		outList = append(outList, item.Recommendation)
	}
	return outList
}

func appendActionPlanRecommendations(out *[]recommendationCandidate, planType string, items []string) {
	for _, item := range items {
		text := strings.TrimSpace(item)
		if text == "" {
			continue
		}
		severity := recommendationSeverityInfo
		impact := recommendationImpactLow
		priority := 80
		switch planType {
		case "mediumEffort":
			severity = recommendationSeverityWarning
			impact = recommendationImpactMedium
			priority = 90
		case "deepFixes":
			severity = recommendationSeverityWarning
			impact = recommendationImpactHigh
			priority = 100
		}
		rec := Recommendation{
			ID:       makeRecommendationID("action_plan", planType, text),
			Category: inferRecommendationCategory(text, "ATS"),
			Severity: severity,
			Title:    text,
			Why:      "Suggested in the action plan.",
			Action:   text,
			Impact:   impact,
		}
		*out = append(*out, recommendationCandidate{Recommendation: rec, priority: priority})
	}
}

func dedupeRecommendations(items []recommendationCandidate) []recommendationCandidate {
	seen := make(map[string]bool, len(items))
	out := make([]recommendationCandidate, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		out = append(out, item)
	}
	return out
}

func sortRecommendations(items []recommendationCandidate) {
	sort.Slice(items, func(i, j int) bool {
		a := items[i]
		b := items[j]
		if severityRank(a.Severity) != severityRank(b.Severity) {
			return severityRank(a.Severity) > severityRank(b.Severity)
		}
		if impactRank(a.Impact) != impactRank(b.Impact) {
			return impactRank(a.Impact) > impactRank(b.Impact)
		}
		if a.priority != b.priority {
			return a.priority < b.priority
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
}

func severityRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case recommendationSeverityCritical:
		return 3
	case recommendationSeverityWarning:
		return 2
	default:
		return 1
	}
}

func impactRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case recommendationImpactHigh:
		return 3
	case recommendationImpactMedium:
		return 2
	default:
		return 1
	}
}

func mapIssueSeverity(value IssueSeverityV1) (string, string) {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case "critical":
		return recommendationSeverityCritical, recommendationImpactHigh
	case "high":
		return recommendationSeverityWarning, recommendationImpactHigh
	case "medium":
		return recommendationSeverityWarning, recommendationImpactMedium
	default:
		return recommendationSeverityInfo, recommendationImpactLow
	}
}

func inferRecommendationCategory(text string, fallback string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "skill") || strings.Contains(lower, "keyword"):
		return "SKILLS"
	case strings.Contains(lower, "format") || strings.Contains(lower, "bullet") || strings.Contains(lower, "font") || strings.Contains(lower, "layout"):
		return "FORMATTING"
	case strings.Contains(lower, "experience") || strings.Contains(lower, "role") || strings.Contains(lower, "project"):
		return "EXPERIENCE"
	case strings.Contains(lower, "structure") || strings.Contains(lower, "section") || strings.Contains(lower, "summary") || strings.Contains(lower, "header") || strings.Contains(lower, "order"):
		return "STRUCTURE"
	case strings.Contains(lower, "ats"):
		return "ATS"
	default:
		if strings.TrimSpace(fallback) == "" {
			return "ATS"
		}
		return strings.ToUpper(strings.TrimSpace(fallback))
	}
}

func makeRecommendationID(parts ...string) string {
	hasher := fnv.New64a()
	for _, part := range parts {
		_, _ = hasher.Write([]byte(strings.TrimSpace(part)))
		_, _ = hasher.Write([]byte{0})
	}
	return fmt.Sprintf("rec_%x", hasher.Sum64())
}

func normalizeRecommendations(value []Recommendation) []Recommendation {
	if value == nil {
		return []Recommendation{}
	}
	return value
}
