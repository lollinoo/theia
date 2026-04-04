package worker

import (
	"context"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/metrics"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/ws"
	"github.com/google/uuid"
)

// SNMPPollFunc polls a single device via SNMP for live CPU/MEM/UPTIME/TEMP metrics.
// It is used as a fallback when Prometheus returns no data for a device.
// vendorName is used to resolve vendor-specific SNMP OIDs.
type SNMPPollFunc func(target string, creds domain.SNMPCredentials, vendorName string) (domain.DeviceMetrics, error)

// SNMPLinkPollFunc polls a single device via SNMP for interface octet counters
// (ifHCInOctets / ifHCOutOctets). Returns raw counter snapshots that the
// MetricsCollector converts to rates by comparing successive polls.
type SNMPLinkPollFunc func(target string, creds domain.SNMPCredentials) ([]SNMPIfCounter, error)

// SNMPIfCounter holds a single poll's raw 64-bit counter values for one interface.
type SNMPIfCounter struct {
	IfName    string
	InOctets  uint64
	OutOctets uint64
}

// snmpCounterSample stores a previous counter reading for rate computation.
type snmpCounterSample struct {
	InOctets  uint64
	OutOctets uint64
	Time      time.Time
}

// MetricsCollector queries Prometheus on a cadence and pushes full-state snapshots over WebSockets.
type MetricsCollector struct {
	promClient       *metrics.PromClient
	hub              *ws.Hub
	cache            *cache.DeviceLinkCache
	deviceRepo       domain.DeviceRepository // kept for Update() writes
	settingsRepo     domain.SettingsRepository
	vendorRegistry   *vendor.Registry
	snmpPollFunc     SNMPPollFunc     // optional; polls SNMP-sourced devices and fallback devices
	snmpLinkPollFunc SNMPLinkPollFunc // optional; polls interface counters for SNMP link metrics

	mu              sync.RWMutex
	lastSnapshot    *ws.SnapshotPayload
	promAvailable   bool // last known Prometheus availability
	cancel          context.CancelFunc
	done            chan struct{}
	healthDone      chan struct{} // closed when health check goroutine exits

	// prevCounters stores the previous SNMP counter sample per device IP + interface name,
	// used to compute byte rates between successive polls.
	prevCountersMu sync.Mutex
	prevCounters   map[string]map[string]snmpCounterSample // deviceIP → ifName → sample
}

// NewMetricsCollector creates a background collector for live Prometheus data.
// snmpPollFunc may be nil; when provided it is used as a fallback for devices
// that Prometheus has no metrics for.
// snmpLinkPollFunc may be nil; when provided it polls interface counters for
// SNMP-sourced devices to produce link throughput metrics.
func NewMetricsCollector(
	promClient *metrics.PromClient,
	hub *ws.Hub,
	cache *cache.DeviceLinkCache,
	deviceRepo domain.DeviceRepository,
	settingsRepo domain.SettingsRepository,
	vendorRegistry *vendor.Registry,
	snmpPollFunc SNMPPollFunc,
	snmpLinkPollFunc SNMPLinkPollFunc,
) *MetricsCollector {
	return &MetricsCollector{
		promClient:       promClient,
		hub:              hub,
		cache:            cache,
		deviceRepo:       deviceRepo,
		settingsRepo:     settingsRepo,
		vendorRegistry:   vendorRegistry,
		snmpPollFunc:     snmpPollFunc,
		snmpLinkPollFunc: snmpLinkPollFunc,
		lastSnapshot:     ws.EmptySnapshot(),
		promAvailable:    true, // assume available until proven otherwise
		prevCounters:     make(map[string]map[string]snmpCounterSample),
		done:           make(chan struct{}),
	}
}

// Start begins the periodic collection loop.
func (c *MetricsCollector) Start(ctx context.Context) {
	collectCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	c.healthDone = make(chan struct{})

	// Background health check: probe Prometheus every 5s with a fast timeout.
	// Broadcasts prometheus_status immediately on transitions.
	go func() {
		defer close(c.healthDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-collectCtx.Done():
				return
			case <-ticker.C:
				c.refreshPromClient()
				client := c.getPromClient()
				err := client.CheckHealthFast(collectCtx)
				if collectCtx.Err() != nil {
					return
				}
				nowAvailable := err == nil

				c.mu.Lock()
				changed := c.promAvailable != nowAvailable
				c.promAvailable = nowAvailable
				c.mu.Unlock()

				if changed {
					status := "available"
					if !nowAvailable {
						status = "unreachable"
					}
					log.Printf("Prometheus health check: %s", status)

					payload := ws.PrometheusStatusPayload{Available: nowAvailable}
					if !nowAvailable && err != nil {
						payload.Error = err.Error()
					}
					c.hub.Broadcast(ws.Message{
						Type:    ws.MessageTypePrometheusStatus,
						Payload: payload,
					})
				}
			}
		}
	}()

	// Run first collection asynchronously. The lastSnapshot is initialized to
	// EmptySnapshot() in the constructor, so WebSocket clients connecting before
	// the first collection completes will receive an empty (but valid) snapshot.
	// Previously this ran synchronously, which blocked the HTTP server from
	// starting for minutes when SNMP devices were unreachable.
	go func() {
		defer close(c.done)

		// Collect immediately on startup, then on the configured interval.
		c.collectAndBroadcast(collectCtx)

		for {
			interval := GetPollingInterval(c.settingsRepo)
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
		<-c.healthDone
	}
	log.Println("Metrics collector stopped")
}

// GetSnapshot returns a copy of the most recently collected snapshot.
func (c *MetricsCollector) GetSnapshot() *ws.SnapshotPayload {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ws.CloneSnapshot(c.lastSnapshot)
}

// IsPromAvailable returns the last known Prometheus health status.
func (c *MetricsCollector) IsPromAvailable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.promAvailable
}

// isPromAvailable reads the last known Prometheus health status (set by the health check goroutine).
func (c *MetricsCollector) isPromAvailable() bool {
	return c.IsPromAvailable()
}

// getPromClient returns the current PromClient, safe for concurrent use.
func (c *MetricsCollector) getPromClient() *metrics.PromClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.promClient
}

// refreshPromClient re-reads the Prometheus URL from settings and replaces
// the PromClient when the URL has changed. This ensures that runtime URL
// changes made via the settings API take effect without restarting.
func (c *MetricsCollector) refreshPromClient() {
	newURL, err := c.settingsRepo.Get(domain.SettingPrometheusURL)
	if err != nil || newURL == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.promClient.BaseURL() != strings.TrimRight(newURL, "/") {
		log.Printf("Prometheus URL changed to %s, recreating client", newURL)
		c.promClient = metrics.NewPromClient(newURL, nil)
	}
}

func (c *MetricsCollector) collectAndBroadcast(ctx context.Context) {
	c.refreshPromClient()

	// Apply a timeout to the entire collection cycle so that unreachable
	// Prometheus doesn't block snapshot broadcasts indefinitely.
	collectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	snapshot, promNowAvailable, promErr := c.buildSnapshot(collectCtx)

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
	// The health check goroutine handles most transitions, but this catches
	// cases where a query succeeds/fails within the collection cycle.
	if prevAvailable != promNowAvailable {
		if promNowAvailable {
			log.Println("Prometheus is now reachable")
		} else {
			log.Printf("Prometheus is unreachable: %s", promErr)
		}
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
	promClient := c.getPromClient()
	snapshot := ws.EmptySnapshot()

	devices, err := c.cache.GetDevices()
	if err != nil {
		log.Printf("Metrics collector: failed to load devices: %v", err)
		return snapshot, true, ""
	}

	links, err := c.cache.GetLinks()
	if err != nil {
		log.Printf("Metrics collector: failed to load links: %v", err)
		links = nil
	}

	// Group Prometheus-sourced devices by label name + vendor so we can batch queries
	// per label type using vendor-resolved PromQL templates.
	// Both MetricsSourcePrometheus and MetricsSourcePrometheusSNMPFallback are queried via Prometheus.
	type promGroupKey struct {
		labelName  string
		vendorName string
	}
	type promGroup struct {
		labelValues    []string
		ipByLabelValue map[string]string // label value → device IP (for link metrics translation)
		promMetrics    vendor.PrometheusMetrics
	}
	promGroups := make(map[promGroupKey]*promGroup)

	// Also keep a flat set of all labelName→labelValues for link/hostname/interface queries
	// (those are vendor-agnostic).
	allPromLabelGroups := make(map[string]*promGroup)

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

		vendorName := device.Vendor
		if vendorName == "" {
			vendorName = "default"
		}

		key := promGroupKey{labelName: labelName, vendorName: vendorName}
		if promGroups[key] == nil {
			promGroups[key] = &promGroup{
				ipByLabelValue: make(map[string]string),
				promMetrics:    c.vendorRegistry.ResolvePrometheusMetrics(vendorName),
			}
		}
		promGroups[key].labelValues = append(promGroups[key].labelValues, labelValue)
		promGroups[key].ipByLabelValue[labelValue] = device.IP

		// Also add to the flat label group for vendor-agnostic queries
		if allPromLabelGroups[labelName] == nil {
			allPromLabelGroups[labelName] = &promGroup{ipByLabelValue: make(map[string]string)}
		}
		allPromLabelGroups[labelName].labelValues = append(allPromLabelGroups[labelName].labelValues, labelValue)
		allPromLabelGroups[labelName].ipByLabelValue[labelValue] = device.IP
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

	// Skip all Prometheus queries when the health check already knows it's down.
	// The health check goroutine will notify clients when it comes back.
	promKnownDown := len(allPromLabelGroups) > 0 && !c.isPromAvailable()
	if promKnownDown {
		promQueryTotal = 1
		promQueryErrors = 1
		firstPromError = "prometheus unreachable (health check)"
		log.Printf("Metrics collector: skipping Prometheus queries (health check reports unreachable)")
	}

	// Query device metrics per vendor group (vendor-specific PromQL templates).
	for key, group := range promGroups {
		if promKnownDown {
			break
		}

		// Bail out early if the collection context has expired or a previous query failed.
		if ctx.Err() != nil || promQueryErrors > 0 {
			if promQueryErrors == 0 {
				promQueryErrors++
				promQueryTotal++
				if firstPromError == "" {
					firstPromError = ctx.Err().Error()
				}
			}
			break
		}

		promQueryTotal++
		if m, err := promClient.QueryDeviceMetrics(ctx, key.labelName, group.labelValues, group.promMetrics); err != nil {
			log.Printf("Metrics collector: failed to query device metrics (label=%s, vendor=%s): %v", key.labelName, key.vendorName, err)
			promQueryErrors++
			if firstPromError == "" {
				firstPromError = err.Error()
			}
			break // fail-fast: skip remaining groups
		} else {
			for k, v := range m {
				metricsByLabelValue[k] = v
			}
		}
	}

	// Query link metrics and hostnames per label group (vendor-agnostic).
	for labelName, group := range allPromLabelGroups {
		if promKnownDown || ctx.Err() != nil || promQueryErrors > 0 {
			break
		}

		if m, err := promClient.QueryLinkMetrics(ctx, labelName, group.labelValues); err != nil {
			log.Printf("Metrics collector: failed to query link metrics (label=%s): %v", labelName, err)
			promQueryErrors++
			if firstPromError == "" {
				firstPromError = err.Error()
			}
			break // fail-fast: skip remaining groups
		} else {
			for k, v := range m {
				linkMetricsByLabelValue[k] = v
			}
		}

		if ctx.Err() != nil {
			break
		}
		// Hostname query is best-effort: missing sysName metric is not an error.
		if names, err := promClient.QueryHostnames(ctx, labelName, group.labelValues); err != nil {
			log.Printf("Metrics collector: failed to query sysName (label=%s): %v", labelName, err)
		} else {
			for k, v := range names {
				hostnamesByLabelValue[k] = v
			}
		}
	}

	// Discover interfaces from Prometheus for devices that have no SNMP-discovered interfaces.
	// Build a filtered set of label values for devices that need interface discovery.
	type ifaceNeedKey struct {
		labelName  string
		labelValue string
		deviceIdx  int
	}
	var ifaceNeeds []ifaceNeedKey
	needsByLabel := make(map[string][]string)
	for i, dev := range devices {
		if len(dev.Interfaces) > 0 {
			continue
		}
		src := dev.MetricsSource
		if src == "" {
			src = domain.MetricsSourcePrometheus
		}
		if src != domain.MetricsSourcePrometheus && src != domain.MetricsSourcePrometheusSNMPFallback {
			continue
		}
		labelName := dev.PrometheusLabelName
		if labelName == "" {
			labelName = "instance"
		}
		lv := dev.PrometheusLabelValue
		if lv == "" {
			lv = dev.IP
		}
		if lv == "" {
			continue
		}
		ifaceNeeds = append(ifaceNeeds, ifaceNeedKey{labelName: labelName, labelValue: lv, deviceIdx: i})
		needsByLabel[labelName] = append(needsByLabel[labelName], lv)
	}

	if len(needsByLabel) > 0 && ctx.Err() == nil {
		interfacesByLabelValue := make(map[string][]domain.Interface)
		for labelName, labelValues := range needsByLabel {
			if ctx.Err() != nil {
				break
			}
			if ifMap, err := promClient.QueryInterfaces(ctx, labelName, labelValues); err != nil {
				log.Printf("Metrics collector: failed to query interfaces from Prometheus (label=%s): %v", labelName, err)
			} else {
				for k, v := range ifMap {
					interfacesByLabelValue[k] = v
				}
			}
		}

		for _, need := range ifaceNeeds {
			promIfaces, ok := interfacesByLabelValue[need.labelValue]
			if !ok || len(promIfaces) == 0 {
				continue
			}
			dev := &devices[need.deviceIdx]
			dev.Interfaces = promIfaces
			for j := range dev.Interfaces {
				dev.Interfaces[j].DeviceID = dev.ID
				dev.Interfaces[j].ID = uuid.New()
			}
			if err := c.deviceRepo.Update(dev); err != nil {
				log.Printf("Metrics collector: failed to save Prometheus-discovered interfaces for %s: %v", dev.IP, err)
			} else {
				log.Printf("Metrics collector: discovered %d interfaces from Prometheus for %s", len(promIfaces), dev.IP)
			}
		}
	}

	// Prometheus is considered available if all queries succeeded (or no devices use Prometheus).
	promNowAvailable := promQueryTotal == 0 || promQueryErrors == 0

	// Alerts use the "instance" label (device IP) regardless of the configured label name.
	if len(allPromLabelGroups) > 0 && promNowAvailable {
		if alerts, err := promClient.QueryAlerts(ctx); err != nil {
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
				vendorName := dev.Vendor
				if vendorName == "" {
					vendorName = "default"
				}
				m, err := c.snmpPollFunc(dev.IP, dev.SNMPCredentials, vendorName)
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

	// Poll SNMP interface counters for link throughput on SNMP-sourced devices.
	// Converts raw counter deltas to bit rates and injects them as LinkMetrics.
	snmpLinkMetricsByIP := make(map[string][]domain.LinkMetrics)
	if c.snmpLinkPollFunc != nil {
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, dev := range devices {
			src := dev.MetricsSource
			if src == "" {
				src = domain.MetricsSourcePrometheus
			}
			if src != domain.MetricsSourceSNMP {
				continue
			}
			if dev.IP == "" {
				continue
			}

			dev := dev
			wg.Add(1)
			go func() {
				defer wg.Done()
				counters, err := c.snmpLinkPollFunc(dev.IP, dev.SNMPCredentials)
				if err != nil {
					log.Printf("SNMP link poll failed for %s: %v", dev.IP, err)
					return
				}
				now := time.Now()
				var linkMetrics []domain.LinkMetrics

				c.prevCountersMu.Lock()
				prev := c.prevCounters[dev.IP]
				if prev == nil {
					prev = make(map[string]snmpCounterSample)
					c.prevCounters[dev.IP] = prev
				}
				for _, counter := range counters {
					if old, ok := prev[counter.IfName]; ok {
						dt := now.Sub(old.Time).Seconds()
						if dt > 0 {
							// Handle 64-bit counter wraps gracefully
							inDelta := counter.InOctets - old.InOctets
							outDelta := counter.OutOctets - old.OutOctets
							rxBps := float64(inDelta) * 8.0 / dt
							txBps := float64(outDelta) * 8.0 / dt
							linkMetrics = append(linkMetrics, domain.LinkMetrics{
								IfName:      counter.IfName,
								TxBps:       &txBps,
								RxBps:       &rxBps,
								CollectedAt: now.UTC(),
							})
						}
					}
					prev[counter.IfName] = snmpCounterSample{
						InOctets:  counter.InOctets,
						OutOctets: counter.OutOctets,
						Time:      now,
					}
				}
				c.prevCountersMu.Unlock()

				if len(linkMetrics) > 0 {
					mu.Lock()
					snmpLinkMetricsByIP[dev.IP] = linkMetrics
					mu.Unlock()
				}
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
	// Merge SNMP link metrics (does not overwrite Prometheus data — only SNMP-sourced devices reach here)
	for ip, metrics := range snmpLinkMetricsByIP {
		linkMetricsByIP[ip] = append(linkMetricsByIP[ip], metrics...)
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
	if promNowAvailable && len(allPromLabelGroups) > 0 {
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

		if probeStatuses, err := promClient.QueryProbeStatus(ctx, promIPs); err != nil {
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

