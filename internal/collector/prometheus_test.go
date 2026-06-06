package collector

// This file exercises prometheus behavior so refactors preserve the documented contract.

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
)

type hostnameCall struct {
	labelName   string
	labelValues []string
}

type stubPrometheusEnrichmentClient struct {
	hostnames     map[string]string
	probeStatuses map[string]bool
	alerts        []domain.AlertState

	hostnameCalls []hostnameCall
	probeCalls    [][]string
	alertCalls    int

	hostnameErr error
	probeErr    error
	alertErr    error
}

func (c *stubPrometheusEnrichmentClient) QueryHostnames(_ context.Context, labelName string, labelValues []string) (map[string]string, error) {
	c.hostnameCalls = append(c.hostnameCalls, hostnameCall{
		labelName:   labelName,
		labelValues: append([]string(nil), labelValues...),
	})
	if c.hostnameErr != nil {
		return nil, c.hostnameErr
	}

	results := make(map[string]string, len(labelValues))
	for _, labelValue := range labelValues {
		if hostname, ok := c.hostnames[labelValue]; ok {
			results[labelValue] = hostname
		}
	}

	return results, nil
}

func (c *stubPrometheusEnrichmentClient) QueryProbeStatus(_ context.Context, deviceIPs []string) (map[string]bool, error) {
	c.probeCalls = append(c.probeCalls, append([]string(nil), deviceIPs...))
	if c.probeErr != nil {
		return nil, c.probeErr
	}

	results := make(map[string]bool, len(deviceIPs))
	for _, ip := range deviceIPs {
		if status, ok := c.probeStatuses[ip]; ok {
			results[ip] = status
		}
	}

	return results, nil
}

func (c *stubPrometheusEnrichmentClient) QueryAlerts(context.Context) ([]domain.AlertState, error) {
	c.alertCalls++
	if c.alertErr != nil {
		return nil, c.alertErr
	}

	results := make([]domain.AlertState, len(c.alerts))
	copy(results, c.alerts)
	return results, nil
}

func TestResolvePrometheusLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		device         domain.Device
		wantLabelName  string
		wantLabelValue string
		wantOK         bool
	}{
		{
			name: "uses explicit label pair",
			device: domain.Device{
				PrometheusLabelName:  "identity",
				PrometheusLabelValue: "core-sw-1",
				IP:                   "192.0.2.10",
			},
			wantLabelName:  "identity",
			wantLabelValue: "core-sw-1",
			wantOK:         true,
		},
		{
			name: "falls back to instance and device ip",
			device: domain.Device{
				IP: "192.0.2.11",
			},
			wantLabelName:  "instance",
			wantLabelValue: "192.0.2.11",
			wantOK:         true,
		},
		{
			name: "returns false when no safe target exists",
			device: domain.Device{
				PrometheusLabelName: "identity",
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labelName, labelValue, ok := ResolvePrometheusLabel(tt.device)

			if ok != tt.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tt.wantOK)
			}
			if labelName != tt.wantLabelName {
				t.Fatalf("labelName = %q, want %q", labelName, tt.wantLabelName)
			}
			if labelValue != tt.wantLabelValue {
				t.Fatalf("labelValue = %q, want %q", labelValue, tt.wantLabelValue)
			}
		})
	}
}

func TestPrometheusCollector(t *testing.T) {
	t.Parallel()

	t.Run("collects hostname and probe status without querying alerts", func(t *testing.T) {
		deviceID := mustParseUUID(t, "11111111-1111-1111-1111-111111111111")
		collectedAt := time.Date(2026, 4, 12, 16, 0, 0, 0, time.FixedZone("plus2", 2*60*60))
		client := &stubPrometheusEnrichmentClient{
			hostnames: map[string]string{
				"core-sw-1": "edge-core-01",
			},
			probeStatuses: map[string]bool{
				"192.0.2.10": true,
			},
		}

		collector := NewPrometheusCollector(client)
		collector.now = func() time.Time { return collectedAt }

		result, err := collector.CollectDeviceEnrichment(context.Background(), domain.Device{
			ID:                   deviceID,
			IP:                   "192.0.2.10",
			PrometheusLabelName:  "identity",
			PrometheusLabelValue: "core-sw-1",
		})
		if err != nil {
			t.Fatalf("CollectDeviceEnrichment() error = %v", err)
		}

		if result.DeviceID != deviceID {
			t.Fatalf("DeviceID = %s, want %s", result.DeviceID, deviceID)
		}
		if result.Hostname != "edge-core-01" {
			t.Fatalf("Hostname = %q, want %q", result.Hostname, "edge-core-01")
		}
		if result.ProbeReachable == nil || !*result.ProbeReachable {
			t.Fatalf("ProbeReachable = %v, want true", result.ProbeReachable)
		}
		if !result.CollectedAt.Equal(collectedAt.UTC()) {
			t.Fatalf("CollectedAt = %s, want %s", result.CollectedAt, collectedAt.UTC())
		}
		if len(client.hostnameCalls) != 1 {
			t.Fatalf("hostname calls = %d, want 1", len(client.hostnameCalls))
		}
		if client.hostnameCalls[0].labelName != "identity" {
			t.Fatalf("hostname labelName = %q, want %q", client.hostnameCalls[0].labelName, "identity")
		}
		if !reflect.DeepEqual(client.hostnameCalls[0].labelValues, []string{"core-sw-1"}) {
			t.Fatalf("hostname labelValues = %#v, want %#v", client.hostnameCalls[0].labelValues, []string{"core-sw-1"})
		}
		if len(client.probeCalls) != 1 {
			t.Fatalf("probe calls = %d, want 1", len(client.probeCalls))
		}
		if !reflect.DeepEqual(client.probeCalls[0], []string{"192.0.2.10"}) {
			t.Fatalf("probe call = %#v, want %#v", client.probeCalls[0], []string{"192.0.2.10"})
		}
		if client.alertCalls != 0 {
			t.Fatalf("alert calls = %d, want 0", client.alertCalls)
		}
	})

	t.Run("returns empty enrichment fields without error when no label and no ip exist", func(t *testing.T) {
		deviceID := mustParseUUID(t, "22222222-2222-2222-2222-222222222222")
		collectedAt := time.Date(2026, 4, 12, 16, 5, 0, 0, time.UTC)
		client := &stubPrometheusEnrichmentClient{}

		collector := NewPrometheusCollector(client)
		collector.now = func() time.Time { return collectedAt }

		result, err := collector.CollectDeviceEnrichment(context.Background(), domain.Device{ID: deviceID})
		if err != nil {
			t.Fatalf("CollectDeviceEnrichment() error = %v", err)
		}

		if result.DeviceID != deviceID {
			t.Fatalf("DeviceID = %s, want %s", result.DeviceID, deviceID)
		}
		if result.Hostname != "" {
			t.Fatalf("Hostname = %q, want empty", result.Hostname)
		}
		if result.ProbeReachable != nil {
			t.Fatalf("ProbeReachable = %v, want nil", *result.ProbeReachable)
		}
		if !result.CollectedAt.Equal(collectedAt.UTC()) {
			t.Fatalf("CollectedAt = %s, want %s", result.CollectedAt, collectedAt.UTC())
		}
		if len(client.hostnameCalls) != 0 {
			t.Fatalf("hostname calls = %d, want 0", len(client.hostnameCalls))
		}
		if len(client.probeCalls) != 0 {
			t.Fatalf("probe calls = %d, want 0", len(client.probeCalls))
		}
		if client.alertCalls != 0 {
			t.Fatalf("alert calls = %d, want 0", client.alertCalls)
		}
	})

	t.Run("collect alerts batches through the narrowed client", func(t *testing.T) {
		deviceA := domain.Device{
			ID:                   mustParseUUID(t, "33333333-3333-3333-3333-333333333333"),
			IP:                   "192.0.2.30",
			PrometheusLabelName:  "instance",
			PrometheusLabelValue: "192.0.2.30:9116",
		}
		deviceB := domain.Device{
			ID: mustParseUUID(t, "44444444-4444-4444-4444-444444444444"),
			IP: "192.0.2.31",
		}
		client := &stubPrometheusEnrichmentClient{
			alerts: []domain.AlertState{
				{Instance: "192.0.2.31", Severity: "warning", AlertName: "DiskHigh"},
				{Instance: "192.0.2.30:9116", Severity: "critical", AlertName: "Down"},
			},
		}

		collector := NewPrometheusCollector(client)

		grouped, err := collector.CollectAlerts(context.Background(), []domain.Device{deviceB, deviceA})
		if err != nil {
			t.Fatalf("CollectAlerts() error = %v", err)
		}

		if client.alertCalls != 1 {
			t.Fatalf("alert calls = %d, want 1", client.alertCalls)
		}
		if len(grouped) != 2 {
			t.Fatalf("group count = %d, want 2", len(grouped))
		}
		if got := grouped[deviceA.ID][0].DeviceID; got != deviceA.ID {
			t.Fatalf("device A alert DeviceID = %s, want %s", got, deviceA.ID)
		}
		if got := grouped[deviceB.ID][0].DeviceID; got != deviceB.ID {
			t.Fatalf("device B alert DeviceID = %s, want %s", got, deviceB.ID)
		}
	})

	t.Run("client interface stays enrichment only", func(t *testing.T) {
		tp := reflect.TypeOf((*PrometheusEnrichmentClient)(nil)).Elem()

		if tp.NumMethod() != 3 {
			t.Fatalf("NumMethod = %d, want 3", tp.NumMethod())
		}

		required := []string{"QueryHostnames", "QueryProbeStatus", "QueryAlerts"}
		for _, name := range required {
			if _, ok := tp.MethodByName(name); !ok {
				t.Fatalf("missing method %q", name)
			}
		}

		forbidden := []string{"QueryDeviceMetrics", "QueryLinkMetrics", "QueryInterfaces"}
		for _, name := range forbidden {
			if _, ok := tp.MethodByName(name); ok {
				t.Fatalf("forbidden method %q is present", name)
			}
		}
	})
}

func TestMapAlertsToDevices(t *testing.T) {
	t.Parallel()

	deviceA := domain.Device{
		ID:                   mustParseUUID(t, "55555555-5555-5555-5555-555555555555"),
		IP:                   "192.0.2.40",
		PrometheusLabelName:  "instance",
		PrometheusLabelValue: "192.0.2.40:9116",
	}
	deviceB := domain.Device{
		ID: mustParseUUID(t, "66666666-6666-6666-6666-666666666666"),
		IP: "192.0.2.41",
	}

	grouped := MapAlertsToDevices(
		[]domain.Device{deviceB, deviceA},
		[]domain.AlertState{
			{Instance: "192.0.2.40:9116", Severity: "warning", AlertName: "BGPFlap", Summary: "warn"},
			{Instance: "192.0.2.41", Severity: "critical", AlertName: "Down", Summary: "down"},
			{Instance: "192.0.2.40:9116", Severity: "critical", AlertName: "Down", Summary: "down"},
			{Instance: "192.0.2.99", Severity: "warning", AlertName: "Ignored", Summary: "skip"},
		},
	)

	if len(grouped) != 2 {
		t.Fatalf("group count = %d, want 2", len(grouped))
	}

	wantA := []domain.AlertState{
		{DeviceID: deviceA.ID, Instance: "192.0.2.40:9116", Severity: "critical", AlertName: "Down", Summary: "down"},
		{DeviceID: deviceA.ID, Instance: "192.0.2.40:9116", Severity: "warning", AlertName: "BGPFlap", Summary: "warn"},
	}
	if !reflect.DeepEqual(grouped[deviceA.ID], wantA) {
		t.Fatalf("grouped[%s] = %#v, want %#v", deviceA.ID, grouped[deviceA.ID], wantA)
	}

	wantB := []domain.AlertState{
		{DeviceID: deviceB.ID, Instance: "192.0.2.41", Severity: "critical", AlertName: "Down", Summary: "down"},
	}
	if !reflect.DeepEqual(grouped[deviceB.ID], wantB) {
		t.Fatalf("grouped[%s] = %#v, want %#v", deviceB.ID, grouped[deviceB.ID], wantB)
	}
}

func mustParseUUID(t *testing.T, raw string) uuid.UUID {
	t.Helper()

	parsed, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("uuid.Parse(%q) error = %v", raw, err)
	}

	return parsed
}
