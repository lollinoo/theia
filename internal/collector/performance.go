package collector

// This file defines performance metrics collection behavior and normalized collector output.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

const snmpCollectorDebugSlowThreshold = 2 * time.Second

const (
	performanceIfHCInOctetsWalkOperation  = "if_hc_in_octets_walk"
	performanceIfHCOutOctetsWalkOperation = "if_hc_out_octets_walk"
	performanceCounterCooldownEarlyExit   = "if_hc_counter_walk_cooldown"
)

// CounterWalkCooldownPolicy stores volatile backoff state for expensive
// high-capacity interface counter walks.
type CounterWalkCooldownPolicy interface {
	ShouldSkipCounterWalk(deviceID uuid.UUID, operation string, now time.Time) bool
	RecordCounterWalkResult(deviceID uuid.UUID, operation string, result string, now time.Time, expectedInterval time.Duration)
}

// PerformancePollOptions carries runtime policies that are intentionally kept
// outside the stateless performance collector.
type PerformancePollOptions struct {
	ExpectedInterval time.Duration
	CounterCooldown  CounterWalkCooldownPolicy
}

// PerformanceCollector polls SNMP performance metrics and raw interface
// counters for a single device without retaining collector-side state.
type PerformanceCollector struct {
	registry  *vendor.Registry
	newClient NewSNMPClientFunc
	now       func() time.Time
}

// NewPerformanceCollector constructs a stateless SNMP-primary performance
// collector that reuses the shared vendor registry and SNMP client factory.
func NewPerformanceCollector(registry *vendor.Registry, newClient NewSNMPClientFunc) *PerformanceCollector {
	return &PerformanceCollector{
		registry:  registry,
		newClient: newClient,
		now:       time.Now,
	}
}

// Poll collects performance metrics and raw counters for a device using a
// single SNMP client for the entire poll cycle.
func (c *PerformanceCollector) Poll(ctx context.Context, device domain.Device, timeout time.Duration, retries int) PerformanceResult {
	return c.PollWithOptions(ctx, device, timeout, retries, PerformancePollOptions{})
}

// PollWithOptions collects performance metrics and raw counters for a device
// while applying optional runtime policies around expensive counter walks.
func (c *PerformanceCollector) PollWithOptions(ctx context.Context, device domain.Device, timeout time.Duration, retries int, options PerformancePollOptions) PerformanceResult {
	if ctx == nil {
		ctx = context.Background()
	}

	collectedAt := time.Now().UTC()
	if c != nil && c.now != nil {
		collectedAt = c.now().UTC()
	}

	result := PerformanceResult{
		DeviceID:    device.ID,
		CollectedAt: collectedAt,
		Metrics: domain.DeviceMetrics{
			DeviceID:    device.ID,
			CollectedAt: collectedAt,
		},
	}
	deviceLabels := snmpCollectorDeviceMetricLabels(device)

	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	if c == nil {
		result.Err = errors.New("performance collector is nil")
		return result
	}
	if c.registry == nil {
		result.Err = errors.New("performance collector registry is nil")
		return result
	}
	if c.newClient == nil {
		result.Err = errors.New("performance collector SNMP client factory is nil")
		return result
	}

	client, err := c.newClient(device.IP, device.SNMPCredentials, timeout, retries)
	if err != nil {
		result.Err = fmt.Errorf("create SNMP client: %w", err)
		return result
	}
	if client == nil {
		result.Err = errors.New("create SNMP client: nil client")
		return result
	}
	if err := client.Connect(); err != nil {
		result.Err = fmt.Errorf("connect SNMP client: %w", err)
		return result
	}
	defer func() {
		_ = client.Close()
	}()

	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	instrumentedClient := instrumentedSNMPBulkWalkClient{
		delegate:           client,
		collector:          "performance",
		bulkWalkOperations: performanceBulkWalkOperations(),
		deviceLabels:       deviceLabels,
	}

	hasInterfaceCounters := hasCachedInterfaceIndexes(device.Interfaces)
	counterWalkSkipped := false
	if hasInterfaceCounters {
		speedByName, speedByDescr := indexInterfaceSpeeds(device.Interfaces)
		counters, skipped, counterErr := c.pollInterfaceCountersWithOptions(instrumentedClient, device, options, deviceLabels, collectedAt)
		counterWalkSkipped = skipped
		if counterErr != nil {
			result.Err = counterErr
		}
		result.Counters = make([]InterfaceCounterSnapshot, 0, len(counters))
		for _, counter := range counters {
			result.Counters = append(result.Counters, InterfaceCounterSnapshot{
				IfName:    counter.IfName,
				InOctets:  counter.InOctets,
				OutOctets: counter.OutOctets,
				SpeedBps:  lookupInterfaceSpeed(counter.IfName, speedByName, speedByDescr),
			})
		}
	}
	sort.Slice(result.Counters, func(i, j int) bool {
		return result.Counters[i].IfName < result.Counters[j].IfName
	})
	if hasInterfaceCounters && !counterWalkSkipped && result.Err == nil && performanceResultHasNoSNMPData(result) {
		result.Err = errors.New("performance poll returned no SNMP data")
	}

	return result
}

func performanceBulkWalkOperations() map[string]string {
	return map[string]string{
		snmp.OidIfHCInOctets:  performanceIfHCInOctetsWalkOperation,
		snmp.OidIfHCOutOctets: performanceIfHCOutOctetsWalkOperation,
	}
}

type performanceCounterWalk struct {
	oid       string
	operation string
}

func performanceCounterWalks() []performanceCounterWalk {
	return []performanceCounterWalk{
		{oid: snmp.OidIfHCInOctets, operation: performanceIfHCInOctetsWalkOperation},
		{oid: snmp.OidIfHCOutOctets, operation: performanceIfHCOutOctetsWalkOperation},
	}
}

func (c *PerformanceCollector) pollInterfaceCountersWithOptions(
	client snmp.ClientInterface,
	device domain.Device,
	options PerformancePollOptions,
	deviceLabels snmpCollectorDeviceLabels,
	now time.Time,
) ([]snmp.InterfaceCounter, bool, error) {
	walks := performanceCounterWalks()
	if shouldSkipPerformanceCounterWalks(options.CounterCooldown, device.ID, walks, now) {
		observability.Default().IncSNMPCollectorEarlyExit("performance", performanceCounterCooldownEarlyExit)
		for _, walk := range walks {
			observeSNMPCollectorOperation(deviceLabels, "performance", walk.operation, "skipped_cooldown", 0, snmpCollectorDebugSlowThreshold)
			options.CounterCooldown.RecordCounterWalkResult(device.ID, walk.operation, "skipped_cooldown", now, options.ExpectedInterval)
		}
		return nil, true, nil
	}

	ifNames := performanceInterfaceNamesByIndex(device.Interfaces)
	inOctets := make(map[int]uint64)
	outOctets := make(map[int]uint64)
	var firstErr error

	for _, walk := range walks {
		target := inOctets
		if walk.operation == performanceIfHCOutOctetsWalkOperation {
			target = outOctets
		}
		err := snmp.VisitBulkWalk(client, walk.oid, func(pdu gosnmp.SnmpPDU) error {
			if idx := performanceLastOIDIndex(pdu.Name, walk.oid); idx >= 0 {
				target[idx] = performanceUint64FromPDU(pdu)
			}
			return nil
		})
		result := classifySNMPCollectorResult(err)
		if options.CounterCooldown != nil {
			options.CounterCooldown.RecordCounterWalkResult(device.ID, walk.operation, result, now, options.ExpectedInterval)
		}
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("walking %s: %w", walk.operation, err)
		}
	}

	if firstErr != nil {
		return nil, false, firstErr
	}

	counters := make([]snmp.InterfaceCounter, 0, len(ifNames))
	for idx, name := range ifNames {
		_, hasInOctets := inOctets[idx]
		_, hasOutOctets := outOctets[idx]
		if !hasInOctets && !hasOutOctets {
			continue
		}
		counters = append(counters, snmp.InterfaceCounter{
			IfIndex:   idx,
			IfName:    name,
			InOctets:  inOctets[idx],
			OutOctets: outOctets[idx],
		})
	}
	return counters, false, nil
}

func shouldSkipPerformanceCounterWalks(policy CounterWalkCooldownPolicy, deviceID uuid.UUID, walks []performanceCounterWalk, now time.Time) bool {
	if policy == nil {
		return false
	}
	for _, walk := range walks {
		if policy.ShouldSkipCounterWalk(deviceID, walk.operation, now) {
			return true
		}
	}
	return false
}

func performanceInterfaceNamesByIndex(interfaces []domain.Interface) map[int]string {
	if len(interfaces) == 0 {
		return nil
	}
	ifNames := make(map[int]string, len(interfaces))
	for _, iface := range interfaces {
		if iface.IfIndex <= 0 {
			continue
		}
		name := strings.TrimSpace(iface.IfName)
		if name == "" {
			name = strings.TrimSpace(iface.IfDescr)
		}
		if name == "" {
			continue
		}
		ifNames[iface.IfIndex] = name
	}
	return ifNames
}

func performanceLastOIDIndex(oid, prefix string) int {
	suffix := strings.TrimPrefix(oid, prefix+".")
	if suffix == oid || strings.Contains(suffix, ".") {
		return -1
	}
	idx, err := strconv.Atoi(suffix)
	if err != nil {
		return -1
	}
	return idx
}

func performanceUint64FromPDU(pdu gosnmp.SnmpPDU) uint64 {
	switch v := pdu.Value.(type) {
	case uint64:
		return v
	case uint:
		return uint64(v)
	case uint32:
		return uint64(v)
	case int:
		if v >= 0 {
			return uint64(v)
		}
	case int64:
		if v >= 0 {
			return uint64(v)
		}
	}
	return 0
}

func deviceHealthBulkWalkOperations(perfOIDs vendor.PerformanceOIDs) map[string]string {
	cpuOID := strings.TrimSpace(perfOIDs.CPUOID)
	if cpuOID == "" {
		cpuOID = snmp.OidHrProcessorLoad
	}

	return map[string]string{
		cpuOID:                      "cpu_walk",
		snmp.OidHrStorageType:       "memory_type_walk",
		snmp.OidHrStorageAllocUnits: "memory_alloc_units_walk",
		snmp.OidHrStorageSize:       "memory_size_walk",
		snmp.OidHrStorageUsed:       "memory_used_walk",
		snmp.OidEntPhySensorType:    "temperature_sensor_type_walk",
		snmp.OidEntPhySensorValue:   "temperature_sensor_value_walk",
	}
}

type instrumentedSNMPBulkWalkClient struct {
	delegate           snmp.ClientInterface
	collector          string
	getOperations      map[string]string
	bulkWalkOperations map[string]string
	deviceLabels       snmpCollectorDeviceLabels
	slowThreshold      time.Duration
}

// Get retrieves get data from the collector.
func (c instrumentedSNMPBulkWalkClient) Get(oids []string) ([]gosnmp.SnmpPDU, error) {
	operation := c.getOperation(oids)
	if operation == "" {
		return c.delegate.Get(oids)
	}

	startedAt := time.Now()
	pdus, err := c.delegate.Get(oids)
	duration := time.Since(startedAt)
	result := classifySNMPCollectorResult(err)
	observeSNMPCollectorOperation(c.deviceLabels, c.collector, operation, result, duration, c.slowThreshold)
	logSNMPCollectorDebug(c.collector, operation, result, duration, len(pdus), err)
	return pdus, err
}

func (c instrumentedSNMPBulkWalkClient) BulkWalk(rootOID string) ([]gosnmp.SnmpPDU, error) {
	operation := c.bulkWalkOperation(rootOID)
	startedAt := time.Now()
	pdus, err := c.delegate.BulkWalk(rootOID)
	duration := time.Since(startedAt)
	result := classifySNMPCollectorResult(err)
	observeSNMPCollectorOperation(c.deviceLabels, c.collector, operation, result, duration, c.slowThreshold)
	logSNMPCollectorDebug(c.collector, operation, result, duration, len(pdus), err)
	return pdus, err
}

func (c instrumentedSNMPBulkWalkClient) BulkWalkEach(rootOID string, visit func(gosnmp.SnmpPDU) error) error {
	operation := c.bulkWalkOperation(rootOID)
	startedAt := time.Now()
	pduCount := 0
	err := snmp.VisitBulkWalk(c.delegate, rootOID, func(pdu gosnmp.SnmpPDU) error {
		pduCount++
		return visit(pdu)
	})
	duration := time.Since(startedAt)
	result := classifySNMPCollectorResult(err)
	observeSNMPCollectorOperation(c.deviceLabels, c.collector, operation, result, duration, c.slowThreshold)
	logSNMPCollectorDebug(c.collector, operation, result, duration, pduCount, err)
	return err
}

type snmpCollectorDeviceLabels struct {
	ID     string
	Name   string
	Target string
}

func snmpCollectorDeviceMetricLabels(device domain.Device) snmpCollectorDeviceLabels {
	id := strings.TrimSpace(device.ID.String())
	name := firstNonEmptyString(device.Hostname, device.SysName, device.IP, id)
	target := firstNonEmptyString(device.IP, device.PrometheusLabelValue, name, id)
	return snmpCollectorDeviceLabels{
		ID:     id,
		Name:   name,
		Target: target,
	}
}

func observeSNMPCollectorOperation(labels snmpCollectorDeviceLabels, collector, operation, result string, duration time.Duration, slowThreshold time.Duration) {
	observability.Default().ObserveSNMPCollectorOperation(collector, operation, result, duration)
	if slowThreshold <= 0 {
		slowThreshold = snmpCollectorDebugSlowThreshold
	}
	observability.Default().ObserveSNMPCollectorDeviceOperation(
		labels.ID,
		labels.Name,
		labels.Target,
		collector,
		operation,
		result,
		duration,
		duration >= slowThreshold,
	)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "unknown"
}

func (c instrumentedSNMPBulkWalkClient) getOperation(oids []string) string {
	if len(oids) != 1 {
		return ""
	}
	return c.getOperations[strings.TrimSpace(oids[0])]
}

func (c instrumentedSNMPBulkWalkClient) bulkWalkOperation(rootOID string) string {
	if operation := c.bulkWalkOperations[strings.TrimSpace(rootOID)]; operation != "" {
		return operation
	}
	return "bulk_walk"
}

func classifySNMPCollectorResult(err error) string {
	if err == nil {
		return "success"
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") {
		return "timeout"
	}
	return "error"
}

func logSNMPCollectorDebug(collector string, operation string, result string, duration time.Duration, pduCount int, err error) {
	slow := duration >= snmpCollectorDebugSlowThreshold
	if err == nil && !slow {
		return
	}
	logging.Debugf(
		"snmp collector operation collector=%s operation=%s result=%s duration_ms=%d pdu_count=%d error_set=%t slow=%t",
		collector,
		operation,
		result,
		duration.Milliseconds(),
		pduCount,
		err != nil,
		slow,
	)
}

func performanceResultHasNoSNMPData(result PerformanceResult) bool {
	return result.Metrics.CPUPercent == nil &&
		result.Metrics.MemPercent == nil &&
		result.Metrics.UptimeSecs == nil &&
		result.Metrics.TempCelsius == nil &&
		len(result.Counters) == 0
}

func indexInterfaceSpeeds(interfaces []domain.Interface) (map[string]int64, map[string]int64) {
	byName := make(map[string]int64, len(interfaces))
	byDescr := make(map[string]int64, len(interfaces))
	for _, iface := range interfaces {
		byName[iface.IfName] = iface.Speed
		byDescr[iface.IfDescr] = iface.Speed
	}
	return byName, byDescr
}

func hasCachedInterfaceIndexes(interfaces []domain.Interface) bool {
	for _, iface := range interfaces {
		if iface.IfIndex <= 0 {
			continue
		}
		if strings.TrimSpace(iface.IfName) != "" || strings.TrimSpace(iface.IfDescr) != "" {
			return true
		}
	}
	return false
}

func lookupInterfaceSpeed(ifName string, speedByName map[string]int64, speedByDescr map[string]int64) int64 {
	if speed, ok := speedByName[ifName]; ok {
		return speed
	}
	if speed, ok := speedByDescr[ifName]; ok {
		return speed
	}
	return 0
}
