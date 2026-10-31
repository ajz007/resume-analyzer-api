package analyses

// Legacy reports retain their historical reuse semantics. V3 is versioned by
// prompt, model, provider and analysis configuration, preventing stale layouts.
func compatibleAnalysis(existing, requested Analysis) bool {
	if existing.PromptVersion != "v3" && requested.PromptVersion != "v3" {
		return true
	}
	return existing.PromptVersion == requested.PromptVersion && existing.Model == requested.Model && existing.Provider == requested.Provider && existing.AnalysisVersion == requested.AnalysisVersion
}
