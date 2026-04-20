package worker

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

func TestBuildPipelineSnapshotPreservesOverviewSections(t *testing.T) {
	deviceID := uuid.New()
	peerID := uuid.New()
	linkID := uuid.New()
	collectedAt := time.Date(2026, 4, 13, 9, 30, 0, 0, time.UTC)
	expectedInterval := 45 * time.Second

	devices := []domain.Device{
		{
			ID:            deviceID,
			IP:            "192.0.2.10",
			Status:        domain.DeviceStatusUnknown,
			SysName:       "core-sw-1",
			HardwareModel: "CRS326-24G-2S+",
			Interfaces: []domain.Interface{
				{IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
			},
		},
		{
			ID:            peerID,
			IP:            "192.0.2.11",
			Status:        domain.DeviceStatusProbing,
			HardwareModel: "Unknown",
			Interfaces: []domain.Interface{
				{IfName: "ether2", IfDescr: "downlink", Speed: 1_000_000_000},
			},
		},
	}
	links := []domain.Link{
		{
			ID:             linkID,
			SourceDeviceID: deviceID,
			SourceIfName:   "ether1",
			TargetDeviceID: peerID,
			TargetIfName:   "ether2",
		},
	}
	states := map[uuid.UUID]state.DeviceState{
		deviceID: {
			Metrics: domain.DeviceMetrics{
				DeviceID:    deviceID,
				CPUPercent:  floatPtr(41.5),
				MemPercent:  floatPtr(62.0),
				TempCelsius: floatPtr(48.0),
				UptimeSecs:  floatPtr(7200),
				CollectedAt: collectedAt,
			},
			LinkMetrics: []domain.LinkMetrics{
				{
					IfName:      "uplink",
					TxBps:       floatPtr(125_000_000),
					RxBps:       floatPtr(250_000_000),
					CollectedAt: collectedAt,
				},
			},
			Health:           state.HealthStatusWarning,
			Reachability:     state.ReachabilityUp,
			Stale:            true,
			ExpectedInterval: expectedInterval,
		},
	}
	alerts := map[uuid.UUID][]domain.AlertState{
		deviceID: {
			{
				DeviceID:  deviceID,
				Severity:  "critical",
				AlertName: "HighCPU",
				State:     "firing",
				Summary:   "CPU high",
			},
		},
	}
	hostnameOverrides := map[uuid.UUID]string{
		deviceID: "prom-host-ignored",
		peerID:   "edge-sw-2",
	}

	snapshot := buildPipelineSnapshot(devices, links, states, alerts, hostnameOverrides)

	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if len(snapshot.DeviceMetrics) != 2 {
		t.Fatalf("expected device_metrics entries for both devices, got %d", len(snapshot.DeviceMetrics))
	}
	metric, ok := snapshot.DeviceMetrics[deviceID.String()]
	if !ok {
		t.Fatalf("expected metrics for device %s", deviceID)
	}
	if metric.CPUPercent == nil || *metric.CPUPercent != 41.5 {
		t.Fatalf("expected CPU 41.5, got %#v", metric.CPUPercent)
	}
	if metric.Health != string(state.HealthStatusWarning) {
		t.Fatalf("expected health %q, got %q", state.HealthStatusWarning, metric.Health)
	}
	if metric.Reachability != string(state.ReachabilityUp) {
		t.Fatalf("expected reachability %q, got %q", state.ReachabilityUp, metric.Reachability)
	}
	if metric.Stale == nil || !*metric.Stale {
		t.Fatalf("expected stale true, got %#v", metric.Stale)
	}

	linkMetrics, ok := snapshot.LinkMetrics[deviceID.String()]
	if !ok {
		t.Fatalf("expected link_metrics section for device %s", deviceID)
	}
	if len(linkMetrics) != 1 {
		t.Fatalf("expected 1 link metric, got %d", len(linkMetrics))
	}
	if linkMetrics[0].IfName != "uplink" {
		t.Fatalf("expected interface uplink, got %q", linkMetrics[0].IfName)
	}
	if linkMetrics[0].Utilization == nil || *linkMetrics[0].Utilization != 0.25 {
		t.Fatalf("expected utilization 0.25, got %#v", linkMetrics[0].Utilization)
	}

	if got := snapshot.DeviceStatuses[deviceID.String()]; got != string(domain.DeviceStatusUp) {
		t.Fatalf("expected status up, got %q", got)
	}
	if got := snapshot.DeviceStatuses[peerID.String()]; got != string(domain.DeviceStatusProbing) {
		t.Fatalf("expected peer status probing, got %q", got)
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if _, ok := decoded["alerts"]; ok {
		t.Fatal("expected slim snapshot to omit alerts")
	}
	if _, ok := decoded["device_hostnames"]; ok {
		t.Fatal("expected slim snapshot to omit device_hostnames")
	}
	if _, ok := decoded["device_models"]; ok {
		t.Fatal("expected slim snapshot to omit device_models")
	}

	decodedMetrics, ok := decoded["device_metrics"].(map[string]any)
	if !ok {
		t.Fatalf("device_metrics = %#v, want object", decoded["device_metrics"])
	}
	decodedMetric, ok := decodedMetrics[deviceID.String()].(map[string]any)
	if !ok {
		t.Fatalf("device_metrics[%s] = %#v, want object", deviceID, decodedMetrics[deviceID.String()])
	}
	if _, ok := decodedMetric["temp_celsius"]; ok {
		t.Fatal("expected slim device_metrics to omit temp_celsius")
	}
	if _, ok := decodedMetric["uptime_secs"]; ok {
		t.Fatal("expected slim device_metrics to omit uptime_secs")
	}
	if _, ok := decodedMetric["last_polled_at"]; ok {
		t.Fatal("expected slim device_metrics to omit last_polled_at")
	}
	if _, ok := decodedMetric["expected_poll_interval_seconds"]; ok {
		t.Fatal("expected slim device_metrics to omit expected_poll_interval_seconds")
	}
}

func TestBuildPipelineSnapshotJSONOmitsDetailOnlyDeviceMetricFields(t *testing.T) {
	deviceID := uuid.New()
	snapshot := buildPipelineSnapshot(
		[]domain.Device{{
			ID:     deviceID,
			IP:     "192.0.2.10",
			Status: domain.DeviceStatusUnknown,
		}},
		nil,
		map[uuid.UUID]state.DeviceState{
			deviceID: {
				Metrics: domain.DeviceMetrics{
					DeviceID:    deviceID,
					CPUPercent:  floatPtr(12),
					TempCelsius: floatPtr(48),
					UptimeSecs:  floatPtr(3600),
					CollectedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
				},
				ExpectedInterval: 30 * time.Second,
				LastPolledAt:     time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
			},
		},
		nil,
		nil,
	)

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	deviceMetrics, ok := decoded["device_metrics"].(map[string]any)
	if !ok {
		t.Fatalf("device_metrics = %#v, want object", decoded["device_metrics"])
	}
	metric, ok := deviceMetrics[deviceID.String()].(map[string]any)
	if !ok {
		t.Fatalf("device_metrics[%s] = %#v, want object", deviceID, deviceMetrics[deviceID.String()])
	}
	if _, ok := metric["temp_celsius"]; ok {
		t.Fatal("expected slim overview device_metrics to omit temp_celsius")
	}
	if _, ok := metric["uptime_secs"]; ok {
		t.Fatal("expected slim overview device_metrics to omit uptime_secs")
	}
	if _, ok := metric["last_polled_at"]; ok {
		t.Fatal("expected slim overview device_metrics to omit last_polled_at")
	}
	if _, ok := metric["expected_poll_interval_seconds"]; ok {
		t.Fatal("expected slim overview device_metrics to omit expected_poll_interval_seconds")
	}
}

func TestBuildPipelineSnapshotMapsReachabilityToExistingStatusStrings(t *testing.T) {
	upID := uuid.New()
	softID := uuid.New()
	hardID := uuid.New()
	fallbackID := uuid.New()

	devices := []domain.Device{
		{ID: upID, Status: domain.DeviceStatusUnknown},
		{ID: softID, Status: domain.DeviceStatusUnknown},
		{ID: hardID, Status: domain.DeviceStatusUnknown},
		{ID: fallbackID, Status: domain.DeviceStatusProbing},
	}
	states := map[uuid.UUID]state.DeviceState{
		upID:       {Reachability: state.ReachabilityUp},
		softID:     {Reachability: state.ReachabilitySoftDown},
		hardID:     {Reachability: state.ReachabilityHardDown},
		fallbackID: {},
	}

	snapshot := buildPipelineSnapshot(devices, nil, states, nil, nil)

	if got := snapshot.DeviceStatuses[upID.String()]; got != string(domain.DeviceStatusUp) {
		t.Fatalf("expected up to map to up, got %q", got)
	}
	if got := snapshot.DeviceStatuses[softID.String()]; got != string(domain.DeviceStatusDown) {
		t.Fatalf("expected soft_down to map to down, got %q", got)
	}
	if got := snapshot.DeviceStatuses[hardID.String()]; got != string(domain.DeviceStatusDown) {
		t.Fatalf("expected hard_down to map to down, got %q", got)
	}
	if got := snapshot.DeviceStatuses[fallbackID.String()]; got != string(domain.DeviceStatusProbing) {
		t.Fatalf("expected fallback status probing, got %q", got)
	}
}

func TestBuildPipelineSnapshot_VirtualNoIPIgnoresReachabilityOverride(t *testing.T) {
	deviceID := uuid.New()

	snapshot := buildPipelineSnapshot(
		[]domain.Device{
			{
				ID:         deviceID,
				DeviceType: domain.DeviceTypeVirtual,
				IP:         "",
				Status:     domain.DeviceStatusUnknown,
			},
		},
		nil,
		map[uuid.UUID]state.DeviceState{
			deviceID: {
				Reachability: state.ReachabilityHardDown,
			},
		},
		nil,
		nil,
	)

	if got := snapshot.DeviceStatuses[deviceID.String()]; got != string(domain.DeviceStatusUnknown) {
		t.Fatalf("expected virtual no-IP status unknown, got %q", got)
	}
}

func TestBuildPipelineSnapshot_FallsBackToPollOverrideAndUnknownHealthBeforeFirstPoll(t *testing.T) {
	deviceID := uuid.New()

	snapshot := buildPipelineSnapshot(
		[]domain.Device{
			{
				ID:                   deviceID,
				Status:               domain.DeviceStatusUnknown,
				PollClass:            domain.PollClassCore,
				PollIntervalOverride: intPtr(15),
			},
		},
		nil,
		nil,
		nil,
		nil,
	)

	deviceMetrics, ok := snapshot.DeviceMetrics[deviceID.String()]
	if !ok {
		t.Fatalf("expected overview metrics for %s", deviceID)
	}
	if deviceMetrics.Health != string(state.HealthStatusUnknown) {
		t.Fatalf("Health = %q, want %q", deviceMetrics.Health, state.HealthStatusUnknown)
	}
	if deviceMetrics.Stale == nil || *deviceMetrics.Stale {
		t.Fatalf("Stale = %#v, want false", deviceMetrics.Stale)
	}
}

func TestBuildPipelineSnapshot_FallsBackToPollClassIntervalWhenOverrideAbsent(t *testing.T) {
	deviceID := uuid.New()

	snapshot := buildPipelineSnapshot(
		[]domain.Device{
			{
				ID:        deviceID,
				Status:    domain.DeviceStatusUnknown,
				PollClass: domain.PollClassLow,
			},
		},
		nil,
		nil,
		nil,
		nil,
	)

	if _, ok := snapshot.DeviceMetrics[deviceID.String()]; !ok {
		t.Fatalf("expected overview metrics for %s", deviceID)
	}
}

func TestBuildDeviceDetailDelta_EmbedsOptionalDetailFieldsInDeviceMetrics(t *testing.T) {
	deviceID := uuid.New()
	collectedAt := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)

	delta := buildDeviceDetailDelta(
		domain.Device{
			ID:     deviceID,
			Status: domain.DeviceStatusUnknown,
		},
		state.DeviceState{
			Metrics: domain.DeviceMetrics{
				DeviceID:    deviceID,
				CPUPercent:  floatPtr(55),
				MemPercent:  floatPtr(62),
				TempCelsius: floatPtr(48),
				UptimeSecs:  floatPtr(3600),
				CollectedAt: collectedAt,
			},
			Health:           state.HealthStatusWarning,
			Reachability:     state.ReachabilityUp,
			Stale:            true,
			ExpectedInterval: 45 * time.Second,
		},
	)

	deviceMetrics, ok := delta.DeviceMetrics[deviceID.String()]
	if !ok {
		t.Fatalf("expected device detail delta for %s", deviceID)
	}

	if deviceMetrics.Health != string(state.HealthStatusWarning) {
		t.Fatalf("Health = %q, want %q", deviceMetrics.Health, state.HealthStatusWarning)
	}
	if deviceMetrics.Reachability != string(state.ReachabilityUp) {
		t.Fatalf("Reachability = %q, want %q", deviceMetrics.Reachability, state.ReachabilityUp)
	}
	if deviceMetrics.Stale == nil || !*deviceMetrics.Stale {
		t.Fatalf("Stale = %#v, want true", deviceMetrics.Stale)
	}
	if got := delta.DeviceStatuses[deviceID.String()]; got != string(domain.DeviceStatusUp) {
		t.Fatalf("DeviceStatuses[%s] = %q, want %q", deviceID, got, domain.DeviceStatusUp)
	}
}

func TestBuildDeviceDetailDelta_IncludesSelectedDeviceLinkMetricsOnly(t *testing.T) {
	deviceID := uuid.New()
	collectedAt := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)

	delta := buildDeviceDetailDelta(
		domain.Device{ID: deviceID},
		state.DeviceState{
			Metrics: domain.DeviceMetrics{
				DeviceID:    deviceID,
				CollectedAt: collectedAt,
			},
			LinkMetrics: []domain.LinkMetrics{
				{
					IfName:      "ether1",
					TxBps:       floatPtr(125_000_000),
					RxBps:       floatPtr(250_000_000),
					Utilization: floatPtr(0.25),
					CollectedAt: collectedAt,
				},
			},
		},
	)

	if len(delta.LinkMetrics) != 1 {
		t.Fatalf("LinkMetrics length = %d, want 1", len(delta.LinkMetrics))
	}

	linkMetrics, ok := delta.LinkMetrics[deviceID.String()]
	if !ok {
		t.Fatalf("expected LinkMetrics[%s] entry", deviceID)
	}
	if len(linkMetrics) != 1 {
		t.Fatalf("LinkMetrics[%s] length = %d, want 1", deviceID, len(linkMetrics))
	}
	if linkMetrics[0].DeviceID != deviceID.String() {
		t.Fatalf("LinkMetrics[%s][0].DeviceID = %q, want %q", deviceID, linkMetrics[0].DeviceID, deviceID)
	}
	if linkMetrics[0].IfName != "ether1" {
		t.Fatalf("LinkMetrics[%s][0].IfName = %q, want ether1", deviceID, linkMetrics[0].IfName)
	}
	if linkMetrics[0].TxBps == nil {
		t.Fatalf("LinkMetrics[%s][0].TxBps = nil, want value", deviceID)
	}
	if linkMetrics[0].RxBps == nil {
		t.Fatalf("LinkMetrics[%s][0].RxBps = nil, want value", deviceID)
	}
	if linkMetrics[0].Utilization == nil {
		t.Fatalf("LinkMetrics[%s][0].Utilization = nil, want value", deviceID)
	}
	if linkMetrics[0].CollectedAt != collectedAt.Format(time.RFC3339) {
		t.Fatalf("LinkMetrics[%s][0].CollectedAt = %q, want %q", deviceID, linkMetrics[0].CollectedAt, collectedAt.Format(time.RFC3339))
	}
}

func TestComputeSnapshotHashes_DeviceMetricHashIgnoresRemovedDetailFields(t *testing.T) {
	deviceID := uuid.New().String()
	stale := true

	baseMetric := ws.DeviceMetricsDTO{
		DeviceID:     deviceID,
		CPUPercent:   floatPtr(55),
		CollectedAt:  "2026-04-13T12:00:00Z",
		Health:       "warning",
		Reachability: "up",
		Stale:        &stale,
	}

	current := ws.EmptySnapshot()
	current.DeviceMetrics[deviceID] = baseMetric

	updated := ws.EmptySnapshot()
	updated.DeviceMetrics[deviceID] = baseMetric
	updated.DeviceMetrics[deviceID] = ws.DeviceMetricsDTO{
		DeviceID:     baseMetric.DeviceID,
		CPUPercent:   baseMetric.CPUPercent,
		CollectedAt:  baseMetric.CollectedAt,
		Health:       baseMetric.Health,
		Reachability: baseMetric.Reachability,
		Stale:        baseMetric.Stale,
	}

	currentHashes := computeSnapshotHashes(current)
	updatedHashes := computeSnapshotHashes(updated)

	if currentHashes.deviceMetrics[deviceID] != updatedHashes.deviceMetrics[deviceID] {
		t.Fatal("expected removed detail field changes to be ignored by slim device metric hash")
	}
}

func TestBuildDeltaSuppressesUnchangedSections(t *testing.T) {
	deviceID := uuid.New().String()
	sameCPU := floatPtr(10)
	sameTx := floatPtr(100)
	changedUtilization := floatPtr(0.5)

	previous := &ws.SnapshotPayload{
		DeviceMetrics: map[string]ws.DeviceMetricsDTO{
			deviceID: {
				DeviceID:    deviceID,
				CPUPercent:  sameCPU,
				CollectedAt: "2026-04-13T09:30:00Z",
			},
		},
		LinkMetrics: map[string][]ws.LinkMetricsDTO{
			deviceID: {{
				DeviceID:    deviceID,
				IfName:      "ether1",
				TxBps:       sameTx,
				CollectedAt: "2026-04-13T09:30:00Z",
			}},
		},
		DeviceStatuses:  map[string]string{deviceID: "up"},
	}
	current := &ws.SnapshotPayload{
		DeviceMetrics: map[string]ws.DeviceMetricsDTO{
			deviceID: {
				DeviceID:    deviceID,
				CPUPercent:  sameCPU,
				CollectedAt: "2026-04-13T09:30:00Z",
			},
		},
		LinkMetrics: map[string][]ws.LinkMetricsDTO{
			deviceID: {{
				DeviceID:    deviceID,
				IfName:      "ether1",
				TxBps:       sameTx,
				Utilization: changedUtilization,
				CollectedAt: "2026-04-13T09:30:00Z",
			}},
		},
		DeviceStatuses:  map[string]string{deviceID: "up"},
	}

	delta := buildDelta(current, computeSnapshotHashes(current), computeSnapshotHashes(previous))
	if delta == nil {
		t.Fatal("expected delta")
	}
	if len(delta.DeviceMetrics) != 0 {
		t.Fatalf("expected unchanged device_metrics to be suppressed, got %d entries", len(delta.DeviceMetrics))
	}
	if len(delta.DeviceStatuses) != 0 {
		t.Fatalf("expected unchanged device_statuses to be suppressed, got %d entries", len(delta.DeviceStatuses))
	}
	if len(delta.LinkMetrics) != 1 {
		t.Fatalf("expected changed link_metrics to remain, got %d entries", len(delta.LinkMetrics))
	}
}

func intPtr(value int) *int {
	return &value
}
