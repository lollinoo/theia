package worker

import (
	"fmt"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

// mockWorkerSettingsRepo implements domain.SettingsRepository for worker tests.
type mockWorkerSettingsRepo struct {
	settings map[string]string
}

func newMockWorkerSettingsRepo() *mockWorkerSettingsRepo {
	return &mockWorkerSettingsRepo{settings: domain.DefaultSettings()}
}

func (r *mockWorkerSettingsRepo) Get(key string) (string, error) {
	v, ok := r.settings[key]
	if !ok {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	return v, nil
}

func (r *mockWorkerSettingsRepo) Set(key, value string) error {
	r.settings[key] = value
	return nil
}

func (r *mockWorkerSettingsRepo) GetAll() (map[string]string, error) {
	cp := make(map[string]string)
	for k, v := range r.settings {
		cp[k] = v
	}
	return cp, nil
}

// ---------------------------------------------------------------------------
// TestGetPollingInterval (DEBT-03)
// ---------------------------------------------------------------------------
// Verifies that the shared GetPollingInterval helper exists and returns the
// correct duration from settings. Before the fix, this function did not exist;
// Poller and MetricsCollector each had their own private getPollingInterval
// methods with duplicated logic.
func TestGetPollingInterval(t *testing.T) {
	repo := newMockWorkerSettingsRepo()

	// Test with default value (60 seconds)
	interval := GetPollingInterval(repo)
	if interval != 60*time.Second {
		t.Fatalf("DEBT-03: GetPollingInterval returned %v for default -- expected 60s", interval)
	}

	// Test with custom value
	repo.Set(domain.SettingPollingInterval, "30")
	interval = GetPollingInterval(repo)
	if interval != 30*time.Second {
		t.Fatalf("DEBT-03: GetPollingInterval returned %v for setting=30 -- expected 30s", interval)
	}

	// Test with invalid value falls back to default
	repo.Set(domain.SettingPollingInterval, "invalid")
	interval = GetPollingInterval(repo)
	if interval != 60*time.Second {
		t.Fatalf("DEBT-03: GetPollingInterval returned %v for invalid setting -- expected 60s fallback", interval)
	}

	// Test with zero falls back to default
	repo.Set(domain.SettingPollingInterval, "0")
	interval = GetPollingInterval(repo)
	if interval != 60*time.Second {
		t.Fatalf("DEBT-03: GetPollingInterval returned %v for setting=0 -- expected 60s fallback", interval)
	}

	// Test with negative value falls back to default
	repo.Set(domain.SettingPollingInterval, "-5")
	interval = GetPollingInterval(repo)
	if interval != 60*time.Second {
		t.Fatalf("DEBT-03: GetPollingInterval returned %v for setting=-5 -- expected 60s fallback", interval)
	}
}
