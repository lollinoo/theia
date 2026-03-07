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

// QueryDeviceMetrics fetches CPU, memory, uptime, and temperature for the requested device IPs.
func (c *PromClient) QueryDeviceMetrics(ctx context.Context, deviceIPs []string) (map[string]domain.DeviceMetrics, error) {
	results := make(map[string]domain.DeviceMetrics, len(deviceIPs))
	if len(deviceIPs) == 0 {
		return results, nil
	}

	collectedAt := time.Now().UTC()
	for _, ip := range uniqueSorted(deviceIPs) {
		results[ip] = domain.DeviceMetrics{CollectedAt: collectedAt}
	}

	targets := buildTargetMatcher(deviceIPs)
	queries := []struct {
		promql string
		apply  func(domain.DeviceMetrics, float64) domain.DeviceMetrics
	}{
		{
			promql: fmt.Sprintf(`avg by (instance) (hrProcessorLoad{instance=~"%s"})`, targets),
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.CPUPercent = floatPtr(value)
				return metric
			},
		},
		{
			promql: fmt.Sprintf(`100 * (hrStorageUsed{hrStorageDescr="Physical memory",instance=~"%s"} / hrStorageSize{hrStorageDescr="Physical memory",instance=~"%s"})`, targets, targets),
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.MemPercent = floatPtr(value)
				return metric
			},
		},
		{
			promql: fmt.Sprintf(`sysUpTime{instance=~"%s"} / 100`, targets),
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.UptimeSecs = floatPtr(value)
				return metric
			},
		},
		{
			promql: fmt.Sprintf(`max by (instance) (entPhySensorValue{entPhySensorType="8",instance=~"%s"})`, targets),
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
			instance := sample.Metric["instance"]
			metric, ok := results[instance]
			if !ok {
				continue
			}
			value, err := sample.SampleValue()
			if err != nil {
				return nil, err
			}
			results[instance] = query.apply(metric, value)
		}
	}

	return results, nil
}

// QueryLinkMetrics fetches interface throughput and utilization for the requested device IPs.
func (c *PromClient) QueryLinkMetrics(ctx context.Context, deviceIPs []string) (map[string][]domain.LinkMetrics, error) {
	results := make(map[string][]domain.LinkMetrics, len(deviceIPs))
	if len(deviceIPs) == 0 {
		return results, nil
	}

	collectedAt := time.Now().UTC()
	for _, ip := range uniqueSorted(deviceIPs) {
		results[ip] = nil
	}

	targets := buildTargetMatcher(deviceIPs)
	interfaces := make(map[string]map[string]*linkAccumulator)

	type linkQuery struct {
		promql string
		apply  func(*linkAccumulator, float64)
	}

	queries := []linkQuery{
		{
			promql: fmt.Sprintf(`rate(ifHCOutOctets{instance=~"%s"}[5m]) * 8`, targets),
			apply: func(metric *linkAccumulator, value float64) {
				metric.txBps = floatPtr(value)
			},
		},
		{
			promql: fmt.Sprintf(`rate(ifHCInOctets{instance=~"%s"}[5m]) * 8`, targets),
			apply: func(metric *linkAccumulator, value float64) {
				metric.rxBps = floatPtr(value)
			},
		},
		{
			promql: fmt.Sprintf(`ifSpeed{instance=~"%s"}`, targets),
			apply: func(metric *linkAccumulator, value float64) {
				metric.ifSpeed = floatPtr(value)
			},
		},
		{
			promql: fmt.Sprintf(`ifHighSpeed{instance=~"%s"}`, targets),
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
			instance := sample.Metric["instance"]
			if _, ok := results[instance]; !ok {
				continue
			}

			value, err := sample.SampleValue()
			if err != nil {
				return nil, err
			}

			entry := getLinkAccumulator(interfaces, instance, sample.Metric)
			query.apply(entry, value)
		}
	}

	for _, ip := range uniqueSorted(deviceIPs) {
		interfaceMap := interfaces[ip]
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
				LinkID:      fmt.Sprintf("%s:%s", ip, key),
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

			results[ip] = append(results[ip], linkMetric)
		}
	}

	return results, nil
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
		parts[i] = regexpQuote(ip)
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
