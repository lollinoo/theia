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
