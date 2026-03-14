package worker

import (
	"context"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/azmin/mikrotik-theia/internal/metrics"
	"github.com/azmin/mikrotik-theia/internal/ws"
	"github.com/google/uuid"
)

// SNMPPollFunc polls a single device via SNMP for live CPU/MEM/UPTIME/TEMP metrics.
// It is used as a fallback when Prometheus returns no data for a device.
type SNMPPollFunc func(target string, creds domain.SNMPCredentials) (domain.DeviceMetrics, error)

// MetricsCollector queries Prometheus on a cadence and pushes full-state snapshots over WebSockets.
type MetricsCollector struct {
	promClient   *metrics.PromClient
	hub          *ws.Hub
	deviceRepo   domain.DeviceRepository
	linkRepo     domain.LinkRepository
	settingsRepo domain.SettingsRepository
	snmpPollFunc SNMPPollFunc // optional; polls SNMP-sourced devices and fallback devices

	mu              sync.RWMutex
	lastSnapshot    *ws.SnapshotPayload
	promAvailable   bool // last known Prometheus availability
	cancel          context.CancelFunc
	done            chan struct{}
}

// NewMetricsCollector creates a background collector for live Prometheus data.
// snmpPollFunc may be nil; when provided it is used as a fallback for devices
// that Prometheus has no metrics for.
func NewMetricsCollector(
	promClient *metrics.PromClient,
	hub *ws.Hub,
	deviceRepo domain.DeviceRepository,
	linkRepo domain.LinkRepository,
	settingsRepo domain.SettingsRepository,
	snmpPollFunc SNMPPollFunc,
) *MetricsCollector {
	return &MetricsCollector{
		promClient:    promClient,
		hub:           hub,
		deviceRepo:    deviceRepo,
		linkRepo:      linkRepo,
		settingsRepo:  settingsRepo,
		snmpPollFunc:  snmpPollFunc,
		lastSnapshot:  ws.EmptySnapshot(),
		promAvailable: true, // assume available until proven otherwise
		done:          make(chan struct{}),
	}
}

// Start begins the periodic collection loop.
func (c *MetricsCollector) Start(ctx context.Context) {
	collectCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})

	go func() {
		defer close(c.done)

		c.collectAndBroadcast(collectCtx)

		for {
			interval := c.getPollingInterval()
			select {
			case <-collectCtx.Done():
				log.Println("Metrics collector shutting down")
				return
			case <-time.After(interval):
				c.collectAndBroadcast(collectCtx)
			}
		}
	}()

	log.Println("Metrics collector started")
}

// Stop terminates the background collection loop.
func (c *MetricsCollector) Stop() {
	if c.cancel != nil {
		c.cancel()
		<-c.done
	}
	log.Println("Metrics collector stopped")
}

// GetSnapshot returns a copy of the most recently collected snapshot.
func (c *MetricsCollector) GetSnapshot() *ws.SnapshotPayload {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ws.CloneSnapshot(c.lastSnapshot)
}

func (c *MetricsCollector) collectAndBroadcast(ctx context.Context) {
	snapshot, promNowAvailable, promErr := c.buildSnapshot(ctx)

	c.mu.Lock()
	c.lastSnapshot = snapshot
	prevAvailable := c.promAvailable
	c.promAvailable = promNowAvailable
	c.mu.Unlock()

	c.hub.Broadcast(ws.Message{
		Type:    ws.MessageTypeSnapshot,
		Payload: snapshot,
	})

	// Notify clients when Prometheus availability changes.
	if prevAvailable != promNowAvailable {
		payload := ws.PrometheusStatusPayload{Available: promNowAvailable}
		if !promNowAvailable && promErr != "" {
			payload.Error = promErr
		}
		c.hub.Broadcast(ws.Message{
			Type:    ws.MessageTypePrometheusStatus,
			Payload: payload,
		})
	}
}

func (c *MetricsCollector) buildSnapshot(ctx context.Context) (*ws.SnapshotPayload, bool, string) {
	snapshot := ws.EmptySnapshot()

	devices, err := c.deviceRepo.GetAll()
	if err != nil {
		log.Printf("Metrics collector: failed to load devices: %v", err)
		return snapshot, true, ""
	}

	links, err := c.linkRepo.GetAll()
	if err != nil {
		log.Printf("Metrics collector: failed to load links: %v", err)
		links = nil
	}

	// Group Prometheus-sourced devices by label name so we can batch queries per label type.
	// Both MetricsSourcePrometheus and MetricsSourcePrometheusSNMPFallback are queried via Prometheus.
	type promGroup struct {
		labelValues    []string
		ipByLabelValue map[string]string // label value → device IP (for link metrics translation)
	}
	promGroups := make(map[string]*promGroup) // keyed by labelName

	for _, device := range devices {
		src := device.MetricsSource
		if src == "" {
			src = domain.MetricsSourcePrometheus
		}
		if src != domain.MetricsSourcePrometheus && src != domain.MetricsSourcePrometheusSNMPFallback {
			continue
		}

		labelName := device.PrometheusLabelName
		if labelName == "" {
			labelName = "instance"
		}
		labelValue := device.PrometheusLabelValue
		if labelValue == "" {
			labelValue = device.IP
		}
		if labelValue == "" {
			continue
		}

		if promGroups[labelName] == nil {
			promGroups[labelName] = &promGroup{ipByLabelValue: make(map[string]string)}
		}
		promGroups[labelName].labelValues = append(promGroups[labelName].labelValues, labelValue)
		promGroups[labelName].ipByLabelValue[labelValue] = device.IP
	}

	// Query Prometheus for each label group and merge results.
	// Track errors to determine Prometheus availability.
	metricsByLabelValue := make(map[string]domain.DeviceMetrics)
	linkMetricsByLabelValue := make(map[string][]domain.LinkMetrics)
	hostnamesByLabelValue := make(map[string]string)
	alertStates := []domain.AlertState{}

	promQueryErrors := 0
	promQueryTotal := 0
	var firstPromError string

	for labelName, group := range promGroups {
		promQueryTotal++
		if m, err := c.promClient.QueryDeviceMetrics(ctx, labelName, group.labelValues); err != nil {
			log.Printf("Metrics collector: failed to query device metrics (label=%s): %v", labelName, err)
			promQueryErrors++
			if firstPromError == "" {
				firstPromError = err.Error()
			}
		} else {
			for k, v := range m {
				metricsByLabelValue[k] = v
			}
		}

		if m, err := c.promClient.QueryLinkMetrics(ctx, labelName, group.labelValues); err != nil {
			log.Printf("Metrics collector: failed to query link metrics (label=%s): %v", labelName, err)
		} else {
			for k, v := range m {
				linkMetricsByLabelValue[k] = v
			}
		}

		// Hostname query is best-effort: missing sysName metric is not an error.
		if names, err := c.promClient.QueryHostnames(ctx, labelName, group.labelValues); err != nil {
			log.Printf("Metrics collector: failed to query sysName (label=%s): %v", labelName, err)
		} else {
			for k, v := range names {
				hostnamesByLabelValue[k] = v
			}
		}
	}

	// Prometheus is considered available if all queries succeeded (or no devices use Prometheus).
	promNowAvailable := promQueryTotal == 0 || promQueryErrors == 0

	// Alerts use the "instance" label (device IP) regardless of the configured label name.
	if len(promGroups) > 0 && promNowAvailable {
		if alerts, err := c.promClient.QueryAlerts(ctx); err != nil {
			log.Printf("Metrics collector: failed to query alerts: %v", err)
		} else {
			alertStates = alerts
		}
	}

	// Helper: resolve the Prometheus label value for a device (covers both prometheus source modes).
	effectiveLabelValue := func(d domain.Device) string {
		src := d.MetricsSource
		if src == "" {
			src = domain.MetricsSourcePrometheus
		}
		if src != domain.MetricsSourcePrometheus && src != domain.MetricsSourcePrometheusSNMPFallback {
			return ""
		}
		if d.PrometheusLabelValue != "" {
			return d.PrometheusLabelValue
		}
		return d.IP
	}

	deviceMetricsByID := attachDeviceMetrics(devices, metricsByLabelValue, effectiveLabelValue)

	// Poll SNMP for:
	//   - MetricsSourceSNMP devices: always
	//   - MetricsSourcePrometheusSNMPFallback devices: when Prometheus is unavailable or returned no data
	// MetricsSourcePrometheus devices do NOT fall back to SNMP automatically.
	if c.snmpPollFunc != nil {
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, dev := range devices {
			src := dev.MetricsSource
			if src == "" {
				src = domain.MetricsSourcePrometheus
			}

			shouldPoll := false
			switch src {
			case domain.MetricsSourceSNMP:
				shouldPoll = true
			case domain.MetricsSourcePrometheusSNMPFallback:
				existing := deviceMetricsByID[dev.ID.String()]
				if !promNowAvailable || (existing.CPUPercent == nil && existing.MemPercent == nil && existing.UptimeSecs == nil) {
					shouldPoll = true
				}
			}

			if !shouldPoll {
				continue
			}

			dev := dev
			wg.Add(1)
			go func() {
				defer wg.Done()
				m, err := c.snmpPollFunc(dev.IP, dev.SNMPCredentials)
				if err != nil {
					log.Printf("SNMP metrics poll failed for %s: %v", dev.IP, err)
					return
				}
				m.DeviceID = dev.ID
				m.CollectedAt = time.Now().UTC()
				mu.Lock()
				deviceMetricsByID[dev.ID.String()] = m
				mu.Unlock()
			}()
		}
		wg.Wait()
	}

	// Translate link metrics from label-value keying back to device-IP keying for attachLinkMetrics.
	linkMetricsByIP := make(map[string][]domain.LinkMetrics)
	for _, device := range devices {
		lv := effectiveLabelValue(device)
		if lv == "" {
			continue
		}
		if metrics, ok := linkMetricsByLabelValue[lv]; ok {
			linkMetricsByIP[device.IP] = metrics
		}
	}

	linkMetricsByID := attachLinkMetrics(devices, links, linkMetricsByIP)
	alertsByDevice := attachAlerts(devices, alertStates)

	snapshot.DeviceMetrics = ws.DeviceMetricsToDTOs(deviceMetricsByID)
	snapshot.LinkMetrics = ws.LinkMetricsToDTOs(linkMetricsByID)
	snapshot.Alerts = ws.AlertsToDTOs(alertsByDevice)

	// Map Prometheus label-value hostnames → device IDs.
	hostnames := make(map[string]string, len(devices))
	for _, dev := range devices {
		lv := effectiveLabelValue(dev)
		if lv == "" {
			continue
		}
		if name, ok := hostnamesByLabelValue[lv]; ok {
			hostnames[dev.ID.String()] = name
		}
	}
	snapshot.DeviceHostnames = hostnames

	statuses := make(map[string]string, len(devices))
	for _, dev := range devices {
		statuses[dev.ID.String()] = string(dev.Status)
	}

	// Override device status with blackbox_exporter probe_success when Prometheus is
	// the metrics source and probe data is available. This replaces the SNMP-based
	// status for devices where probe_success{instance=~"<ip>"} exists in Prometheus.
	if promNowAvailable && len(promGroups) > 0 {
		promIPs := make([]string, 0, len(devices))
		promDeviceByIP := make(map[string]domain.Device, len(devices))
		for _, dev := range devices {
			src := dev.MetricsSource
			if src == "" {
				src = domain.MetricsSourcePrometheus
			}
			if src != domain.MetricsSourcePrometheus && src != domain.MetricsSourcePrometheusSNMPFallback {
				continue
			}
			if dev.IP == "" {
				continue
			}
			promIPs = append(promIPs, dev.IP)
			promDeviceByIP[dev.IP] = dev
		}

		if probeStatuses, err := c.promClient.QueryProbeStatus(ctx, promIPs); err != nil {
			log.Printf("Metrics collector: failed to query probe_success: %v", err)
		} else {
			for ip, isUp := range probeStatuses {
				dev, ok := promDeviceByIP[ip]
				if !ok {
					continue
				}
				if isUp {
					statuses[dev.ID.String()] = string(domain.DeviceStatusUp)
				} else {
					statuses[dev.ID.String()] = string(domain.DeviceStatusDown)
				}
			}
		}
	}

	snapshot.DeviceStatuses = statuses

	return snapshot, promNowAvailable, firstPromError
}

func attachDeviceMetrics(
	devices []domain.Device,
	metricsByLabel map[string]domain.DeviceMetrics,
	labelFor func(domain.Device) string,
) map[string]domain.DeviceMetrics {
	collectedAt := time.Now().UTC()
	metricsByID := make(map[string]domain.DeviceMetrics, len(devices))

	for _, device := range devices {
		var metric domain.DeviceMetrics
		if lv := labelFor(device); lv != "" {
			var ok bool
			metric, ok = metricsByLabel[lv]
			if !ok {
				metric = domain.DeviceMetrics{CollectedAt: collectedAt}
			}
		} else {
			metric = domain.DeviceMetrics{CollectedAt: collectedAt}
		}
		if metric.CollectedAt.IsZero() {
			metric.CollectedAt = collectedAt
		}
		metric.DeviceID = device.ID
		metricsByID[device.ID.String()] = metric
	}

	return metricsByID
}

func attachLinkMetrics(
	devices []domain.Device,
	links []domain.Link,
	metricsByIP map[string][]domain.LinkMetrics,
) map[string][]domain.LinkMetrics {
	metricsByID := make(map[string][]domain.LinkMetrics, len(devices))
	linksByDevice := make(map[uuid.UUID][]domain.Link)

	for _, link := range links {
		linksByDevice[link.SourceDeviceID] = append(linksByDevice[link.SourceDeviceID], link)
		linksByDevice[link.TargetDeviceID] = append(linksByDevice[link.TargetDeviceID], link)
	}

	for _, device := range devices {
		deviceKey := device.ID.String()
		metricsByID[deviceKey] = []domain.LinkMetrics{}

		for _, metric := range metricsByIP[device.IP] {
			linkID := matchLinkID(device, linksByDevice[device.ID], metric.IfName)
			if linkID == "" {
				continue
			}

			metric.DeviceID = device.ID
			metric.LinkID = linkID
			if metric.CollectedAt.IsZero() {
				metric.CollectedAt = time.Now().UTC()
			}

			if utilization := computeUtilization(device, metric.IfName, metric); utilization != nil {
				metric.Utilization = utilization
			}

			metricsByID[deviceKey] = append(metricsByID[deviceKey], metric)
		}

		sort.Slice(metricsByID[deviceKey], func(i, j int) bool {
			if metricsByID[deviceKey][i].IfName != metricsByID[deviceKey][j].IfName {
				return metricsByID[deviceKey][i].IfName < metricsByID[deviceKey][j].IfName
			}
			return metricsByID[deviceKey][i].LinkID < metricsByID[deviceKey][j].LinkID
		})
	}

	return metricsByID
}

func attachAlerts(devices []domain.Device, alerts []domain.AlertState) []domain.AlertState {
	deviceByIP := make(map[string]domain.Device, len(devices))
	for _, device := range devices {
		deviceByIP[device.IP] = device
	}

	mapped := make([]domain.AlertState, 0, len(alerts))
	for _, alert := range alerts {
		device, ok := deviceByIP[alert.Instance]
		if !ok {
			continue
		}
		alert.DeviceID = device.ID
		mapped = append(mapped, alert)
	}

	sort.Slice(mapped, func(i, j int) bool {
		if mapped[i].DeviceID != mapped[j].DeviceID {
			return mapped[i].DeviceID.String() < mapped[j].DeviceID.String()
		}
		if mapped[i].Severity != mapped[j].Severity {
			return mapped[i].Severity < mapped[j].Severity
		}
		return mapped[i].AlertName < mapped[j].AlertName
	})

	return mapped
}

func matchLinkID(device domain.Device, links []domain.Link, metricIfName string) string {
	for _, link := range links {
		switch {
		case link.SourceDeviceID == device.ID && sameInterface(device, metricIfName, link.SourceIfName):
			return link.ID.String()
		case link.TargetDeviceID == device.ID && sameInterface(device, metricIfName, link.TargetIfName):
			return link.ID.String()
		}
	}
	return ""
}

func sameInterface(device domain.Device, observedIfName, linkIfName string) bool {
	observed := normalizeInterfaceName(observedIfName)
	linkName := normalizeInterfaceName(linkIfName)
	if observed == "" || linkName == "" {
		return false
	}
	if observed == linkName {
		return true
	}

	for _, iface := range device.Interfaces {
		ifaceName := normalizeInterfaceName(iface.IfName)
		ifaceDescr := normalizeInterfaceName(iface.IfDescr)

		if (observed == ifaceName || observed == ifaceDescr) &&
			(linkName == ifaceName || linkName == ifaceDescr) {
			return true
		}
	}

	return false
}

func computeUtilization(device domain.Device, observedIfName string, metric domain.LinkMetrics) *float64 {
	speed := interfaceSpeed(device, observedIfName)
	if speed <= 0 {
		return metric.Utilization
	}

	var (
		maxRate float64
		hasRate bool
	)

	if metric.TxBps != nil {
		maxRate = *metric.TxBps
		hasRate = true
	}
	if metric.RxBps != nil && (!hasRate || *metric.RxBps > maxRate) {
		maxRate = *metric.RxBps
		hasRate = true
	}

	if !hasRate {
		return nil
	}

	utilization := maxRate / float64(speed)
	utilization = math.Max(0, utilization)
	return &utilization
}

func interfaceSpeed(device domain.Device, observedIfName string) int64 {
	observed := normalizeInterfaceName(observedIfName)
	for _, iface := range device.Interfaces {
		if observed == normalizeInterfaceName(iface.IfName) || observed == normalizeInterfaceName(iface.IfDescr) {
			return iface.Speed
		}
	}
	return 0
}

func normalizeInterfaceName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (c *MetricsCollector) getPollingInterval() time.Duration {
	val, err := c.settingsRepo.Get(domain.SettingPollingInterval)
	if err != nil {
		return 60 * time.Second
	}
	seconds, err := strconv.Atoi(val)
	if err != nil || seconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(seconds) * time.Second
}
