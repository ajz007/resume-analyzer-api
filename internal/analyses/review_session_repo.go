package analyses

import (
	"context"
	"encoding/json"
	"time"
)

// JSONB CAS avoids a new table and prevents stale tabs overwriting review choices.
// The original result/report and analysis_raw remain untouched.
func (r *PGRepo) CompareAndSwapReview(ctx context.Context, id, userID string, expected int, state ReviewSession) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE analyses
 SET analysis_result = jsonb_set(COALESCE(analysis_result, result, '{}'::jsonb), '{_reviewSession}', $1::jsonb, true), updated_at = NOW()
 WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL AND status = 'completed'
 AND COALESCE((COALESCE(analysis_result, result)->'_reviewSession'->>'version')::integer, 0) = $4`, string(data), id, userID, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrReviewConflict
	}
	return nil
}

func (r *MemoryRepo) CompareAndSwapReview(ctx context.Context, id, userID string, expected int, state ReviewSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	analysis, ok := r.byID[id]
	if !ok || analysis.UserID != userID || analysis.Status != StatusCompleted {
		return ErrReviewConflict
	}
	old, err := reviewState(analysis.Result)
	if err != nil {
		return err
	}
	if old.Version != expected {
		return ErrReviewConflict
	}
	// Clone at the persistence boundary; readers cannot mutate the stored generation lock.
	raw, err := json.Marshal(analysis.Result)
	if err != nil {
		return err
	}
	result := map[string]any{}
	if err = json.Unmarshal(raw, &result); err != nil {
		return err
	}
	stateRaw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	var saved any
	if err = json.Unmarshal(stateRaw, &saved); err != nil {
		return err
	}
	result[reviewSessionKey] = saved
	analysis.Result = result
	analysis.UpdatedAt = time.Now().UTC()
	r.byID[id] = analysis
	for i := range r.byUser[userID] {
		if r.byUser[userID][i].ID == id {
			r.byUser[userID][i] = analysis
		}
	}
	return nil
}
