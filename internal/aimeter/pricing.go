package aimeter

// Standard global text estimate, verified 2026-11-01 against the official model page.
// An estimate is not an invoice, and unknown models/tiers remain unpriced.
func estimate(a *Attempt, nonText bool) {
	a.UnpricedReason = "unknown_usage"
	if a.UsageState != "known" {
		return
	}
	if a.RequestedModel != "gpt-6.1-sol" || a.ResponseModel != "gpt-6.1-sol" {
		a.UnpricedReason = "unsupported_model"
		return
	}
	if a.ServiceTier != "default" {
		a.UnpricedReason = "unsupported_tier"
		return
	}
	if nonText {
		a.UnpricedReason = "non_text_usage"
		return
	}
	input, cached, write, output := int64(2000), int64(100), int64(2500), int64(10000) // nano-USD/token
	if *a.InputTokens > 272000 {
		input *= 2
		cached *= 2
		write *= 2
		output = 15000
	}
	value := (*a.InputTokens-*a.CachedTokens-*a.CacheWriteTokens)*input + *a.CachedTokens*cached + *a.CacheWriteTokens*write + *a.OutputTokens*output
	a.EstimateNanoUSD = &value
	a.PricingVersion = "openai-gpt-6.1-sol-standard-global-2026-11-01"
	a.UnpricedReason = ""
}
