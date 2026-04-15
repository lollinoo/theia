package state

import (
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
)

func ptrFloat(v float64) *float64 { return &v }

func TestHealth_WorstOf_AllOK(t *testing.T) {
	s := &DeviceState{}
	m := &domain.DeviceMetrics{
		DeviceID:    uuid.New(),
		CPUPercent:  ptrFloat(50),
		MemPercent:  ptrFloat(50),
		TempCelsius: ptrFloat(50),
	}
	evaluateHealth(s, m)
	if s.CPUSeverity != MetricSeverityOK {
		t.Errorf("CPUSeverity = %q, want %q", s.CPUSeverity, MetricSeverityOK)
	}
	if s.MemSeverity != MetricSeverityOK {
		t.Errorf("MemSeverity = %q, want %q", s.MemSeverity, MetricSeverityOK)
	}
	if s.TempSeverity != MetricSeverityOK {
		t.Errorf("TempSeverity = %q, want %q", s.TempSeverity, MetricSeverityOK)
	}
	if s.Health != HealthStatusHealthy {
		t.Errorf("Health = %q, want %q", s.Health, HealthStatusHealthy)
	}
}

func TestHealth_WorstOf_OneWarning(t *testing.T) {
	s := &DeviceState{}
	m := &domain.DeviceMetrics{
		CPUPercent:  ptrFloat(75),
		MemPercent:  ptrFloat(50),
		TempCelsius: ptrFloat(50),
	}
	evaluateHealth(s, m)
	if s.CPUSeverity != MetricSeverityWarning {
		t.Errorf("CPUSeverity = %q, want %q", s.CPUSeverity, MetricSeverityWarning)
	}
	if s.Health != HealthStatusWarning {
		t.Errorf("Health = %q, want %q", s.Health, HealthStatusWarning)
	}
}

func TestHealth_WorstOf_OneCritical(t *testing.T) {
	s := &DeviceState{}
	m := &domain.DeviceMetrics{
		CPUPercent:  ptrFloat(50),
		MemPercent:  ptrFloat(95),
		TempCelsius: ptrFloat(50),
	}
	evaluateHealth(s, m)
	if s.MemSeverity != MetricSeverityCritical {
		t.Errorf("MemSeverity = %q, want %q", s.MemSeverity, MetricSeverityCritical)
	}
	if s.Health != HealthStatusCritical {
		t.Errorf("Health = %q, want %q", s.Health, HealthStatusCritical)
	}
}

func TestHealth_WorstOf_WarningAndCritical(t *testing.T) {
	s := &DeviceState{}
	m := &domain.DeviceMetrics{
		CPUPercent:  ptrFloat(75),
		MemPercent:  ptrFloat(95),
		TempCelsius: ptrFloat(50),
	}
	evaluateHealth(s, m)
	if s.Health != HealthStatusCritical {
		t.Errorf("Health = %q, want %q (critical must outrank warning)", s.Health, HealthStatusCritical)
	}
}

func TestHealth_NilMetricsDoNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("evaluateHealth panicked on nil metrics: %v", r)
		}
	}()
	s := &DeviceState{}
	m := &domain.DeviceMetrics{} // all pointer fields nil
	evaluateHealth(s, m)
	// With zero observations the aggregate must be Unknown, not Healthy:
	// we cannot report a device as healthy when no metric has been
	// evaluated (WR-03).
	if s.Health != HealthStatusUnknown {
		t.Errorf("Health = %q, want %q (all nil => unknown)", s.Health, HealthStatusUnknown)
	}
}

func TestHealth_PartialNilMetrics(t *testing.T) {
	s := &DeviceState{}
	m := &domain.DeviceMetrics{
		MemPercent: ptrFloat(75), // only memory reported
	}
	evaluateHealth(s, m)
	if s.MemSeverity != MetricSeverityWarning {
		t.Errorf("MemSeverity = %q, want %q", s.MemSeverity, MetricSeverityWarning)
	}
	if s.Health != HealthStatusWarning {
		t.Errorf("Health = %q, want %q", s.Health, HealthStatusWarning)
	}
}

func TestHysteresis(t *testing.T) {
	// All thresholds use the CPU config (70/60/90/80) per D-12.
	cfg := defaultThresholds["cpu"]

	cases := []struct {
		name    string
		current MetricSeverity
		value   float64
		want    MetricSeverity
	}{
		// Rising edge across WarnRise (>=70 triggers warning)
		{"OK_to_Warning_exact_boundary", MetricSeverityOK, 70, MetricSeverityWarning},
		{"OK_stays_OK_just_below_WarnRise", MetricSeverityOK, 69.9, MetricSeverityOK},
		// Warning falls to OK only strictly below WarnFall
		{"Warning_stays_Warning_at_WarnFall", MetricSeverityWarning, 60, MetricSeverityWarning},
		{"Warning_clears_just_below_WarnFall", MetricSeverityWarning, 59.9, MetricSeverityOK},
		// Warning escalates to Critical
		{"Warning_to_Critical_exact_boundary", MetricSeverityWarning, 90, MetricSeverityCritical},
		// Critical drops to Warning only strictly below CriticalFall
		{"Critical_stays_Critical_at_CriticalFall", MetricSeverityCritical, 80, MetricSeverityCritical},
		{"Critical_drops_to_Warning_just_below_CriticalFall", MetricSeverityCritical, 79.9, MetricSeverityWarning},
		// Critical direct drop to OK skips warning
		{"Critical_direct_drop_to_OK_below_WarnFall", MetricSeverityCritical, 59, MetricSeverityOK},
		// OK jumps directly to Critical skipping warning
		{"OK_to_Critical_skips_warning", MetricSeverityOK, 95, MetricSeverityCritical},
		// NaN leaves severity unchanged
		{"NaN_leaves_Warning_unchanged", MetricSeverityWarning, math.NaN(), MetricSeverityWarning},
		{"NaN_leaves_OK_unchanged", MetricSeverityOK, math.NaN(), MetricSeverityOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evaluateMetricSeverity(tc.value, tc.current, cfg)
			if got != tc.want {
				t.Errorf("evaluateMetricSeverity(%v, %q) = %q, want %q", tc.value, tc.current, got, tc.want)
			}
		})
	}
}

func TestHysteresis_FlapPrevention(t *testing.T) {
	// A value oscillating at 69/71 must NOT toggle OK/Warning; once we enter
	// Warning at 71, dropping to 69 stays Warning because 69 >= WarnFall=60.
	cfg := defaultThresholds["cpu"]
	sequence := []float64{69, 71, 69, 71, 69, 71}
	want := []MetricSeverity{
		MetricSeverityOK,      // 69 from OK => OK
		MetricSeverityWarning, // 71 => Warning
		MetricSeverityWarning, // 69 from Warning, >= WarnFall 60 => still Warning
		MetricSeverityWarning,
		MetricSeverityWarning,
		MetricSeverityWarning,
	}
	current := MetricSeverityOK
	for i, v := range sequence {
		current = evaluateMetricSeverity(v, current, cfg)
		if current != want[i] {
			t.Errorf("step %d value=%v: got %q, want %q", i, v, current, want[i])
		}
	}
}
