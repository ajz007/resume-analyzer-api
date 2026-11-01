package usage

import "errors"

// ErrLimitReached indicates the user exceeded their usage limit.
var ErrLimitReached = errors.New("limit reached")

// ErrApplyRunNotFound indicates an apply run was not found.
var ErrApplyRunNotFound = errors.New("apply run not found")

// ErrAnalysisNotFound indicates an analysis record was not found.
var ErrAnalysisNotFound = errors.New("analysis not found")

// ErrReservationConflict prevents recharging a closed or differently owned review.
var ErrReservationConflict = errors.New("review usage reservation is not available")
