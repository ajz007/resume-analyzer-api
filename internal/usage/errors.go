package usage

import "errors"

// ErrLimitReached indicates the user exceeded their usage limit.
var ErrLimitReached = errors.New("limit reached")
