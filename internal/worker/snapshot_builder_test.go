package worker

import (
	"encoding/json"
	"strings"
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
	snapshot := buildPipelineSnapshot(devices, links, states, alerts, ws.PrometheusStatusPayload{Enabled: true, Available: true})

	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if len(snapshot.Devices) != 2 {
		t.Fatalf("expected devices entries for both devices, got %d", len(snapshot.Devices))
	}
	metric, ok := snapshot.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected runtime for device %s", deviceID)
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

	linkRuntime, ok := snapshot.Links[linkID.String()]
	if !ok {
		t.Fatalf("expected links entry for %s", linkID)
	}
	if linkRuntime.SourceIfName != "ether1" {
		t.Fatalf("expected source_if_name ether1, got %q", linkRuntime.SourceIfName)
	}
	if linkRuntime.Utilization == nil || *linkRuntime.Utilization != 0.25 {
		t.Fatalf("expected utilization 0.25, got %#v", linkRuntime.Utilization)
	}
	if got := snapshot.Devices[deviceID.String()].OperationalStatus; got != string(domain.DeviceStatusUp) {
		t.Fatalf("expected status up, got %q", got)
	}
	if got := snapshot.Devices[peerID.String()].OperationalStatus; got != string(domain.DeviceStatusProbing) {
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

	decodedMetrics, ok := decoded["devices"].(map[string]any)
	if !ok {
		t.Fatalf("devices = %#v, want object", decoded["devices"])
	}
	decodedMetric, ok := decodedMetrics[deviceID.String()].(map[string]any)
	if !ok {
		t.Fatalf("devices[%s] = %#v, want object", deviceID, decodedMetrics[deviceID.String()])
	}
	if got := decodedMetric["temp_celsius"]; got != 48.0 {
		t.Fatalf("expected temp_celsius 48, got %#v", got)
	}
	if got := decodedMetric["uptime_secs"]; got != 7200.0 {
		t.Fatalf("expected uptime_secs 7200, got %#v", got)
	}
	if _, ok := decodedMetric["last_polled_at"]; !ok {
		t.Fatal("expected normalized devices payload to include last_polled_at")
	}
	if _, ok := decodedMetric["expected_poll_interval_seconds"]; !ok {
		t.Fatal("expected normalized devices payload to include expected_poll_interval_seconds")
	}
}

func TestBuildPipelineSnapshot_NormalizesRuntimeCollections(t *testing.T) {
	deviceID := uuid.New()
	peerID := uuid.New()
	linkID := uuid.New()
	collectedAt := time.Date(2026, 4, 20, 9, 30, 0, 0, time.UTC)

	snapshot := buildPipelineSnapshot(
		[]domain.Device{
			{
				ID:            deviceID,
				IP:            "192.0.2.10",
				MetricsSource: domain.MetricsSourceSNMP,
				Interfaces:    []domain.Interface{{IfName: "ether1", Speed: 1_000_000_000}},
			},
			{
				ID:            peerID,
				IP:            "192.0.2.11",
				MetricsSource: domain.MetricsSourceSNMP,
				Interfaces:    []domain.Interface{{IfName: "ether2", Speed: 1_000_000_000}},
			},
		},
		[]domain.Link{{
			ID:             linkID,
			SourceDeviceID: deviceID,
			SourceIfName:   "ether1",
			TargetDeviceID: peerID,
			TargetIfName:   "ether2",
		}},
		map[uuid.UUID]state.DeviceState{
			deviceID: {
				Metrics: domain.DeviceMetrics{
					DeviceID:    deviceID,
					CPUPercent:  floatPtr(41.5),
					MemPercent:  floatPtr(62),
					TempCelsius: floatPtr(48),
					UptimeSecs:  floatPtr(7200),
					CollectedAt: collectedAt,
				},
				LinkMetrics:  []domain.LinkMetrics{{IfName: "ether1", TxBps: floatPtr(125_000_000), RxBps: floatPtr(250_000_000), CollectedAt: collectedAt}},
				Health:       state.HealthStatusWarning,
				Reachability: state.ReachabilityUp,
			},
			peerID: {
				Reachability: state.ReachabilityUp,
			},
		},
		map[uuid.UUID][]domain.AlertState{deviceID: {{DeviceID: deviceID, Severity: "critical", State: "firing", AlertName: "HighCPU"}}},
		ws.PrometheusStatusPayload{Enabled: true, Available: true},
	)

	deviceRuntime, ok := snapshot.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected devices[%s] entry", deviceID)
	}
	if deviceRuntime.OperationalStatus != "up" {
		t.Fatalf("OperationalStatus = %q, want up", deviceRuntime.OperationalStatus)
	}
	if deviceRuntime.MetricsStatus != "available" {
		t.Fatalf("MetricsStatus = %q, want available", deviceRuntime.MetricsStatus)
	}
	if deviceRuntime.AlertStatus != string(domain.AlertStatusDown) {
		t.Fatalf("AlertStatus = %q, want %q", deviceRuntime.AlertStatus, domain.AlertStatusDown)
	}

	linkRuntime, ok := snapshot.Links[linkID.String()]
	if !ok {
		t.Fatalf("expected links[%s] entry", linkID)
	}
	if linkRuntime.MetricsStatus != "available" {
		t.Fatalf("MetricsStatus = %q, want available", linkRuntime.MetricsStatus)
	}
	if linkRuntime.Utilization == nil || *linkRuntime.Utilization != 0.25 {
		t.Fatalf("Utilization = %#v, want 0.25", linkRuntime.Utilization)
	}
}

func TestBuildPipelineSnapshot_CompatibilityLinkMetricsPreserveTargetMetricProvenance(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()
	linkID := uuid.New()
	collectedAt := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)

	snapshot := buildPipelineSnapshot(
		[]domain.Device{
			{
				ID:         sourceID,
				IP:         "192.0.2.10",
				Interfaces: []domain.Interface{{IfName: "ether1", Speed: 1_000_000_000}},
			},
			{
				ID:         targetID,
				IP:         "192.0.2.11",
				Interfaces: []domain.Interface{{IfName: "ether2", Speed: 1_000_000_000}},
			},
		},
		[]domain.Link{{
			ID:             linkID,
			SourceDeviceID: sourceID,
			SourceIfName:   "ether1",
			TargetDeviceID: targetID,
			TargetIfName:   "ether2",
		}},
		map[uuid.UUID]state.DeviceState{
			sourceID: {Reachability: state.ReachabilityUp},
			targetID: {
				Reachability: state.ReachabilityUp,
				LinkMetrics: []domain.LinkMetrics{{
					IfName:      "ether2",
					TxBps:       floatPtr(10),
					RxBps:       floatPtr(20),
					CollectedAt: collectedAt,
				}},
			},
		},
		nil,
		ws.PrometheusStatusPayload{},
	)

	linkRuntime, ok := snapshot.Links[linkID.String()]
	if !ok {
		t.Fatalf("expected links[%s] entry", linkID)
	}
	if linkRuntime.SourceDeviceID != sourceID.String() {
		t.Fatalf("Links[%s].SourceDeviceID = %q, want %q", linkID, linkRuntime.SourceDeviceID, sourceID)
	}
	if linkRuntime.SourceIfName != "ether1" {
		t.Fatalf("Links[%s].SourceIfName = %q, want ether1", linkID, linkRuntime.SourceIfName)
	}

	legacyMetrics, ok := snapshot.LinkMetrics[targetID.String()]
	if !ok {
		t.Fatalf("expected compatibility link_metrics entry for target device %s", targetID)
	}
	if _, ok := snapshot.LinkMetrics[sourceID.String()]; ok {
		t.Fatalf("did not expect compatibility link_metrics entry for source device %s", sourceID)
	}
	if len(legacyMetrics) != 1 {
		t.Fatalf("LinkMetrics[%s] length = %d, want 1", targetID, len(legacyMetrics))
	}
	if legacyMetrics[0].DeviceID != targetID.String() {
		t.Fatalf("LinkMetrics[%s][0].DeviceID = %q, want %q", targetID, legacyMetrics[0].DeviceID, targetID)
	}
	if legacyMetrics[0].IfName != "ether2" {
		t.Fatalf("LinkMetrics[%s][0].IfName = %q, want ether2", targetID, legacyMetrics[0].IfName)
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
		ws.PrometheusStatusPayload{},
	)

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	deviceMetrics, ok := decoded["devices"].(map[string]any)
	if !ok {
		t.Fatalf("devices = %#v, want object", decoded["devices"])
	}
	metric, ok := deviceMetrics[deviceID.String()].(map[string]any)
	if !ok {
		t.Fatalf("devices[%s] = %#v, want object", deviceID, deviceMetrics[deviceID.String()])
	}
	if got := metric["temp_celsius"]; got != 48.0 {
		t.Fatalf("temp_celsius = %#v, want 48", got)
	}
	if got := metric["uptime_secs"]; got != 3600.0 {
		t.Fatalf("uptime_secs = %#v, want 3600", got)
	}
	if got := metric["last_polled_at"]; got != "2026-04-13T12:00:00Z" {
		t.Fatalf("last_polled_at = %#v, want 2026-04-13T12:00:00Z", got)
	}
	if got := metric["expected_poll_interval_seconds"]; got != 30.0 {
		t.Fatalf("expected_poll_interval_seconds = %#v, want 30", got)
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

	snapshot := buildPipelineSnapshot(devices, nil, states, nil, ws.PrometheusStatusPayload{})

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
		ws.PrometheusStatusPayload{},
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
		ws.PrometheusStatusPayload{},
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
		ws.PrometheusStatusPayload{},
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
		nil,
		ws.PrometheusStatusPayload{},
	)

	deviceMetrics, ok := delta.Devices[deviceID.String()]
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
	if got := delta.Devices[deviceID.String()].OperationalStatus; got != string(domain.DeviceStatusUp) {
		t.Fatalf("Devices[%s].OperationalStatus = %q, want %q", deviceID, got, domain.DeviceStatusUp)
	}
}

func TestBuildDeviceDetailDelta_UsesPrometheusStatusAndAlerts(t *testing.T) {
	deviceID := uuid.New()

	delta := buildDeviceDetailDelta(
		domain.Device{ID: deviceID, MetricsSource: domain.MetricsSourcePrometheus},
		state.DeviceState{},
		[]domain.AlertState{{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
		}},
		ws.PrometheusStatusPayload{Enabled: true, Available: false},
	)

	deviceMetrics, ok := delta.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected device detail delta for %s", deviceID)
	}
	if deviceMetrics.PrimaryReason != normalizedReasonUpstreamUnavailable {
		t.Fatalf("PrimaryReason = %q, want %q", deviceMetrics.PrimaryReason, normalizedReasonUpstreamUnavailable)
	}
	if deviceMetrics.MetricsReason != normalizedReasonUpstreamUnavailable {
		t.Fatalf("MetricsReason = %q, want %q", deviceMetrics.MetricsReason, normalizedReasonUpstreamUnavailable)
	}
	if deviceMetrics.AlertStatus != string(domain.AlertStatusDown) {
		t.Fatalf("AlertStatus = %q, want %q", deviceMetrics.AlertStatus, domain.AlertStatusDown)
	}
	if deviceMetrics.FiringAlertCount != 1 {
		t.Fatalf("FiringAlertCount = %d, want 1", deviceMetrics.FiringAlertCount)
	}
}

func TestBuildDeviceDetailDelta_IncludesSelectedDeviceLinkMetricsOnly(t *testing.T) {
	deviceID := uuid.New()
	peerID := uuid.New()
	linkID := uuid.New()
	collectedAt := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)

	device := domain.Device{ID: deviceID, Interfaces: []domain.Interface{{IfName: "ether1", Speed: 1_000_000_000}}}
	peer := domain.Device{ID: peerID, Interfaces: []domain.Interface{{IfName: "ether2", Speed: 1_000_000_000}}}
	link := domain.Link{ID: linkID, SourceDeviceID: deviceID, SourceIfName: "ether1", TargetDeviceID: peerID, TargetIfName: "ether2"}
	deviceState := state.DeviceState{
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
	}
	linkRuntimes := buildDeviceLinkRuntimeDTOs(
		device,
		deviceState,
		map[uuid.UUID]domain.Device{deviceID: device, peerID: peer},
		map[uuid.UUID]state.DeviceState{deviceID: deviceState, peerID: {Reachability: state.ReachabilityUp}},
		[]domain.Link{link},
		ws.PrometheusStatusPayload{},
	)
	delta := buildDeviceDetailDeltaWithLinks(
		device,
		state.DeviceState{
			Metrics:     deviceState.Metrics,
			LinkMetrics: deviceState.LinkMetrics,
		},
		linkRuntimes,
		nil,
		ws.PrometheusStatusPayload{},
	)

	if len(delta.Links) != 1 {
		t.Fatalf("Links length = %d, want 1", len(delta.Links))
	}

	linkMetrics, ok := delta.Links[linkID.String()]
	if !ok {
		t.Fatalf("expected Links[%s] entry", linkID)
	}
	if linkMetrics.SourceDeviceID != deviceID.String() {
		t.Fatalf("Links[%s].SourceDeviceID = %q, want %q", linkID, linkMetrics.SourceDeviceID, deviceID)
	}
	if linkMetrics.SourceIfName != "ether1" {
		t.Fatalf("Links[%s].SourceIfName = %q, want ether1", linkID, linkMetrics.SourceIfName)
	}
	if linkMetrics.TxBps == nil {
		t.Fatalf("Links[%s].TxBps = nil, want value", linkID)
	}
	if linkMetrics.RxBps == nil {
		t.Fatalf("Links[%s].RxBps = nil, want value", linkID)
	}
	if linkMetrics.Utilization == nil {
		t.Fatalf("Links[%s].Utilization = nil, want value", linkID)
	}
	if linkMetrics.LastCollectedAt == nil || *linkMetrics.LastCollectedAt != collectedAt.Format(time.RFC3339) {
		t.Fatalf("Links[%s].LastCollectedAt = %#v, want %q", linkID, linkMetrics.LastCollectedAt, collectedAt.Format(time.RFC3339))
	}
}

func TestComputeSnapshotHashes_DeviceMetricHashIgnoresRemovedDetailFields(t *testing.T) {
	deviceID := uuid.New().String()
	stale := true

	baseMetric := ws.DeviceRuntimeDTO{
		DeviceID:          deviceID,
		OperationalStatus: "up",
		Reachability:      "up",
		Health:            "warning",
		Freshness:         "stale",
		PrimaryReason:     "stale",
		MetricsStatus:     "partial",
		MetricsReason:     "ok",
		AlertStatus:       "normal",
		CPUPercent:        floatPtr(55),
		LastCollectedAt:   stringPtr("2026-04-13T12:00:00Z"),
		Stale:             &stale,
	}

	current := ws.EmptySnapshot()
	current.Devices[deviceID] = baseMetric
	syncSnapshotCompatibility(current)

	updated := ws.EmptySnapshot()
	updated.Devices[deviceID] = baseMetric
	syncSnapshotCompatibility(updated)

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
	linkID := uuid.New().String()

	previous := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			deviceID: {
				DeviceID:          deviceID,
				OperationalStatus: "up",
				Reachability:      "up",
				Health:            "unknown",
				Freshness:         "fresh",
				PrimaryReason:     "ok",
				MetricsStatus:     "partial",
				MetricsReason:     "ok",
				AlertStatus:       "normal",
				CPUPercent:        sameCPU,
				LastCollectedAt:   stringPtr("2026-04-13T09:30:00Z"),
			},
		},
		Links: map[string]ws.LinkRuntimeDTO{
			linkID: {
				LinkID:          linkID,
				SourceDeviceID:  deviceID,
				TargetDeviceID:  uuid.New().String(),
				SourceIfName:    "ether1",
				TargetIfName:    "ether2",
				MetricsStatus:   "partial",
				MetricsReason:   "ok",
				TxBps:           sameTx,
				LastCollectedAt: stringPtr("2026-04-13T09:30:00Z"),
			},
		},
	}
	current := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			deviceID: {
				DeviceID:          deviceID,
				OperationalStatus: "up",
				Reachability:      "up",
				Health:            "unknown",
				Freshness:         "fresh",
				PrimaryReason:     "ok",
				MetricsStatus:     "partial",
				MetricsReason:     "ok",
				AlertStatus:       "normal",
				CPUPercent:        sameCPU,
				LastCollectedAt:   stringPtr("2026-04-13T09:30:00Z"),
			},
		},
		Links: map[string]ws.LinkRuntimeDTO{
			linkID: {
				LinkID:          linkID,
				SourceDeviceID:  deviceID,
				TargetDeviceID:  previous.Links[linkID].TargetDeviceID,
				SourceIfName:    "ether1",
				TargetIfName:    "ether2",
				MetricsStatus:   "available",
				MetricsReason:   "ok",
				TxBps:           sameTx,
				Utilization:     changedUtilization,
				LastCollectedAt: stringPtr("2026-04-13T09:30:00Z"),
			},
		},
	}
	syncSnapshotCompatibility(previous)
	syncSnapshotCompatibility(current)

	delta := buildDelta(current, computeSnapshotHashes(current), computeSnapshotHashes(previous))
	if delta == nil {
		t.Fatal("expected delta")
	}
	if len(delta.Devices) != 0 {
		t.Fatalf("expected unchanged devices to be suppressed, got %d entries", len(delta.Devices))
	}
	if len(delta.Links) != 1 {
		t.Fatalf("expected changed links to remain, got %d entries", len(delta.Links))
	}
}

func TestBuildRuntimeDeltaPatchIncludesOnlyChangedRuntimeFields(t *testing.T) {
	deviceID := uuid.New().String()
	targetID := uuid.New().String()
	linkID := uuid.New().String()
	oldCPU := 10.0
	newCPU := 25.0
	tx := 100.0

	previous := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			deviceID: {
				DeviceID:          deviceID,
				OperationalStatus: "up",
				PrimaryHealth:     "up_fresh",
				RuntimeFlags:      []string{},
				FieldStates: map[string]string{
					"uptime": "ok",
					"cpu":    "ok",
					"memory": "ok",
				},
				NetworkReachable: "true",
				SNMPReachable:    "true",
				Reachability:     "up",
				Health:           "healthy",
				Freshness:        "fresh",
				PrimaryReason:    "ok",
				MetricsStatus:    "available",
				MetricsReason:    "ok",
				AlertStatus:      "normal",
				FiringAlertCount: 0,
				CPUPercent:       &oldCPU,
				LastCollectedAt:  stringPtr("2026-05-01T10:00:00Z"),
			},
		},
		Links: map[string]ws.LinkRuntimeDTO{
			linkID: {
				LinkID:          linkID,
				SourceDeviceID:  deviceID,
				TargetDeviceID:  targetID,
				SourceIfName:    "ether1",
				TargetIfName:    "ether2",
				MetricsStatus:   "available",
				MetricsReason:   "ok",
				TxBps:           &tx,
				LastCollectedAt: stringPtr("2026-05-01T10:00:00Z"),
			},
		},
	}
	delta := ws.EmptySnapshot()
	delta.Devices[deviceID] = previous.Devices[deviceID]
	changedDevice := delta.Devices[deviceID]
	changedDevice.CPUPercent = &newCPU
	delta.Devices[deviceID] = changedDevice
	delta.Links[linkID] = previous.Links[linkID]

	patch := buildRuntimeDeltaPatch(delta, previous)
	if patch == nil {
		t.Fatal("expected runtime delta patch")
	}
	if len(patch.Devices) != 1 {
		t.Fatalf("patch devices = %d, want 1", len(patch.Devices))
	}
	devicePatch := patch.Devices[deviceID]
	if got := devicePatch["device_id"]; got != deviceID {
		t.Fatalf("device_id = %#v, want %s", got, deviceID)
	}
	if got := devicePatch["cpu_percent"]; got != newCPU {
		t.Fatalf("cpu_percent = %#v, want %v", got, newCPU)
	}
	if _, ok := devicePatch["operational_status"]; ok {
		t.Fatalf("unexpected unchanged operational_status in device patch: %#v", devicePatch)
	}
	if len(patch.Links) != 0 {
		t.Fatalf("expected unchanged incident link to be omitted, got %#v", patch.Links)
	}
}

func TestBuildRuntimeDeltaPatchSerializesClearedRuntimeFlagsAsArray(t *testing.T) {
	deviceID := uuid.New().String()

	previous := ws.EmptySnapshot()
	previous.Devices[deviceID] = ws.DeviceRuntimeDTO{
		DeviceID:     deviceID,
		RuntimeFlags: []string{"partial_telemetry"},
	}

	delta := ws.EmptySnapshot()
	delta.Devices[deviceID] = ws.DeviceRuntimeDTO{
		RuntimeFlags: []string{},
	}

	patch := buildRuntimeDeltaPatch(delta, previous)
	if patch == nil {
		t.Fatal("expected runtime delta patch")
	}

	raw, err := json.Marshal(patch.Devices[deviceID])
	if err != nil {
		t.Fatalf("marshal device patch: %v", err)
	}
	if !strings.Contains(string(raw), `"runtime_flags":[]`) {
		t.Fatalf("runtime_flags JSON = %s, want an empty array", raw)
	}
}

func TestBuildRuntimeDeltaPatchUsesMapKeysAsPatchIdentifiers(t *testing.T) {
	deviceID := uuid.New().String()
	linkID := uuid.New().String()
	oldCPU := 10.0
	newCPU := 25.0
	oldTx := 100.0
	newTx := 200.0

	previous := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			deviceID: {
				DeviceID:    deviceID,
				CPUPercent:  &oldCPU,
				FieldStates: map[string]string{"cpu": "ok", "memory": "ok", "uptime": "ok"},
			},
		},
		Links: map[string]ws.LinkRuntimeDTO{
			linkID: {
				LinkID: linkID,
				TxBps:  &oldTx,
			},
		},
	}
	delta := ws.EmptySnapshot()
	delta.Devices[deviceID] = ws.DeviceRuntimeDTO{
		CPUPercent:  &newCPU,
		FieldStates: map[string]string{"cpu": "ok", "memory": "ok", "uptime": "ok"},
	}
	delta.Links[linkID] = ws.LinkRuntimeDTO{
		TxBps: &newTx,
	}

	patch := buildRuntimeDeltaPatch(delta, previous)
	if patch == nil {
		t.Fatal("expected runtime delta patch")
	}
	if got := patch.Devices[deviceID]["device_id"]; got != deviceID {
		t.Fatalf("device_id = %#v, want map key %s", got, deviceID)
	}
	if got := patch.Links[linkID]["link_id"]; got != linkID {
		t.Fatalf("link_id = %#v, want map key %s", got, linkID)
	}
}

func TestBuildRuntimeDeltaPatchDoesNotEmitInvalidZeroSemanticFields(t *testing.T) {
	deviceID := uuid.New().String()
	linkID := uuid.New().String()
	oldCPU := 10.0
	newCPU := 25.0
	oldTx := 100.0
	newTx := 200.0

	previous := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			deviceID: {
				DeviceID:          deviceID,
				OperationalStatus: "up",
				PrimaryHealth:     "up_fresh",
				FieldStates:       map[string]string{"cpu": "ok", "memory": "ok", "uptime": "ok"},
				Reachability:      "up",
				Health:            "healthy",
				Freshness:         "fresh",
				PrimaryReason:     "ok",
				MetricsStatus:     "available",
				MetricsReason:     "ok",
				AlertStatus:       "normal",
				CPUPercent:        &oldCPU,
			},
		},
		Links: map[string]ws.LinkRuntimeDTO{
			linkID: {
				LinkID:        linkID,
				MetricsStatus: "available",
				MetricsReason: "ok",
				TxBps:         &oldTx,
			},
		},
	}
	delta := ws.EmptySnapshot()
	delta.Devices[deviceID] = ws.DeviceRuntimeDTO{CPUPercent: &newCPU}
	delta.Links[linkID] = ws.LinkRuntimeDTO{TxBps: &newTx}

	patch := buildRuntimeDeltaPatch(delta, previous)
	if patch == nil {
		t.Fatal("expected runtime delta patch")
	}
	devicePatch := patch.Devices[deviceID]
	for _, key := range []string{
		"operational_status",
		"primary_health",
		"field_states",
		"reachability",
		"health",
		"freshness",
		"primary_reason",
		"metrics_status",
		"metrics_reason",
		"alert_status",
	} {
		if _, ok := devicePatch[key]; ok {
			t.Fatalf("unexpected invalid zero-value device field %q in patch: %#v", key, devicePatch)
		}
	}
	linkPatch := patch.Links[linkID]
	for _, key := range []string{"metrics_status", "metrics_reason"} {
		if _, ok := linkPatch[key]; ok {
			t.Fatalf("unexpected invalid zero-value link field %q in patch: %#v", key, linkPatch)
		}
	}
}

func stringPtr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}
