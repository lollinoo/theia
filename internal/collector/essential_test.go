package collector

// This file exercises essential behavior so refactors preserve the documented contract.

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

type scriptedEssentialClient struct {
	getResponses  map[string][]gosnmp.SnmpPDU
	getErr        error
	getErrs       map[string]error
	bulkWalkCalls []string
	connectErr    error
	connects      int
	closes        int
}

func (c *scriptedEssentialClient) Get(oids []string) ([]gosnmp.SnmpPDU, error) {
	if c.getErr != nil {
		return nil, c.getErr
	}
	var pdus []gosnmp.SnmpPDU
	for _, oid := range oids {
		if err := c.getErrs[oid]; err != nil {
			return nil, err
		}
		pdus = append(pdus, c.getResponses[oid]...)
	}
	return pdus, nil
}

func (c *scriptedEssentialClient) BulkWalk(rootOID string) ([]gosnmp.SnmpPDU, error) {
	c.bulkWalkCalls = append(c.bulkWalkCalls, rootOID)
	return nil, nil
}

func (c *scriptedEssentialClient) Connect() error {
	c.connects++
	return c.connectErr
}

func (c *scriptedEssentialClient) Close() error {
	c.closes++
	return nil
}

type assertiveError string

func (e assertiveError) Error() string { return string(e) }

func TestEssentialCollectorUsesConfiguredNetworkProbePorts(t *testing.T) {
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}

	collector := NewEssentialCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return &scriptedEssentialClient{connectErr: assertiveError("timeout")}, nil
	})
	var capturedPorts []int
	collector.networkProbe = func(_ context.Context, _ string, _ time.Duration, ports []int) error {
		capturedPorts = append([]int(nil), ports...)
		return nil
	}

	result := collector.Poll(context.Background(), domain.Device{ID: uuid.New(), IP: "10.0.0.1"}, time.Second, 0, []int{2222})

	if result.NetworkReachable != polling.TriStateTrue {
		t.Fatalf("NetworkReachable = %q, want true", result.NetworkReachable)
	}
	if !reflect.DeepEqual(capturedPorts, []int{2222}) {
		t.Fatalf("probe ports = %v, want [2222]", capturedPorts)
	}
}

func TestEssentialCollectorConnectFailureProducesFailedResult(t *testing.T) {
	deviceID := uuid.New()
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}

	collector := NewEssentialCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return &scriptedEssentialClient{connectErr: assertiveError("timeout")}, nil
	})
	probeCalls := 0
	collector.networkProbe = func(_ context.Context, target string, timeout time.Duration, _ []int) error {
		probeCalls++
		if target != "10.0.0.1" {
			t.Fatalf("target = %q, want 10.0.0.1", target)
		}
		if timeout != time.Second {
			t.Fatalf("timeout = %s, want 1s", timeout)
		}
		return assertiveError("tcp probe failed")
	}

	result := collector.Poll(context.Background(), domain.Device{ID: deviceID, IP: "10.0.0.1"}, time.Second, 0, nil)

	if result.DeviceID != deviceID {
		t.Fatalf("DeviceID = %s, want %s", result.DeviceID, deviceID)
	}
	if result.PollStatus != polling.PollStatusFailed {
		t.Fatalf("PollStatus = %q, want failed", result.PollStatus)
	}
	if result.SNMPReachable != polling.TriStateFalse {
		t.Fatalf("SNMPReachable = %q, want false", result.SNMPReachable)
	}
	if result.NetworkReachable != polling.TriStateFalse {
		t.Fatalf("NetworkReachable = %q, want false", result.NetworkReachable)
	}
	if probeCalls != 1 {
		t.Fatalf("network probe calls = %d, want 1", probeCalls)
	}
}

func TestEssentialCollectorSysUpTimeFailureRecordsUnreachableNetworkProbe(t *testing.T) {
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}
	client := &scriptedEssentialClient{
		getErrs: map[string]error{
			snmp.OidSysUpTime: assertiveError("request timeout"),
		},
	}
	collector := NewEssentialCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return client, nil
	})
	collector.networkProbe = func(context.Context, string, time.Duration, []int) error {
		return assertiveError("tcp probe failed")
	}

	result := collector.Poll(context.Background(), domain.Device{ID: uuid.New(), IP: "10.0.0.2"}, time.Second, 0, nil)

	if result.Err == nil {
		t.Fatal("Err = nil, want sysUpTime failure")
	}
	if result.PollStatus != polling.PollStatusFailed {
		t.Fatalf("PollStatus = %q, want failed", result.PollStatus)
	}
	if result.SNMPReachable != polling.TriStateFalse {
		t.Fatalf("SNMPReachable = %q, want false", result.SNMPReachable)
	}
	if result.NetworkReachable != polling.TriStateFalse {
		t.Fatalf("NetworkReachable = %q, want false", result.NetworkReachable)
	}
	if result.Uptime != polling.FieldStateError {
		t.Fatalf("Uptime = %q, want error", result.Uptime)
	}
}

func TestEssentialCollectorSysUpTimeFailureRecordsReachableNetworkProbe(t *testing.T) {
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}
	client := &scriptedEssentialClient{
		getErrs: map[string]error{
			snmp.OidSysUpTime: assertiveError("request timeout"),
		},
	}
	collector := NewEssentialCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return client, nil
	})
	collector.networkProbe = func(_ context.Context, target string, timeout time.Duration, _ []int) error {
		if target != "10.0.0.3" {
			t.Fatalf("target = %q, want 10.0.0.3", target)
		}
		if timeout != time.Second {
			t.Fatalf("timeout = %s, want 1s", timeout)
		}
		return nil
	}

	result := collector.Poll(context.Background(), domain.Device{ID: uuid.New(), IP: "10.0.0.3"}, time.Second, 0, nil)

	if result.Err == nil {
		t.Fatal("Err = nil, want sysUpTime failure")
	}
	if result.PollStatus != polling.PollStatusFailed {
		t.Fatalf("PollStatus = %q, want failed", result.PollStatus)
	}
	if result.NetworkReachable != polling.TriStateTrue {
		t.Fatalf("NetworkReachable = %q, want true", result.NetworkReachable)
	}
	if result.SNMPReachable != polling.TriStateFalse {
		t.Fatalf("SNMPReachable = %q, want false", result.SNMPReachable)
	}
}

func TestEssentialCollectorCompleteResult(t *testing.T) {
	registry := essentialTestRegistry(t, vendor.PerformanceOIDs{
		CPUOID:         ".1.3.6.1.4.1.9999.1.0",
		MemoryUsedOID:  ".1.3.6.1.4.1.9999.2.0",
		MemoryTotalOID: ".1.3.6.1.4.1.9999.3.0",
	})
	deviceID := uuid.New()
	client := &scriptedEssentialClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysUpTime: {
				{Name: snmp.OidSysUpTime, Value: uint32(4500)},
			},
			".1.3.6.1.4.1.9999.1.0": {
				{Name: ".1.3.6.1.4.1.9999.1.0", Value: int(12)},
			},
			".1.3.6.1.4.1.9999.2.0": {
				{Name: ".1.3.6.1.4.1.9999.2.0", Value: int(30)},
			},
			".1.3.6.1.4.1.9999.3.0": {
				{Name: ".1.3.6.1.4.1.9999.3.0", Value: int(60)},
			},
		},
	}
	collector := NewEssentialCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return client, nil
	})
	collector.networkProbe = func(context.Context, string, time.Duration, []int) error {
		return nil
	}

	result := collector.Poll(context.Background(), domain.Device{ID: deviceID, IP: "10.0.0.2"}, time.Second, 0, nil)

	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if result.PollStatus != polling.PollStatusComplete {
		t.Fatalf("PollStatus = %q, want complete", result.PollStatus)
	}
	if result.NetworkReachable != polling.TriStateTrue || result.SNMPReachable != polling.TriStateTrue {
		t.Fatalf("reachability = network %q snmp %q, want true/true", result.NetworkReachable, result.SNMPReachable)
	}
	if result.Uptime != polling.FieldStateOK || result.CPU != polling.FieldStateOK || result.Memory != polling.FieldStateOK {
		t.Fatalf("field states = uptime %q cpu %q memory %q, want all ok", result.Uptime, result.CPU, result.Memory)
	}
	assertFloatPtrEqual(t, result.UptimeSecs, 45, "UptimeSecs")
	assertFloatPtrEqual(t, result.CPUPercent, 12, "CPUPercent")
	assertFloatPtrEqual(t, result.MemPercent, 50, "MemPercent")
	if client.connects != 1 || client.closes != 1 {
		t.Fatalf("connects/closes = %d/%d, want 1/1", client.connects, client.closes)
	}
	if len(client.bulkWalkCalls) != 0 {
		t.Fatalf("BulkWalk calls = %v, want none", client.bulkWalkCalls)
	}
}

func TestEssentialCollectorPartialWhenDefaultRootsAreMissing(t *testing.T) {
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}
	client := &scriptedEssentialClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysUpTime: {
				{Name: snmp.OidSysUpTime, Value: uint32(12000)},
			},
		},
	}
	collector := NewEssentialCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return client, nil
	})
	collector.networkProbe = func(context.Context, string, time.Duration, []int) error {
		return nil
	}

	result := collector.Poll(context.Background(), domain.Device{ID: uuid.New(), IP: "10.0.0.3"}, time.Second, 0, nil)

	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if result.PollStatus != polling.PollStatusPartial {
		t.Fatalf("PollStatus = %q, want partial", result.PollStatus)
	}
	if result.Uptime != polling.FieldStateOK || result.CPU != polling.FieldStateMissing || result.Memory != polling.FieldStateMissing {
		t.Fatalf("field states = uptime %q cpu %q memory %q, want ok/missing/missing", result.Uptime, result.CPU, result.Memory)
	}
	if result.CPUPercent != nil || result.MemPercent != nil {
		t.Fatalf("CPUPercent/MemPercent = %v/%v, want nil/nil", result.CPUPercent, result.MemPercent)
	}
	if len(client.bulkWalkCalls) != 0 {
		t.Fatalf("BulkWalk calls = %v, want none", client.bulkWalkCalls)
	}
}

func TestEssentialCollectorPartialWhenMemoryErrors(t *testing.T) {
	registry := essentialTestRegistry(t, vendor.PerformanceOIDs{
		CPUOID:         ".1.3.6.1.4.1.9999.1.0",
		MemoryUsedOID:  ".1.3.6.1.4.1.9999.2.0",
		MemoryTotalOID: ".1.3.6.1.4.1.9999.3.0",
	})
	client := &scriptedEssentialClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysUpTime: {
				{Name: snmp.OidSysUpTime, Value: uint32(12000)},
			},
			".1.3.6.1.4.1.9999.1.0": {
				{Name: ".1.3.6.1.4.1.9999.1.0", Value: int(12)},
			},
		},
		getErrs: map[string]error{
			".1.3.6.1.4.1.9999.2.0": assertiveError("memory timeout"),
		},
	}
	collector := NewEssentialCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return client, nil
	})
	collector.networkProbe = func(context.Context, string, time.Duration, []int) error {
		return nil
	}

	result := collector.Poll(context.Background(), domain.Device{ID: uuid.New(), IP: "10.0.0.4"}, time.Second, 0, nil)

	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if result.PollStatus != polling.PollStatusPartial {
		t.Fatalf("PollStatus = %q, want partial", result.PollStatus)
	}
	if result.Uptime != polling.FieldStateOK || result.CPU != polling.FieldStateOK || result.Memory != polling.FieldStateError {
		t.Fatalf("field states = uptime %q cpu %q memory %q, want ok/ok/error", result.Uptime, result.CPU, result.Memory)
	}
	assertFloatPtrEqual(t, result.UptimeSecs, 120, "UptimeSecs")
	assertFloatPtrEqual(t, result.CPUPercent, 12, "CPUPercent")
	if result.MemPercent != nil {
		t.Fatalf("MemPercent = %v, want nil", *result.MemPercent)
	}
}

func TestEssentialCollectorSuccessfulPollUsesConfiguredNetworkResult(t *testing.T) {
	registry := essentialTestRegistry(t, vendor.PerformanceOIDs{
		CPUOID:         ".1.3.6.1.4.1.9999.1.0",
		MemoryUsedOID:  ".1.3.6.1.4.1.9999.2.0",
		MemoryTotalOID: ".1.3.6.1.4.1.9999.3.0",
	})
	deviceID := uuid.New()
	client := &scriptedEssentialClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysUpTime: {
				{Name: snmp.OidSysUpTime, Value: uint32(4500)},
			},
			".1.3.6.1.4.1.9999.1.0": {
				{Name: ".1.3.6.1.4.1.9999.1.0", Value: int(12)},
			},
			".1.3.6.1.4.1.9999.2.0": {
				{Name: ".1.3.6.1.4.1.9999.2.0", Value: int(30)},
			},
			".1.3.6.1.4.1.9999.3.0": {
				{Name: ".1.3.6.1.4.1.9999.3.0", Value: int(60)},
			},
		},
	}
	collector := NewEssentialCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return client, nil
	})
	collector.networkProbe = func(context.Context, string, time.Duration, []int) error {
		return assertiveError("tcp probe failed")
	}

	result := collector.Poll(context.Background(), domain.Device{ID: deviceID, IP: "10.0.0.2"}, time.Second, 0, []int{26})

	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if result.SNMPReachable != polling.TriStateTrue {
		t.Fatalf("SNMPReachable = %q, want true", result.SNMPReachable)
	}
	if result.NetworkReachable != polling.TriStateFalse {
		t.Fatalf("NetworkReachable = %q, want false", result.NetworkReachable)
	}
}

func essentialTestRegistry(t *testing.T, perfOIDs vendor.PerformanceOIDs) *vendor.Registry {
	t.Helper()

	cfg := vendor.VendorConfig{
		Vendor: vendor.VendorInfo{Name: "default", DisplayName: "Default"},
		SNMP: vendor.SNMPConfig{
			Performance: perfOIDs,
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal registry config error = %v", err)
	}
	registry, err := vendor.LoadRegistryFromDB([]vendor.DBVendorRecord{{
		Name:       "default",
		ConfigJSON: string(data),
	}})
	if err != nil {
		t.Fatalf("LoadRegistryFromDB() error = %v", err)
	}
	return registry
}
