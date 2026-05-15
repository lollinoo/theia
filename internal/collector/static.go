package collector

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

// StaticCollector polls low-volatility inventory and topology data for a
// single device by wrapping the existing discovery flow.
type StaticCollector struct {
	registry  *vendor.Registry
	newClient NewSNMPClientFunc
	now       func() time.Time
}

// NewStaticCollector constructs a stateless static collector that reuses the
// shared discovery path and vendor registry.
func NewStaticCollector(registry *vendor.Registry, newClient NewSNMPClientFunc) *StaticCollector {
	return &StaticCollector{
		registry:  registry,
		newClient: newClient,
		now:       time.Now,
	}
}

// Poll discovers static inventory and topology data using a single SNMP client
// for the whole poll cycle.
func (c *StaticCollector) Poll(ctx context.Context, device domain.Device, timeout time.Duration, retries int, topologyMode domain.TopologyDiscoveryMode) StaticResult {
	if ctx == nil {
		ctx = context.Background()
	}

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

	instrumentedClient := instrumentedSNMPBulkWalkClient{
		delegate:  client,
		collector: "static",
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
