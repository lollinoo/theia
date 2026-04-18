package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

type scriptedCollectorClient struct {
	getResponses      map[string][]gosnmp.SnmpPDU
	getErrs           map[string]error
	bulkWalkResponses map[string][]gosnmp.SnmpPDU
	bulkWalkErrs      map[string]error
	bulkWalkCalls     []string
	connectErr        error
	connectCalls      int
	closeCalls        int
}

func (c *scriptedCollectorClient) Get(oids []string) ([]gosnmp.SnmpPDU, error) {
	var pdus []gosnmp.SnmpPDU
	for _, oid := range oids {
		if err := c.getErrs[oid]; err != nil {
			return nil, err
		}
		pdus = append(pdus, c.getResponses[oid]...)
	}
	return pdus, nil
}

func (c *scriptedCollectorClient) BulkWalk(rootOID string) ([]gosnmp.SnmpPDU, error) {
	c.bulkWalkCalls = append(c.bulkWalkCalls, rootOID)
	if err := c.bulkWalkErrs[rootOID]; err != nil {
		return nil, err
	}
	return c.bulkWalkResponses[rootOID], nil
}

func (c *scriptedCollectorClient) Connect() error {
	c.connectCalls++
	return c.connectErr
}

func (c *scriptedCollectorClient) Close() error {
	c.closeCalls++
	return nil
}

type collectorCtorCall struct {
	target  string
	creds   domain.SNMPCredentials
	timeout time.Duration
	retries int
}

func TestOperationalCollectorPoll(t *testing.T) {
	t.Parallel()

	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}

	collectedAt := time.Date(2026, 4, 12, 15, 0, 0, 0, time.FixedZone("plus2", 2*60*60))
	timeout := 9 * time.Second
	retries := 2

	tests := []struct {
		name      string
		device    domain.Device
		newClient func() *scriptedCollectorClient
		assert    func(t *testing.T, result OperationalResult, client *scriptedCollectorClient, calls []collectorCtorCall)
	}{
		{
			name: "happy path returns typed operational result",
			device: domain.Device{
				ID:     uuid.New(),
				IP:     "192.0.2.21",
				Vendor: "",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
			newClient: func() *scriptedCollectorClient {
				return &scriptedCollectorClient{
					getResponses: map[string][]gosnmp.SnmpPDU{
						snmp.OidSysUpTime: {
							{Name: snmp.OidSysUpTime, Value: uint32(6000)},
						},
					},
					bulkWalkResponses: map[string][]gosnmp.SnmpPDU{
						snmp.OidIfName: {
							{Name: snmp.OidIfName + ".1", Value: "ether1"},
							{Name: snmp.OidIfName + ".2", Value: "ether2"},
						},
						snmp.OidIfOperStatus: {
							{Name: snmp.OidIfOperStatus + ".1", Value: int(1)},
							{Name: snmp.OidIfOperStatus + ".2", Value: int(2)},
						},
					},
				}
			},
			assert: func(t *testing.T, result OperationalResult, client *scriptedCollectorClient, calls []collectorCtorCall) {
				t.Helper()

				if len(calls) != 1 {
					t.Fatalf("newClient calls = %d, want 1", len(calls))
				}
				if calls[0].target != "192.0.2.21" {
					t.Fatalf("target = %q, want %q", calls[0].target, "192.0.2.21")
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
				if !result.Reachable {
					t.Fatal("Reachable = false, want true")
				}
				assertFloatPtrEqual(t, result.UptimeSecs, 60, "UptimeSecs")
				if len(result.InterfaceStatuses) != 2 {
					t.Fatalf("status count = %d, want 2", len(result.InterfaceStatuses))
				}
				if result.InterfaceStatuses["ether1"] != "up" {
					t.Fatalf("InterfaceStatuses[ether1] = %q, want %q", result.InterfaceStatuses["ether1"], "up")
				}
				if result.InterfaceStatuses["ether2"] != "down" {
					t.Fatalf("InterfaceStatuses[ether2] = %q, want %q", result.InterfaceStatuses["ether2"], "down")
				}
			},
		},
		{
			name: "partial result keeps collector reachable",
			device: domain.Device{
				ID:     uuid.New(),
				IP:     "192.0.2.22",
				Vendor: "default",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
			newClient: func() *scriptedCollectorClient {
				return &scriptedCollectorClient{
					bulkWalkResponses: map[string][]gosnmp.SnmpPDU{
						snmp.OidIfDescr: {
							{Name: snmp.OidIfDescr + ".7", Value: "uplink"},
						},
						snmp.OidIfOperStatus: {
							{Name: snmp.OidIfOperStatus + ".7", Value: int(1)},
						},
					},
				}
			},
			assert: func(t *testing.T, result OperationalResult, client *scriptedCollectorClient, _ []collectorCtorCall) {
				t.Helper()

				if result.Err != nil {
					t.Fatalf("Err = %v, want nil", result.Err)
				}
				if !result.Reachable {
					t.Fatal("Reachable = false, want true")
				}
				if result.UptimeSecs != nil {
					t.Fatalf("UptimeSecs = %v, want nil", *result.UptimeSecs)
				}
				if len(result.InterfaceStatuses) != 1 {
					t.Fatalf("status count = %d, want 1", len(result.InterfaceStatuses))
				}
				if result.InterfaceStatuses["uplink"] != "up" {
					t.Fatalf("InterfaceStatuses[uplink] = %q, want %q", result.InterfaceStatuses["uplink"], "up")
				}
				if client.closeCalls != 1 {
					t.Fatalf("close calls = %d, want 1", client.closeCalls)
				}
			},
		},
		{
			name: "query error returns error without fabricated fields",
			device: domain.Device{
				ID:     uuid.New(),
				IP:     "192.0.2.23",
				Vendor: "default",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
			newClient: func() *scriptedCollectorClient {
				return &scriptedCollectorClient{
					getResponses: map[string][]gosnmp.SnmpPDU{
						snmp.OidSysUpTime: {
							{Name: snmp.OidSysUpTime, Value: uint32(1200)},
						},
					},
					bulkWalkErrs: map[string]error{
						snmp.OidIfOperStatus: errors.New("walk failed"),
					},
				}
			},
			assert: func(t *testing.T, result OperationalResult, client *scriptedCollectorClient, _ []collectorCtorCall) {
				t.Helper()

				if result.Err == nil {
					t.Fatal("expected error")
				}
				if result.Reachable {
					t.Fatal("Reachable = true, want false")
				}
				if result.UptimeSecs != nil {
					t.Fatalf("UptimeSecs = %v, want nil", *result.UptimeSecs)
				}
				if result.InterfaceStatuses != nil {
					t.Fatalf("InterfaceStatuses = %#v, want nil", result.InterfaceStatuses)
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

			collector := NewOperationalCollector(registry, func(target string, creds domain.SNMPCredentials, gotTimeout time.Duration, gotRetries int) (SNMPClient, error) {
				calls = append(calls, collectorCtorCall{
					target:  target,
					creds:   creds,
					timeout: gotTimeout,
					retries: gotRetries,
				})
				return client, nil
			})
			collector.now = func() time.Time { return collectedAt }

			result := collector.Poll(context.Background(), tt.device, timeout, retries)

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

func TestOperationalResultImplementsStateUpdate(t *testing.T) {
	t.Parallel()

	var _ StateUpdate = OperationalResult{}
}
