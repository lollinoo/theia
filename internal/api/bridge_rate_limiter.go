package api

import (
	"sync"
	"time"
)

type bridgeRateLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	now      func() time.Time
	attempts map[string]bridgeRateLimitBucket
}

type bridgeRateLimitBucket struct {
	count      int
	windowEnds time.Time
}

func newBridgeRateLimiter(limit int, window time.Duration, now func() time.Time) *bridgeRateLimiter {
	if limit <= 0 {
		limit = 20
	}
	if window <= 0 {
		window = time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &bridgeRateLimiter{
		limit:    limit,
		window:   window,
		now:      now,
		attempts: make(map[string]bridgeRateLimitBucket),
	}
}

func (l *bridgeRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	bucket := l.attempts[key]
	if bucket.windowEnds.IsZero() || !now.Before(bucket.windowEnds) {
		l.attempts[key] = bridgeRateLimitBucket{count: 1, windowEnds: now.Add(l.window)}
		return true
	}
	if bucket.count >= l.limit {
		return false
	}
	bucket.count++
	l.attempts[key] = bucket
	return true
}
