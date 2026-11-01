package aimeter

import (
	"context"
	"database/sql"
)

type PGRecorder struct{ DB *sql.DB }

func nullableID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
func (r *PGRecorder) Start(ctx context.Context, a Attempt) error  { return r.save(ctx, a, false) }
func (r *PGRecorder) Finish(ctx context.Context, a Attempt) error { return r.save(ctx, a, true) }
func (r *PGRecorder) save(ctx context.Context, a Attempt, terminal bool) error {
	query := `INSERT INTO ai_usage_attempts(id,operation_id,operation,analysis_id,object_id,stage,retry_kind,attempt_number,provider,requested_model,response_model,provider_request_id,response_id,service_tier,started_at,finished_at,latency_ms,http_status,outcome,usage_state,unknown_reason,input_tokens,output_tokens,total_tokens,cached_tokens,cache_write_tokens,reasoning_tokens,estimate_nano_usd,pricing_version,unpriced_reason)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30)
 ON CONFLICT(id) DO NOTHING`
	if terminal {
		query = `INSERT INTO ai_usage_attempts(id,operation_id,operation,analysis_id,object_id,stage,retry_kind,attempt_number,provider,requested_model,response_model,provider_request_id,response_id,service_tier,started_at,finished_at,latency_ms,http_status,outcome,usage_state,unknown_reason,input_tokens,output_tokens,total_tokens,cached_tokens,cache_write_tokens,reasoning_tokens,estimate_nano_usd,pricing_version,unpriced_reason)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30)
 ON CONFLICT(id) DO UPDATE SET response_model=EXCLUDED.response_model,provider_request_id=EXCLUDED.provider_request_id,response_id=EXCLUDED.response_id,service_tier=EXCLUDED.service_tier,finished_at=EXCLUDED.finished_at,latency_ms=EXCLUDED.latency_ms,http_status=EXCLUDED.http_status,outcome=EXCLUDED.outcome,usage_state=EXCLUDED.usage_state,unknown_reason=EXCLUDED.unknown_reason,input_tokens=EXCLUDED.input_tokens,output_tokens=EXCLUDED.output_tokens,total_tokens=EXCLUDED.total_tokens,cached_tokens=EXCLUDED.cached_tokens,cache_write_tokens=EXCLUDED.cache_write_tokens,reasoning_tokens=EXCLUDED.reasoning_tokens,estimate_nano_usd=EXCLUDED.estimate_nano_usd,pricing_version=EXCLUDED.pricing_version,unpriced_reason=EXCLUDED.unpriced_reason WHERE ai_usage_attempts.usage_state='pending'`
	}
	_, err := r.DB.ExecContext(ctx, query, a.ID, a.OperationID, a.Operation, nullableID(a.AnalysisID), nullableID(a.ObjectID), a.Stage, a.Retry, a.Number, a.Provider, a.RequestedModel, a.ResponseModel, a.ProviderRequestID, a.ResponseID, a.ServiceTier, a.StartedAt, a.FinishedAt, a.LatencyMS, a.HTTPStatus, a.Outcome, a.UsageState, a.UnknownReason, a.InputTokens, a.OutputTokens, a.TotalTokens, a.CachedTokens, a.CacheWriteTokens, a.ReasoningTokens, a.EstimateNanoUSD, a.PricingVersion, a.UnpricedReason)
	return err
}
