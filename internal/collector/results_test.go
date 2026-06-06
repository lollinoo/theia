package collector

// This file exercises results behavior so refactors preserve the documented contract.

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"

	"github.com/lollinoo/theia/internal/domain"
)

type stubSNMPClient struct{}

func (stubSNMPClient) Get([]string) ([]gosnmp.SnmpPDU, error) {
	return nil, nil
}

func (stubSNMPClient) BulkWalk(string) ([]gosnmp.SnmpPDU, error) {
	return nil, nil
}

func (stubSNMPClient) Connect() error {
	return nil
}

func (stubSNMPClient) Close() error {
	return nil
}

func TestResultTypesSatisfyStateUpdate(t *testing.T) {
	t.Parallel()

	var _ StateUpdate = PerformanceResult{}
	var _ StateUpdate = OperationalResult{}
	var _ StateUpdate = StaticResult{}
}

func TestNewSNMPClientFuncSignature(t *testing.T) {
	t.Parallel()

	var ctor NewSNMPClientFunc = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (SNMPClient, error) {
		return stubSNMPClient{}, nil
	}

	client, err := ctor("192.0.2.1", domain.SNMPCredentials{}, 5*time.Second, 2)
	if err != nil {
		t.Fatalf("ctor returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected SNMP client")
	}
}

func TestPerformanceResultToStoreUpdate(t *testing.T) {
	t.Parallel()

	deviceID := uuid.New()
	collectedAt := time.Date(2026, 4, 12, 14, 0, 0, 0, time.UTC)
	cpu := 42.5
	mem := 61.25
	temp := 55.5
	uptime := 12345.0

	result := PerformanceResult{
		DeviceID: deviceID,
		Metrics: domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  &cpu,
			MemPercent:  &mem,
			TempCelsius: &temp,
			UptimeSecs:  &uptime,
			CollectedAt: collectedAt,
		},
		CollectedAt: collectedAt,
	}

	update := result.ToStoreUpdate(30 * time.Second)
	if update.DeviceID != deviceID {
		t.Fatalf("DeviceID = %s, want %s", update.DeviceID, deviceID)
	}
	if !update.PollSuccess {
		t.Fatal("expected PollSuccess true")
	}
	if update.ExpectedInterval != 30*time.Second {
		t.Fatalf("ExpectedInterval = %s, want %s", update.ExpectedInterval, 30*time.Second)
	}
	if !update.Timestamp.Equal(collectedAt) {
		t.Fatalf("Timestamp = %s, want %s", update.Timestamp, collectedAt)
	}
	if update.Metrics == nil {
		t.Fatal("expected metrics")
	}
	assertFloatPtrEqual(t, update.Metrics.CPUPercent, cpu, "CPUPercent")
	assertFloatPtrEqual(t, update.Metrics.MemPercent, mem, "MemPercent")
	assertFloatPtrEqual(t, update.Metrics.TempCelsius, temp, "TempCelsius")
	assertFloatPtrEqual(t, update.Metrics.UptimeSecs, uptime, "UptimeSecs")
}

func TestPerformanceResultToStoreUpdateFailure(t *testing.T) {
	t.Parallel()

	deviceID := uuid.New()
	collectedAt := time.Date(2026, 4, 12, 14, 1, 0, 0, time.UTC)

	update := (PerformanceResult{
		DeviceID:    deviceID,
		CollectedAt: collectedAt,
		Err:         errors.New("poll failed"),
	}).ToStoreUpdate(45 * time.Second)

	if update.DeviceID != deviceID {
		t.Fatalf("DeviceID = %s, want %s", update.DeviceID, deviceID)
	}
	if update.PollSuccess {
		t.Fatal("expected PollSuccess false")
	}
	if update.Metrics != nil {
		t.Fatal("expected nil metrics")
	}
	if update.ExpectedInterval != 45*time.Second {
		t.Fatalf("ExpectedInterval = %s, want %s", update.ExpectedInterval, 45*time.Second)
	}
	if !update.Timestamp.Equal(collectedAt) {
		t.Fatalf("Timestamp = %s, want %s", update.Timestamp, collectedAt)
	}
}

func TestPerformanceResultToStoreUpdateSetsPerformanceVolatility(t *testing.T) {
	t.Parallel()

	update := (PerformanceResult{
		DeviceID:    uuid.New(),
		CollectedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
	}).ToStoreUpdate(30 * time.Second)

	if update.VolatilityClass != domain.VolatilityClassPerformance {
		t.Fatalf("VolatilityClass = %q, want %q", update.VolatilityClass, domain.VolatilityClassPerformance)
	}
}

func TestOperationalResultToStoreUpdateUsesReachability(t *testing.T) {
	t.Parallel()

	collectedAt := time.Date(2026, 4, 12, 14, 5, 0, 0, time.UTC)
	deviceID := uuid.New()
	uptime := 600.0

	tests := []struct {
		name       string
		reachable  bool
		uptimeSecs *float64
		wantMetric bool
	}{
		{
			name:       "reachable",
			reachable:  true,
			uptimeSecs: &uptime,
			wantMetric: true,
		},
		{
			name:      "unreachable",
			reachable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OperationalResult{
				DeviceID:    deviceID,
				Reachable:   tt.reachable,
				UptimeSecs:  tt.uptimeSecs,
				CollectedAt: collectedAt,
			}

			update := result.ToStoreUpdate(time.Minute)
			if update.PollSuccess != tt.reachable {
				t.Fatalf("PollSuccess = %t, want %t", update.PollSuccess, tt.reachable)
			}
			if !update.Timestamp.Equal(collectedAt) {
				t.Fatalf("Timestamp = %s, want %s", update.Timestamp, collectedAt)
			}
			if tt.wantMetric {
				if update.Metrics == nil {
					t.Fatal("expected metrics")
				}
				assertFloatPtrEqual(t, update.Metrics.UptimeSecs, uptime, "UptimeSecs")
			} else if update.Metrics != nil {
				t.Fatal("expected nil metrics")
			}
		})
	}
}

func TestOperationalResultToStoreUpdateSetsOperationalVolatility(t *testing.T) {
	t.Parallel()

	update := (OperationalResult{
		DeviceID:    uuid.New(),
		Reachable:   true,
		CollectedAt: time.Date(2026, 4, 13, 12, 5, 0, 0, time.UTC),
	}).ToStoreUpdate(time.Minute)

	if update.VolatilityClass != domain.VolatilityClassOperational {
		t.Fatalf("VolatilityClass = %q, want %q", update.VolatilityClass, domain.VolatilityClassOperational)
	}
}

func TestResultGetVolatilityClass(t *testing.T) {
	t.Parallel()

	if got := (PerformanceResult{}).GetVolatilityClass(); got != domain.VolatilityClassPerformance {
		t.Fatalf("PerformanceResult volatility = %q, want %q", got, domain.VolatilityClassPerformance)
	}
	if got := (OperationalResult{}).GetVolatilityClass(); got != domain.VolatilityClassOperational {
		t.Fatalf("OperationalResult volatility = %q, want %q", got, domain.VolatilityClassOperational)
	}
	if got := (StaticResult{}).GetVolatilityClass(); got != domain.VolatilityClassStatic {
		t.Fatalf("StaticResult volatility = %q, want %q", got, domain.VolatilityClassStatic)
	}
}

func assertFloatPtrEqual(t *testing.T, got *float64, want float64, field string) {
	t.Helper()

	if got == nil {
		t.Fatalf("%s = nil, want %v", field, want)
	}
	if *got != want {
		t.Fatalf("%s = %v, want %v", field, *got, want)
	}
}
