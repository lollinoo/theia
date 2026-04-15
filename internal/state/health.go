package state

import (
	"math"

	"github.com/lollinoo/theia/internal/domain"
)

// ThresholdConfig defines a hysteresis threshold pair for a single metric.
// Rise thresholds trigger severity increases; fall thresholds trigger
// severity decreases. The gap between rise and fall prevents flapping (D-12).
type ThresholdConfig struct {
	WarnRise     float64 // cross above  => warning   (70%)
	WarnFall     float64 // cross below  => clear warning  (60%)
	CriticalRise float64 // cross above  => critical  (90%)
	CriticalFall float64 // cross below  => clear critical (80%)
}

// defaultThresholds are the hardcoded per-metric hysteresis thresholds
// per D-12, D-13. CPU, memory, and temperature all use the same 70/60/90/80
// pattern. Configurable thresholds are deferred to THRESH-01/02.
var defaultThresholds = map[string]ThresholdConfig{
	"cpu":  {WarnRise: 70, WarnFall: 60, CriticalRise: 90, CriticalFall: 80},
	"mem":  {WarnRise: 70, WarnFall: 60, CriticalRise: 90, CriticalFall: 80},
	"temp": {WarnRise: 70, WarnFall: 60, CriticalRise: 90, CriticalFall: 80},
}

// evaluateMetricSeverity applies hysteresis rules to determine the new
// severity for a metric given its current value and current severity.
// The comparison direction depends on current severity (Pattern 3 in
// 38-RESEARCH.md §Architecture Patterns).
//
// Transitions:
//
//	OK       + value >= CriticalRise => Critical
//	OK       + value >= WarnRise     => Warning
//	Warning  + value >= CriticalRise => Critical
//	Warning  + value <  WarnFall     => OK
//	Critical + value <  WarnFall     => OK (skip warning)
//	Critical + value <  CriticalFall => Warning
func evaluateMetricSeverity(value float64, current MetricSeverity, threshold ThresholdConfig) MetricSeverity {
	// Defensive: NaN values do not change severity.
	if math.IsNaN(value) {
		return current
	}
	switch current {
	case MetricSeverityWarning:
		if value >= threshold.CriticalRise {
			return MetricSeverityCritical
		}
		if value < threshold.WarnFall {
			return MetricSeverityOK
		}
		return MetricSeverityWarning
	case MetricSeverityCritical:
		if value < threshold.WarnFall {
			return MetricSeverityOK
		}
		if value < threshold.CriticalFall {
			return MetricSeverityWarning
		}
		return MetricSeverityCritical
	default:
		// Unknown / OK / first evaluation — treat as a fresh evaluation.
		if value >= threshold.CriticalRise {
			return MetricSeverityCritical
		}
		if value >= threshold.WarnRise {
			return MetricSeverityWarning
		}
		return MetricSeverityOK
	}
}

// evaluateHealth mutates the given DeviceState by re-evaluating each metric
// severity against defaultThresholds, then aggregates to an overall
// HealthStatus using worst-of semantics (D-03). Nil metric pointers leave
// the corresponding severity unchanged (D-02 / Pitfall 3 in research).
//
// Callers MUST NOT invoke this function when the device is soft-down or
// hard-down — health must be frozen at the last known value per D-02.
// The Store.Update method in Plan 02 enforces this precondition.
func evaluateHealth(state *DeviceState, metrics *domain.DeviceMetrics) {
	if metrics != nil {
		if metrics.CPUPercent != nil {
			state.CPUSeverity = evaluateMetricSeverity(*metrics.CPUPercent, state.CPUSeverity, defaultThresholds["cpu"])
		}
		if metrics.MemPercent != nil {
			state.MemSeverity = evaluateMetricSeverity(*metrics.MemPercent, state.MemSeverity, defaultThresholds["mem"])
		}
		if metrics.TempCelsius != nil {
			state.TempSeverity = evaluateMetricSeverity(*metrics.TempCelsius, state.TempSeverity, defaultThresholds["temp"])
		}
	}
	state.Health = aggregateHealth(state.CPUSeverity, state.MemSeverity, state.TempSeverity)
}

// aggregateHealth returns the worst-of the three per-metric severities per
// D-03. Any Critical outranks Warning; any Warning outranks OK. If every
// severity is the empty-string zero value (i.e. nothing has been evaluated
// yet — all metric pointers were nil on every poll), the result is Unknown
// rather than Healthy: we cannot report a device as healthy based on zero
// observations. Once at least one metric has been evaluated to OK, the
// result is Healthy.
func aggregateHealth(cpu, mem, temp MetricSeverity) HealthStatus {
	severities := []MetricSeverity{cpu, mem, temp}
	hasWarning := false
	allEmpty := true
	for _, s := range severities {
		if s == MetricSeverityCritical {
			return HealthStatusCritical
		}
		if s != "" {
			allEmpty = false
		}
		if s == MetricSeverityWarning {
			hasWarning = true
		}
	}
	if allEmpty {
		return HealthStatusUnknown
	}
	if hasWarning {
		return HealthStatusWarning
	}
	return HealthStatusHealthy
}
