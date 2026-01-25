package analyses

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryRepo stores analyses in memory and is safe for concurrent use.
type MemoryRepo struct {
	mu     sync.RWMutex
	byID   map[string]Analysis
	byUser map[string][]Analysis
}

// NewMemoryRepo constructs a MemoryRepo.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		byID:   make(map[string]Analysis),
		byUser: make(map[string][]Analysis),
	}
}

// GetOrCreateForDocument returns the latest analysis for a document or creates a new one.
func (r *MemoryRepo) GetOrCreateForDocument(ctx context.Context, analysis Analysis, allowRetry bool, allowCreate func() error) (Analysis, bool, error) {
	if err := ctx.Err(); err != nil {
		return Analysis{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var latest *Analysis
	for _, existing := range r.byUser[analysis.UserID] {
		if existing.DocumentID != analysis.DocumentID {
			continue
		}
		if latest == nil || existing.CreatedAt.After(latest.CreatedAt) {
			copy := existing
			latest = &copy
		}
	}

	if latest != nil {
		switch latest.Status {
		case StatusQueued, StatusProcessing:
			return *latest, false, nil
		case StatusCompleted:
			return *latest, false, nil
		case StatusFailed:
			if !allowRetry {
				return *latest, false, ErrRetryRequired
			}
		}
	}

	if allowCreate != nil {
		if err := allowCreate(); err != nil {
			return Analysis{}, false, err
		}
	}

	r.byID[analysis.ID] = analysis
	r.byUser[analysis.UserID] = append(r.byUser[analysis.UserID], analysis)
	return analysis, true, nil
}

// Create stores the analysis.
func (r *MemoryRepo) Create(ctx context.Context, analysis Analysis) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[analysis.ID] = analysis
	r.byUser[analysis.UserID] = append(r.byUser[analysis.UserID], analysis)
	return nil
}

// GetByID returns an analysis by its ID.
func (r *MemoryRepo) GetByID(ctx context.Context, analysisID string) (Analysis, error) {
	if err := ctx.Err(); err != nil {
		return Analysis{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	analysis, ok := r.byID[analysisID]
	if !ok {
		return Analysis{}, ErrNotFound
	}
	return analysis, nil
}

// UpdateStatus updates the status and result for an existing analysis.
func (r *MemoryRepo) UpdateStatus(ctx context.Context, analysisID, status string, result map[string]any) error {
	return r.UpdateStatusResultAndError(ctx, analysisID, status, result, nil, nil, nil, nil, nil)
}

// UpdateStatusResultAndError updates status/result/error fields and timestamps.
func (r *MemoryRepo) UpdateStatusResultAndError(ctx context.Context, analysisID, status string, result map[string]any, errorCode *string, errorMessage *string, errorRetryable *bool, startedAt *time.Time, completedAt *time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	analysis, ok := r.byID[analysisID]
	if !ok {
		return ErrNotFound
	}
	analysis.Status = status
	if result != nil {
		analysis.Result = result
	}
	if errorCode != nil {
		analysis.ErrorCode = *errorCode
	}
	if errorMessage != nil {
		analysis.ErrorMessage = errorMessage
	}
	if errorRetryable != nil {
		analysis.ErrorRetryable = *errorRetryable
	}
	if startedAt != nil {
		analysis.StartedAt = startedAt
	} else if status == StatusProcessing && analysis.StartedAt == nil {
		now := time.Now().UTC()
		analysis.StartedAt = &now
	}
	if completedAt != nil {
		analysis.CompletedAt = completedAt
	} else if (status == StatusCompleted || status == StatusFailed) && analysis.CompletedAt == nil {
		now := time.Now().UTC()
		analysis.CompletedAt = &now
	}
	analysis.UpdatedAt = time.Now().UTC()
	r.byID[analysisID] = analysis

	// update in user slice
	userAnalyses := r.byUser[analysis.UserID]
	for i := range userAnalyses {
		if userAnalyses[i].ID == analysisID {
			userAnalyses[i] = analysis
			break
		}
	}
	r.byUser[analysis.UserID] = userAnalyses

	return nil
}

// UpdateAnalysisRaw stores the raw analysis payload.
func (r *MemoryRepo) UpdateAnalysisRaw(ctx context.Context, analysisID string, raw any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	analysis, ok := r.byID[analysisID]
	if !ok {
		return ErrNotFound
	}
	if raw != nil {
		analysis.AnalysisRaw = raw
	}
	analysis.UpdatedAt = time.Now().UTC()
	r.byID[analysisID] = analysis

	userAnalyses := r.byUser[analysis.UserID]
	for i := range userAnalyses {
		if userAnalyses[i].ID == analysisID {
			userAnalyses[i] = analysis
			break
		}
	}
	r.byUser[analysis.UserID] = userAnalyses
	return nil
}

// UpdateAnalysisResult stores the normalized analysis result and completion timestamp.
func (r *MemoryRepo) UpdateAnalysisResult(ctx context.Context, analysisID string, result map[string]any, completedAt *time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	analysis, ok := r.byID[analysisID]
	if !ok {
		return ErrNotFound
	}
	if result != nil {
		analysis.Result = result
	}
	analysis.Status = StatusCompleted
	if completedAt != nil {
		analysis.AnalysisCompletedAt = completedAt
		analysis.CompletedAt = completedAt
	} else if analysis.AnalysisCompletedAt == nil {
		now := time.Now().UTC()
		analysis.AnalysisCompletedAt = &now
		analysis.CompletedAt = &now
	}
	analysis.UpdatedAt = time.Now().UTC()
	r.byID[analysisID] = analysis

	userAnalyses := r.byUser[analysis.UserID]
	for i := range userAnalyses {
		if userAnalyses[i].ID == analysisID {
			userAnalyses[i] = analysis
			break
		}
	}
	r.byUser[analysis.UserID] = userAnalyses
	return nil
}

// UpdatePromptMetadata updates analysis_version and prompt_hash fields.
func (r *MemoryRepo) UpdatePromptMetadata(ctx context.Context, analysisID, analysisVersion, promptHash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	analysis, ok := r.byID[analysisID]
	if !ok {
		return ErrNotFound
	}
	if analysisVersion != "" {
		analysis.AnalysisVersion = analysisVersion
	}
	if promptHash != "" {
		analysis.PromptHash = promptHash
	}
	analysis.UpdatedAt = time.Now().UTC()
	r.byID[analysisID] = analysis

	userAnalyses := r.byUser[analysis.UserID]
	for i := range userAnalyses {
		if userAnalyses[i].ID == analysisID {
			userAnalyses[i] = analysis
			break
		}
	}
	r.byUser[analysis.UserID] = userAnalyses
	return nil
}

// ListByUser returns analyses for a user, newest first, with limit/offset.
func (r *MemoryRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]Analysis, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}

	r.mu.RLock()
	userAnalyses := r.byUser[userID]
	r.mu.RUnlock()

	if len(userAnalyses) == 0 || offset >= len(userAnalyses) {
		return []Analysis{}, nil
	}

	analyses := make([]Analysis, len(userAnalyses))
	copy(analyses, userAnalyses)
	sort.Slice(analyses, func(i, j int) bool {
		return analyses[i].CreatedAt.After(analyses[j].CreatedAt)
	})

	end := len(analyses)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return analyses[offset:end], nil
}

// ClaimGuest reassigns analyses owned by a guest user to an authenticated user.
func (r *MemoryRepo) ClaimGuest(ctx context.Context, guestUserID, authedUserID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	guestAnalyses := r.byUser[guestUserID]
	if len(guestAnalyses) == 0 {
		return 0, nil
	}
	for i := range guestAnalyses {
		guestAnalyses[i].UserID = authedUserID
		r.byID[guestAnalyses[i].ID] = guestAnalyses[i]
	}
	r.byUser[authedUserID] = append(r.byUser[authedUserID], guestAnalyses...)
	delete(r.byUser, guestUserID)
	return len(guestAnalyses), nil
}
