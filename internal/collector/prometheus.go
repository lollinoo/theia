package collector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/metrics"
)

// PrometheusEnrichmentClient is the collector-facing Prometheus client contract.
// It intentionally exposes enrichment-only queries so SNMP stays authoritative
// for core metrics and interface throughput.
type PrometheusEnrichmentClient interface {
	QueryHostnames(ctx context.Context, labelName string, labelValues []string) (map[string]string, error)
	QueryProbeStatus(ctx context.Context, deviceIPs []string) (map[string]bool, error)
	QueryAlerts(ctx context.Context) ([]domain.AlertState, error)
}

// PrometheusCollector queries Prometheus for non-authoritative enrichment data.
type PrometheusCollector struct {
	mu        sync.RWMutex
	client    PrometheusEnrichmentClient
	factory   func(string) PrometheusEnrichmentClient
	now       func() time.Time
	swappable bool
}

// NewPrometheusCollector constructs a stateless Prometheus enrichment collector.
func NewPrometheusCollector(client PrometheusEnrichmentClient) *PrometheusCollector {
	_, swappable := client.(*metrics.PromClient)
	if client == nil {
		swappable = true
	}

	var factory func(string) PrometheusEnrichmentClient
	if promClient, ok := client.(*metrics.PromClient); ok {
		factory = func(baseURL string) PrometheusEnrichmentClient {
			return promClient.WithBaseURL(baseURL)
		}
	}
	return &PrometheusCollector{
		client:    client,
		factory:   factory,
		now:       time.Now,
		swappable: swappable,
	}
}

// SetClient swaps the underlying Prometheus client at runtime.
func (c *PrometheusCollector) SetClient(client PrometheusEnrichmentClient) {
	if c == nil {
		return
	}

	c.mu.Lock()
	if !c.swappable && client != nil {
		c.mu.Unlock()
		return
	}
	c.client = client
	if promClient, ok := client.(*metrics.PromClient); ok {
		c.factory = func(baseURL string) PrometheusEnrichmentClient {
			return promClient.WithBaseURL(baseURL)
		}
	}
	c.mu.Unlock()
}

// SetClientFactory overrides how runtime Prometheus URL swaps create clients.
func (c *PrometheusCollector) SetClientFactory(factory func(string) PrometheusEnrichmentClient) {
	if c == nil || factory == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.swappable {
		return
	}
	c.factory = factory
}

// SetPrometheusURL swaps the Prometheus client while preserving its shared HTTP config.
func (c *PrometheusCollector) SetPrometheusURL(baseURL string) {
	if c == nil {
		return
	}

	baseURL = strings.TrimSpace(baseURL)

	c.mu.Lock()
	defer c.mu.Unlock()

	if baseURL == "" {
		c.client = nil
		return
	}
	if !c.swappable {
		return
	}
	if c.factory == nil {
		c.factory = func(url string) PrometheusEnrichmentClient {
			return metrics.NewPromClient(url, &http.Client{Timeout: 4 * time.Second})
		}
	}
	c.client = c.factory(baseURL)
}

// Enabled reports whether Prometheus queries are currently configured.
func (c *PrometheusCollector) Enabled() bool {
	return c.currentClient() != nil
}

// ResolvePrometheusLabel returns a safe Prometheus label selector target for a
// device. A full explicit label pair is preferred; otherwise the device IP is
// used as the `instance` label fallback.
func ResolvePrometheusLabel(device domain.Device) (labelName string, labelValue string, ok bool) {
	explicitName := strings.TrimSpace(device.PrometheusLabelName)
	explicitValue := strings.TrimSpace(device.PrometheusLabelValue)
	if explicitName != "" && explicitValue != "" {
		return explicitName, explicitValue, true
	}

	ip := strings.TrimSpace(device.IP)
	if ip == "" {
		return "", "", false
	}

	return "instance", ip, true
}

// CollectDeviceEnrichment fetches hostname and probe reachability for one
// device without querying Prometheus for core device or link metrics.
func (c *PrometheusCollector) CollectDeviceEnrichment(ctx context.Context, device domain.Device) (PrometheusEnrichment, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	result := PrometheusEnrichment{
		DeviceID:    device.ID,
		CollectedAt: collectorNowUTC(c),
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}
	if c == nil {
		return result, errors.New("prometheus collector is nil")
	}
	client := c.currentClient()
	if client == nil {
		return result, errors.New("prometheus collector client is nil")
	}

	if labelName, labelValue, ok := ResolvePrometheusLabel(device); ok {
		hostnames, err := client.QueryHostnames(ctx, labelName, []string{labelValue})
		if err != nil {
			return result, fmt.Errorf("query hostnames: %w", err)
		}
		result.Hostname = hostnames[labelValue]
	}

	if ip := strings.TrimSpace(device.IP); ip != "" {
		probeStatuses, err := client.QueryProbeStatus(ctx, []string{ip})
		if err != nil {
			return result, fmt.Errorf("query probe status: %w", err)
		}
		if reachable, ok := probeStatuses[ip]; ok {
			result.ProbeReachable = boolPtr(reachable)
		}
	}

	return result, nil
}

// CollectAlerts fetches the current Prometheus alert set and groups it by
// device for downstream collector consumers.
func (c *PrometheusCollector) CollectAlerts(ctx context.Context, devices []domain.Device) (map[uuid.UUID][]domain.AlertState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("prometheus collector is nil")
	}
	client := c.currentClient()
	if client == nil {
		return nil, errors.New("prometheus collector client is nil")
	}

	alerts, err := client.QueryAlerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("query alerts: %w", err)
	}

	return MapAlertsToDevices(devices, alerts), nil
}

// MapAlertsToDevices maps Prometheus alert instances back to known devices and
// groups the results by device ID. Alerts are ordered deterministically within
// each device group by severity, alert name, and summary.
func MapAlertsToDevices(devices []domain.Device, alerts []domain.AlertState) map[uuid.UUID][]domain.AlertState {
	grouped := make(map[uuid.UUID][]domain.AlertState)
	if len(devices) == 0 || len(alerts) == 0 {
		return grouped
	}

	sortedDevices := append([]domain.Device(nil), devices...)
	sort.Slice(sortedDevices, func(i, j int) bool {
		return sortedDevices[i].ID.String() < sortedDevices[j].ID.String()
	})

	deviceIDsByInstance := make(map[string]uuid.UUID, len(sortedDevices)*2)
	for _, device := range sortedDevices {
		if ip := strings.TrimSpace(device.IP); ip != "" {
			deviceIDsByInstance[ip] = device.ID
		}

		labelName, labelValue, ok := ResolvePrometheusLabel(device)
		if ok && labelName == "instance" {
			deviceIDsByInstance[labelValue] = device.ID
		}
	}

	for _, alert := range alerts {
		instance := strings.TrimSpace(alert.Instance)
		if instance == "" {
			continue
		}

		deviceID, ok := deviceIDsByInstance[instance]
		if !ok {
			continue
		}

		mapped := alert
		mapped.DeviceID = deviceID
		grouped[deviceID] = append(grouped[deviceID], mapped)
	}

	for deviceID := range grouped {
		sort.Slice(grouped[deviceID], func(i, j int) bool {
			if grouped[deviceID][i].Severity != grouped[deviceID][j].Severity {
				return grouped[deviceID][i].Severity < grouped[deviceID][j].Severity
			}
			if grouped[deviceID][i].AlertName != grouped[deviceID][j].AlertName {
				return grouped[deviceID][i].AlertName < grouped[deviceID][j].AlertName
			}
			return grouped[deviceID][i].Summary < grouped[deviceID][j].Summary
		})
	}

	return grouped
}

func collectorNowUTC(c *PrometheusCollector) time.Time {
	if c != nil && c.now != nil {
		return c.now().UTC()
	}

	return time.Now().UTC()
}

func (c *PrometheusCollector) currentClient() PrometheusEnrichmentClient {
	if c == nil {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

func boolPtr(value bool) *bool {
	cloned := value
	return &cloned
}
