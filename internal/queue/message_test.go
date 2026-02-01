package queue

import (
	"reflect"
	"testing"
)

func TestMessageRoundTrip(t *testing.T) {
	msg := Message{
		AnalysisID: "analysis-123",
		RequestID:  "request-456",
		EnqueuedAt: "2026-01-30T22:00:00Z",
		Version:    1,
	}

	payload, err := EncodeMessage(msg)
	if err != nil {
		t.Fatalf("encode message: %v", err)
	}

	got, err := DecodeMessage(payload)
	if err != nil {
		t.Fatalf("decode message: %v", err)
	}

	if !reflect.DeepEqual(got, msg) {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, msg)
	}
}
