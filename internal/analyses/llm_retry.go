package analyses

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"strings"
	"time"

	"resume-backend/internal/llm"
)

const llmRetryBaseDelay = 300 * time.Millisecond

type retryingLLM struct {
	base       llm.Client
	requestID  string
	analysisID string
}

func newRetryingLLM(base llm.Client, analysisID, requestID string) llm.Client {
	if base == nil {
		return nil
	}
	return retryingLLM{
		base:       base,
		requestID:  requestID,
		analysisID: analysisID,
	}
}

func (r retryingLLM) AnalyzeResume(ctx context.Context, input llm.AnalyzeInput) (json.RawMessage, error) {
	resp, err := r.base.AnalyzeResume(ctx, input)
	if err == nil || !shouldRetryLLM(err) {
		return resp, err
	}

	delay := llmRetryBaseDelay
	log.Printf("llm retry attempt=1 request_id=%s analysis_id=%s error=%s", r.requestID, r.analysisID, sanitizeError(err))
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return r.base.AnalyzeResume(ctx, input)
}

func shouldRetryLLM(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "http status 5") || strings.Contains(msg, "server_error") {
		return true
	}
	if strings.Contains(msg, "timeout") && (strings.Contains(msg, "openai") || strings.Contains(msg, "llm") || strings.Contains(msg, "client.timeout")) {
		return true
	}
	if strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "tls handshake timeout") ||
		strings.Contains(msg, "eof") {
		return true
	}

	return false
}
