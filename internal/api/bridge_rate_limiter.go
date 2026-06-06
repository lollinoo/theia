package api

// This file defines bridge rate limiter HTTP handler behavior and request/response boundaries.

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
	l.evictExpiredLocked(now)

	bucket, ok := l.attempts[key]
	if !ok {
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

func (l *bridgeRateLimiter) evictExpiredLocked(now time.Time) {
	for key, bucket := range l.attempts {
		if bucket.windowEnds.IsZero() || !now.Before(bucket.windowEnds) {
			delete(l.attempts, key)
		}
	}
}
