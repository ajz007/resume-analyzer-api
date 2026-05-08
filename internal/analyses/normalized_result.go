package analyses

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"resume-backend/internal/analyses/recommendations"
)

// NormalizedAnalysisResult is the single normalized response schema returned by the API.
type NormalizedAnalysisResult struct {
	Meta                  MetaV2                    `json:"meta"`
	Summary               SummaryV1                 `json:"summary"`
	FinalScore            float64                   `json:"finalScore"`
	MatchScore            float64                   `json:"matchScore"`
	ATS                   NormalizedATS             `json:"ats"`
	JobRequirementProfile JobRequirementProfileV1   `json:"jobRequirementProfile,omitempty"`
	JobMatchScoring       JobMatchScoringV1         `json:"jobMatchScoring,omitempty"`
	AIScreening           AIScreeningV1             `json:"aiScreening,omitempty"`
	FixThisFirst          []FixThisFirstItemV1      `json:"fixThisFirst,omitempty"`
	Issues                []IssueV2_2               `json:"issues"`
	BulletRewrites        []NormalizedBulletRewrite `json:"bulletRewrites"`
	Recommendations       []Recommendation          `json:"recommendations"`
	MissingInformation    []string                  `json:"missingInformation"`
	ActionPlan            ActionPlanV1              `json:"actionPlan"`
}

func (r NormalizedAnalysisResult) MarshalJSON() ([]byte, error) {
	type normalizedResponse struct {
		Meta                  MetaV2                    `json:"meta"`
		Summary               SummaryV1                 `json:"summary"`
		FinalScore            float64                   `json:"finalScore"`
		MatchScore            float64                   `json:"matchScore"`
		ATS                   NormalizedATS             `json:"ats"`
		JobRequirementProfile *JobRequirementProfileV1  `json:"jobRequirementProfile,omitempty"`
		JobMatchScoring       *JobMatchScoringV1        `json:"jobMatchScoring,omitempty"`
		AIScreening           *AIScreeningV1            `json:"aiScreening,omitempty"`
		FixThisFirst          *[]FixThisFirstItemV1     `json:"fixThisFirst,omitempty"`
		Issues                []IssueV2_2               `json:"issues"`
		BulletRewrites        []NormalizedBulletRewrite `json:"bulletRewrites"`
		Recommendations       []Recommendation          `json:"recommendations"`
		MissingInformation    []string                  `json:"missingInformation"`
		ActionPlan            ActionPlanV1              `json:"actionPlan"`
	}
	out := normalizedResponse{
		Meta:               r.Meta,
		Summary:            r.Summary,
		FinalScore:         r.FinalScore,
		MatchScore:         r.MatchScore,
		ATS:                r.ATS,
		Issues:             r.Issues,
		BulletRewrites:     r.BulletRewrites,
		Recommendations:    r.Recommendations,
		MissingInformation: r.MissingInformation,
		ActionPlan:         r.ActionPlan,
	}
	if strings.EqualFold(r.Meta.PromptVersion, "v2_4") {
		out.JobRequirementProfile = &r.JobRequirementProfile
		out.JobMatchScoring = &r.JobMatchScoring
		out.AIScreening = &r.AIScreening
		out.FixThisFirst = &r.FixThisFirst
	}
	return json.Marshal(out)
}

type NormalizedATS struct {
	Score            float64            `json:"score"`
	ScoreBreakdown   ScoreBreakdownV2   `json:"scoreBreakdown"`
	ScoreReasoning   []string           `json:"scoreReasoning"`
	ScoreExplanation ScoreExplanationV1 `json:"scoreExplanation"`
	MissingKeywords  MissingKeywordsV2  `json:"missingKeywords"`
	FormattingIssues []string           `json:"formattingIssues"`
}

type NormalizedBulletRewrite struct {
	Section            string   `json:"section"`
	Before             string   `json:"before"`
	After              string   `json:"after"`
	Rationale          string   `json:"rationale"`
	MetricsSource      string   `json:"metricsSource"`
	PlaceholdersNeeded []string `json:"placeholdersNeeded"`
	ClaimSupport       string   `json:"claimSupport"`
	Evidence           string   `json:"evidence"`
}

func normalizeAnalysisResult(raw json.RawMessage, analysis Analysis) (map[string]any, error) {
	normalized, err := normalizeToFinal(raw, analysis)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	normalizeResultOrdering(result)
	return result, nil
}

func normalizeToFinal(raw json.RawMessage, analysis Analysis) (NormalizedAnalysisResult, error) {
	if len(raw) == 0 {
		return NormalizedAnalysisResult{}, errors.New("empty analysis result")
	}
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return NormalizedAnalysisResult{}, err
	}
	if err := requireTopLevelFields(top); err != nil {
		return NormalizedAnalysisResult{}, err
	}

	var envelope struct {
		Meta struct {
			PromptVersion string `json:"promptVersion"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(raw, &envelope)

	hasMeta := false
	if _, ok := top["meta"]; ok {
		hasMeta = true
	}

	switch {
	case hasMeta && strings.EqualFold(envelope.Meta.PromptVersion, "v2_4"):
		var parsed AnalysisResultV2_4
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		out := normalizeFromV2_4(parsed, analysis)
		out.Recommendations = normalizeRecommendations(recommendations.GenerateRecommendations(buildRecommendationInput(out)))
		applyScoresV2_4(&out, analysis.Mode)
		return out, validateNormalized(out)
	case hasMeta && strings.EqualFold(envelope.Meta.PromptVersion, "v2_3"):
		var parsed AnalysisResultV2_3
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		out := normalizeFromV2_3(parsed, analysis)
		out.Recommendations = normalizeRecommendations(recommendations.GenerateRecommendations(buildRecommendationInput(out)))
		applyScores(&out, analysis.Mode, extractFloat(top["matchScore"]))
		return out, validateNormalized(out)
	case hasMeta && strings.EqualFold(envelope.Meta.PromptVersion, "v2_2"):
		var parsed AnalysisResultV2_2
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		out := normalizeFromV2_2(parsed, analysis)
		out.Recommendations = normalizeRecommendations(recommendations.GenerateRecommendations(buildRecommendationInput(out)))
		applyScores(&out, analysis.Mode, extractFloat(top["matchScore"]))
		return out, validateNormalized(out)
	case hasMeta && strings.EqualFold(envelope.Meta.PromptVersion, "v2_1"):
		var parsed AnalysisResultV2_1
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		out := normalizeFromV2_1(parsed, analysis)
		out.Recommendations = normalizeRecommendations(recommendations.GenerateRecommendations(buildRecommendationInput(out)))
		applyScores(&out, analysis.Mode, extractFloat(top["matchScore"]))
		return out, validateNormalized(out)
	case hasMeta && strings.EqualFold(envelope.Meta.PromptVersion, "v2"):
		var parsed AnalysisResultV2
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		out := normalizeFromV2(parsed, analysis)
		out.Recommendations = normalizeRecommendations(recommendations.GenerateRecommendations(buildRecommendationInput(out)))
		applyScores(&out, analysis.Mode, extractFloat(top["matchScore"]))
		return out, validateNormalized(out)
	default:
		var parsed AnalysisResultV1
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return NormalizedAnalysisResult{}, err
		}
		topMissing := extractStringSlice(top["missingKeywords"])
		topFormatting := extractStringSlice(top["formattingIssues"])
		out := normalizeFromV1(parsed, analysis, topMissing, topFormatting)
		out.Recommendations = normalizeRecommendations(recommendations.GenerateRecommendations(buildRecommendationInput(out)))
		applyScores(&out, analysis.Mode, extractFloat(top["matchScore"]))
		return out, validateNormalized(out)
	}
}

func requireTopLevelFields(raw map[string]any) error {
	required := []string{"summary", "ats", "issues", "bulletRewrites", "missingInformation", "actionPlan"}
	for _, key := range required {
		if _, ok := raw[key]; !ok {
			return fmt.Errorf("missing field: %s", key)
		}
	}
	return nil
}

func validateNormalized(out NormalizedAnalysisResult) error {
	if strings.TrimSpace(out.Summary.OverallAssessment) == "" {
		return errors.New("summary.overallAssessment is required")
	}
	if strings.TrimSpace(out.Meta.PromptVersion) == "" || strings.TrimSpace(out.Meta.Model) == "" {
		return errors.New("meta.promptVersion and meta.model are required")
	}
	if out.Recommendations == nil {
		return errors.New("recommendations must be a list")
	}
	return nil
}

func normalizeFromV1(r AnalysisResultV1, analysis Analysis, topMissing, topFormatting []string) NormalizedAnalysisResult {
	meta := MetaV2{
		PromptVersion:          fallbackString(analysis.PromptVersion, "v1"),
		Model:                  analysis.Model,
		JobDescriptionProvided: strings.TrimSpace(analysis.JobDescription) != "",
		Confidence:             0,
		Assumptions:            []string{},
		Limitations:            []string{},
		Mode:                   "",
		PrimaryScoreType:       "",
	}
	if meta.Model == "" {
		meta.Model = "unknown"
	}
	missingKeywords := []string(r.ATS.MissingKeywords)
	if len(topMissing) > 0 {
		missingKeywords = topMissing
	}
	formattingIssues := r.ATS.FormattingIssues
	if len(topFormatting) > 0 {
		formattingIssues = topFormatting
	}
	ats := NormalizedATS{
		Score:            clampScore(r.ATS.Score),
		ScoreBreakdown:   ScoreBreakdownV2{},
		ScoreReasoning:   []string{},
		ScoreExplanation: ScoreExplanationV1{},
		MissingKeywords:  MissingKeywordsV2{FromJobDescription: ensureStringSlice(missingKeywords), IndustryCommon: []string{}},
		FormattingIssues: ensureStringSlice(formattingIssues),
	}
	issues := make([]IssueV2_2, 0, len(r.Issues))
	for _, issue := range r.Issues {
		issues = append(issues, IssueV2_2{
			Severity:          issue.Severity,
			Section:           issue.Section,
			Problem:           issue.Problem,
			WhyItMatters:      issue.WhyItMatters,
			Suggestion:        issue.Suggestion,
			Evidence:          "",
			FixEffort:         "",
			Priority:          0,
			AutoFixable:       false,
			RequiresUserInput: []string{},
		})
	}
	bullets := make([]NormalizedBulletRewrite, 0, len(r.BulletRewrites))
	for _, br := range r.BulletRewrites {
		bullets = append(bullets, NormalizedBulletRewrite{
			Section:            br.Section,
			Before:             br.Before,
			After:              br.After,
			Rationale:          br.Rationale,
			MetricsSource:      "resume",
			PlaceholdersNeeded: []string{},
			ClaimSupport:       "inferred",
			Evidence:           "notFound",
		})
	}

	out := NormalizedAnalysisResult{
		Meta:               normalizeMeta(meta, analysis),
		Summary:            normalizeSummary(r.Summary),
		ATS:                normalizeATS(ats),
		Issues:             ensureIssueList(issues),
		BulletRewrites:     ensureBulletList(bullets),
		MissingInformation: ensureStringSlice(r.MissingInformation),
		ActionPlan:         normalizeActionPlan(r.ActionPlan),
		Recommendations:    []Recommendation{},
	}
	return out
}

func normalizeFromV2(r AnalysisResultV2, analysis Analysis) NormalizedAnalysisResult {
	meta := normalizeMeta(r.Meta, analysis)
	ats := NormalizedATS{
		Score:            clampScore(r.ATS.Score),
		ScoreBreakdown:   clampScoreBreakdown(r.ATS.ScoreBreakdown),
		ScoreReasoning:   []string{},
		ScoreExplanation: ScoreExplanationV1{},
		MissingKeywords:  normalizeMissingKeywords(r.ATS.MissingKeywords),
		FormattingIssues: ensureStringSlice(r.ATS.FormattingIssues),
	}
	issues := make([]IssueV2_2, 0, len(r.Issues))
	for _, issue := range r.Issues {
		issues = append(issues, IssueV2_2{
			Severity:          issue.Severity,
			Section:           issue.Section,
			Problem:           issue.Problem,
			WhyItMatters:      issue.WhyItMatters,
			Suggestion:        issue.Suggestion,
			Evidence:          issue.Evidence,
			FixEffort:         issue.FixEffort,
			Priority:          0,
			AutoFixable:       false,
			RequiresUserInput: []string{},
		})
	}
	bullets := make([]NormalizedBulletRewrite, 0, len(r.BulletRewrites))
	for _, br := range r.BulletRewrites {
		bullets = append(bullets, NormalizedBulletRewrite{
			Section:            br.Section,
			Before:             br.Before,
			After:              br.After,
			Rationale:          br.Rationale,
			MetricsSource:      "resume",
			PlaceholdersNeeded: []string{},
			ClaimSupport:       "inferred",
			Evidence:           "notFound",
		})
	}
	return NormalizedAnalysisResult{
		Meta:               meta,
		Summary:            normalizeSummary(r.Summary),
		ATS:                normalizeATS(ats),
		Issues:             ensureIssueList(issues),
		BulletRewrites:     ensureBulletList(bullets),
		MissingInformation: ensureStringSlice(r.MissingInformation),
		ActionPlan:         normalizeActionPlan(r.ActionPlan),
		Recommendations:    []Recommendation{},
	}
}

func normalizeFromV2_1(r AnalysisResultV2_1, analysis Analysis) NormalizedAnalysisResult {
	meta := normalizeMeta(r.Meta, analysis)
	ats := NormalizedATS{
		Score:            clampScore(r.ATS.Score),
		ScoreBreakdown:   clampScoreBreakdown(r.ATS.ScoreBreakdown),
		ScoreReasoning:   []string{},
		ScoreExplanation: ScoreExplanationV1{},
		MissingKeywords:  normalizeMissingKeywords(r.ATS.MissingKeywords),
		FormattingIssues: ensureStringSlice(r.ATS.FormattingIssues),
	}
	issues := make([]IssueV2_2, 0, len(r.Issues))
	for _, issue := range r.Issues {
		issues = append(issues, IssueV2_2{
			Severity:          issue.Severity,
			Section:           issue.Section,
			Problem:           issue.Problem,
			WhyItMatters:      issue.WhyItMatters,
			Suggestion:        issue.Suggestion,
			Evidence:          issue.Evidence,
			FixEffort:         issue.FixEffort,
			Priority:          issue.Priority,
			AutoFixable:       false,
			RequiresUserInput: []string{},
		})
	}
	bullets := make([]NormalizedBulletRewrite, 0, len(r.BulletRewrites))
	for _, br := range r.BulletRewrites {
		bullets = append(bullets, NormalizedBulletRewrite{
			Section:            br.Section,
			Before:             br.Before,
			After:              br.After,
			Rationale:          br.Rationale,
			MetricsSource:      normalizeMetricsSource(br.MetricsSource),
			PlaceholdersNeeded: ensureStringSlice(br.PlaceholdersNeeded),
			ClaimSupport:       "inferred",
			Evidence:           "notFound",
		})
	}
	return NormalizedAnalysisResult{
		Meta:               meta,
		Summary:            normalizeSummary(r.Summary),
		ATS:                normalizeATS(ats),
		Issues:             ensureIssueList(issues),
		BulletRewrites:     ensureBulletList(bullets),
		MissingInformation: ensureStringSlice(r.MissingInformation),
		ActionPlan:         normalizeActionPlan(r.ActionPlan),
		Recommendations:    []Recommendation{},
	}
}

func normalizeFromV2_2(r AnalysisResultV2_2, analysis Analysis) NormalizedAnalysisResult {
	meta := normalizeMeta(r.Meta, analysis)
	ats := NormalizedATS{
		Score:            clampScore(r.ATS.Score),
		ScoreBreakdown:   clampScoreBreakdown(r.ATS.ScoreBreakdown),
		ScoreReasoning:   ensureStringSlice(r.ATS.ScoreReasoning),
		ScoreExplanation: ScoreExplanationV1{},
		MissingKeywords:  normalizeMissingKeywords(r.ATS.MissingKeywords),
		FormattingIssues: ensureStringSlice(r.ATS.FormattingIssues),
	}
	bullets := make([]NormalizedBulletRewrite, 0, len(r.BulletRewrites))
	for _, br := range r.BulletRewrites {
		bullets = append(bullets, NormalizedBulletRewrite{
			Section:            br.Section,
			Before:             br.Before,
			After:              br.After,
			Rationale:          br.Rationale,
			MetricsSource:      normalizeMetricsSource(br.MetricsSource),
			PlaceholdersNeeded: ensureStringSlice(br.PlaceholdersNeeded),
			ClaimSupport:       "inferred",
			Evidence:           "notFound",
		})
	}
	return NormalizedAnalysisResult{
		Meta:               meta,
		Summary:            normalizeSummary(r.Summary),
		ATS:                normalizeATS(ats),
		Issues:             ensureIssueList(r.Issues),
		BulletRewrites:     ensureBulletList(bullets),
		MissingInformation: ensureStringSlice(r.MissingInformation),
		ActionPlan:         normalizeActionPlan(r.ActionPlan),
		Recommendations:    []Recommendation{},
	}
}

func normalizeFromV2_3(r AnalysisResultV2_3, analysis Analysis) NormalizedAnalysisResult {
	meta := normalizeMeta(r.Meta, analysis)
	ats := NormalizedATS{
		Score:            clampScore(r.ATS.Score),
		ScoreBreakdown:   clampScoreBreakdown(r.ATS.ScoreBreakdown),
		ScoreReasoning:   ensureStringSlice(r.ATS.ScoreReasoning),
		ScoreExplanation: r.ATS.ScoreExplanation,
		MissingKeywords:  normalizeMissingKeywords(r.ATS.MissingKeywords),
		FormattingIssues: ensureStringSlice(r.ATS.FormattingIssues),
	}
	bullets := make([]NormalizedBulletRewrite, 0, len(r.BulletRewrites))
	for _, br := range r.BulletRewrites {
		bullets = append(bullets, NormalizedBulletRewrite{
			Section:            br.Section,
			Before:             br.Before,
			After:              br.After,
			Rationale:          br.Rationale,
			MetricsSource:      normalizeMetricsSource(br.MetricsSource),
			PlaceholdersNeeded: ensureStringSlice(br.PlaceholdersNeeded),
			ClaimSupport:       normalizeClaimSupport(br.ClaimSupport),
			Evidence:           normalizeEvidence(br.Evidence),
		})
	}
	return NormalizedAnalysisResult{
		Meta:               meta,
		Summary:            normalizeSummary(r.Summary),
		ATS:                normalizeATS(ats),
		Issues:             ensureIssueList(r.Issues),
		BulletRewrites:     ensureBulletList(bullets),
		MissingInformation: ensureStringSlice(r.MissingInformation),
		ActionPlan:         normalizeActionPlan(r.ActionPlan),
		Recommendations:    []Recommendation{},
	}
}

func normalizeFromV2_4(r AnalysisResultV2_4, analysis Analysis) NormalizedAnalysisResult {
	out := normalizeFromV2_3(AnalysisResultV2_3{
		Meta:               r.Meta,
		Summary:            r.Summary,
		ATS:                r.ATS,
		Issues:             r.Issues,
		BulletRewrites:     r.BulletRewrites,
		MissingInformation: r.MissingInformation,
		ActionPlan:         r.ActionPlan,
	}, analysis)
	out.JobRequirementProfile = normalizeJobRequirementProfileV1(r.JobRequirementProfile)
	out.JobMatchScoring = normalizeJobMatchScoringV1(r.JobMatchScoring)
	out.AIScreening = normalizeAIScreeningV1(r.AIScreening)
	out.FixThisFirst = normalizeFixThisFirstV1(r.FixThisFirst)
	if len(out.FixThisFirst) == 0 {
		out.FixThisFirst = BuildFixThisFirst(out.JobRequirementProfile, out.JobMatchScoring)
	}
	if len(out.FixThisFirst) > 3 {
		out.FixThisFirst = out.FixThisFirst[:3]
	}
	return out
}

func normalizeJobRequirementProfileV1(profile JobRequirementProfileV1) JobRequirementProfileV1 {
	profile.PrimaryRole = strings.TrimSpace(profile.PrimaryRole)
	profile.Seniority = strings.TrimSpace(profile.Seniority)
	profile.RoleType = strings.TrimSpace(profile.RoleType)
	profile.RecruiterIntentSummary = strings.TrimSpace(profile.RecruiterIntentSummary)
	if profile.TopPriorities == nil {
		profile.TopPriorities = []JobPriorityV1{}
	}
	for i := range profile.TopPriorities {
		profile.TopPriorities[i].ID = strings.TrimSpace(profile.TopPriorities[i].ID)
		profile.TopPriorities[i].Priority = strings.TrimSpace(profile.TopPriorities[i].Priority)
		profile.TopPriorities[i].Importance = strings.TrimSpace(profile.TopPriorities[i].Importance)
		profile.TopPriorities[i].EvidenceExpected = strings.TrimSpace(profile.TopPriorities[i].EvidenceExpected)
		profile.TopPriorities[i].ResumeMatchStatus = strings.TrimSpace(profile.TopPriorities[i].ResumeMatchStatus)
		profile.TopPriorities[i].WhyItMatters = strings.TrimSpace(profile.TopPriorities[i].WhyItMatters)
	}
	if profile.HiddenExpectations == nil {
		profile.HiddenExpectations = []HiddenExpectationV1{}
	}
	for i := range profile.HiddenExpectations {
		profile.HiddenExpectations[i].ID = strings.TrimSpace(profile.HiddenExpectations[i].ID)
		profile.HiddenExpectations[i].Expectation = strings.TrimSpace(profile.HiddenExpectations[i].Expectation)
		profile.HiddenExpectations[i].ResumeMatchStatus = strings.TrimSpace(profile.HiddenExpectations[i].ResumeMatchStatus)
		profile.HiddenExpectations[i].WhyItMatters = strings.TrimSpace(profile.HiddenExpectations[i].WhyItMatters)
	}
	if profile.NiceToHaveSignals == nil {
		profile.NiceToHaveSignals = []NiceToHaveSignalV1{}
	}
	for i := range profile.NiceToHaveSignals {
		profile.NiceToHaveSignals[i].ID = strings.TrimSpace(profile.NiceToHaveSignals[i].ID)
		profile.NiceToHaveSignals[i].Signal = strings.TrimSpace(profile.NiceToHaveSignals[i].Signal)
		profile.NiceToHaveSignals[i].ResumeMatchStatus = strings.TrimSpace(profile.NiceToHaveSignals[i].ResumeMatchStatus)
		profile.NiceToHaveSignals[i].WhyItMatters = strings.TrimSpace(profile.NiceToHaveSignals[i].WhyItMatters)
	}
	return profile
}

func normalizeJobMatchScoringV1(scoring JobMatchScoringV1) JobMatchScoringV1 {
	scoring.ScoringStrategy = strings.TrimSpace(scoring.ScoringStrategy)
	scoring.Explanation = strings.TrimSpace(scoring.Explanation)
	if scoring.RequirementScores == nil {
		scoring.RequirementScores = []RequirementScoreV1{}
	}
	for i := range scoring.RequirementScores {
		scoring.RequirementScores[i].RequirementID = strings.TrimSpace(scoring.RequirementScores[i].RequirementID)
		scoring.RequirementScores[i].Requirement = strings.TrimSpace(scoring.RequirementScores[i].Requirement)
		scoring.RequirementScores[i].MatchStatus = strings.TrimSpace(scoring.RequirementScores[i].MatchStatus)
		scoring.RequirementScores[i].Evidence = strings.TrimSpace(scoring.RequirementScores[i].Evidence)
		scoring.RequirementScores[i].Gap = strings.TrimSpace(scoring.RequirementScores[i].Gap)
		scoring.RequirementScores[i].Score = clampScore(scoring.RequirementScores[i].Score)
		scoring.RequirementScores[i].WeightedContribution = scoring.RequirementScores[i].Score * scoring.RequirementScores[i].Weight / 100
	}
	scoring.Score = CalculateJobMatchScore(scoring)
	return scoring
}

func normalizeAIScreeningV1(screening AIScreeningV1) AIScreeningV1 {
	screening.Verdict.Tier = strings.TrimSpace(screening.Verdict.Tier)
	screening.Verdict.Title = strings.TrimSpace(screening.Verdict.Title)
	screening.Verdict.Summary = strings.TrimSpace(screening.Verdict.Summary)
	screening.Verdict.ScreeningRisk = strings.TrimSpace(screening.Verdict.ScreeningRisk)
	if screening.ScoreBreakdown == nil {
		screening.ScoreBreakdown = []AIScreeningBreakdownItemV1{}
	}
	for i := range screening.ScoreBreakdown {
		screening.ScoreBreakdown[i].ID = strings.TrimSpace(screening.ScoreBreakdown[i].ID)
		screening.ScoreBreakdown[i].Label = strings.TrimSpace(screening.ScoreBreakdown[i].Label)
		screening.ScoreBreakdown[i].Score = clampScore(screening.ScoreBreakdown[i].Score)
		screening.ScoreBreakdown[i].Status = strings.TrimSpace(screening.ScoreBreakdown[i].Status)
		screening.ScoreBreakdown[i].Explanation = strings.TrimSpace(screening.ScoreBreakdown[i].Explanation)
		screening.ScoreBreakdown[i].ImprovementFocus = strings.TrimSpace(screening.ScoreBreakdown[i].ImprovementFocus)
	}
	screening.AIRecruiterVerdict.OneLineVerdict = strings.TrimSpace(screening.AIRecruiterVerdict.OneLineVerdict)
	screening.AIRecruiterVerdict.MainConcern = strings.TrimSpace(screening.AIRecruiterVerdict.MainConcern)
	screening.AIRecruiterVerdict.StrongestSignal = strings.TrimSpace(screening.AIRecruiterVerdict.StrongestSignal)
	screening.AIRecruiterVerdict.WeakestSignal = strings.TrimSpace(screening.AIRecruiterVerdict.WeakestSignal)
	screening.Score = CalculateAIScreeningScore(screening)
	screening.Verdict.Tier = ResolveAIScreeningTier(screening.Score)
	screening.Verdict.ScreeningRisk = ResolveAIScreeningRisk(screening.Score)
	return screening
}

func normalizeFixThisFirstV1(items []FixThisFirstItemV1) []FixThisFirstItemV1 {
	if items == nil {
		return []FixThisFirstItemV1{}
	}
	out := make([]FixThisFirstItemV1, 0, len(items))
	for _, item := range items {
		item.Title = strings.TrimSpace(item.Title)
		item.Why = strings.TrimSpace(item.Why)
		item.LinkedRequirementID = strings.TrimSpace(item.LinkedRequirementID)
		item.ExpectedImpact = strings.TrimSpace(item.ExpectedImpact)
		item.Effort = strings.TrimSpace(item.Effort)
		item.Action = strings.TrimSpace(item.Action)
		if item.Priority < 1 || item.Priority > 5 {
			continue
		}
		if item.Title == "" || item.Action == "" {
			continue
		}
		out = append(out, item)
	}
	if len(out) > 3 {
		return out[:3]
	}
	return out
}

func normalizeMeta(meta MetaV2, analysis Analysis) MetaV2 {
	meta.PromptVersion = fallbackString(meta.PromptVersion, analysis.PromptVersion)
	meta.Model = fallbackString(meta.Model, analysis.Model)
	if meta.Model == "" {
		meta.Model = "unknown"
	}
	mode := analysis.Mode
	if mode == "" {
		mode = ModeJobMatch
	}
	meta.Mode = string(mode)
	switch mode {
	case ModeATS:
		meta.PrimaryScoreType = string(ModeATS)
	case ModeJobMatch:
		meta.PrimaryScoreType = string(ModeJobMatch)
	default:
		meta.PrimaryScoreType = string(ModeJobMatch)
	}
	if meta.Assumptions == nil {
		meta.Assumptions = []string{}
	}
	if meta.Limitations == nil {
		meta.Limitations = []string{}
	}
	return meta
}

func normalizeSummary(summary SummaryV1) SummaryV1 {
	if summary.Strengths == nil {
		summary.Strengths = []string{}
	}
	if summary.Weaknesses == nil {
		summary.Weaknesses = []string{}
	}
	return summary
}

func normalizeATS(ats NormalizedATS) NormalizedATS {
	ats.Score = clampScore(ats.Score)
	ats.ScoreBreakdown = clampScoreBreakdown(ats.ScoreBreakdown)
	ats.ScoreReasoning = ensureStringSlice(ats.ScoreReasoning)
	ats.ScoreExplanation = normalizeScoreExplanation(ats.ScoreExplanation)
	ats.MissingKeywords = normalizeMissingKeywords(ats.MissingKeywords)
	ats.FormattingIssues = ensureStringSlice(ats.FormattingIssues)
	return ats
}

func applyScores(out *NormalizedAnalysisResult, mode AnalysisMode, matchScore *float64) {
	if out == nil {
		return
	}
	if mode == "" {
		mode = ModeJobMatch
	}
	var computedMatch *float64
	if matchScore != nil {
		val := clampScore(*matchScore)
		computedMatch = &val
	} else if out.Meta.JobDescriptionProvided {
		val := calculateMatchScore(out.ATS.MissingKeywords.FromJobDescription)
		computedMatch = &val
	}
	if computedMatch != nil {
		out.MatchScore = *computedMatch
	}

	switch mode {
	case ModeATS:
		out.FinalScore = clampScore(out.ATS.Score)
	case ModeJobMatch:
		if computedMatch != nil {
			out.FinalScore = *computedMatch
		} else {
			out.FinalScore = clampScore(out.ATS.Score)
			out.Meta.Limitations = append(out.Meta.Limitations, "finalScore fell back to ats.score because matchScore was unavailable")
		}
	default:
		out.FinalScore = clampScore(out.ATS.Score)
	}
}

func applyScoresV2_4(out *NormalizedAnalysisResult, mode AnalysisMode) {
	if out == nil {
		return
	}
	if mode == "" {
		mode = ModeJobMatch
	}
	switch mode {
	case ModeATS:
		out.MatchScore = 0
		out.FinalScore = clampScore(out.ATS.Score)
	case ModeJobMatch:
		out.MatchScore = clampScore(out.JobMatchScoring.Score)
		out.FinalScore = out.MatchScore
	default:
		out.MatchScore = clampScore(out.JobMatchScoring.Score)
		out.FinalScore = out.MatchScore
	}
}

func calculateMatchScore(missingJDKeywords []string) float64 {
	missing := len(ensureStringSlice(missingJDKeywords))
	if missing <= 0 {
		return 100
	}
	score := 100 - float64(missing*5)
	return clampScore(score)
}

func buildRecommendationInput(out NormalizedAnalysisResult) recommendations.Input {
	issues := make([]recommendations.Issue, 0, len(out.Issues))
	for _, issue := range out.Issues {
		issues = append(issues, recommendations.Issue{
			Severity:     string(issue.Severity),
			Section:      issue.Section,
			Problem:      issue.Problem,
			WhyItMatters: issue.WhyItMatters,
			Suggestion:   issue.Suggestion,
		})
	}
	jobPriorities := make([]recommendations.JobPriorityInput, 0, len(out.JobRequirementProfile.TopPriorities))
	for _, item := range out.JobRequirementProfile.TopPriorities {
		jobPriorities = append(jobPriorities, recommendations.JobPriorityInput{
			ID:                item.ID,
			Priority:          item.Priority,
			Importance:        item.Importance,
			Weight:            item.Weight,
			EvidenceExpected:  item.EvidenceExpected,
			ResumeMatchStatus: item.ResumeMatchStatus,
			WhyItMatters:      item.WhyItMatters,
		})
	}
	requirementScores := make([]recommendations.RequirementScoreInput, 0, len(out.JobMatchScoring.RequirementScores))
	for _, item := range out.JobMatchScoring.RequirementScores {
		requirementScores = append(requirementScores, recommendations.RequirementScoreInput{
			RequirementID: item.RequirementID,
			Requirement:   item.Requirement,
			Weight:        item.Weight,
			Score:         item.Score,
			MatchStatus:   item.MatchStatus,
			Evidence:      item.Evidence,
			Gap:           item.Gap,
		})
	}
	aiBreakdown := make([]recommendations.AIScreeningBreakdownInput, 0, len(out.AIScreening.ScoreBreakdown))
	for _, item := range out.AIScreening.ScoreBreakdown {
		aiBreakdown = append(aiBreakdown, recommendations.AIScreeningBreakdownInput{
			ID:               item.ID,
			Label:            item.Label,
			Score:            item.Score,
			Weight:           item.Weight,
			Status:           item.Status,
			Explanation:      item.Explanation,
			ImprovementFocus: item.ImprovementFocus,
		})
	}
	fixThisFirst := make([]recommendations.FixThisFirstInput, 0, len(out.FixThisFirst))
	for _, item := range out.FixThisFirst {
		fixThisFirst = append(fixThisFirst, recommendations.FixThisFirstInput{
			Priority:            item.Priority,
			Title:               item.Title,
			Why:                 item.Why,
			LinkedRequirementID: item.LinkedRequirementID,
			ExpectedImpact:      item.ExpectedImpact,
			Effort:              item.Effort,
			Action:              item.Action,
			RequiresUserInput:   item.RequiresUserInput,
		})
	}
	actionPlan := recommendations.ActionPlan{
		QuickWins:    ensureStringSlice(out.ActionPlan.QuickWins),
		MediumEffort: ensureStringSlice(out.ActionPlan.MediumEffort),
		DeepFixes:    ensureStringSlice(out.ActionPlan.DeepFixes),
	}
	return recommendations.Input{
		Issues:               issues,
		MissingJDKeywords:    ensureStringSlice(out.ATS.MissingKeywords.FromJobDescription),
		MissingIndustryTerms: ensureStringSlice(out.ATS.MissingKeywords.IndustryCommon),
		FormattingIssues:     ensureStringSlice(out.ATS.FormattingIssues),
		ActionPlan:           actionPlan,
		MissingInformation:   ensureStringSlice(out.MissingInformation),
		JobPriorities:        jobPriorities,
		RequirementScores:    requirementScores,
		AIScreeningBreakdown: aiBreakdown,
		FixThisFirst:         fixThisFirst,
	}
}

func normalizeMissingKeywords(m MissingKeywordsV2) MissingKeywordsV2 {
	m.FromJobDescription = ensureStringSlice(m.FromJobDescription)
	m.IndustryCommon = ensureStringSlice(m.IndustryCommon)
	return m
}

func normalizeActionPlan(plan ActionPlanV1) ActionPlanV1 {
	if plan.QuickWins == nil {
		plan.QuickWins = []string{}
	}
	if plan.MediumEffort == nil {
		plan.MediumEffort = []string{}
	}
	if plan.DeepFixes == nil {
		plan.DeepFixes = []string{}
	}
	return plan
}

func ensureStringSlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

func ensureIssueList(value []IssueV2_2) []IssueV2_2 {
	if value == nil {
		return []IssueV2_2{}
	}
	for i := range value {
		if value[i].RequiresUserInput == nil {
			value[i].RequiresUserInput = []string{}
		}
	}
	return value
}

func ensureBulletList(value []NormalizedBulletRewrite) []NormalizedBulletRewrite {
	if value == nil {
		return []NormalizedBulletRewrite{}
	}
	for i := range value {
		if value[i].PlaceholdersNeeded == nil {
			value[i].PlaceholdersNeeded = []string{}
		}
		if strings.TrimSpace(value[i].MetricsSource) == "" {
			value[i].MetricsSource = "resume"
		}
		if strings.TrimSpace(value[i].ClaimSupport) == "" {
			value[i].ClaimSupport = "inferred"
		}
		if strings.TrimSpace(value[i].Evidence) == "" {
			value[i].Evidence = "notFound"
		}
	}
	return value
}

func normalizeMetricsSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "resume", "placeholder":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "resume"
	}
}

func normalizeClaimSupport(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "supported", "inferred", "placeholder":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "inferred"
	}
}

func normalizeEvidence(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "notFound"
	}
	return trimmed
}

func clampScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func clampScoreBreakdown(b ScoreBreakdownV2) ScoreBreakdownV2 {
	b.Skills = clampScore(b.Skills)
	b.Experience = clampScore(b.Experience)
	b.Impact = clampScore(b.Impact)
	b.Formatting = clampScore(b.Formatting)
	b.RoleFit = clampScore(b.RoleFit)
	return b
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func extractStringSlice(value any) []string {
	switch raw := value.(type) {
	case []string:
		return raw
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func extractFloat(value any) *float64 {
	switch raw := value.(type) {
	case float64:
		return &raw
	case float32:
		val := float64(raw)
		return &val
	case int:
		val := float64(raw)
		return &val
	case int64:
		val := float64(raw)
		return &val
	case json.Number:
		if parsed, err := raw.Float64(); err == nil {
			return &parsed
		}
	}
	return nil
}
