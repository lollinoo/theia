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
