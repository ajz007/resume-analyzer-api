package aimeter

import (
	"context"
	"time"
)

// Attempt stores provider accounting metadata; never prompts, responses or raw errors.
type Attempt struct {
	ID, OperationID, Operation, AnalysisID, ObjectID, Stage, Retry                          string
	Number                                                                                  int64
	Provider, RequestedModel, ResponseModel, ProviderRequestID, ResponseID, ServiceTier     string
	StartedAt                                                                               time.Time
	FinishedAt                                                                              *time.Time
	LatencyMS                                                                               int64
	HTTPStatus                                                                              int
	Outcome, UsageState, UnknownReason                                                      string
	InputTokens, OutputTokens, TotalTokens, CachedTokens, CacheWriteTokens, ReasoningTokens *int64
	EstimateNanoUSD                                                                         *int64
	PricingVersion, UnpricedReason                                                          string
}
type Recorder interface {
	Start(context.Context, Attempt) error
	Finish(context.Context, Attempt) error
}
