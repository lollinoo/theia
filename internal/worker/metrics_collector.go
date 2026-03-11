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
	snmpPollFunc SNMPPollFunc // optional; fills gaps when Prometheus has no data

	mu           sync.RWMutex
	lastSnapshot *ws.SnapshotPayload
	cancel       context.CancelFunc
	done         chan struct{}
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
		promClient:   promClient,
		hub:          hub,
		deviceRepo:   deviceRepo,
		linkRepo:     linkRepo,
		settingsRepo: settingsRepo,
		snmpPollFunc: snmpPollFunc,
		lastSnapshot: ws.EmptySnapshot(),
		done:         make(chan struct{}),
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
	snapshot := c.buildSnapshot(ctx)

	c.mu.Lock()
	c.lastSnapshot = snapshot
	c.mu.Unlock()

	c.hub.Broadcast(ws.Message{
		Type:    ws.MessageTypeSnapshot,
		Payload: snapshot,
	})
}

func (c *MetricsCollector) buildSnapshot(ctx context.Context) *ws.SnapshotPayload {
	snapshot := ws.EmptySnapshot()

	devices, err := c.deviceRepo.GetAll()
	if err != nil {
		log.Printf("Metrics collector: failed to load devices: %v", err)
		return snapshot
	}

	links, err := c.linkRepo.GetAll()
	if err != nil {
		log.Printf("Metrics collector: failed to load links: %v", err)
		links = nil
	}

	deviceIPs := make([]string, 0, len(devices))
	for _, device := range devices {
		if device.IP != "" {
			deviceIPs = append(deviceIPs, device.IP)
		}
	}

	deviceMetricsByIP := map[string]domain.DeviceMetrics{}
	linkMetricsByIP := map[string][]domain.LinkMetrics{}
	alertStates := []domain.AlertState{}

	if len(deviceIPs) > 0 {
		if metrics, err := c.promClient.QueryDeviceMetrics(ctx, deviceIPs); err != nil {
			log.Printf("Metrics collector: failed to query device metrics: %v", err)
		} else {
			deviceMetricsByIP = metrics
		}

		if metrics, err := c.promClient.QueryLinkMetrics(ctx, deviceIPs); err != nil {
			log.Printf("Metrics collector: failed to query link metrics: %v", err)
		} else {
			linkMetricsByIP = metrics
		}

		if alerts, err := c.promClient.QueryAlerts(ctx); err != nil {
			log.Printf("Metrics collector: failed to query alerts: %v", err)
		} else {
			alertStates = alerts
		}
	}

	deviceMetricsByID := attachDeviceMetrics(devices, deviceMetricsByIP)

	// For devices where Prometheus returned no metrics, fall back to direct SNMP polling.
	if c.snmpPollFunc != nil {
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, dev := range devices {
			existing := deviceMetricsByID[dev.ID.String()]
			if existing.CPUPercent != nil || existing.MemPercent != nil || existing.UptimeSecs != nil {
				continue // Prometheus already provided data
			}
			dev := dev
			wg.Add(1)
			go func() {
				defer wg.Done()
				m, err := c.snmpPollFunc(dev.IP, dev.SNMPCredentials)
				if err != nil {
					log.Printf("SNMP metrics fallback failed for %s: %v", dev.IP, err)
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

	linkMetricsByID := attachLinkMetrics(devices, links, linkMetricsByIP)
	alertsByDevice := attachAlerts(devices, alertStates)

	snapshot.DeviceMetrics = ws.DeviceMetricsToDTOs(deviceMetricsByID)
	snapshot.LinkMetrics = ws.LinkMetricsToDTOs(linkMetricsByID)
	snapshot.Alerts = ws.AlertsToDTOs(alertsByDevice)

	return snapshot
}

func attachDeviceMetrics(devices []domain.Device, metricsByIP map[string]domain.DeviceMetrics) map[string]domain.DeviceMetrics {
	collectedAt := time.Now().UTC()
	metricsByID := make(map[string]domain.DeviceMetrics, len(devices))

	for _, device := range devices {
		metric, ok := metricsByIP[device.IP]
		if !ok {
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
