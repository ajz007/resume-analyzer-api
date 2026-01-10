package analyses

// JSON Schema (v1):
// {
//   "summary": {
//     "overallAssessment": "string",
//     "strengths": ["string"],
//     "weaknesses": ["string"]
//   },
//   "ats": {
//     "score": "number (0-100)",
//     "missingKeywords": ["string"],
//     "formattingIssues": ["string"]
//   },
//   "issues": [
//     {
//       "severity": "critical | high | medium | low",
//       "section": "string",
//       "problem": "string",
//       "whyItMatters": "string",
//       "suggestion": "string"
//     }
//   ],
//   "bulletRewrites": [
//     {
//       "section": "string",
//       "before": "string",
//       "after": "string",
//       "rationale": "string"
//     }
//   ],
//   "missingInformation": ["string"],
//   "actionPlan": {
//     "quickWins": ["string"],
//     "mediumEffort": ["string"],
//     "deepFixes": ["string"]
//   }
// }
type AnalysisResultV1 struct {
	Summary            SummaryV1          `json:"summary"`
	ATS                ATSV1              `json:"ats"`
	Issues             []IssueV1          `json:"issues"`
	BulletRewrites     []BulletRewriteV1  `json:"bulletRewrites"`
	MissingInformation []string           `json:"missingInformation"`
	ActionPlan         ActionPlanV1       `json:"actionPlan"`
}

type SummaryV1 struct {
	OverallAssessment string   `json:"overallAssessment"`
	Strengths         []string `json:"strengths"`
	Weaknesses        []string `json:"weaknesses"`
}

type ATSV1 struct {
	Score            float64  `json:"score"`
	MissingKeywords  []string `json:"missingKeywords"`
	FormattingIssues []string `json:"formattingIssues"`
}

type IssueSeverityV1 string

const (
	IssueSeverityCritical IssueSeverityV1 = "critical"
	IssueSeverityHigh     IssueSeverityV1 = "high"
	IssueSeverityMedium   IssueSeverityV1 = "medium"
	IssueSeverityLow      IssueSeverityV1 = "low"
)

type IssueV1 struct {
	Severity     IssueSeverityV1 `json:"severity"`
	Section      string          `json:"section"`
	Problem      string          `json:"problem"`
	WhyItMatters string          `json:"whyItMatters"`
	Suggestion   string          `json:"suggestion"`
}

type BulletRewriteV1 struct {
	Section   string `json:"section"`
	Before    string `json:"before"`
	After     string `json:"after"`
	Rationale string `json:"rationale"`
}

type ActionPlanV1 struct {
	QuickWins    []string `json:"quickWins"`
	MediumEffort []string `json:"mediumEffort"`
	DeepFixes    []string `json:"deepFixes"`
}
