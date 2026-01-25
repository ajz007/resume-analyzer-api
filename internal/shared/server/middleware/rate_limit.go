package middleware

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	defaultRateLimitGroup = "DEFAULT"
)

type RateLimitRule struct {
	Rate  float64
	Burst int
}

type RateLimitConfig struct {
	Rules        map[string]RateLimitRule
	DefaultGroup string
	GroupFor     func(*gin.Context) string
	Limiter      *RateLimiter
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	now     func() time.Time
}

type rateBucket struct {
	tokens float64
	last   time.Time
}

func NewRateLimiter(now func() time.Time) *RateLimiter {
	if now == nil {
		now = time.Now
	}
	return &RateLimiter{
		buckets: make(map[string]*rateBucket),
		now:     now,
	}
}

func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	if cfg.Limiter == nil {
		cfg.Limiter = NewRateLimiter(nil)
	}
	if cfg.DefaultGroup == "" {
		cfg.DefaultGroup = defaultRateLimitGroup
	}
	return func(c *gin.Context) {
		group := cfg.DefaultGroup
		if cfg.GroupFor != nil {
			if g := strings.TrimSpace(cfg.GroupFor(c)); g != "" {
				group = g
			}
		}
		rule, ok := cfg.Rules[group]
		if !ok {
			c.Next()
			return
		}
		principal := strings.TrimSpace(UserIDFromContext(c))
		if principal == "" {
			principal = strings.TrimSpace(c.ClientIP())
		}
		key := principal + "|" + group
		allowed, retryAfter := cfg.Limiter.Allow(key, rule)
		if allowed {
			c.Next()
			return
		}
		retryAfterMs := int(retryAfter / time.Millisecond)
		if retryAfterMs <= 0 {
			retryAfterMs = 1000
		}
		retryAfterSeconds := int(math.Ceil(float64(retryAfterMs) / 1000.0))
		if retryAfterSeconds <= 0 {
			retryAfterSeconds = 1
		}
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":        "rate_limited",
			"retryAfterMs": retryAfterMs,
		})
		c.Abort()
	}
}

func (l *RateLimiter) Allow(key string, rule RateLimitRule) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	if rule.Rate <= 0 || rule.Burst <= 0 {
		return true, 0
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket, ok := l.buckets[key]
	if !ok {
		bucket = &rateBucket{
			tokens: float64(rule.Burst),
			last:   now,
		}
		l.buckets[key] = bucket
	}
	elapsed := now.Sub(bucket.last).Seconds()
	if elapsed > 0 {
		bucket.tokens = math.Min(float64(rule.Burst), bucket.tokens+elapsed*rule.Rate)
		bucket.last = now
	}
	if bucket.tokens >= 1 {
		bucket.tokens -= 1
		return true, 0
	}
	needed := 1 - bucket.tokens
	waitSec := needed / rule.Rate
	if waitSec < 0 {
		waitSec = 0
	}
	retryAfter := time.Duration(math.Ceil(waitSec*1000.0)) * time.Millisecond
	return false, retryAfter
}
