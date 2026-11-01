package analyses

import (
	"context"
	"errors"
	"fmt"
	"time"

	"resume-backend/internal/queue"
	"resume-backend/internal/shared/telemetry"
)

const analysisExecutionBudget = 10 * time.Minute

type durableJobs interface {
	DurableJobs() bool
	AdoptDispatch(context.Context, string) error
	ClaimDispatch(context.Context, string) (string, string, bool, error)
	FinishDispatch(context.Context, string, string, bool) error
	ExpireJob(context.Context, string) (bool, error)
	RecoveryIDs(context.Context, int) ([]string, error)
}

func (s *Service) usesDurableJobs() bool { r, ok := s.Repo.(durableJobs); return ok && r.DurableJobs() }

func (s *Service) recoverSavedAnalysis(ctx context.Context, a Analysis) (Analysis, error) {
	r, ok := s.Repo.(durableJobs)
	if !ok || !r.DurableJobs() {
		return a, nil
	}
	changed, err := r.ExpireJob(ctx, a.ID)
	if err != nil {
		return a, err
	}
	if changed {
		s.settleAnalysisUsage(a.ID)
		return s.Repo.GetByID(ctx, a.ID)
	}
	if a.Status != StatusQueued {
		return a, nil
	}
	if err := r.AdoptDispatch(ctx, a.ID); err != nil {
		return a, err
	}
	if s.JobQueue == nil {
		return a, ErrJobQueueNotConfigured
	}
	token, requestID, claimed, err := r.ClaimDispatch(ctx, a.ID)
	if err != nil || !claimed {
		return a, err
	}
	// Never hold a database transaction while calling SQS. Expired dispatch leases
	// may resend messages; the analysis processing claim prevents duplicate AI work.
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	sendErr := s.JobQueue.Send(sendCtx, queue.Message{Type: queue.MessageTypeAnalysis, AnalysisID: a.ID, RequestID: requestID, EnqueuedAt: time.Now().UTC().Format(time.RFC3339Nano), Version: 1})
	cancel()
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finishCancel()
	finishErr := r.FinishDispatch(finishCtx, a.ID, token, sendErr == nil)
	return a, errors.Join(sendErr, finishErr)
}
func (s *Service) acceptedDurableAnalysis(ctx context.Context, a Analysis) Analysis {
	recovered, err := s.recoverSavedAnalysis(ctx, a)
	if err != nil {
		telemetry.Info("analysis.dispatch.pending", map[string]any{"analysis_id": a.ID, "error": err.Error()})
	}
	return recovered
}

// GetOwnedAndRecover checks ownership before any recovery side effect.
func (s *Service) GetOwnedAndRecover(ctx context.Context, userID, id string) (Analysis, error) {
	a, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		return Analysis{}, err
	}
	if a.UserID != userID {
		return Analysis{}, ErrNotFound
	}
	if s.usesDurableJobs() {
		return s.acceptedDurableAnalysis(ctx, a), nil
	}
	return a, nil
}

// RecoverBatch is for trusted scheduled/operator invocation, not a public route.
func (s *Service) RecoverBatch(ctx context.Context) (int, error) {
	r, ok := s.Repo.(durableJobs)
	if !ok || !r.DurableJobs() {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ids, err := r.RecoveryIDs(ctx, 25)
	if err != nil {
		return 0, err
	}
	var failures []error
	checked := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return checked, errors.Join(append(failures, ctx.Err())...)
		}
		// Deleted analyses are not readable through the owner repository, but a
		// saved cancellation must still settle any established reservation.
		expired, err := r.ExpireJob(ctx, id)
		if expired {
			if s.Usage != nil {
				err = s.Usage.SettleAnalysis(ctx, id, StatusFailed)
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("settle expired %s: %w", id, err))
			}
			checked++
			continue
		}
		if err != nil {
			failures = append(failures, err)
			checked++
			continue
		}
		a, err := s.Repo.GetByID(ctx, id)
		if errors.Is(err, ErrNotFound) {
			// A deleted terminal row may have been saved before settlement crashed.
			if s.Usage != nil {
				err = s.Usage.SettleAnalysis(ctx, id, StatusFailed)
			} else {
				err = nil
			}
		} else if err == nil {
			if (a.Status == StatusCompleted || a.Status == StatusFailed) && s.Usage != nil {
				err = s.Usage.SettleAnalysis(ctx, id, a.Status)
			} else {
				_, err = s.recoverSavedAnalysis(ctx, a)
			}
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("recover %s: %w", id, err))
		}
		checked++
	}
	return checked, errors.Join(failures...)
}
