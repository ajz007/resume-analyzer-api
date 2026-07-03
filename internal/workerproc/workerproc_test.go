package workerproc

import (
	"context"
	"testing"

	"resume-backend/internal/bootstrap"
	"resume-backend/internal/queue"
)

type fakeAnalysisProcessor struct {
	analysisIDs []string
}

func (f *fakeAnalysisProcessor) ProcessAnalysis(ctx context.Context, analysisID string) error {
	_ = ctx
	f.analysisIDs = append(f.analysisIDs, analysisID)
	return nil
}

type fakeResumeGenerationProcessor struct {
	jobIDs []string
}

func (f *fakeResumeGenerationProcessor) ProcessResumeGenerationJob(ctx context.Context, generationID string) error {
	_ = ctx
	f.jobIDs = append(f.jobIDs, generationID)
	return nil
}

func TestHandleMessageDispatchesTypedResumeGeneration(t *testing.T) {
	analysisProc := &fakeAnalysisProcessor{}
	resumeProc := &fakeResumeGenerationProcessor{}
	app := &bootstrap.App{
		AnalysisProcessor:         analysisProc,
		ResumeGenerationProcessor: resumeProc,
	}
	body := `{"type":"resume_generation","jobId":"job-1","requestId":"req-1"}`
	if err := HandleMessage(context.Background(), app, body); err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if len(resumeProc.jobIDs) != 1 || resumeProc.jobIDs[0] != "job-1" {
		t.Fatalf("expected resume generation processor to receive job-1, got %#v", resumeProc.jobIDs)
	}
	if len(analysisProc.analysisIDs) != 0 {
		t.Fatalf("expected no analysis dispatch, got %#v", analysisProc.analysisIDs)
	}
}

func TestHandleMessageDispatchesTypedAnalysis(t *testing.T) {
	analysisProc := &fakeAnalysisProcessor{}
	app := &bootstrap.App{AnalysisProcessor: analysisProc}
	body := `{"type":"analysis","analysisId":"analysis-1","requestId":"req-1"}`
	if err := HandleMessage(context.Background(), app, body); err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if len(analysisProc.analysisIDs) != 1 || analysisProc.analysisIDs[0] != "analysis-1" {
		t.Fatalf("expected analysis processor to receive analysis-1, got %#v", analysisProc.analysisIDs)
	}
}

func TestHandleMessageSupportsLegacyAnalysisMessage(t *testing.T) {
	analysisProc := &fakeAnalysisProcessor{}
	app := &bootstrap.App{AnalysisProcessor: analysisProc}
	payload, err := queue.EncodeMessage(queue.Message{AnalysisID: "analysis-legacy", RequestID: "req-legacy"})
	if err != nil {
		t.Fatalf("encode message: %v", err)
	}
	if err := HandleMessage(context.Background(), app, string(payload)); err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if len(analysisProc.analysisIDs) != 1 || analysisProc.analysisIDs[0] != "analysis-legacy" {
		t.Fatalf("expected legacy analysis dispatch, got %#v", analysisProc.analysisIDs)
	}
}

func TestParseMessageRejectsUnknownTypedMessage(t *testing.T) {
	_, _, err := ParseMessage(`{"type":"other_job","jobId":"job-1","requestId":"req-1"}`)
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	unsupported, ok := err.(ErrUnsupportedType)
	if !ok {
		t.Fatalf("expected ErrUnsupportedType, got %T", err)
	}
	if unsupported.MessageType != "other_job" {
		t.Fatalf("expected message type other_job, got %#v", unsupported)
	}
}
