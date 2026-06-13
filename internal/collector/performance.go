package collector

// This file defines performance metrics collection behavior and normalized collector output.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

const snmpCollectorDebugSlowThreshold = 2 * time.Second

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

	vendorName := strings.TrimSpace(device.Vendor)
	if vendorName == "" {
		vendorName = "default"
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
	probeStartedAt := time.Now()
	_, err = client.Get([]string{snmp.OidSysUpTime})
	observability.Default().ObserveSNMPCollectorOperation(
		"performance",
		"sysuptime_probe",
		classifySNMPCollectorResult(err),
		time.Since(probeStartedAt),
	)
	if err != nil {
		observability.Default().IncSNMPCollectorEarlyExit("performance", "sysuptime_probe_failed")
		result.Err = fmt.Errorf("performance uptime probe: %w", err)
		return result
	}

	perfOIDs := c.registry.ResolvePerformanceOIDs(vendorName)
	instrumentedClient := instrumentedSNMPBulkWalkClient{
		delegate:           client,
		collector:          "performance",
		bulkWalkOperations: performanceBulkWalkOperations(perfOIDs),
	}
	cpuPercent, memPercent, uptimeSecs, tempCelsius := snmp.PollDeviceMetrics(instrumentedClient, perfOIDs)
	result.Metrics.CPUPercent = cloneFloat64Ptr(cpuPercent)
	result.Metrics.MemPercent = cloneFloat64Ptr(memPercent)
	result.Metrics.UptimeSecs = cloneFloat64Ptr(uptimeSecs)
	result.Metrics.TempCelsius = cloneFloat64Ptr(tempCelsius)

	speedByName, speedByDescr := indexInterfaceSpeeds(device.Interfaces)
	counters := snmp.PollInterfaceCountersWithInterfaces(instrumentedClient, device.Interfaces)
	result.Counters = make([]InterfaceCounterSnapshot, 0, len(counters))
	for _, counter := range counters {
		result.Counters = append(result.Counters, InterfaceCounterSnapshot{
			IfName:    counter.IfName,
			InOctets:  counter.InOctets,
			OutOctets: counter.OutOctets,
			SpeedBps:  lookupInterfaceSpeed(counter.IfName, speedByName, speedByDescr),
		})
	}
	sort.Slice(result.Counters, func(i, j int) bool {
		return result.Counters[i].IfName < result.Counters[j].IfName
	})
	if performanceResultHasNoSNMPData(result) {
		result.Err = errors.New("performance poll returned no SNMP data")
	}

	return result
}

func performanceBulkWalkOperations(perfOIDs vendor.PerformanceOIDs) map[string]string {
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
		snmp.OidIfName:              "if_name_walk",
		snmp.OidIfDescr:             "if_descr_walk",
		snmp.OidIfHCInOctets:        "if_hc_in_octets_walk",
		snmp.OidIfHCOutOctets:       "if_hc_out_octets_walk",
	}
}

type instrumentedSNMPBulkWalkClient struct {
	delegate           snmp.ClientInterface
	collector          string
	getOperations      map[string]string
	bulkWalkOperations map[string]string
	earlyExitReasons   map[string]string
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
	observability.Default().ObserveSNMPCollectorOperation(
		c.collector,
		operation,
		result,
		duration,
	)
	logSNMPCollectorDebug(c.collector, operation, result, duration, len(pdus), err)
	if err != nil {
		if reason := c.earlyExitReasons[operation]; reason != "" {
			observability.Default().IncSNMPCollectorEarlyExit(c.collector, reason)
		}
	}
	return pdus, err
}

func (c instrumentedSNMPBulkWalkClient) BulkWalk(rootOID string) ([]gosnmp.SnmpPDU, error) {
	operation := c.bulkWalkOperation(rootOID)
	startedAt := time.Now()
	pdus, err := c.delegate.BulkWalk(rootOID)
	duration := time.Since(startedAt)
	result := classifySNMPCollectorResult(err)
	observability.Default().ObserveSNMPCollectorOperation(
		c.collector,
		operation,
		result,
		duration,
	)
	logSNMPCollectorDebug(c.collector, operation, result, duration, len(pdus), err)
	return pdus, err
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

func lookupInterfaceSpeed(ifName string, speedByName map[string]int64, speedByDescr map[string]int64) int64 {
	if speed, ok := speedByName[ifName]; ok {
		return speed
	}
	if speed, ok := speedByDescr[ifName]; ok {
		return speed
	}
	return 0
}
