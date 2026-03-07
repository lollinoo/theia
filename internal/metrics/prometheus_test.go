package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueryDeviceMetricsParsesPrometheusResponses(t *testing.T) {
	t.Parallel()

	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		query := r.URL.Query().Get("query")
		queries = append(queries, query)

		switch {
		case strings.Contains(query, "hrProcessorLoad"):
			writeVectorResponse(t, w, []map[string]any{
				{"metric": map[string]string{"instance": "172.28.10.10"}, "value": []any{1741374000.0, "41.5"}},
				{"metric": map[string]string{"instance": "172.28.10.11"}, "value": []any{1741374000.0, "12"}},
			})
		case strings.Contains(query, "hrStorageUsed"):
			writeVectorResponse(t, w, []map[string]any{
				{"metric": map[string]string{"instance": "172.28.10.10"}, "value": []any{1741374000.0, "67.25"}},
			})
		case strings.Contains(query, "sysUpTime"):
			writeVectorResponse(t, w, []map[string]any{
				{"metric": map[string]string{"instance": "172.28.10.10"}, "value": []any{1741374000.0, "86400"}},
				{"metric": map[string]string{"instance": "172.28.10.11"}, "value": []any{1741374000.0, "5400"}},
			})
		case strings.Contains(query, "entPhySensorValue"):
			writeVectorResponse(t, w, []map[string]any{
				{"metric": map[string]string{"instance": "172.28.10.10"}, "value": []any{1741374000.0, "48"}},
			})
		default:
			t.Fatalf("unexpected query: %s", query)
		}
	}))
	defer server.Close()

	client := NewPromClient(server.URL, server.Client())
	metrics, err := client.QueryDeviceMetrics(context.Background(), []string{"172.28.10.11", "172.28.10.10"})
	if err != nil {
		t.Fatalf("QueryDeviceMetrics returned error: %v", err)
	}

	if len(queries) != 4 {
		t.Fatalf("expected 4 PromQL queries, got %d", len(queries))
	}
	expectedMatcher := `instance=~"^(?:172\\.28\\.10\\.10|172\\.28\\.10\\.11)$"`
	for _, query := range queries {
		if !strings.Contains(query, expectedMatcher) {
			t.Fatalf("query %q missing matcher %q", query, expectedMatcher)
		}
	}

	router := metrics["172.28.10.10"]
	if router.CPUPercent == nil || *router.CPUPercent != 41.5 {
		t.Fatalf("expected router CPU 41.5, got %#v", router.CPUPercent)
	}
	if router.MemPercent == nil || *router.MemPercent != 67.25 {
		t.Fatalf("expected router memory 67.25, got %#v", router.MemPercent)
	}
	if router.UptimeSecs == nil || *router.UptimeSecs != 86400 {
		t.Fatalf("expected router uptime 86400, got %#v", router.UptimeSecs)
	}
	if router.TempCelsius == nil || *router.TempCelsius != 48 {
		t.Fatalf("expected router temp 48, got %#v", router.TempCelsius)
	}
	if router.CollectedAt.IsZero() {
		t.Fatal("expected router CollectedAt to be set")
	}

	switchMetrics := metrics["172.28.10.11"]
	if switchMetrics.CPUPercent == nil || *switchMetrics.CPUPercent != 12 {
		t.Fatalf("expected switch CPU 12, got %#v", switchMetrics.CPUPercent)
	}
	if switchMetrics.MemPercent != nil {
		t.Fatalf("expected switch memory to be nil, got %#v", switchMetrics.MemPercent)
	}
	if switchMetrics.TempCelsius != nil {
		t.Fatalf("expected switch temp to be nil, got %#v", switchMetrics.TempCelsius)
	}
}

func TestQueryDeviceMetricsEmptyResultsReturnNilPointers(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeVectorResponse(t, w, nil)
	}))
	defer server.Close()

	client := NewPromClient(server.URL, server.Client())
	metrics, err := client.QueryDeviceMetrics(context.Background(), []string{"172.28.10.10"})
	if err != nil {
		t.Fatalf("QueryDeviceMetrics returned error: %v", err)
	}

	result := metrics["172.28.10.10"]
	if result.CPUPercent != nil || result.MemPercent != nil || result.TempCelsius != nil || result.UptimeSecs != nil {
		t.Fatalf("expected nil metric pointers, got %+v", result)
	}
	if result.CollectedAt.IsZero() {
		t.Fatal("expected CollectedAt to be set")
	}
}

func TestQueryLinkMetricsParsesThroughputAndUtilization(t *testing.T) {
	t.Parallel()

	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		queries = append(queries, query)

		switch {
		case strings.Contains(query, "ifHCOutOctets"):
			writeVectorResponse(t, w, []map[string]any{
				{"metric": map[string]string{"instance": "172.28.10.10", "ifIndex": "1", "ifName": "ether1"}, "value": []any{1741374000.0, "8000000"}},
			})
		case strings.Contains(query, "ifHCInOctets"):
			writeVectorResponse(t, w, []map[string]any{
				{"metric": map[string]string{"instance": "172.28.10.10", "ifIndex": "1", "ifName": "ether1"}, "value": []any{1741374000.0, "4000000"}},
			})
		case strings.Contains(query, "ifSpeed"):
			writeVectorResponse(t, w, []map[string]any{
				{"metric": map[string]string{"instance": "172.28.10.10", "ifIndex": "1", "ifName": "ether1"}, "value": []any{1741374000.0, "1000000000"}},
			})
		case strings.Contains(query, "ifHighSpeed"):
			writeVectorResponse(t, w, nil)
		default:
			t.Fatalf("unexpected query: %s", query)
		}
	}))
	defer server.Close()

	client := NewPromClient(server.URL, server.Client())
	results, err := client.QueryLinkMetrics(context.Background(), []string{"172.28.10.10"})
	if err != nil {
		t.Fatalf("QueryLinkMetrics returned error: %v", err)
	}

	if len(queries) != 4 {
		t.Fatalf("expected 4 PromQL queries, got %d", len(queries))
	}

	deviceMetrics := results["172.28.10.10"]
	if len(deviceMetrics) != 1 {
		t.Fatalf("expected 1 link metric, got %d", len(deviceMetrics))
	}

	linkMetric := deviceMetrics[0]
	if linkMetric.LinkID != "172.28.10.10:1" {
		t.Fatalf("expected LinkID 172.28.10.10:1, got %s", linkMetric.LinkID)
	}
	if linkMetric.IfName != "ether1" {
		t.Fatalf("expected IfName ether1, got %s", linkMetric.IfName)
	}
	if linkMetric.TxBps == nil || *linkMetric.TxBps != 8000000 {
		t.Fatalf("expected TxBps 8000000, got %#v", linkMetric.TxBps)
	}
	if linkMetric.RxBps == nil || *linkMetric.RxBps != 4000000 {
		t.Fatalf("expected RxBps 4000000, got %#v", linkMetric.RxBps)
	}
	if linkMetric.Utilization == nil || *linkMetric.Utilization != 0.008 {
		t.Fatalf("expected utilization 0.008, got %#v", linkMetric.Utilization)
	}
}

func TestQueryAlertsParsesFiringAlerts(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/alerts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		response := map[string]any{
			"status": "success",
			"data": map[string]any{
				"alerts": []map[string]any{
					{
						"labels": map[string]string{
							"alertname": "DeviceDown",
							"severity":  "critical",
							"instance":  "172.28.10.10",
						},
						"annotations": map[string]string{
							"summary": "gw-core-01 is unreachable",
						},
						"state": "firing",
					},
					{
						"labels": map[string]string{
							"alertname": "HighCPU",
							"severity":  "warning",
							"instance":  "172.28.10.11",
						},
						"annotations": map[string]string{
							"summary": "CPU is elevated",
						},
						"state": "pending",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewPromClient(server.URL, server.Client())
	alerts, err := client.QueryAlerts(context.Background())
	if err != nil {
		t.Fatalf("QueryAlerts returned error: %v", err)
	}

	if len(alerts) != 1 {
		t.Fatalf("expected 1 firing alert, got %d", len(alerts))
	}
	if alerts[0].Severity != "critical" {
		t.Fatalf("expected severity critical, got %s", alerts[0].Severity)
	}
	if alerts[0].Instance != "172.28.10.10" {
		t.Fatalf("expected instance 172.28.10.10, got %s", alerts[0].Instance)
	}
	if alerts[0].AlertName != "DeviceDown" {
		t.Fatalf("expected alert name DeviceDown, got %s", alerts[0].AlertName)
	}
	if alerts[0].State != "firing" {
		t.Fatalf("expected state firing, got %s", alerts[0].State)
	}
	if alerts[0].Summary != "gw-core-01 is unreachable" {
		t.Fatalf("expected summary to parse, got %s", alerts[0].Summary)
	}
}

func writeVectorResponse(t *testing.T, w http.ResponseWriter, results []map[string]any) {
	t.Helper()

	response := map[string]any{
		"status": "success",
		"data": map[string]any{
			"resultType": "vector",
			"result":     results,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
