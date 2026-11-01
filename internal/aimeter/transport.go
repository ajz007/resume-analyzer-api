package aimeter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
)

const maxCaptureBytes = 2 << 20

var providerModelPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,160}$`)
var providerIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,160}$`)

func modelID(s string) string {
	if providerModelPattern.MatchString(s) {
		return s
	}
	return ""
}
func providerID(s string) string {
	if providerIDPattern.MatchString(s) {
		return s
	}
	return ""
}

type Transport struct {
	Base     http.RoundTripper
	Recorder Recorder
	Model    string
}

// NewTransport replaces a previous meter instead of nesting accounting wrappers.
func NewTransport(base http.RoundTripper, recorder Recorder, model string) *Transport {
	if previous, ok := base.(*Transport); ok {
		base = previous.Base
	}
	return &Transport{Base: base, Recorder: recorder, Model: model}
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if t.Recorder == nil {
		return base.RoundTrip(req)
	}
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	s, n := nextScope(req.Context())
	a := Attempt{ID: uuid.NewString(), OperationID: s.ID, Operation: s.Kind, AnalysisID: s.AnalysisID, ObjectID: s.ObjectID, Stage: s.Stage, Retry: s.Retry, Number: n, Provider: "openai", RequestedModel: t.Model, StartedAt: time.Now().UTC(), UsageState: "pending", Outcome: "pending", UnknownReason: "awaiting_response"}
	writeRecord(t.Recorder, a, false)
	start := time.Now()
	resp, err := base.RoundTrip(req)
	if err != nil {
		a.UsageState = "unknown"
		a.Outcome = "transport_error"
		a.UnknownReason = "transport_error"
		if errors.Is(err, context.DeadlineExceeded) || req.Context().Err() != nil {
			a.Outcome = "timeout_or_cancelled"
		}
		finish(t.Recorder, &a, start, nil, false, a.UnknownReason)
		return resp, err
	}
	a.HTTPStatus = resp.StatusCode
	a.ProviderRequestID = providerID(resp.Header.Get("X-Request-Id"))
	a.Outcome = "response"
	if resp.StatusCode >= 400 {
		a.Outcome = "http_error"
	}
	if resp.Body == nil {
		finish(t.Recorder, &a, start, nil, false, "missing_body")
		return resp, nil
	}
	resp.Body = &observedBody{ReadCloser: resp.Body, recorder: t.Recorder, attempt: a, start: start}
	return resp, nil
}

type observedBody struct {
	io.ReadCloser
	recorder Recorder
	attempt  Attempt
	start    time.Time
	buffer   bytes.Buffer
	overflow bool
	done     sync.Once
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if !b.overflow && n > 0 {
		if b.buffer.Len()+n <= maxCaptureBytes {
			_, _ = b.buffer.Write(p[:n])
		} else {
			b.overflow = true
			b.buffer.Reset()
		}
	}
	if err != nil {
		b.done.Do(func() {
			reason := "response_read_error"
			if err == io.EOF {
				reason = ""
			}
			if b.overflow {
				reason = "capture_limit"
			}
			finish(b.recorder, &b.attempt, b.start, b.buffer.Bytes(), err == io.EOF && !b.overflow, reason)
		})
	}
	return n, err
}
func (b *observedBody) Close() error {
	err := b.ReadCloser.Close()
	b.done.Do(func() { finish(b.recorder, &b.attempt, b.start, nil, false, "incomplete_body") })
	return err
}

type usageEnvelope struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	ServiceTier string `json:"service_tier"`
	Usage       *struct {
		Input  *int64 `json:"prompt_tokens"`
		Output *int64 `json:"completion_tokens"`
		Total  *int64 `json:"total_tokens"`
		Prompt struct {
			Cached int64 `json:"cached_tokens"`
			Write  int64 `json:"cache_write_tokens"`
			Audio  int64 `json:"audio_tokens"`
			Image  int64 `json:"image_tokens"`
		} `json:"prompt_tokens_details"`
		Completion struct {
			Reasoning int64 `json:"reasoning_tokens"`
			Audio     int64 `json:"audio_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

func finish(r Recorder, a *Attempt, start time.Time, body []byte, complete bool, reason string) {
	now := time.Now().UTC()
	a.FinishedAt = &now
	a.LatencyMS = time.Since(start).Milliseconds()
	a.UsageState = "unknown"
	a.UnknownReason = reason
	nonText := false
	if complete {
		var envelope usageEnvelope
		if err := json.Unmarshal(body, &envelope); err != nil {
			a.UnknownReason = "invalid_response"
		} else {
			a.ResponseID = providerID(envelope.ID)
			// Model/tier are provider metadata, never derived from generated content.
			a.ResponseModel = modelID(envelope.Model)
			a.ServiceTier = providerID(envelope.ServiceTier)
			u := envelope.Usage
			switch {
			case u == nil:
				a.UnknownReason = "missing_usage"
			case u.Input == nil || u.Output == nil || u.Total == nil:
				a.UnknownReason = "incomplete_usage"
			case *u.Input < 0 || *u.Output < 0 || *u.Total < 0 || *u.Input > 1000000000 || *u.Output > 1000000000 || *u.Total != *u.Input+*u.Output || u.Prompt.Cached < 0 || u.Prompt.Write < 0 || u.Prompt.Cached > *u.Input || u.Prompt.Write > *u.Input || u.Prompt.Cached+u.Prompt.Write > *u.Input || u.Completion.Reasoning < 0 || u.Completion.Reasoning > *u.Output || u.Prompt.Audio < 0 || u.Prompt.Image < 0 || u.Completion.Audio < 0:
				a.UnknownReason = "invalid_usage"
			default:
				a.UsageState = "known"
				a.UnknownReason = ""
				a.InputTokens = u.Input
				a.OutputTokens = u.Output
				a.TotalTokens = u.Total
				a.CachedTokens = &u.Prompt.Cached
				a.CacheWriteTokens = &u.Prompt.Write
				a.ReasoningTokens = &u.Completion.Reasoning
				nonText = u.Prompt.Audio > 0 || u.Prompt.Image > 0 || u.Completion.Audio > 0
			}
		}
	}
	if a.UnknownReason == "" && a.UsageState != "known" {
		a.UnknownReason = "missing_usage"
	}
	estimate(a, nonText)
	writeRecord(r, *a, true)
}
func writeRecord(r Recorder, a Attempt, terminal bool) {
	// Accounting failure must never cause a retry of a billable provider operation.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var err error
	if terminal {
		err = r.Finish(ctx, a)
	} else {
		err = r.Start(ctx, a)
	}
	if err != nil {
		log.Printf("ai.usage.record_gap attempt_id=%s phase=%t", a.ID, terminal)
	}
}
