package main

import (
	"context"
	"encoding/json"
	"os"
	"resume-backend/internal/analyses"
	"resume-backend/internal/bootstrap"
	"sync"
	"sync/atomic"
	"testing"
)

func TestInvocationRouting(t *testing.T) {
	for _, tc := range []struct {
		name, body        string
		recovery, wantErr bool
		records           int
	}{
		{"scheduled", `{"type":"analysis_recovery"}`, true, false, 0},
		{"SQS", `{"Records":[{"messageId":"one","body":"{}"}]}`, false, false, 1},
		{"unknown", `{"type":"other"}`, false, true, 0},
		{"missing", `{}`, false, true, 0},
		{"mixed", `{"type":"analysis_recovery","Records":[]}`, false, true, 0},
		{"bad JSON", `{`, false, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recovery, event, err := decodeInvocation(json.RawMessage(tc.body))
			if (err != nil) != tc.wantErr || recovery != tc.recovery || len(event.Records) != tc.records {
				t.Fatalf("recovery=%v event=%+v err=%v", recovery, event, err)
			}
		})
	}
}

type forbiddenSQSProcessor struct{ calls atomic.Int32 }

func (p *forbiddenSQSProcessor) ProcessAnalysis(context.Context, string) error {
	p.calls.Add(1)
	return nil
}
func TestScheduledRecoveryDoesNotExecuteSQSAnalysisProcessor(t *testing.T) {
	originalApp, originalErr := app, initErr
	initOnce = sync.Once{}
	initOnce.Do(func() {})
	defer func() { app = originalApp; initErr = originalErr; initOnce = sync.Once{} }()
	processor := &forbiddenSQSProcessor{}
	app = &bootstrap.App{AnalysisProcessor: processor, AnalysesService: &analyses.Service{Repo: analyses.NewMemoryRepo()}}
	initErr = nil
	if _, err := handler(context.Background(), json.RawMessage(`{"type":"analysis_recovery"}`)); err != nil {
		t.Fatal(err)
	}
	if processor.calls.Load() != 0 {
		t.Fatal("scheduler started provider work inline")
	}
}
func TestRecoveryTemplatePayloadMatchesHandler(t *testing.T) {
	body, err := os.ReadFile("../../infra/analysis-recovery.json")
	if err != nil {
		t.Fatal(err)
	}
	var template struct {
		Parameters map[string]struct{ Default string }
		Resources  map[string]struct {
			Properties struct {
				Targets            []struct{ Input string }
				ScheduleExpression string
			}
		}
	}
	if err := json.Unmarshal(body, &template); err != nil {
		t.Fatal(err)
	}
	if template.Parameters["ScheduleState"].Default != "DISABLED" {
		t.Fatal("schedule is unexpectedly enabled by default")
	}
	rule := template.Resources["RecoveryRule"].Properties
	if rule.ScheduleExpression != "rate(30 minutes)" || len(rule.Targets) != 1 {
		t.Fatal("unexpected recovery cadence")
	}
	recovery, _, err := decodeInvocation(json.RawMessage(rule.Targets[0].Input))
	if err != nil || !recovery {
		t.Fatalf("scheduled input rejected: %v", err)
	}
}
