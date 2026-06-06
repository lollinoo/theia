package metrics

// This file defines prometheus Prometheus metrics registration and reporting behavior.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/vendor"
)

const defaultPrometheusOperationTimeout = 3 * time.Second

// PromClient queries the Prometheus HTTP API for live topology metrics.
type PromClient struct {
	baseURL          string
	httpClient       *http.Client
	operationTimeout time.Duration
}

// NewPromClient creates a Prometheus HTTP API client.
func NewPromClient(baseURL string, httpClient *http.Client) *PromClient {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	return &PromClient{
		baseURL:          strings.TrimRight(baseURL, "/"),
		httpClient:       client,
		operationTimeout: defaultPrometheusOperationTimeout,
	}
}

// BaseURL returns the current Prometheus base URL.
func (c *PromClient) BaseURL() string {
	return c.baseURL
}

// WithBaseURL clones the client configuration for a different Prometheus base URL.
func (c *PromClient) WithBaseURL(baseURL string) *PromClient {
	if c == nil {
		return NewPromClient(baseURL, nil)
	}

	return &PromClient{
		baseURL:          strings.TrimRight(baseURL, "/"),
		httpClient:       c.httpClient,
		operationTimeout: c.runtimeOperationTimeout(),
	}
}

// QueryDeviceMetrics fetches CPU, memory, uptime, and temperature for the requested label values.
// labelName is the Prometheus label to filter on (e.g. "instance", "identity", "vendor").
// labelValues are the label values to include.
// promMetrics contains vendor-resolved PromQL query templates with %[1]s (labelName)
// and %[2]s (target matcher) placeholders.
// Returns a map keyed by the matched label value.
func (c *PromClient) QueryDeviceMetrics(ctx context.Context, labelName string, labelValues []string, promMetrics vendor.PrometheusMetrics) (map[string]domain.DeviceMetrics, error) {
	results := make(map[string]domain.DeviceMetrics, len(labelValues))
	if len(labelValues) == 0 {
		return results, nil
	}

	collectedAt := time.Now().UTC()
	for _, v := range uniqueSorted(labelValues) {
		results[v] = domain.DeviceMetrics{}
	}

	targets := buildTargetMatcher(labelValues)
	queries := []struct {
		template string
		apply    func(domain.DeviceMetrics, float64) domain.DeviceMetrics
	}{
		{
			template: promMetrics.CPU,
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.CPUPercent = floatPtr(value)
				return metric
			},
		},
		{
			template: promMetrics.Memory,
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.MemPercent = floatPtr(value)
				return metric
			},
		},
		{
			template: promMetrics.Uptime,
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.UptimeSecs = floatPtr(value)
				return metric
			},
		},
		{
			template: promMetrics.Temperature,
			apply: func(metric domain.DeviceMetrics, value float64) domain.DeviceMetrics {
				metric.TempCelsius = floatPtr(value)
				return metric
			},
		},
	}

	for _, query := range queries {
		if query.template == "" {
			continue
		}
		promql := fmt.Sprintf(query.template, labelName, targets)
		samples, err := c.queryVector(ctx, promql)
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
			metric = query.apply(metric, value)
			if metric.CollectedAt.IsZero() {
				metric.CollectedAt = collectedAt
			}
			results[labelValue] = metric
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

// QueryInterfaces discovers interfaces from Prometheus SNMP exporter metrics.
// It queries ifDescr (or ifHCInOctets as fallback) to find all interfaces for a device,
// extracting ifName, ifDescr, ifIndex, and ifSpeed from metric labels.
// Returns a map keyed by the matched label value → slice of domain.Interface.
func (c *PromClient) QueryInterfaces(ctx context.Context, labelName string, labelValues []string) (map[string][]domain.Interface, error) {
	results := make(map[string][]domain.Interface, len(labelValues))
	if len(labelValues) == 0 {
		return results, nil
	}

	targets := buildTargetMatcher(labelValues)

	// Use ifDescr metric as the primary source, fall back to ifHCInOctets
	promql := fmt.Sprintf(`ifDescr{%s=~"%s"}`, labelName, targets)
	samples, err := c.queryVector(ctx, promql)
	if err != nil || len(samples) == 0 {
		promql = fmt.Sprintf(`ifHCInOctets{%s=~"%s"}`, labelName, targets)
		samples, err = c.queryVector(ctx, promql)
		if err != nil {
			return results, nil
		}
	}

	// Group by label value, dedup by ifIndex or ifName
	type ifaceKey struct {
		labelValue string
		ifKey      string
	}
	seen := make(map[ifaceKey]bool)

	for _, sample := range samples {
		labelValue := sample.Metric[labelName]
		if labelValue == "" {
			continue
		}

		ifName := interfaceName(sample.Metric)
		key := ifaceKey{labelValue: labelValue, ifKey: ifName}
		if seen[key] {
			continue
		}
		seen[key] = true

		iface := domain.Interface{
			IfName:  ifName,
			IfDescr: sample.Metric["ifDescr"],
		}
		if idx, err := strconv.Atoi(sample.Metric["ifIndex"]); err == nil {
			iface.IfIndex = idx
		}

		// Determine speed from ifHighSpeed or ifSpeed labels
		if hs, err := strconv.ParseInt(sample.Metric["ifHighSpeed"], 10, 64); err == nil && hs > 0 {
			iface.Speed = hs * 1_000_000
		} else if spd, err := strconv.ParseInt(sample.Metric["ifSpeed"], 10, 64); err == nil && spd > 0 {
			iface.Speed = spd
		}

		// Assume operational if we see traffic data
		iface.OperStatus = "up"
		iface.AdminStatus = "up"

		results[labelValue] = append(results[labelValue], iface)
	}

	// Also try to get operational status from ifOperStatus metric
	statusQL := fmt.Sprintf(`ifOperStatus{%s=~"%s"}`, labelName, targets)
	statusSamples, err := c.queryVector(ctx, statusQL)
	if err == nil {
		for _, sample := range statusSamples {
			labelValue := sample.Metric[labelName]
			ifName := interfaceName(sample.Metric)
			val, err := sample.SampleValue()
			if err != nil {
				continue
			}
			status := "down"
			if val == 1 {
				status = "up"
			}
			for i := range results[labelValue] {
				if results[labelValue][i].IfName == ifName {
					results[labelValue][i].OperStatus = status
					break
				}
			}
		}
	}

	// Try to get ifSpeed from the separate metric if not from labels
	speedQL := fmt.Sprintf(`ifHighSpeed{%s=~"%s"}`, labelName, targets)
	speedSamples, err := c.queryVector(ctx, speedQL)
	if err == nil {
		for _, sample := range speedSamples {
			labelValue := sample.Metric[labelName]
			ifName := interfaceName(sample.Metric)
			val, err := sample.SampleValue()
			if err != nil || val <= 0 {
				continue
			}
			for i := range results[labelValue] {
				if results[labelValue][i].IfName == ifName && results[labelValue][i].Speed == 0 {
					results[labelValue][i].Speed = int64(val) * 1_000_000
					break
				}
			}
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

// CheckHealthFast verifies Prometheus reachability with a short timeout (2s).
// Used by the background health probe to detect outages quickly.
func (c *PromClient) CheckHealthFast(ctx context.Context) error {
	fastCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := c.queryVector(fastCtx, "vector(1)")
	return err
}

// QueryAlerts fetches currently firing alerts from the Prometheus HTTP API.
func (c *PromClient) QueryAlerts(ctx context.Context) ([]domain.AlertState, error) {
	ctx, cancel := c.withRuntimeTimeout(ctx)
	defer cancel()

	startedAt := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/alerts", nil)
	if err != nil {
		observability.Default().ObservePrometheusRuntimeRequest("alerts", classifyPrometheusRequestResult(err), time.Since(startedAt))
		return nil, fmt.Errorf("build alerts request: %w", err)
	}

	var response promAlertsResponse
	if err := c.do(req, &response); err != nil {
		observability.Default().ObservePrometheusRuntimeRequest("alerts", classifyPrometheusRequestResult(err), time.Since(startedAt))
		return nil, err
	}
	if response.Status != "success" {
		err := fmt.Errorf("prometheus alerts returned status %q", response.Status)
		observability.Default().ObservePrometheusRuntimeRequest("alerts", classifyPrometheusRequestResult(err), time.Since(startedAt))
		return nil, err
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

	observability.Default().ObservePrometheusRuntimeRequest("alerts", classifyPrometheusRequestResult(nil), time.Since(startedAt))
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
	ctx, cancel := c.withRuntimeTimeout(ctx)
	defer cancel()

	startedAt := time.Now()
	endpoint, err := url.Parse(c.baseURL + "/api/v1/query")
	if err != nil {
		observability.Default().ObservePrometheusRuntimeRequest("query", classifyPrometheusRequestResult(err), time.Since(startedAt))
		return nil, fmt.Errorf("parse prometheus URL: %w", err)
	}

	values := endpoint.Query()
	values.Set("query", promql)
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		observability.Default().ObservePrometheusRuntimeRequest("query", classifyPrometheusRequestResult(err), time.Since(startedAt))
		return nil, fmt.Errorf("build query request: %w", err)
	}

	var response promQueryResponse
	if err := c.do(req, &response); err != nil {
		observability.Default().ObservePrometheusRuntimeRequest("query", classifyPrometheusRequestResult(err), time.Since(startedAt))
		return nil, err
	}
	if response.Status != "success" {
		err := fmt.Errorf("prometheus query returned status %q", response.Status)
		observability.Default().ObservePrometheusRuntimeRequest("query", classifyPrometheusRequestResult(err), time.Since(startedAt))
		return nil, err
	}
	if response.Data.ResultType != "vector" {
		err := fmt.Errorf("unexpected prometheus result type %q", response.Data.ResultType)
		observability.Default().ObservePrometheusRuntimeRequest("query", classifyPrometheusRequestResult(err), time.Since(startedAt))
		return nil, err
	}

	observability.Default().ObservePrometheusRuntimeRequest("query", classifyPrometheusRequestResult(nil), time.Since(startedAt))
	return response.Data.Result, nil
}

func (c *PromClient) withRuntimeTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, c.runtimeOperationTimeout())
}

func (c *PromClient) runtimeOperationTimeout() time.Duration {
	if c != nil && c.operationTimeout > 0 {
		return c.operationTimeout
	}
	return defaultPrometheusOperationTimeout
}

func classifyPrometheusRequestResult(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	return "error"
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
