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

type JobPriorityInput struct {
	ID                string
	Priority          string
	Importance        string
	Weight            float64
	EvidenceExpected  string
	ResumeMatchStatus string
	WhyItMatters      string
}

type RequirementScoreInput struct {
	RequirementID string
	Requirement   string
	Weight        float64
	Score         float64
	MatchStatus   string
	Evidence      string
	Gap           string
}

type AIScreeningBreakdownInput struct {
	ID               string
	Label            string
	Score            float64
	Weight           float64
	Status           string
	Explanation      string
	ImprovementFocus string
}

type FixThisFirstInput struct {
	Priority            int
	Title               string
	Why                 string
	LinkedRequirementID string
	ExpectedImpact      string
	Effort              string
	Action              string
	RequiresUserInput   bool
}

// Input is the normalized data needed for recommendation generation.
type Input struct {
	Issues               []Issue
	MissingJDKeywords    []string
	MissingIndustryTerms []string
	FormattingIssues     []string
	ActionPlan           ActionPlan
	MissingInformation   []string
	JobPriorities        []JobPriorityInput
	RequirementScores    []RequirementScoreInput
	AIScreeningBreakdown []AIScreeningBreakdownInput
	FixThisFirst         []FixThisFirstInput
}
