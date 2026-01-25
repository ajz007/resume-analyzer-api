package analyses

import (
	"sync"
	"time"
)

const pollLimitWindow = 1 * time.Second

type pollLimiter struct {
	mu      sync.Mutex
	lastHit map[string]time.Time
	now     func() time.Time
	window  time.Duration
}

func newPollLimiter(window time.Duration, now func() time.Time) *pollLimiter {
	if now == nil {
		now = time.Now
	}
	if window <= 0 {
		window = pollLimitWindow
	}
	return &pollLimiter{
		lastHit: make(map[string]time.Time),
		now:     now,
		window:  window,
	}
}

func (l *pollLimiter) Allow(userID, documentID string) bool {
	if l == nil {
		return true
	}
	key := userID + "|" + documentID
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if last, ok := l.lastHit[key]; ok {
		if now.Sub(last) < l.window {
			return false
		}
	}
	l.lastHit[key] = now
	return true
}

func (l *pollLimiter) RetryAfterSeconds() int {
	if l == nil {
		return int(pollLimitWindow.Seconds())
	}
	return int(l.window.Seconds())
}
