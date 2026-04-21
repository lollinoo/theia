package worker

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

func TestNormalizeDeviceRuntimeDTO_VirtualNoIPIsUnmonitored(t *testing.T) {
	deviceID := uuid.New()

	dto := normalizeDeviceRuntimeDTO(
		domain.Device{ID: deviceID, DeviceType: domain.DeviceTypeVirtual},
		state.DeviceState{},
		nil,
		ws.PrometheusStatusPayload{},
	)

	if dto.DeviceID != deviceID.String() {
		t.Fatalf("DeviceID = %q, want %q", dto.DeviceID, deviceID)
	}
	if dto.OperationalStatus != "unmonitored" {
		t.Fatalf("OperationalStatus = %q, want unmonitored", dto.OperationalStatus)
	}
	if dto.Reachability != "unmonitored" {
		t.Fatalf("Reachability = %q, want unmonitored", dto.Reachability)
	}
	if dto.Freshness != "unmonitored" {
		t.Fatalf("Freshness = %q, want unmonitored", dto.Freshness)
	}
	if dto.PrimaryReason != normalizedReasonUnmonitored {
		t.Fatalf("PrimaryReason = %q, want %q", dto.PrimaryReason, normalizedReasonUnmonitored)
	}
	if dto.MetricsStatus != "unmonitored" {
		t.Fatalf("MetricsStatus = %q, want unmonitored", dto.MetricsStatus)
	}
	if dto.MetricsReason != normalizedReasonUnmonitored {
		t.Fatalf("MetricsReason = %q, want %q", dto.MetricsReason, normalizedReasonUnmonitored)
	}
	if dto.LastCollectedAt != nil {
		t.Fatalf("LastCollectedAt = %#v, want nil", dto.LastCollectedAt)
	}
}

func TestNormalizeDeviceRuntimeDTO_PrometheusOutageUsesUpstreamUnavailableReason(t *testing.T) {
	deviceID := uuid.New()

	dto := normalizeDeviceRuntimeDTO(
		domain.Device{ID: deviceID, IP: "192.0.2.10", MetricsSource: domain.MetricsSourcePrometheus},
		state.DeviceState{},
		nil,
		ws.PrometheusStatusPayload{Enabled: true, Available: false, Error: "down"},
	)

	if dto.OperationalStatus != "unknown" {
		t.Fatalf("OperationalStatus = %q, want unknown", dto.OperationalStatus)
	}
	if dto.Freshness != "awaiting_poll" {
		t.Fatalf("Freshness = %q, want awaiting_poll", dto.Freshness)
	}
	if dto.PrimaryReason != normalizedReasonUpstreamUnavailable {
		t.Fatalf("PrimaryReason = %q, want %q", dto.PrimaryReason, normalizedReasonUpstreamUnavailable)
	}
	if dto.MetricsStatus != "unavailable" {
		t.Fatalf("MetricsStatus = %q, want unavailable", dto.MetricsStatus)
	}
	if dto.MetricsReason != normalizedReasonUpstreamUnavailable {
		t.Fatalf("MetricsReason = %q, want %q", dto.MetricsReason, normalizedReasonUpstreamUnavailable)
	}
}

func TestNormalizeDeviceRuntimeDTO_AttachesAlertSummaryAndMetrics(t *testing.T) {
	deviceID := uuid.New()
	collectedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	lastPolledAt := collectedAt.Add(-15 * time.Second)
	expectedInterval := 30 * time.Second

	dto := normalizeDeviceRuntimeDTO(
		domain.Device{ID: deviceID, IP: "192.0.2.10", MetricsSource: domain.MetricsSourceSNMP},
		state.DeviceState{
			Metrics: domain.DeviceMetrics{
				DeviceID:    deviceID,
				CPUPercent:  floatPtr(42),
				MemPercent:  floatPtr(64),
				TempCelsius: floatPtr(49),
				UptimeSecs:  floatPtr(3600),
				CollectedAt: collectedAt,
			},
			Health:           state.HealthStatusWarning,
			Reachability:     state.ReachabilityUp,
			LastPolledAt:     lastPolledAt,
			ExpectedInterval: expectedInterval,
		},
		[]domain.AlertState{{DeviceID: deviceID, Severity: "critical", State: "firing", AlertName: "HighCPU"}},
		ws.PrometheusStatusPayload{Enabled: true, Available: true},
	)

	if dto.OperationalStatus != "up" {
		t.Fatalf("OperationalStatus = %q, want up", dto.OperationalStatus)
	}
	if dto.PrimaryReason != normalizedReasonOK {
		t.Fatalf("PrimaryReason = %q, want %q", dto.PrimaryReason, normalizedReasonOK)
	}
	if dto.MetricsStatus != "available" {
		t.Fatalf("MetricsStatus = %q, want available", dto.MetricsStatus)
	}
	if dto.AlertStatus != string(domain.AlertStatusDown) {
		t.Fatalf("AlertStatus = %q, want %q", dto.AlertStatus, domain.AlertStatusDown)
	}
	if dto.FiringAlertCount != 1 {
		t.Fatalf("FiringAlertCount = %d, want 1", dto.FiringAlertCount)
	}
	if dto.LastCollectedAt == nil || *dto.LastCollectedAt != collectedAt.Format(time.RFC3339) {
		t.Fatalf("LastCollectedAt = %#v, want %q", dto.LastCollectedAt, collectedAt.Format(time.RFC3339))
	}
	if dto.ExpectedPollIntervalSeconds == nil || *dto.ExpectedPollIntervalSeconds != expectedInterval.Seconds() {
		t.Fatalf("ExpectedPollIntervalSeconds = %#v, want %v", dto.ExpectedPollIntervalSeconds, expectedInterval.Seconds())
	}
	if dto.CPUPercent == nil || *dto.CPUPercent != 42 {
		t.Fatalf("CPUPercent = %#v, want 42", dto.CPUPercent)
	}
	if dto.MemPercent == nil || *dto.MemPercent != 64 {
		t.Fatalf("MemPercent = %#v, want 64", dto.MemPercent)
	}
	if dto.TempCelsius == nil || *dto.TempCelsius != 49 {
		t.Fatalf("TempCelsius = %#v, want 49", dto.TempCelsius)
	}
	if dto.UptimeSecs == nil || *dto.UptimeSecs != 3600 {
		t.Fatalf("UptimeSecs = %#v, want 3600", dto.UptimeSecs)
	}
	if dto.LastPolledAt == nil || *dto.LastPolledAt != lastPolledAt.Format(time.RFC3339) {
		t.Fatalf("LastPolledAt = %#v, want %q", dto.LastPolledAt, lastPolledAt.Format(time.RFC3339))
	}
}

func TestNormalizeLinkRuntimeDTO_DeviceDownMakesMetricsUnavailable(t *testing.T) {
	linkID := uuid.New()
	sourceID := uuid.New()
	targetID := uuid.New()

	dto := normalizeLinkRuntimeDTO(
		domain.Link{
			ID:             linkID,
			SourceDeviceID: sourceID,
			SourceIfName:   "ether1",
			TargetDeviceID: targetID,
			TargetIfName:   "ether2",
		},
		nil,
		ws.DeviceRuntimeDTO{DeviceID: sourceID.String(), OperationalStatus: "down", PrimaryReason: normalizedReasonDeviceUnreachable},
		ws.DeviceRuntimeDTO{DeviceID: targetID.String(), OperationalStatus: "up", PrimaryReason: normalizedReasonOK},
	)

	if dto.LinkID != linkID.String() {
		t.Fatalf("LinkID = %q, want %q", dto.LinkID, linkID)
	}
	if dto.MetricsStatus != "unavailable" {
		t.Fatalf("MetricsStatus = %q, want unavailable", dto.MetricsStatus)
	}
	if dto.MetricsReason != normalizedReasonDeviceUnreachable {
		t.Fatalf("MetricsReason = %q, want %q", dto.MetricsReason, normalizedReasonDeviceUnreachable)
	}
	if dto.TxBps != nil || dto.RxBps != nil || dto.Utilization != nil {
		t.Fatal("expected nil link metrics when endpoints cannot provide runtime")
	}
}
