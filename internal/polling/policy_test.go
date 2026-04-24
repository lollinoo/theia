package polling

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
