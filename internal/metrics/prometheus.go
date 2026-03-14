package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/azmin/mikrotik-theia/internal/domain"
)

// PromClient queries the Prometheus HTTP API for live topology metrics.
type PromClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPromClient creates a Prometheus HTTP API client.
func NewPromClient(baseURL string, httpClient *http.Client) *PromClient {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &PromClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: client,
	}
}

// QueryDeviceMetrics fetches CPU, memory, uptime, and temperature for the requested label values.
// labelName is the Prometheus label to filter on (e.g. "instance", "identity", "vendor").
// labelValues are the label values to include.
// Returns a map keyed by the matched label value.
func (c *PromClient) QueryDeviceMetrics(ctx context.Context, labelName string, labelValues []string) (map[string]domain.DeviceMetrics, error) {
	results := make(map[string]domain.DeviceMetrics, len(labelValues))
	if len(labelValues) == 0 {
		return results, nil
	}

	collectedAt := time.Now().UTC()
	for _, v := range uniqueSorted(labelValues) {
		results[v] = domain.DeviceMetrics{CollectedAt: collectedAt}
	}

	targets := buildTargetMatcher(labelValues)
	queries := []struct {
		promql string
		apply  func(domain.DeviceMetrics, float64) domain.DeviceMetrics
	}{
		{
			// MikroTik: mtxrHlCpuLoad (0–100 %)
			// Standard: hrProcessorLoad (0–100 %)
			promql: fmt.Sprintf(
				`avg by (%[1]s) (mtxrHlCpuLoad{%[1]s=~"%[2]s"} or hrProcessorLoad{%[1]s=~"%[2]s"})`,
				labelName, targets),
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.CPUPercent = floatPtr(value)
				return metric
			},
		},
		{
			// MikroTik: 100 * (1 - mtxrHlFreeMemory / mtxrHlTotalMemory)
			// Standard: hrStorageUsed / hrStorageSize for physical/main memory entries
			promql: fmt.Sprintf(
				`(avg by (%[1]s) (100 * (1 - mtxrHlFreeMemory{%[1]s=~"%[2]s"} / on(%[1]s) mtxrHlTotalMemory{%[1]s=~"%[2]s"})))`+
					` or `+
					`(avg by (%[1]s) (100 * hrStorageUsed{hrStorageDescr=~"(?i)physical memory|main memory",%[1]s=~"%[2]s"} / hrStorageSize{hrStorageDescr=~"(?i)physical memory|main memory",%[1]s=~"%[2]s"}))`,
				labelName, targets),
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.MemPercent = floatPtr(value)
				return metric
			},
		},
		{
			// hrSystemUptime (HOST-RESOURCES-MIB) and sysUpTime (SNMPv2-MIB) are both
			// in hundredths of a second; divide by 100 to get seconds.
			promql: fmt.Sprintf(
				`(hrSystemUptime{%[1]s=~"%[2]s"} or sysUpTime{%[1]s=~"%[2]s"}) / 100`,
				labelName, targets),
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.UptimeSecs = floatPtr(value)
				return metric
			},
		},
		{
			// MikroTik: mtxrHlTemperature — already in °C when snmp_exporter applies scale:0.1
			// Standard: entPhySensorValue with sensor type 8 (celsius) is already in °C
			promql: fmt.Sprintf(
				`mtxrHlTemperature{%[1]s=~"%[2]s"}`+
					` or `+
					`max by (%[1]s) (entPhySensorValue{entPhySensorType="8",%[1]s=~"%[2]s"})`,
				labelName, targets),
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.TempCelsius = floatPtr(value)
				return metric
			},
		},
	}

	for _, query := range queries {
		samples, err := c.queryVector(ctx, query.promql)
		if err != nil {
			return nil, err
		}

		for _, sample := range samples {
			labelValue := sample.Metric[labelName]
			metric, ok := results[labelValue]
			if !ok {
				continue
			}
			value, err := sample.SampleValue()
			if err != nil {
				return nil, err
			}
			results[labelValue] = query.apply(metric, value)
		}
	}

	return results, nil
}

// QueryLinkMetrics fetches interface throughput and utilization for the requested label values.
// labelName is the Prometheus label to filter on (e.g. "instance", "identity", "vendor").
// Returns a map keyed by the matched label value.
func (c *PromClient) QueryLinkMetrics(ctx context.Context, labelName string, labelValues []string) (map[string][]domain.LinkMetrics, error) {
	results := make(map[string][]domain.LinkMetrics, len(labelValues))
	if len(labelValues) == 0 {
		return results, nil
	}

	collectedAt := time.Now().UTC()
	for _, v := range uniqueSorted(labelValues) {
		results[v] = nil
	}

	targets := buildTargetMatcher(labelValues)
	interfaces := make(map[string]map[string]*linkAccumulator)

	type linkQuery struct {
		promql string
		apply  func(*linkAccumulator, float64)
	}

	queries := []linkQuery{
		{
			promql: fmt.Sprintf(`rate(ifHCOutOctets{%[1]s=~"%[2]s"}[5m]) * 8`, labelName, targets),
			apply: func(metric *linkAccumulator, value float64) {
				metric.txBps = floatPtr(value)
			},
		},
		{
			promql: fmt.Sprintf(`rate(ifHCInOctets{%[1]s=~"%[2]s"}[5m]) * 8`, labelName, targets),
			apply: func(metric *linkAccumulator, value float64) {
				metric.rxBps = floatPtr(value)
			},
		},
		{
			promql: fmt.Sprintf(`ifSpeed{%[1]s=~"%[2]s"}`, labelName, targets),
			apply: func(metric *linkAccumulator, value float64) {
				metric.ifSpeed = floatPtr(value)
			},
		},
		{
			promql: fmt.Sprintf(`ifHighSpeed{%[1]s=~"%[2]s"}`, labelName, targets),
			apply: func(metric *linkAccumulator, value float64) {
				metric.ifHighSpeedMbps = floatPtr(value)
			},
		},
	}

	for _, query := range queries {
		samples, err := c.queryVector(ctx, query.promql)
		if err != nil {
			return nil, err
		}

		for _, sample := range samples {
			labelValue := sample.Metric[labelName]
			if _, ok := results[labelValue]; !ok {
				continue
			}

			value, err := sample.SampleValue()
			if err != nil {
				return nil, err
			}

			entry := getLinkAccumulator(interfaces, labelValue, sample.Metric)
			query.apply(entry, value)
		}
	}

	for _, v := range uniqueSorted(labelValues) {
		interfaceMap := interfaces[v]
		if len(interfaceMap) == 0 {
			continue
		}

		keys := make([]string, 0, len(interfaceMap))
		for key := range interfaceMap {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			entry := interfaceMap[key]
			linkMetric := domain.LinkMetrics{
				LinkID:      fmt.Sprintf("%s:%s", v, key),
				IfName:      entry.ifName,
				TxBps:       entry.txBps,
				RxBps:       entry.rxBps,
				CollectedAt: collectedAt,
			}

			speedBps := entry.speedBps()
			if speedBps != nil && linkMetric.TxBps != nil && linkMetric.RxBps != nil && *speedBps > 0 {
				utilization := math.Max(*linkMetric.TxBps, *linkMetric.RxBps) / *speedBps
				linkMetric.Utilization = floatPtr(utilization)
			}

			results[v] = append(results[v], linkMetric)
		}
	}

	return results, nil
}

// QueryHostnames fetches the sysName label from Prometheus for the requested label values.
// snmp_exporter exports sysName as a metric where the hostname is a label, e.g.:
//
//	sysName{instance="192.168.1.1", sysName="my-router"} 1
//
// Returns a map keyed by the matched label value → hostname string.
// Entries are only present when a sysName label is found; absent means no data.
func (c *PromClient) QueryHostnames(ctx context.Context, labelName string, labelValues []string) (map[string]string, error) {
	if len(labelValues) == 0 {
		return nil, nil
	}

	targets := buildTargetMatcher(labelValues)
	promql := fmt.Sprintf(`sysName{%s=~"%s"}`, labelName, targets)

	samples, err := c.queryVector(ctx, promql)
	if err != nil {
		return nil, err
	}

	results := make(map[string]string, len(samples))
	for _, sample := range samples {
		labelValue := sample.Metric[labelName]
		if labelValue == "" {
			continue
		}
		if name := sample.Metric["sysName"]; name != "" {
			results[labelValue] = name
		}
	}
	return results, nil
}

// QueryProbeStatus fetches blackbox_exporter probe_success for the given device IPs.
// Returns a map keyed by device IP (port stripped) → true (up) or false (down).
// Only IPs with probe_success data are included; absent entries mean no data available.
func (c *PromClient) QueryProbeStatus(ctx context.Context, deviceIPs []string) (map[string]bool, error) {
	if len(deviceIPs) == 0 {
		return nil, nil
	}

	targets := buildTargetMatcher(deviceIPs)
	promql := fmt.Sprintf(`probe_success{instance=~"%s"}`, targets)

	samples, err := c.queryVector(ctx, promql)
	if err != nil {
		return nil, err
	}

	results := make(map[string]bool, len(samples))
	for _, sample := range samples {
		instance := sample.Metric["instance"]
		if instance == "" {
			continue
		}
		value, err := sample.SampleValue()
		if err != nil {
			continue
		}
		// Strip port suffix for IPv4 (e.g. "192.168.1.1:161" → "192.168.1.1").
		ip := instance
		if !strings.HasPrefix(instance, "[") {
			if i := strings.Index(instance, ":"); i != -1 {
				ip = instance[:i]
			}
		}
		results[ip] = value == 1
	}

	return results, nil
}

// CheckHealth verifies Prometheus is reachable by executing a trivial instant query.
func (c *PromClient) CheckHealth(ctx context.Context) error {
	_, err := c.queryVector(ctx, "vector(1)")
	return err
}

// QueryAlerts fetches currently firing alerts from the Prometheus HTTP API.
func (c *PromClient) QueryAlerts(ctx context.Context) ([]domain.AlertState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/alerts", nil)
	if err != nil {
		return nil, fmt.Errorf("build alerts request: %w", err)
	}

	var response promAlertsResponse
	if err := c.do(req, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("prometheus alerts returned status %q", response.Status)
	}

	alerts := make([]domain.AlertState, 0, len(response.Data.Alerts))
	for _, alert := range response.Data.Alerts {
		if alert.State != "firing" {
			continue
		}
		alerts = append(alerts, domain.AlertState{
			Instance:  alert.Labels["instance"],
			Severity:  alert.Labels["severity"],
			AlertName: alert.Labels["alertname"],
			State:     alert.State,
			Summary:   alert.Annotations["summary"],
		})
	}

	sort.Slice(alerts, func(i, j int) bool {
		if alerts[i].Severity != alerts[j].Severity {
			return alerts[i].Severity < alerts[j].Severity
		}
		if alerts[i].AlertName != alerts[j].AlertName {
			return alerts[i].AlertName < alerts[j].AlertName
		}
		return alerts[i].Summary < alerts[j].Summary
	})

	return alerts, nil
}

type linkAccumulator struct {
	ifName          string
	txBps           *float64
	rxBps           *float64
	ifSpeed         *float64
	ifHighSpeedMbps *float64
}

func (l *linkAccumulator) speedBps() *float64 {
	if l.ifHighSpeedMbps != nil && *l.ifHighSpeedMbps > 0 {
		speed := *l.ifHighSpeedMbps * 1_000_000
		return floatPtr(speed)
	}
	if l.ifSpeed != nil && *l.ifSpeed > 0 {
		return floatPtr(*l.ifSpeed)
	}
	return nil
}

func getLinkAccumulator(store map[string]map[string]*linkAccumulator, instance string, metric map[string]string) *linkAccumulator {
	if store[instance] == nil {
		store[instance] = make(map[string]*linkAccumulator)
	}

	key := linkKey(metric)
	entry := store[instance][key]
	if entry == nil {
		entry = &linkAccumulator{ifName: interfaceName(metric)}
		store[instance][key] = entry
	}
	if entry.ifName == "" {
		entry.ifName = interfaceName(metric)
	}
	return entry
}

func linkKey(metric map[string]string) string {
	if ifIndex := metric["ifIndex"]; ifIndex != "" {
		return ifIndex
	}
	if ifName := metric["ifName"]; ifName != "" {
		return ifName
	}
	if ifDescr := metric["ifDescr"]; ifDescr != "" {
		return ifDescr
	}
	return "unknown"
}

func interfaceName(metric map[string]string) string {
	if ifName := metric["ifName"]; ifName != "" {
		return ifName
	}
	if ifDescr := metric["ifDescr"]; ifDescr != "" {
		return ifDescr
	}
	if ifIndex := metric["ifIndex"]; ifIndex != "" {
		return "ifIndex-" + ifIndex
	}
	return "unknown"
}

func buildTargetMatcher(deviceIPs []string) string {
	parts := uniqueSorted(deviceIPs)
	for i, ip := range parts {
		// Allow optional :<port> suffix so that targets scraped as "192.168.1.1:161"
		// are matched when the device is configured with just "192.168.1.1".
		parts[i] = regexpQuote(ip) + `(?::[0-9]+)?`
	}
	return "^(?:" + strings.Join(parts, "|") + ")$"
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	uniq := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		uniq = append(uniq, value)
	}
	sort.Strings(uniq)
	return uniq
}

func regexpQuote(value string) string {
	return strings.ReplaceAll(regexp.QuoteMeta(value), `\`, `\\`)
}

func floatPtr(value float64) *float64 {
	return &value
}

func (c *PromClient) queryVector(ctx context.Context, promql string) ([]promVectorSample, error) {
	endpoint, err := url.Parse(c.baseURL + "/api/v1/query")
	if err != nil {
		return nil, fmt.Errorf("parse prometheus URL: %w", err)
	}

	values := endpoint.Query()
	values.Set("query", promql)
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build query request: %w", err)
	}

	var response promQueryResponse
	if err := c.do(req, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("prometheus query returned status %q", response.Status)
	}
	if response.Data.ResultType != "vector" {
		return nil, fmt.Errorf("unexpected prometheus result type %q", response.Data.ResultType)
	}

	return response.Data.Result, nil
}

func (c *PromClient) do(req *http.Request, target any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("prometheus request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("prometheus request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode prometheus response: %w", err)
	}
	return nil
}

type promQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string             `json:"resultType"`
		Result     []promVectorSample `json:"result"`
	} `json:"data"`
}

type promVectorSample struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
}

func (s promVectorSample) SampleValue() (float64, error) {
	if len(s.Value) != 2 {
		return 0, fmt.Errorf("unexpected sample shape: %#v", s.Value)
	}

	switch value := s.Value[1].(type) {
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("parse sample value %q: %w", value, err)
		}
		return parsed, nil
	case float64:
		return value, nil
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return 0, fmt.Errorf("parse sample value %q: %w", value.String(), err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unexpected sample value type %T", s.Value[1])
	}
}

type promAlertsResponse struct {
	Status string `json:"status"`
	Data   struct {
		Alerts []promAlert `json:"alerts"`
	} `json:"data"`
}

type promAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
}
