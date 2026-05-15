package collector

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

func TestStaticCollectorPoll(t *testing.T) {
	t.Parallel()

	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}

	collectedAt := time.Date(2026, 4, 12, 15, 10, 0, 0, time.FixedZone("plus2", 2*60*60))
	timeout := 11 * time.Second
	retries := 1

	tests := []struct {
		name      string
		device    domain.Device
		mode      domain.TopologyDiscoveryMode
		newClient func() *scriptedCollectorClient
		assert    func(t *testing.T, result StaticResult, client *scriptedCollectorClient, calls []collectorCtorCall)
	}{
		{
			name: "happy path returns typed discovery data",
			device: domain.Device{
				ID: uuid.New(),
				IP: "192.0.2.31",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
			mode: domain.TopologyDiscoveryModeLLDPCDP,
			newClient: func() *scriptedCollectorClient {
				return &scriptedCollectorClient{
					getResponses: map[string][]gosnmp.SnmpPDU{
						snmp.OidSysName: {
							{Name: snmp.OidSysName, Type: gosnmp.OctetString, Value: []byte("router1")},
						},
						snmp.OidSysDescr: {
							{Name: snmp.OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS RB5009")},
						},
						snmp.OidSysObjectID: {
							{Name: snmp.OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"},
						},
						".1.3.6.1.4.1.14988.1.1.4.4.0": {
							{Name: ".1.3.6.1.4.1.14988.1.1.4.4.0", Type: gosnmp.OctetString, Value: []byte("7.22.1")},
						},
					},
					bulkWalkResponses: map[string][]gosnmp.SnmpPDU{
						snmp.OidIfTable: {
							{Name: snmp.OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
							{Name: snmp.OidIfOperStatus + ".1", Type: gosnmp.Integer, Value: 1},
						},
						snmp.OidIfXTable: {
							{Name: snmp.OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("eth1")},
							{Name: snmp.OidIfHighSpeed + ".1", Type: gosnmp.Gauge32, Value: uint(1000)},
						},
						snmp.OidLLDPLocPortIfIndex: {
							{Name: snmp.OidLLDPLocPortIfIndex + ".1", Type: gosnmp.Integer, Value: 1},
						},
						snmp.OidLLDPRemChassisId: {
							{Name: snmp.OidLLDPRemChassisId + ".1000.1.1", Type: gosnmp.OctetString, Value: []byte("00:11:22:33:44:55")},
						},
						snmp.OidLLDPRemPortId: {
							{Name: snmp.OidLLDPRemPortId + ".1000.1.1", Type: gosnmp.OctetString, Value: []byte("ether2")},
						},
						snmp.OidLLDPRemSysName: {
							{Name: snmp.OidLLDPRemSysName + ".1000.1.1", Type: gosnmp.OctetString, Value: []byte("switch1")},
						},
					},
				}
			},
			assert: func(t *testing.T, result StaticResult, client *scriptedCollectorClient, calls []collectorCtorCall) {
				t.Helper()

				if len(calls) != 1 {
					t.Fatalf("newClient calls = %d, want 1", len(calls))
				}
				if calls[0].target != "192.0.2.31" {
					t.Fatalf("target = %q, want %q", calls[0].target, "192.0.2.31")
				}
				if calls[0].timeout != timeout {
					t.Fatalf("timeout = %s, want %s", calls[0].timeout, timeout)
				}
				if calls[0].retries != retries {
					t.Fatalf("retries = %d, want %d", calls[0].retries, retries)
				}
				if client.connectCalls != 1 {
					t.Fatalf("connect calls = %d, want 1", client.connectCalls)
				}
				if client.closeCalls != 1 {
					t.Fatalf("close calls = %d, want 1", client.closeCalls)
				}
				if result.Err != nil {
					t.Fatalf("Err = %v, want nil", result.Err)
				}
				if result.SysName != "router1" {
					t.Fatalf("SysName = %q, want %q", result.SysName, "router1")
				}
				if result.HardwareModel != "RB5009" {
					t.Fatalf("HardwareModel = %q, want %q", result.HardwareModel, "RB5009")
				}
				if result.OSVersion != "7.22.1" {
					t.Fatalf("OSVersion = %q, want %q", result.OSVersion, "7.22.1")
				}
				if result.Vendor != "mikrotik" {
					t.Fatalf("Vendor = %q, want %q", result.Vendor, "mikrotik")
				}
				if result.DeviceType != domain.DeviceTypeRouter {
					t.Fatalf("DeviceType = %q, want %q", result.DeviceType, domain.DeviceTypeRouter)
				}
				if len(result.Interfaces) != 1 {
					t.Fatalf("interface count = %d, want 1", len(result.Interfaces))
				}
				if result.Interfaces[0].IfName != "eth1" {
					t.Fatalf("Interfaces[0].IfName = %q, want %q", result.Interfaces[0].IfName, "eth1")
				}
				if len(result.Neighbors) != 1 {
					t.Fatalf("neighbor count = %d, want 1", len(result.Neighbors))
				}
				if result.Neighbors[0].RemoteSysName != "switch1" {
					t.Fatalf("Neighbors[0].RemoteSysName = %q, want %q", result.Neighbors[0].RemoteSysName, "switch1")
				}
				if !slices.Equal(result.NeighborDiscoveryProtocols, []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP, domain.DiscoveryProtocolCDP}) {
					t.Fatalf("NeighborDiscoveryProtocols = %v, want [lldp cdp]", result.NeighborDiscoveryProtocols)
				}
				if !slices.Contains(client.bulkWalkCalls, snmp.OidLLDPRemChassisId) {
					t.Fatalf("expected LLDP walks when topology mode is enabled, got %v", client.bulkWalkCalls)
				}
				if !slices.Contains(client.bulkWalkCalls, snmp.OidCDPDeviceID) {
					t.Fatalf("expected CDP walks when topology mode is lldp_cdp, got %v", client.bulkWalkCalls)
				}
			},
		},
		{
			name: "off mode skips topology walks but still returns inventory",
			device: domain.Device{
				ID: uuid.New(),
				IP: "192.0.2.33",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
			mode: domain.TopologyDiscoveryModeOff,
			newClient: func() *scriptedCollectorClient {
				return &scriptedCollectorClient{
					getResponses: map[string][]gosnmp.SnmpPDU{
						snmp.OidSysName: {
							{Name: snmp.OidSysName, Type: gosnmp.OctetString, Value: []byte("router-off")},
						},
						snmp.OidSysDescr: {
							{Name: snmp.OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS")},
						},
						snmp.OidSysObjectID: {
							{Name: snmp.OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"},
						},
					},
					bulkWalkResponses: map[string][]gosnmp.SnmpPDU{
						snmp.OidIfTable: {
							{Name: snmp.OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
						},
						snmp.OidIfXTable: {
							{Name: snmp.OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
						},
					},
				}
			},
			assert: func(t *testing.T, result StaticResult, client *scriptedCollectorClient, _ []collectorCtorCall) {
				t.Helper()

				if result.Err != nil {
					t.Fatalf("Err = %v, want nil", result.Err)
				}
				if result.SysName != "router-off" {
					t.Fatalf("SysName = %q, want %q", result.SysName, "router-off")
				}
				if len(result.Neighbors) != 0 {
					t.Fatalf("neighbor count = %d, want 0", len(result.Neighbors))
				}
				if len(result.NeighborDiscoveryProtocols) != 0 {
					t.Fatalf("NeighborDiscoveryProtocols = %v, want none", result.NeighborDiscoveryProtocols)
				}
				if slices.Contains(client.bulkWalkCalls, snmp.OidLLDPRemChassisId) || slices.Contains(client.bulkWalkCalls, snmp.OidCDPDeviceID) {
					t.Fatalf("expected no LLDP/CDP walks when topology mode is off, got %v", client.bulkWalkCalls)
				}
			},
		},
		{
			name: "discovery error returns typed error result",
			device: domain.Device{
				ID: uuid.New(),
				IP: "192.0.2.32",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
			mode: domain.TopologyDiscoveryModeOff,
			newClient: func() *scriptedCollectorClient {
				return &scriptedCollectorClient{
					getErrs: map[string]error{
						snmp.OidSysName: errors.New("get failed"),
					},
				}
			},
			assert: func(t *testing.T, result StaticResult, client *scriptedCollectorClient, _ []collectorCtorCall) {
				t.Helper()

				if result.Err == nil {
					t.Fatal("expected error")
				}
				if result.SysName != "" || result.SysDescr != "" || result.SysObjectID != "" {
					t.Fatalf("discovery fields = %#v, want zero values", result)
				}
				if len(result.Interfaces) != 0 {
					t.Fatalf("interface count = %d, want 0", len(result.Interfaces))
				}
				if len(result.Neighbors) != 0 {
					t.Fatalf("neighbor count = %d, want 0", len(result.Neighbors))
				}
				if client.connectCalls != 1 {
					t.Fatalf("connect calls = %d, want 1", client.connectCalls)
				}
				if client.closeCalls != 1 {
					t.Fatalf("close calls = %d, want 1", client.closeCalls)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.newClient()
			var calls []collectorCtorCall

			collector := NewStaticCollector(registry, func(target string, creds domain.SNMPCredentials, gotTimeout time.Duration, gotRetries int) (SNMPClient, error) {
				calls = append(calls, collectorCtorCall{
					target:  target,
					creds:   creds,
					timeout: gotTimeout,
					retries: gotRetries,
				})
				return client, nil
			})
			collector.now = func() time.Time { return collectedAt }

			result := collector.Poll(context.Background(), tt.device, timeout, retries, tt.mode)

			if result.DeviceID != tt.device.ID {
				t.Fatalf("DeviceID = %s, want %s", result.DeviceID, tt.device.ID)
			}
			if !result.CollectedAt.Equal(collectedAt.UTC()) {
				t.Fatalf("CollectedAt = %s, want %s", result.CollectedAt, collectedAt.UTC())
			}

			tt.assert(t, result, client, calls)
		})
	}
}

func TestStaticResultImplementsStateUpdate(t *testing.T) {
	t.Parallel()

	var _ StateUpdate = StaticResult{}
}

func TestStaticCollectorPoll_PropagatesNeighborDiscoveryFailures(t *testing.T) {
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}

	client := &scriptedCollectorClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysName: {
				{Name: snmp.OidSysName, Type: gosnmp.OctetString, Value: []byte("router-static")},
			},
			snmp.OidSysDescr: {
				{Name: snmp.OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS RB5009")},
			},
			snmp.OidSysObjectID: {
				{Name: snmp.OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"},
			},
		},
		bulkWalkResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidIfTable: {
				{Name: snmp.OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
			},
			snmp.OidIfXTable: {
				{Name: snmp.OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
			},
		},
		bulkWalkErrs: map[string]error{
			snmp.OidLLDPRemChassisId: errors.New("lldp walk failed"),
		},
	}
	collector := NewStaticCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return client, nil
	})

	result := collector.Poll(context.Background(), domain.Device{
		ID: uuid.New(),
		IP: "192.0.2.52",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}, time.Second, 1, domain.TopologyDiscoveryModeLLDP)

	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if len(result.Neighbors) != 0 {
		t.Fatalf("neighbor count = %d, want 0", len(result.Neighbors))
	}
	if !slices.Equal(result.NeighborDiscoveryProtocols, []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP}) {
		t.Fatalf("NeighborDiscoveryProtocols = %v, want [lldp]", result.NeighborDiscoveryProtocols)
	}
	failure := findStaticNeighborDiscoveryFailure(result.NeighborDiscoveryFailures, domain.DiscoveryProtocolLLDP, snmp.OidLLDPRemChassisId)
	if failure == nil {
		t.Fatalf("expected propagated LLDP failure, got %#v", result.NeighborDiscoveryFailures)
	}
	if !failure.Critical {
		t.Fatalf("failure Critical = false, want true")
	}
	if failure.Error != "lldp walk failed" {
		t.Fatalf("failure Error = %q, want lldp walk failed", failure.Error)
	}
}

func TestStaticCollectorHasNoServiceOrRepositoryCollaborators(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(StaticCollector{})
	if typ.NumField() != 3 {
		t.Fatalf("field count = %d, want 3", typ.NumField())
	}

	wantFields := []string{"registry", "newClient", "now"}
	for i, want := range wantFields {
		if typ.Field(i).Name != want {
			t.Fatalf("field %d = %q, want %q", i, typ.Field(i).Name, want)
		}
		fieldType := strings.ToLower(typ.Field(i).Type.String())
		if strings.Contains(fieldType, "service") || strings.Contains(fieldType, "repo") {
			t.Fatalf("field %q type = %q, want no service or repository collaborator", typ.Field(i).Name, typ.Field(i).Type)
		}
	}
}

func findStaticNeighborDiscoveryFailure(failures []snmp.NeighborDiscoveryFailure, protocol domain.DiscoveryProtocol, oid string) *snmp.NeighborDiscoveryFailure {
	for i := range failures {
		if failures[i].Protocol == protocol && failures[i].OID == oid {
			return &failures[i]
		}
	}
	return nil
}

func TestStaticCollectorPoll_RecordsBulkWalkMetrics(t *testing.T) {
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}
	metrics := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	client := &scriptedCollectorClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysName: {
				{Name: snmp.OidSysName, Type: gosnmp.OctetString, Value: []byte("router-static")},
			},
			snmp.OidSysDescr: {
				{Name: snmp.OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS")},
			},
			snmp.OidSysObjectID: {
				{Name: snmp.OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"},
			},
		},
		bulkWalkResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidIfTable: {
				{Name: snmp.OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
			},
			snmp.OidIfXTable: {
				{Name: snmp.OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
			},
		},
	}
	collector := NewStaticCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return client, nil
	})

	result := collector.Poll(context.Background(), domain.Device{
		ID: uuid.New(),
		IP: "192.0.2.51",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}, time.Second, 1, domain.TopologyDiscoveryModeOff)

	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if len(client.bulkWalkCalls) == 0 {
		t.Fatal("BulkWalk calls = 0, want static discovery walk attempts")
	}

	body := string(metrics.MarshalPrometheus())
	if !strings.Contains(body, `theia_snmp_collector_operations_total{collector="static",operation="bulk_walk",result="success"}`) {
		t.Fatalf("metrics output missing static bulk_walk counter\n%s", body)
	}
	if !strings.Contains(body, `theia_snmp_collector_operation_seconds_count{collector="static",operation="bulk_walk",result="success"}`) {
		t.Fatalf("metrics output missing static bulk_walk histogram\n%s", body)
	}
	if strings.Contains(body, `collector="static",operation="sysuptime_probe"`) {
		t.Fatalf("metrics output unexpectedly recorded static sysuptime_probe:\n%s", body)
	}
}
