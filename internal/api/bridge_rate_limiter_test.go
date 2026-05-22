package api

import (
	"testing"
	"time"
)

func TestBridgeRateLimiterAllowsOnlyConfiguredAttemptsPerWindow(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	limiter := newBridgeRateLimiter(2, time.Minute, func() time.Time { return now })

	if !limiter.allow("192.0.2.10|theia_bridge_test") {
		t.Fatal("first attempt should be allowed")
	}
	if !limiter.allow("192.0.2.10|theia_bridge_test") {
		t.Fatal("second attempt should be allowed")
	}
	if limiter.allow("192.0.2.10|theia_bridge_test") {
		t.Fatal("third attempt in same window should be denied")
	}

	now = now.Add(time.Minute + time.Second)
	if !limiter.allow("192.0.2.10|theia_bridge_test") {
		t.Fatal("attempt after window reset should be allowed")
	}
}

func TestBridgeRateLimiterEvictsExpiredBuckets(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	limiter := newBridgeRateLimiter(2, time.Minute, func() time.Time { return now })

	limiter.allow("192.0.2.10|one")
	limiter.allow("192.0.2.10|two")

	now = now.Add(time.Minute + time.Second)

	if !limiter.allow("192.0.2.10|three") {
		t.Fatal("new attempt after old buckets expired should be allowed")
	}
	if len(limiter.attempts) != 1 {
		t.Fatalf("bucket count = %d, want 1", len(limiter.attempts))
	}
	if _, ok := limiter.attempts["192.0.2.10|three"]; !ok {
		t.Fatal("expected only current bucket to remain")
	}
}
