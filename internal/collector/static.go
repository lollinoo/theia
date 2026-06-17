package collector

// This file defines static metrics collection behavior and normalized collector output.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

const (
	staticOptionalHealthMaxBudget = 2 * time.Second
	staticOptionalHealthCooldown  = 30 * time.Minute
)

// StaticCollector polls low-volatility inventory, topology, and optional
// health data for a single device by wrapping the existing discovery flow.
type StaticCollector struct {
	registry    *vendor.Registry
	newClient   NewSNMPClientFunc
	now         func() time.Time
	healthWalks *staticHealthWalkState
}

// NewStaticCollector constructs a static collector that reuses the shared
// discovery path and vendor registry.
func NewStaticCollector(registry *vendor.Registry, newClient NewSNMPClientFunc) *StaticCollector {
	return &StaticCollector{
		registry:    registry,
		newClient:   newClient,
		now:         time.Now,
		healthWalks: newStaticHealthWalkState(),
	}
}

// Poll discovers static inventory and topology data using a single SNMP client
// for the whole poll cycle.
func (c *StaticCollector) Poll(ctx context.Context, device domain.Device, timeout time.Duration, retries int, topologyMode domain.TopologyDiscoveryMode) StaticResult {
	if ctx == nil {
		ctx = context.Background()
	}

	startedAt := time.Now()
	collectedAt := time.Now().UTC()
	if c != nil && c.now != nil {
		collectedAt = c.now().UTC()
	}

	result := StaticResult{
		DeviceID:    device.ID,
		CollectedAt: collectedAt,
	}

	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	if c == nil {
		result.Err = errors.New("static collector is nil")
		return result
	}
	if c.registry == nil {
		result.Err = errors.New("static collector registry is nil")
		return result
	}
	if c.newClient == nil {
		result.Err = errors.New("static collector SNMP client factory is nil")
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

	perfOIDs := c.registry.ResolvePerformanceOIDs(device.Vendor)
	instrumentedClient := instrumentedSNMPBulkWalkClient{
		delegate:  client,
		collector: "static",
		bulkWalkOperations: mergeBulkWalkOperations(map[string]string{
			snmp.OidIfDescr:       "if_descr_walk",
			snmp.OidIfSpeed:       "if_speed_walk",
			snmp.OidIfAdminStatus: "if_admin_status_walk",
			snmp.OidIfOperStatus:  "if_oper_status_walk",
			snmp.OidIfName:        "if_name_walk",
			snmp.OidIfHighSpeed:   "if_high_speed_walk",
		}, deviceHealthBulkWalkOperations(perfOIDs)),
		deviceLabels: snmpCollectorDeviceMetricLabels(device),
	}
	discovery, err := snmp.DiscoverDeviceWithPolicy(instrumentedClient, c.registry, snmp.NeighborDiscoveryPolicyFromMode(topologyMode))
	if err != nil {
		result.Err = fmt.Errorf("discover device: %w", err)
		return result
	}
	if discovery == nil {
		result.Err = errors.New("discover device: nil result")
		return result
	}

	perfOIDs = c.registry.ResolvePerformanceOIDs(discovery.Vendor)
	instrumentedClient.bulkWalkOperations = mergeBulkWalkOperations(instrumentedClient.bulkWalkOperations, deviceHealthBulkWalkOperations(perfOIDs))
	healthClient := c.optionalHealthSNMPClient(device, instrumentedClient, perfOIDs, startedAt, timeout)
	cpuPercent, memPercent, tempCelsius := snmp.PollDeviceHealthMetrics(healthClient, perfOIDs)
	result.Metrics = domain.DeviceMetrics{
		DeviceID:    device.ID,
		CPUPercent:  cloneFloat64Ptr(cpuPercent),
		MemPercent:  cloneFloat64Ptr(memPercent),
		TempCelsius: cloneFloat64Ptr(tempCelsius),
		CollectedAt: collectedAt,
	}

	result.SysName = discovery.SysName
	result.SysDescr = discovery.SysDescr
	result.SysObjectID = discovery.SysObjectID
	result.HardwareModel = discovery.HardwareModel
	result.OSVersion = discovery.OSVersion
	result.Vendor = discovery.Vendor
	result.DeviceType = discovery.DeviceType
	result.Interfaces = append([]domain.Interface(nil), discovery.Interfaces...)
	result.Neighbors = append([]snmp.NeighborInfo(nil), discovery.Neighbors...)
	result.NeighborDiscoveryProtocols = append([]domain.DiscoveryProtocol(nil), discovery.NeighborDiscoveryProtocols...)
	result.NeighborDiscoveryFailures = append([]snmp.NeighborDiscoveryFailure(nil), discovery.NeighborDiscoveryFailures...)

	return result
}

func (c *StaticCollector) optionalHealthSNMPClient(device domain.Device, delegate snmp.ClientInterface, perfOIDs vendor.PerformanceOIDs, startedAt time.Time, timeout time.Duration) snmp.ClientInterface {
	state := c.healthWalks
	if state == nil {
		state = newStaticHealthWalkState()
		c.healthWalks = state
	}
	cpuOID := strings.TrimSpace(perfOIDs.CPUOID)
	if cpuOID == "" {
		cpuOID = snmp.OidHrProcessorLoad
	}
	return optionalStaticHealthClient{
		delegate:   delegate,
		state:      state,
		deviceKey:  device.ID.String(),
		startedAt:  startedAt,
		budget:     staticOptionalHealthBudget(timeout),
		cooldown:   staticOptionalHealthCooldown,
		cpuOID:     cpuOID,
		tempScalar: strings.TrimSpace(perfOIDs.TemperatureOID),
	}
}

func staticOptionalHealthBudget(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return staticOptionalHealthMaxBudget
	}
	half := timeout / 2
	if half <= 0 {
		return timeout
	}
	if half < staticOptionalHealthMaxBudget {
		return half
	}
	return staticOptionalHealthMaxBudget
}

func mergeBulkWalkOperations(maps ...map[string]string) map[string]string {
	total := 0
	for _, operations := range maps {
		total += len(operations)
	}
	merged := make(map[string]string, total)
	for _, operations := range maps {
		for oid, operation := range operations {
			merged[oid] = operation
		}
	}
	return merged
}

type optionalStaticHealthClient struct {
	delegate   snmp.ClientInterface
	state      *staticHealthWalkState
	deviceKey  string
	startedAt  time.Time
	budget     time.Duration
	cooldown   time.Duration
	cpuOID     string
	tempScalar string
}

func (c optionalStaticHealthClient) Get(oids []string) ([]gosnmp.SnmpPDU, error) {
	if len(oids) == 1 && c.healthGroupForGet(oids[0]) != "" {
		group := c.healthGroupForGet(oids[0])
		if c.skip(group, time.Now()) {
			return nil, nil
		}
		pdus, err := c.delegate.Get(oids)
		if err != nil {
			c.cooldownGroup(group, time.Now())
		}
		return pdus, err
	}
	return c.delegate.Get(oids)
}

func (c optionalStaticHealthClient) BulkWalk(rootOID string) ([]gosnmp.SnmpPDU, error) {
	group := c.healthGroupForBulkWalk(rootOID)
	if group == "" {
		return c.delegate.BulkWalk(rootOID)
	}
	if c.skip(group, time.Now()) {
		return nil, nil
	}
	pdus, err := c.delegate.BulkWalk(rootOID)
	if err != nil {
		c.cooldownGroup(group, time.Now())
	}
	return pdus, err
}

func (c optionalStaticHealthClient) BulkWalkEach(rootOID string, visit func(gosnmp.SnmpPDU) error) error {
	group := c.healthGroupForBulkWalk(rootOID)
	if group == "" {
		return snmp.VisitBulkWalk(c.delegate, rootOID, visit)
	}
	if c.skip(group, time.Now()) {
		return nil
	}
	var visitErr error
	err := snmp.VisitBulkWalk(c.delegate, rootOID, func(pdu gosnmp.SnmpPDU) error {
		if err := visit(pdu); err != nil {
			visitErr = err
			return err
		}
		return nil
	})
	if err != nil && visitErr == nil {
		c.cooldownGroup(group, time.Now())
	}
	return err
}

func (c optionalStaticHealthClient) skip(group string, now time.Time) bool {
	if c.budget > 0 && !c.startedAt.IsZero() && !now.Before(c.startedAt.Add(c.budget)) {
		return true
	}
	return c.state != nil && c.state.coolingDown(c.deviceKey, group, now)
}

func (c optionalStaticHealthClient) cooldownGroup(group string, now time.Time) {
	if c.state == nil || c.cooldown <= 0 {
		return
	}
	c.state.cooldown(c.deviceKey, group, now.Add(c.cooldown))
}

func (c optionalStaticHealthClient) healthGroupForGet(oid string) string {
	if strings.TrimSpace(oid) == c.tempScalar && c.tempScalar != "" {
		return "temperature"
	}
	return ""
}

func (c optionalStaticHealthClient) healthGroupForBulkWalk(rootOID string) string {
	switch strings.TrimSpace(rootOID) {
	case c.cpuOID:
		return "cpu"
	case snmp.OidHrStorageType, snmp.OidHrStorageAllocUnits, snmp.OidHrStorageSize, snmp.OidHrStorageUsed:
		return "memory"
	case snmp.OidEntPhySensorType, snmp.OidEntPhySensorValue:
		return "temperature"
	default:
		return ""
	}
}

type staticHealthWalkState struct {
	mu        sync.Mutex
	cooldowns map[string]time.Time
}

func newStaticHealthWalkState() *staticHealthWalkState {
	return &staticHealthWalkState{cooldowns: make(map[string]time.Time)}
}

func (s *staticHealthWalkState) coolingDown(deviceKey string, group string, now time.Time) bool {
	if s == nil || deviceKey == "" || group == "" {
		return false
	}
	key := staticHealthCooldownKey(deviceKey, group)
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.cooldowns[key]
	if !ok {
		return false
	}
	if now.Before(until) {
		return true
	}
	delete(s.cooldowns, key)
	return false
}

func (s *staticHealthWalkState) cooldown(deviceKey string, group string, until time.Time) {
	if s == nil || deviceKey == "" || group == "" || until.IsZero() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldowns[staticHealthCooldownKey(deviceKey, group)] = until
}

func staticHealthCooldownKey(deviceKey string, group string) string {
	return deviceKey + "|" + group
}
