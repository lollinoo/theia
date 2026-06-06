package collector

// This file exercises prometheus contract behavior so refactors preserve the documented contract.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/metrics"
)

type prometheusContractFixture struct {
	Version int `json:"version"`
	Device  struct {
		IP         string `json:"ip"`
		LabelName  string `json:"label_name"`
		LabelValue string `json:"label_value"`
	} `json:"device"`
	Probe []struct {
		Instance string  `json:"instance"`
		Value    float64 `json:"value"`
	} `json:"probe"`
	Links []struct {
		Instance string  `json:"instance"`
		IfIndex  string  `json:"if_index"`
		IfName   string  `json:"if_name"`
		TxBps    float64 `json:"tx_bps"`
		RxBps    float64 `json:"rx_bps"`
		IfSpeed  float64 `json:"if_speed"`
	} `json:"links"`
}

func TestPrometheusCollectorContractCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fixture    string
		wantStatus domain.DeviceStatus
	}{
		{name: "valid", fixture: "valid.json", wantStatus: domain.DeviceStatusUp},
		{name: "empty", fixture: "empty.json", wantStatus: domain.DeviceStatusUnknown},
		{name: "partial", fixture: "partial.json", wantStatus: domain.DeviceStatusUp},
		{name: "mismatch", fixture: "mismatch.json", wantStatus: domain.DeviceStatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := loadPrometheusContractFixture(t, tt.fixture)
			server := newPrometheusContractServer(t, fixture)
			defer server.Close()

			collector := NewPrometheusCollector(metrics.NewPromClient(server.URL, nil))
			enrichment, err := collector.CollectDeviceEnrichment(context.Background(), domain.Device{
				ID:                   uuid.New(),
				IP:                   fixture.Device.IP,
				PrometheusLabelName:  fixture.Device.LabelName,
				PrometheusLabelValue: fixture.Device.LabelValue,
			})
			if err != nil {
				t.Fatalf("CollectDeviceEnrichment() error = %v", err)
			}

			status := domain.DeviceStatusUnknown
			if enrichment.ProbeReachable != nil {
				if *enrichment.ProbeReachable {
					status = domain.DeviceStatusUp
				} else {
					status = domain.DeviceStatusDown
				}
			}

			if status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", status, tt.wantStatus)
			}
		})
	}
}

func loadPrometheusContractFixture(t *testing.T, name string) prometheusContractFixture {
	t.Helper()

	path := filepath.Join("testdata", "prometheus", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	var fixture prometheusContractFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", path, err)
	}
	if fixture.Version != 1 {
		t.Fatalf("fixture version = %d, want 1", fixture.Version)
	}

	return fixture
}

func newPrometheusContractServer(t *testing.T, fixture prometheusContractFixture) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		var result []map[string]any

		switch {
		case strings.Contains(query, "probe_success"):
			result = fixtureProbeSamples(fixture)
		case strings.Contains(query, "rate(ifHCOutOctets"):
			result = fixtureLinkSamples(fixture, "tx")
		case strings.Contains(query, "rate(ifHCInOctets"):
			result = fixtureLinkSamples(fixture, "rx")
		case strings.Contains(query, "ifSpeed{"):
			result = fixtureLinkSamples(fixture, "speed")
		default:
			result = nil
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result":     result,
			},
		}); err != nil {
			t.Fatalf("encode Prometheus fixture response: %v", err)
		}
	}))
}

func fixtureProbeSamples(fixture prometheusContractFixture) []map[string]any {
	result := make([]map[string]any, 0, len(fixture.Probe))
	for _, sample := range fixture.Probe {
		result = append(result, map[string]any{
			"metric": map[string]string{"instance": sample.Instance},
			"value":  []any{1741374000.0, strconv.FormatFloat(sample.Value, 'f', -1, 64)},
		})
	}
	return result
}

func fixtureLinkSamples(fixture prometheusContractFixture, field string) []map[string]any {
	result := make([]map[string]any, 0, len(fixture.Links))
	for _, sample := range fixture.Links {
		value := 0.0
		switch field {
		case "tx":
			value = sample.TxBps / 8
		case "rx":
			value = sample.RxBps / 8
		case "speed":
			value = sample.IfSpeed
		}
		result = append(result, map[string]any{
			"metric": map[string]string{
				"instance": sample.Instance,
				"ifIndex":  sample.IfIndex,
				"ifName":   sample.IfName,
			},
			"value": []any{1741374000.0, strconv.FormatFloat(value, 'f', -1, 64)},
		})
	}
	return result
}
