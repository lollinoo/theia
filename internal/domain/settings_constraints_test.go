package domain

// This file exercises settings constraint behavior so runtime settings validation stays aligned.

import "testing"

func TestNormalizeConstrainedSettingValidatesPollingMaxInflightPerSNMPProfile(t *testing.T) {
	normalized, err := NormalizeConstrainedSetting(SettingPollingMaxInflightPerProfile, " 64 ")
	if err != nil {
		t.Fatalf("NormalizeConstrainedSetting() error = %v", err)
	}
	if normalized != "64" {
		t.Fatalf("normalized value = %q, want 64", normalized)
	}

	if _, err := NormalizeConstrainedSetting(SettingPollingMaxInflightPerProfile, "257"); err == nil {
		t.Fatal("expected out-of-range polling_max_inflight_per_snmp_profile to be rejected")
	}
}

func TestDefaultSettingsLeavesPerformanceCounterSNMPProfileUnseeded(t *testing.T) {
	defaults := DefaultSettings()

	if _, ok := defaults[SettingSNMPPerformanceCounterTimeoutMillis]; ok {
		t.Fatalf("%s should stay unseeded so upgraded DBs preserve derived performance counter fallback", SettingSNMPPerformanceCounterTimeoutMillis)
	}
	if _, ok := defaults[SettingSNMPPerformanceCounterRetries]; ok {
		t.Fatalf("%s should stay unseeded so upgraded DBs preserve derived performance counter fallback", SettingSNMPPerformanceCounterRetries)
	}
}

func TestNormalizeConstrainedSettingValidatesPerformanceCounterSNMPProfile(t *testing.T) {
	normalized, err := NormalizeConstrainedSetting(SettingSNMPPerformanceCounterTimeoutMillis, " 3500 ")
	if err != nil {
		t.Fatalf("NormalizeConstrainedSetting(timeout) error = %v", err)
	}
	if normalized != "3500" {
		t.Fatalf("normalized timeout = %q, want 3500", normalized)
	}

	normalized, err = NormalizeConstrainedSetting(SettingSNMPPerformanceCounterRetries, " 1 ")
	if err != nil {
		t.Fatalf("NormalizeConstrainedSetting(retries) error = %v", err)
	}
	if normalized != "1" {
		t.Fatalf("normalized retries = %q, want 1", normalized)
	}

	if _, err := NormalizeConstrainedSetting(SettingSNMPPerformanceCounterTimeoutMillis, "99"); err == nil {
		t.Fatal("expected timeout below 100ms to be rejected")
	}
	if _, err := NormalizeConstrainedSetting(SettingSNMPPerformanceCounterRetries, "11"); err == nil {
		t.Fatal("expected retries above 10 to be rejected")
	}
}
