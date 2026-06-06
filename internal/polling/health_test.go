package polling

// This file exercises health behavior so refactors preserve the documented contract.

import "testing"

func TestHealthSnapshotStatus(t *testing.T) {
	if got := (HealthSnapshot{}).Status(); got != "ok" {
		t.Fatalf("Status() = %q, want ok", got)
	}
	if got := (HealthSnapshot{DegradedRisk: true}).Status(); got != "degraded-risk" {
		t.Fatalf("Status() = %q, want degraded-risk", got)
	}
	if got := (HealthSnapshot{EssentialOverloaded: true}).Status(); got != "overloaded" {
		t.Fatalf("Status() = %q, want overloaded", got)
	}
}
