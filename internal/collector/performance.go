package collector

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

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

	perfOIDs := c.registry.ResolvePerformanceOIDs(vendorName)
	cpuPercent, memPercent, uptimeSecs, tempCelsius := snmp.PollDeviceMetrics(client, perfOIDs)
	result.Metrics.CPUPercent = cloneFloat64Ptr(cpuPercent)
	result.Metrics.MemPercent = cloneFloat64Ptr(memPercent)
	result.Metrics.UptimeSecs = cloneFloat64Ptr(uptimeSecs)
	result.Metrics.TempCelsius = cloneFloat64Ptr(tempCelsius)

	speedByName, speedByDescr := indexInterfaceSpeeds(device.Interfaces)
	counters := snmp.PollInterfaceCounters(client)
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

	return result
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
