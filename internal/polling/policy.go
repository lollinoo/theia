package polling

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

type SettingsGetter interface {
	Get(string) (string, error)
}

type WarningCode string

const (
	WarningIntervalBelowTimeoutWindow WarningCode = "interval_below_timeout_window"
	WarningEstimatedWorkersExceeded   WarningCode = "estimated_workers_exceeded"
	WarningBudgetExceedsGlobal        WarningCode = "budget_exceeds_global"
)

type CapacityWarning struct {
	Code    WarningCode `json:"code"`
	Message string      `json:"message"`
}

type Policy struct {
	EssentialWorkers      int
	MaxWorkersPerSite     int
	MaxWorkersPerSubnet   int
	MaxWorkersPerDevice   int
	MaxInflightPerProfile int
	WebSocketCoalesce     time.Duration
	PersistenceBatch      time.Duration
	SafetyMargin          float64
	ForceOverCapacity     bool
	DegradedRisk          bool
	Timeouts              map[Lane]TimeoutProfile
}

func PolicyFromSettings(repo SettingsGetter, deviceCount int, observedP95 time.Duration, shortestInterval time.Duration) (Policy, []CapacityWarning) {
	policy := Policy{
		EssentialWorkers:      intSetting(repo, domain.SettingPollingEssentialWorkers, 64),
		MaxWorkersPerSite:     intSetting(repo, domain.SettingPollingMaxWorkersPerSite, 16),
		MaxWorkersPerSubnet:   intSetting(repo, domain.SettingPollingMaxWorkersPerSubnet, 8),
		MaxWorkersPerDevice:   intSetting(repo, domain.SettingPollingMaxWorkersPerDevice, 1),
		MaxInflightPerProfile: intSetting(repo, domain.SettingPollingMaxInflightPerProfile, 16),
		WebSocketCoalesce:     durationMSSetting(repo, domain.SettingPollingWebSocketCoalesceMS, 500*time.Millisecond),
		PersistenceBatch:      durationMSSetting(repo, domain.SettingPollingPersistenceBatchMS, time.Second),
		SafetyMargin:          floatSetting(repo, domain.SettingPollingCapacitySafetyMargin, 1.5),
		ForceOverCapacity:     boolSetting(repo, domain.SettingPollingForceOverCapacity, false),
		Timeouts: map[Lane]TimeoutProfile{
			LaneEssential: {
				Timeout: durationMSSetting(repo, domain.SettingPollingEssentialTimeoutMillis, 1200*time.Millisecond),
				Retries: nonNegativeIntSetting(repo, domain.SettingPollingEssentialRetries, 1),
			},
			LaneBackground: {Timeout: 5 * time.Second, Retries: 1},
			LaneBootstrap:  {Timeout: 10 * time.Second, Retries: 1},
			LaneQuarantine: {Timeout: time.Second, Retries: 0},
		},
	}

	warnings := make([]CapacityWarning, 0, 3)
	timeoutWindow := policy.Timeouts[LaneEssential].Timeout * time.Duration(policy.Timeouts[LaneEssential].Retries+1)
	if shortestInterval > 0 && shortestInterval <= timeoutWindow {
		warnings = append(warnings, CapacityWarning{
			Code:    WarningIntervalBelowTimeoutWindow,
			Message: "essential polling interval is not greater than timeout multiplied by attempts",
		})
	}

	requiredWorkers := EstimateRequiredWorkers(deviceCount, observedP95, shortestInterval, policy.SafetyMargin)
	if requiredWorkers > policy.EssentialWorkers {
		warnings = append(warnings, CapacityWarning{
			Code:    WarningEstimatedWorkersExceeded,
			Message: "estimated essential worker demand exceeds configured workers",
		})
		if !policy.ForceOverCapacity {
			policy.DegradedRisk = true
		}
	}

	if policy.MaxWorkersPerSite > policy.EssentialWorkers ||
		policy.MaxWorkersPerSubnet > policy.EssentialWorkers ||
		policy.MaxInflightPerProfile > policy.EssentialWorkers {
		warnings = append(warnings, CapacityWarning{
			Code:    WarningBudgetExceedsGlobal,
			Message: "one or more isolation budgets exceed the global essential worker budget",
		})
	}

	return policy, warnings
}

func EstimateRequiredWorkers(deviceCount int, observedP95 time.Duration, shortestInterval time.Duration, margin float64) int {
	if deviceCount <= 0 || observedP95 <= 0 || shortestInterval <= 0 {
		return 0
	}
	if margin <= 0 {
		margin = 1
	}
	pollsPerSecond := float64(deviceCount) / shortestInterval.Seconds()
	workers := pollsPerSecond * observedP95.Seconds() * margin
	return int(math.Ceil(workers))
}

func HasWarning(warnings []CapacityWarning, code WarningCode) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

func intSetting(repo SettingsGetter, key string, fallback int) int {
	if repo == nil {
		return fallback
	}
	value, err := repo.Get(key)
	if err != nil {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func nonNegativeIntSetting(repo SettingsGetter, key string, fallback int) int {
	if repo == nil {
		return fallback
	}
	value, err := repo.Get(key)
	if err != nil {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func durationMSSetting(repo SettingsGetter, key string, fallback time.Duration) time.Duration {
	ms := intSetting(repo, key, int(fallback/time.Millisecond))
	return time.Duration(ms) * time.Millisecond
}

func floatSetting(repo SettingsGetter, key string, fallback float64) float64 {
	if repo == nil {
		return fallback
	}
	value, err := repo.Get(key)
	if err != nil {
		return fallback
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fallback
	}
	return parsed
}

func boolSetting(repo SettingsGetter, key string, fallback bool) bool {
	if repo == nil {
		return fallback
	}
	value, err := repo.Get(key)
	if err != nil {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}
