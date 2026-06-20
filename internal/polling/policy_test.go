package polling

// This file exercises policy behavior so refactors preserve the documented contract.

import (
	"errors"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

type fakeSettings map[string]string

func (f fakeSettings) Get(key string) (string, error) {
	value, ok := f[key]
	if !ok {
		return "", errors.New("missing")
	}
	return value, nil
}

func TestPolicyFromSettingsUsesDefaults(t *testing.T) {
	policy, warnings := PolicyFromSettings(nil, 65, 300*time.Millisecond, 30*time.Second)

	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if policy.EssentialWorkers != 64 {
		t.Fatalf("EssentialWorkers = %d, want 64", policy.EssentialWorkers)
	}
	if policy.Timeouts[LaneEssential].Timeout != 1200*time.Millisecond {
		t.Fatalf("essential timeout = %v, want 1200ms", policy.Timeouts[LaneEssential].Timeout)
	}
	if policy.Timeouts[LaneEssential].Retries != 1 {
		t.Fatalf("essential retries = %d, want 1", policy.Timeouts[LaneEssential].Retries)
	}
	if policy.Timeouts[LaneBackground].Timeout != 5*time.Second {
		t.Fatalf("background timeout = %v, want 5s fallback", policy.Timeouts[LaneBackground].Timeout)
	}
	if policy.Timeouts[LaneBackground].Retries != 0 {
		t.Fatalf("background retries = %d, want 0 fallback", policy.Timeouts[LaneBackground].Retries)
	}
}

func TestPolicyFromSettingsAllowsZeroEssentialRetries(t *testing.T) {
	settings := fakeSettings{
		domain.SettingPollingEssentialRetries: "0",
	}

	policy, _ := PolicyFromSettings(settings, 0, 300*time.Millisecond, 30*time.Second)

	if policy.Timeouts[LaneEssential].Retries != 0 {
		t.Fatalf("essential retries = %d, want 0", policy.Timeouts[LaneEssential].Retries)
	}
}

func TestPolicyFromSettingsUsesConfiguredBackgroundSNMPProfile(t *testing.T) {
	settings := fakeSettings{
		domain.SettingSNMPTimeout: "10",
		domain.SettingSNMPRetries: "2",
	}

	policy, _ := PolicyFromSettings(settings, 0, 300*time.Millisecond, 30*time.Second)

	if policy.Timeouts[LaneBackground].Timeout != 10*time.Second {
		t.Fatalf("background timeout = %v, want configured 10s", policy.Timeouts[LaneBackground].Timeout)
	}
	if policy.Timeouts[LaneBackground].Retries != 2 {
		t.Fatalf("background retries = %d, want configured 2", policy.Timeouts[LaneBackground].Retries)
	}
}

func TestPolicyFromSettingsDerivesPerformanceCounterWalksProfileFromBackground(t *testing.T) {
	settings := fakeSettings{
		domain.SettingSNMPTimeout: "10",
		domain.SettingSNMPRetries: "2",
	}

	policy, _ := PolicyFromSettings(settings, 0, 300*time.Millisecond, 30*time.Second)
	profile := policy.Timeouts[LanePerformanceCounterWalks]

	if profile.Timeout != 2*time.Second {
		t.Fatalf("performance counter timeout = %v, want capped 2s", profile.Timeout)
	}
	if profile.Retries != 0 {
		t.Fatalf("performance counter retries = %d, want 0", profile.Retries)
	}
}

func TestPolicyFromSettingsUsesConfiguredPerformanceCounterWalksProfile(t *testing.T) {
	settings := fakeSettings{
		domain.SettingSNMPTimeout:                         "10",
		domain.SettingSNMPRetries:                         "2",
		domain.SettingSNMPPerformanceCounterTimeoutMillis: "3500",
		domain.SettingSNMPPerformanceCounterRetries:       "1",
	}

	policy, _ := PolicyFromSettings(settings, 0, 300*time.Millisecond, 30*time.Second)
	profile := policy.Timeouts[LanePerformanceCounterWalks]

	if profile.Timeout != 3500*time.Millisecond {
		t.Fatalf("performance counter timeout = %v, want configured 3500ms", profile.Timeout)
	}
	if profile.Retries != 1 {
		t.Fatalf("performance counter retries = %d, want configured 1", profile.Retries)
	}
}

func TestPolicyFromSettingsPreservesLowerBackgroundTimeoutForPerformanceCounterWalks(t *testing.T) {
	settings := fakeSettings{
		domain.SettingSNMPTimeout: "1",
		domain.SettingSNMPRetries: "0",
	}

	policy, _ := PolicyFromSettings(settings, 0, 300*time.Millisecond, 30*time.Second)
	profile := policy.Timeouts[LanePerformanceCounterWalks]

	if profile.Timeout != time.Second {
		t.Fatalf("performance counter timeout = %v, want configured lower 1s", profile.Timeout)
	}
	if profile.Retries != 0 {
		t.Fatalf("performance counter retries = %d, want 0", profile.Retries)
	}
}

func TestPolicyFromSettingsDefaultSeedPreservesLowerBackgroundTimeoutForPerformanceCounterWalks(t *testing.T) {
	settings := fakeSettings(domain.DefaultSettings())
	settings[domain.SettingSNMPTimeout] = "1"

	policy, _ := PolicyFromSettings(settings, 0, 300*time.Millisecond, 30*time.Second)
	profile := policy.Timeouts[LanePerformanceCounterWalks]

	if profile.Timeout != time.Second {
		t.Fatalf("performance counter timeout = %v, want derived lower 1s from seeded defaults", profile.Timeout)
	}
	if profile.Retries != 0 {
		t.Fatalf("performance counter retries = %d, want 0", profile.Retries)
	}
}

func TestPolicyWarnsWhenIntervalCannotOutrunTimeout(t *testing.T) {
	settings := fakeSettings{
		domain.SettingPollingEssentialTimeoutMillis: "10000",
		domain.SettingPollingEssentialRetries:       "1",
		domain.SettingPollingEssentialWorkers:       "64",
	}

	_, warnings := PolicyFromSettings(settings, 500, 300*time.Millisecond, 10*time.Second)

	if !HasWarning(warnings, WarningIntervalBelowTimeoutWindow) {
		t.Fatalf("warnings = %#v, want %q", warnings, WarningIntervalBelowTimeoutWindow)
	}
}

func TestPolicyWarnsWhenEstimatedWorkersExceedCapacity(t *testing.T) {
	settings := fakeSettings{
		domain.SettingPollingEssentialWorkers:     "8",
		domain.SettingPollingCapacitySafetyMargin: "1.5",
	}

	policy, warnings := PolicyFromSettings(settings, 500, time.Second, 10*time.Second)

	if !policy.DegradedRisk {
		t.Fatalf("DegradedRisk = false, want true")
	}
	if !HasWarning(warnings, WarningEstimatedWorkersExceeded) {
		t.Fatalf("warnings = %#v, want %q", warnings, WarningEstimatedWorkersExceeded)
	}
}

func TestPolicyFallsBackForNonFiniteSafetyMargin(t *testing.T) {
	settings := fakeSettings{
		domain.SettingPollingEssentialWorkers:     "8",
		domain.SettingPollingCapacitySafetyMargin: "NaN",
	}

	policy, warnings := PolicyFromSettings(settings, 500, time.Second, 10*time.Second)

	if policy.SafetyMargin != 1.5 {
		t.Fatalf("SafetyMargin = %v, want 1.5", policy.SafetyMargin)
	}
	if !policy.DegradedRisk {
		t.Fatalf("DegradedRisk = false, want true")
	}
	if !HasWarning(warnings, WarningEstimatedWorkersExceeded) {
		t.Fatalf("warnings = %#v, want %q", warnings, WarningEstimatedWorkersExceeded)
	}
}

func TestPolicyFromSettingsClampsUnsafePersistedOperationalLimits(t *testing.T) {
	settings := fakeSettings{
		domain.SettingPollingEssentialWorkers:       "1000000",
		domain.SettingPollingMaxWorkersPerSite:      "1000000",
		domain.SettingPollingMaxWorkersPerSubnet:    "1000000",
		domain.SettingPollingMaxWorkersPerDevice:    "1000000",
		domain.SettingPollingMaxInflightPerProfile:  "1000000",
		domain.SettingPollingEssentialTimeoutMillis: "1",
		domain.SettingPollingEssentialRetries:       "1000000",
		domain.SettingPollingWebSocketCoalesceMS:    "1",
		domain.SettingPollingPersistenceBatchMS:     "1000000",
		domain.SettingPollingCapacitySafetyMargin:   "1000000",
	}

	policy, _ := PolicyFromSettings(settings, 0, 300*time.Millisecond, 30*time.Second)

	if policy.EssentialWorkers != 256 {
		t.Fatalf("EssentialWorkers = %d, want 256", policy.EssentialWorkers)
	}
	if policy.MaxWorkersPerSite != 256 {
		t.Fatalf("MaxWorkersPerSite = %d, want 256", policy.MaxWorkersPerSite)
	}
	if policy.MaxWorkersPerSubnet != 256 {
		t.Fatalf("MaxWorkersPerSubnet = %d, want 256", policy.MaxWorkersPerSubnet)
	}
	if policy.MaxWorkersPerDevice != 32 {
		t.Fatalf("MaxWorkersPerDevice = %d, want 32", policy.MaxWorkersPerDevice)
	}
	if policy.MaxInflightPerProfile != 256 {
		t.Fatalf("MaxInflightPerProfile = %d, want 256", policy.MaxInflightPerProfile)
	}
	if policy.Timeouts[LaneEssential].Timeout != 100*time.Millisecond {
		t.Fatalf("essential timeout = %v, want 100ms", policy.Timeouts[LaneEssential].Timeout)
	}
	if policy.Timeouts[LaneEssential].Retries != 10 {
		t.Fatalf("essential retries = %d, want 10", policy.Timeouts[LaneEssential].Retries)
	}
	if policy.WebSocketCoalesce != 50*time.Millisecond {
		t.Fatalf("WebSocketCoalesce = %v, want 50ms", policy.WebSocketCoalesce)
	}
	if policy.PersistenceBatch != 10000*time.Millisecond {
		t.Fatalf("PersistenceBatch = %v, want 10000ms", policy.PersistenceBatch)
	}
	if policy.SafetyMargin != 5.0 {
		t.Fatalf("SafetyMargin = %v, want 5.0", policy.SafetyMargin)
	}
}

func TestPolicyFromSettingsFallsBackForMalformedPersistedValuesAndClampsNegatives(t *testing.T) {
	settings := fakeSettings{
		domain.SettingPollingEssentialWorkers:       "not-a-number",
		domain.SettingPollingEssentialTimeoutMillis: "NaN",
		domain.SettingPollingEssentialRetries:       "-1",
		domain.SettingPollingCapacitySafetyMargin:   "+Inf",
	}

	policy, _ := PolicyFromSettings(settings, 0, 300*time.Millisecond, 30*time.Second)

	if policy.EssentialWorkers != 64 {
		t.Fatalf("EssentialWorkers = %d, want 64", policy.EssentialWorkers)
	}
	if policy.Timeouts[LaneEssential].Timeout != 1200*time.Millisecond {
		t.Fatalf("essential timeout = %v, want 1200ms", policy.Timeouts[LaneEssential].Timeout)
	}
	if policy.Timeouts[LaneEssential].Retries != 0 {
		t.Fatalf("essential retries = %d, want 0", policy.Timeouts[LaneEssential].Retries)
	}
	if policy.SafetyMargin != 1.5 {
		t.Fatalf("SafetyMargin = %v, want 1.5", policy.SafetyMargin)
	}
}

func TestPolicyWarnsForBudgetIsolationBypass(t *testing.T) {
	settings := fakeSettings{
		domain.SettingPollingEssentialWorkers:      "16",
		domain.SettingPollingMaxWorkersPerSite:     "64",
		domain.SettingPollingMaxWorkersPerSubnet:   "64",
		domain.SettingPollingMaxInflightPerProfile: "64",
	}

	_, warnings := PolicyFromSettings(settings, 10, 100*time.Millisecond, 30*time.Second)

	if !HasWarning(warnings, WarningBudgetExceedsGlobal) {
		t.Fatalf("warnings = %#v, want %q", warnings, WarningBudgetExceedsGlobal)
	}
}
