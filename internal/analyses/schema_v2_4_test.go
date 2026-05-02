package analyses

import (
	"encoding/json"
	"testing"
)

func TestAnalysisResultV2_4ValidJobMatchPayload(t *testing.T) {
	payload, err := json.Marshal(validAnalysisResultV2_4())
	if err != nil {
		t.Fatalf("marshal valid v2_4 payload: %v", err)
	}

	var out AnalysisResultV2_4
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("expected v2_4 payload to unmarshal, got error: %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("expected v2_4 payload to validate, got error: %v", err)
	}
}

func TestAnalysisResultV2_4InvalidTopPriorityWeightTotal(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.JobRequirementProfile.TopPriorities[0].Weight = 10

	if err := out.Validate(); err == nil {
		t.Fatalf("expected validation error for top priority weight total")
	}
}

func TestAnalysisResultV2_4InvalidAIScreeningBreakdownWeightTotal(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.AIScreening.ScoreBreakdown[0].Weight = 24

	if err := out.Validate(); err == nil {
		t.Fatalf("expected validation error for aiScreening breakdown weight total")
	}
}

func TestAnalysisResultV2_4ATSModeAllowsJobRequirementProfileNotApplicable(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.Meta.JobDescriptionProvided = false
	out.Meta.Mode = string(ModeATS)
	out.ATS.MissingKeywords.FromJobDescription = nil
	out.JobRequirementProfile.IsApplicable = false
	out.JobRequirementProfile.TopPriorities = nil
	out.JobMatchScoring = JobMatchScoringV1{}

	if err := out.Validate(); err != nil {
		t.Fatalf("expected ATS v2_4 payload to validate, got error: %v", err)
	}
}

func TestAnalysisResultV2_4InvalidEnumRejected(t *testing.T) {
	out := validAnalysisResultV2_4()
	out.JobRequirementProfile.Seniority = "LEAD"

	if err := out.Validate(); err == nil {
		t.Fatalf("expected validation error for invalid enum")
	}
}

func validAnalysisResultV2_4() AnalysisResultV2_4 {
	return AnalysisResultV2_4{
		Meta: MetaV2{
			PromptVersion:          "v2_4",
			Model:                  "gpt-5-mini",
			JobDescriptionProvided: true,
			Confidence:             0.8,
			Assumptions:            []string{},
			Limitations:            []string{},
			Mode:                   string(ModeJobMatch),
		},
		Summary: SummaryV1{
			OverallAssessment: "Strong match for backend platform work.",
			Strengths:         []string{"Backend ownership", "Measurable impact"},
			Weaknesses:        []string{"Could add more scale details"},
		},
		ATS: ATSV2_3{
			Score: 82,
			ScoreBreakdown: ScoreBreakdownV2{
				Skills:     25,
				Experience: 25,
				Impact:     20,
				Formatting: 15,
				RoleFit:    15,
			},
			ScoreReasoning: []string{
				"Core backend skills are clear.",
				"Experience aligns with the target role.",
				"Impact is present but can be sharper.",
			},
			ScoreExplanation: ScoreExplanationV1{Components: []ScoreComponentV1{
				{
					Key:         "atsReadability",
					Label:       "ATS Readability",
					Score:       80,
					Weight:      25,
					Explanation: "Clear headings and parseable structure.",
					Helped:      []string{"Standard section titles"},
					Dragged:     []string{"Dense bullets"},
				},
				{
					Key:         "skillMatch",
					Label:       "Skill Match",
					Score:       85,
					Weight:      30,
					Explanation: "Backend skills match the role.",
					Helped:      []string{"Go", "AWS"},
					Dragged:     []string{"Few queueing details"},
				},
				{
					Key:         "experienceRelevance",
					Label:       "Experience Relevance",
					Score:       82,
					Weight:      30,
					Explanation: "Recent experience fits the job.",
					Helped:      []string{"API ownership"},
					Dragged:     []string{"Limited system scale evidence"},
				},
				{
					Key:         "resumeStructure",
					Label:       "Resume Structure",
					Score:       78,
					Weight:      15,
					Explanation: "Sections are easy to scan.",
					Helped:      []string{"Concise layout"},
					Dragged:     []string{"Some bullets lack metrics"},
				},
			}},
			MissingKeywords: MissingKeywordsV2{
				FromJobDescription: []string{"SQS"},
				IndustryCommon:     []string{"observability"},
			},
			FormattingIssues: []string{},
		},
		Issues: []IssueV2_2{
			{
				Severity:          IssueSeverityMedium,
				Section:           "Experience",
				Problem:           "Scale is not always quantified.",
				WhyItMatters:      "Backend roles look for operating context.",
				Suggestion:        "Add request volume or latency where available.",
				Evidence:          "notFound",
				FixEffort:         "medium",
				Priority:          2,
				AutoFixable:       false,
				RequiresUserInput: []string{"metrics"},
			},
		},
		BulletRewrites: []BulletRewriteV2_3{
			{
				Section:            "Experience",
				Before:             "Built APIs.",
				After:              "Built Go APIs serving X requests per day.",
				Rationale:          "Adds clearer scope with a user-provided metric.",
				MetricsSource:      "placeholder",
				PlaceholdersNeeded: []string{"request_volume"},
				ClaimSupport:       "placeholder",
				Evidence:           "notFound",
			},
		},
		MissingInformation: []string{"request volume"},
		ActionPlan: ActionPlanV1{
			QuickWins:    []string{"Add SQS keyword where relevant."},
			MediumEffort: []string{"Quantify backend scale."},
			DeepFixes:    []string{"Add architecture context to key projects."},
		},
		JobRequirementProfile: JobRequirementProfileV1{
			IsApplicable:           true,
			PrimaryRole:            "Backend Engineer",
			Seniority:              "SENIOR",
			RoleType:               "BACKEND",
			RecruiterIntentSummary: "Hiring for backend ownership in Go and AWS.",
			TopPriorities: []JobPriorityV1{
				{
					ID:                "req_backend_go",
					Priority:          "Go backend services",
					Importance:        "CRITICAL",
					Weight:            40,
					EvidenceExpected:  "Recent Go service ownership.",
					ResumeMatchStatus: "STRONG",
					WhyItMatters:      "The role centers on backend delivery.",
				},
				{
					ID:                "req_aws",
					Priority:          "AWS production experience",
					Importance:        "HIGH",
					Weight:            35,
					EvidenceExpected:  "AWS services used in production.",
					ResumeMatchStatus: "PARTIAL",
					WhyItMatters:      "The team runs services on AWS.",
				},
				{
					ID:                "req_impact",
					Priority:          "Measured operational impact",
					Importance:        "MEDIUM",
					Weight:            25,
					EvidenceExpected:  "Latency, reliability, or scale metrics.",
					ResumeMatchStatus: "WEAK",
					WhyItMatters:      "Metrics prove senior-level outcomes.",
				},
			},
			HiddenExpectations: []HiddenExpectationV1{
				{
					ID:                "hidden_ownership",
					Expectation:       "Own services beyond implementation.",
					ResumeMatchStatus: "PARTIAL",
					WhyItMatters:      "Senior engineers are screened for ownership.",
				},
			},
			NiceToHaveSignals: []NiceToHaveSignalV1{
				{
					ID:                "nice_observability",
					Signal:            "Observability experience",
					ResumeMatchStatus: "UNKNOWN",
					WhyItMatters:      "It supports production readiness.",
				},
			},
		},
		JobMatchScoring: JobMatchScoringV1{
			Score:           78,
			ScoringStrategy: "JD_WEIGHTED",
			RequirementScores: []RequirementScoreV1{
				{
					RequirementID:        "req_backend_go",
					Requirement:          "Go backend services",
					Weight:               40,
					Score:                90,
					WeightedContribution: 36,
					MatchStatus:          "STRONG",
					Evidence:             "Built Go APIs in recent backend role.",
					Gap:                  "",
				},
				{
					RequirementID:        "req_aws",
					Requirement:          "AWS production experience",
					Weight:               35,
					Score:                70,
					WeightedContribution: 24.5,
					MatchStatus:          "PARTIAL",
					Evidence:             "AWS is listed but services are sparse.",
					Gap:                  "Name relevant AWS services.",
				},
				{
					RequirementID:        "req_impact",
					Requirement:          "Measured operational impact",
					Weight:               25,
					Score:                70,
					WeightedContribution: 17.5,
					MatchStatus:          "PARTIAL",
					Evidence:             "Some impact metrics are present.",
					Gap:                  "Add scale or reliability numbers.",
				},
			},
			Explanation: "Strong backend match with some AWS and metrics gaps.",
		},
		AIScreening: AIScreeningV1{
			Score: 80,
			Verdict: AIScreeningVerdictV1{
				Tier:          "GOOD",
				Title:         "Likely to pass initial screening",
				Summary:       "Relevant backend experience with fixable evidence gaps.",
				ScreeningRisk: "MEDIUM",
			},
			ScoreBreakdown: []AIScreeningBreakdownItemV1{
				{ID: "semantic_relevance", Label: "Semantic Relevance", Score: 85, Weight: 25, Status: "STRONG", Explanation: "Experience matches backend work.", ImprovementFocus: "Add SQS details."},
				{ID: "role_intent_alignment", Label: "Role Intent Alignment", Score: 80, Weight: 20, Status: "OK", Explanation: "Resume points toward backend roles.", ImprovementFocus: "Clarify target title."},
				{ID: "evidence_strength", Label: "Evidence Strength", Score: 75, Weight: 20, Status: "OK", Explanation: "Evidence is present but uneven.", ImprovementFocus: "Add production examples."},
				{ID: "impact_strength", Label: "Impact Strength", Score: 75, Weight: 15, Status: "OK", Explanation: "Impact is credible but light.", ImprovementFocus: "Add metrics."},
				{ID: "signal_density", Label: "Signal Density", Score: 80, Weight: 10, Status: "OK", Explanation: "Relevant keywords appear often enough.", ImprovementFocus: "Remove generic bullets."},
				{ID: "clarity_specificity", Label: "Clarity Specificity", Score: 85, Weight: 10, Status: "STRONG", Explanation: "Most bullets are clear.", ImprovementFocus: "Tighten vague wording."},
			},
			AIRecruiterVerdict: AIRecruiterVerdictV1{
				OneLineVerdict:  "Good backend candidate with evidence gaps.",
				MainConcern:     "AWS depth is not specific enough.",
				StrongestSignal: "Recent Go backend ownership.",
				WeakestSignal:   "Limited production scale metrics.",
			},
		},
		FixThisFirst: []FixThisFirstItemV1{
			{
				Priority:            1,
				Title:               "Add AWS service evidence",
				Why:                 "It closes a high-weight requirement gap.",
				LinkedRequirementID: "req_aws",
				ExpectedImpact:      "HIGH",
				Effort:              "LOW",
				Action:              "Name the AWS services used in recent backend work.",
				RequiresUserInput:   true,
			},
		},
	}
}
