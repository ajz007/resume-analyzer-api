package workerproc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"resume-backend/internal/analyses"
	"resume-backend/internal/bootstrap"
	"resume-backend/internal/queue"
	"resume-backend/internal/resumes"
)

// MessageMeta captures details useful for logging and diagnostics.
type MessageMeta struct {
	BodyLen int
	BodySHA string
}

// ComputeMeta returns the body length and SHA-256 hash.
func ComputeMeta(body string) MessageMeta {
	if body == "" {
		return MessageMeta{BodyLen: 0, BodySHA: ""}
	}
	sum := sha256.Sum256([]byte(body))
	return MessageMeta{BodyLen: len(body), BodySHA: hex.EncodeToString(sum[:])}
}

// ErrEmptyBody indicates an empty queue payload.
type ErrEmptyBody struct {
	Meta MessageMeta
}

func (e ErrEmptyBody) Error() string { return "empty message body" }

// ErrDecode indicates a JSON decode failure.
type ErrDecode struct {
	Meta MessageMeta
	Err  error
}

func (e ErrDecode) Error() string {
	if e.Err == nil {
		return "decode message"
	}
	return "decode message: " + e.Err.Error()
}

// ErrMissingAnalysisID indicates a message missing the analysis id.
type ErrMissingAnalysisID struct {
	Meta      MessageMeta
	RequestID string
}

func (e ErrMissingAnalysisID) Error() string { return "missing analysis id" }

type ErrMissingJobID struct {
	Meta      MessageMeta
	RequestID string
}

func (e ErrMissingJobID) Error() string { return "missing job id" }

type ErrUnsupportedType struct {
	Meta        MessageMeta
	MessageType string
	RequestID   string
}

func (e ErrUnsupportedType) Error() string {
	if strings.TrimSpace(e.MessageType) == "" {
		return "unsupported message type"
	}
	return "unsupported message type: " + e.MessageType
}

// ErrProcess indicates processing failed after successful parsing.
type ErrProcess struct {
	Type       string
	AnalysisID string
	JobID      string
	RequestID  string
	Err        error
}

func (e ErrProcess) Error() string {
	if e.Err == nil {
		return "process analysis"
	}
	return "process analysis: " + e.Err.Error()
}

// ParseMessage validates and decodes the queue payload.
func ParseMessage(body string) (queue.Message, MessageMeta, error) {
	meta := ComputeMeta(body)
	if strings.TrimSpace(body) == "" {
		return queue.Message{}, meta, ErrEmptyBody{Meta: meta}
	}

	msg, err := queue.DecodeMessage([]byte(body))
	if err != nil {
		return queue.Message{}, meta, ErrDecode{Meta: meta, Err: err}
	}
	switch strings.TrimSpace(msg.Type) {
	case "", queue.MessageTypeAnalysis:
		if strings.TrimSpace(msg.AnalysisID) == "" {
			return msg, meta, ErrMissingAnalysisID{Meta: meta, RequestID: msg.RequestID}
		}
	case queue.MessageTypeResumeGeneration:
		if strings.TrimSpace(msg.JobID) == "" {
			return msg, meta, ErrMissingJobID{Meta: meta, RequestID: msg.RequestID}
		}
	default:
		return queue.Message{}, meta, ErrUnsupportedType{Meta: meta, MessageType: strings.TrimSpace(msg.Type), RequestID: msg.RequestID}
	}
	return msg, meta, nil
}

type parsedMessageKey struct{}

// WithParsedMessage stores a decoded message in the context for reuse.
func WithParsedMessage(ctx context.Context, msg queue.Message) context.Context {
	return context.WithValue(ctx, parsedMessageKey{}, msg)
}

func parsedMessageFromContext(ctx context.Context) (queue.Message, bool) {
	if ctx == nil {
		return queue.Message{}, false
	}
	msg, ok := ctx.Value(parsedMessageKey{}).(queue.Message)
	return msg, ok
}

// HandleMessage parses, validates, and processes a message payload.
func HandleMessage(ctx context.Context, app *bootstrap.App, body string) error {
	if app == nil {
		return errors.New("analysis service not configured")
	}
	processor := app.AnalysisProcessor
	if processor == nil {
		processor = app.AnalysesService
	}
	if processor == nil {
		return errors.New("analysis service not configured")
	}

	msg, ok := parsedMessageFromContext(ctx)
	if !ok {
		var err error
		msg, _, err = ParseMessage(body)
		if err != nil {
			return err
		}
	}

	switch strings.TrimSpace(msg.Type) {
	case "", queue.MessageTypeAnalysis:
		if strings.TrimSpace(msg.AnalysisID) == "" {
			return ErrMissingAnalysisID{Meta: ComputeMeta(body), RequestID: msg.RequestID}
		}
		ctxWithRequest := analyses.WithRequestID(ctx, msg.RequestID)
		if err := processor.ProcessAnalysis(ctxWithRequest, msg.AnalysisID); err != nil {
			return ErrProcess{Type: queue.MessageTypeAnalysis, AnalysisID: msg.AnalysisID, RequestID: msg.RequestID, Err: err}
		}
		return nil
	case queue.MessageTypeResumeGeneration:
		if strings.TrimSpace(msg.JobID) == "" {
			return ErrMissingJobID{Meta: ComputeMeta(body), RequestID: msg.RequestID}
		}
		if app.ResumeGenerationProcessor == nil {
			return errors.New("resume generation processor not configured")
		}
		ctxWithRequest := resumes.WithRequestLogContext(ctx, msg.RequestID, "")
		if err := app.ResumeGenerationProcessor.ProcessResumeGenerationJob(ctxWithRequest, msg.JobID); err != nil {
			return ErrProcess{Type: queue.MessageTypeResumeGeneration, JobID: msg.JobID, RequestID: msg.RequestID, Err: err}
		}
		return nil
	default:
		return errors.New("unsupported message type")
	}
}
