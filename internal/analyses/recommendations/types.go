package recommendations

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

// Issue is a minimal issue representation used by the recommendation engine.
type Issue struct {
	Severity     string
	Section      string
	Problem      string
	WhyItMatters string
	Suggestion   string
}

// ActionPlan is a minimal action plan representation used by the recommendation engine.
type ActionPlan struct {
	QuickWins    []string
	MediumEffort []string
	DeepFixes    []string
}

// Input is the normalized data needed for recommendation generation.
type Input struct {
	Issues               []Issue
	MissingJDKeywords    []string
	MissingIndustryTerms []string
	FormattingIssues     []string
	ActionPlan           ActionPlan
	MissingInformation   []string
}
