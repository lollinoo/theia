package collector

// This file defines operational metrics collection behavior and normalized collector output.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

// OperationalCollector polls medium-volatility SNMP reachability and interface
// state data for a single device without retaining collector-side state.
type OperationalCollector struct {
	registry  *vendor.Registry
	newClient NewSNMPClientFunc
	now       func() time.Time
}

// NewOperationalCollector constructs a stateless operational collector that
// resolves vendor OIDs through the shared registry.
func NewOperationalCollector(registry *vendor.Registry, newClient NewSNMPClientFunc) *OperationalCollector {
	return &OperationalCollector{
		registry:  registry,
		newClient: newClient,
		now:       time.Now,
	}
}

// Poll collects device reachability, uptime, and interface operational status
// using a single SNMP client for the whole poll cycle.
func (c *OperationalCollector) Poll(ctx context.Context, device domain.Device, timeout time.Duration, retries int) OperationalResult {
	if ctx == nil {
		ctx = context.Background()
	}

	collectedAt := time.Now().UTC()
	if c != nil && c.now != nil {
		collectedAt = c.now().UTC()
	}

	result := OperationalResult{
		DeviceID:    device.ID,
		CollectedAt: collectedAt,
	}

	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	if c == nil {
		result.Err = errors.New("operational collector is nil")
		return result
	}
	if c.registry == nil {
		result.Err = errors.New("operational collector registry is nil")
		return result
	}
	if c.newClient == nil {
		result.Err = errors.New("operational collector SNMP client factory is nil")
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

	operationalOIDs := c.registry.ResolveOperationalOIDs(vendorName)
	uptimeOID := strings.TrimSpace(operationalOIDs.SysUpTimeOID)
	if uptimeOID == "" {
		uptimeOID = snmp.OidSysUpTime
	}
	instrumentedClient := instrumentedSNMPBulkWalkClient{
		delegate:      client,
		collector:     "operational",
		getOperations: map[string]string{uptimeOID: "sysuptime_probe"},
		bulkWalkOperations: map[string]string{
			snmp.OidIfName:       "if_name_walk",
			snmp.OidIfDescr:      "if_descr_walk",
			snmp.OidIfOperStatus: "if_oper_status_walk",
		},
		earlyExitReasons: map[string]string{
			"sysuptime_probe": "sysuptime_probe_failed",
		},
		deviceLabels: snmpCollectorDeviceMetricLabels(device),
	}

	uptimeSecs, statuses, err := snmp.PollOperationalStatusWithInterfaces(instrumentedClient, operationalOIDs, device.Interfaces)
	if err != nil {
		result.Err = fmt.Errorf("poll operational status: %w", err)
		return result
	}

	result.Reachable = true
	result.UptimeSecs = cloneFloat64Ptr(uptimeSecs)
	if len(statuses) > 0 {
		result.InterfaceStatuses = make(map[string]string, len(statuses))
		for ifName, status := range statuses {
			result.InterfaceStatuses[ifName] = status
		}
	}

	return result
}
