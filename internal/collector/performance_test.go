package collector

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

func captureCollectorDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	logging.Configure("debug")
	t.Cleanup(func() {
		logging.Configure("info")
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})
	return &buf
}

type scriptedPerformanceClient struct {
	getResponses      map[string][]gosnmp.SnmpPDU
	bulkWalkResponses map[string][]gosnmp.SnmpPDU
	getErr            error
	bulkWalkErr       error
	bulkWalkCalls     []string
	connectErr        error
	connectCalls      int
	closeCalls        int
}

func (c *scriptedPerformanceClient) Get(oids []string) ([]gosnmp.SnmpPDU, error) {
	if c.getErr != nil {
		return nil, c.getErr
	}
	var pdus []gosnmp.SnmpPDU
	for _, oid := range oids {
		pdus = append(pdus, c.getResponses[oid]...)
	}
	return pdus, nil
}

func (c *scriptedPerformanceClient) BulkWalk(rootOID string) ([]gosnmp.SnmpPDU, error) {
	c.bulkWalkCalls = append(c.bulkWalkCalls, rootOID)
	if c.bulkWalkErr != nil {
		return nil, c.bulkWalkErr
	}
	return c.bulkWalkResponses[rootOID], nil
}

func (c *scriptedPerformanceClient) Connect() error {
	c.connectCalls++
	return c.connectErr
}

func (c *scriptedPerformanceClient) Close() error {
	c.closeCalls++
	return nil
}

func TestPerformanceCollectorPoll(t *testing.T) {
	t.Parallel()

	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}

	collectedAt := time.Date(2026, 4, 12, 14, 10, 0, 0, time.FixedZone("plus2", 2*60*60))
	timeout := 7 * time.Second
	retries := 3

	type ctorCall struct {
		target  string
		creds   domain.SNMPCredentials
		timeout time.Duration
		retries int
	}

	tests := []struct {
		name      string
		device    domain.Device
		newClient func() *scriptedPerformanceClient
		assert    func(t *testing.T, result PerformanceResult, client *scriptedPerformanceClient, calls []ctorCall)
	}{
		{
			name: "happy path returns metrics and counters",
			device: domain.Device{
				ID:     uuid.New(),
				IP:     "192.0.2.10",
				Vendor: "",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
				Interfaces: []domain.Interface{
					{IfName: "ether1", Speed: 1_000_000_000},
					{IfName: "port-2", IfDescr: "Ethernet2", Speed: 2_000_000_000},
				},
			},
			newClient: func() *scriptedPerformanceClient {
				return newMetricsClient(registry, true)
			},
			assert: func(t *testing.T, result PerformanceResult, client *scriptedPerformanceClient, calls []ctorCall) {
				t.Helper()

				if len(calls) != 1 {
					t.Fatalf("newClient calls = %d, want 1", len(calls))
				}
				if calls[0].target != "192.0.2.10" {
					t.Fatalf("target = %q, want %q", calls[0].target, "192.0.2.10")
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
				assertFloatPtrEqual(t, result.Metrics.CPUPercent, 30, "CPUPercent")
				assertFloatPtrEqual(t, result.Metrics.MemPercent, 50, "MemPercent")
				assertFloatPtrEqual(t, result.Metrics.TempCelsius, 48, "TempCelsius")
				assertFloatPtrEqual(t, result.Metrics.UptimeSecs, 1234, "UptimeSecs")
				if len(result.Counters) != 3 {
					t.Fatalf("counter count = %d, want 3", len(result.Counters))
				}
				countersByName := make(map[string]InterfaceCounterSnapshot, len(result.Counters))
				for _, counter := range result.Counters {
					countersByName[counter.IfName] = counter
				}
				if countersByName["ether1"].SpeedBps != 1_000_000_000 {
					t.Fatalf("ether1 SpeedBps = %d, want %d", countersByName["ether1"].SpeedBps, int64(1_000_000_000))
				}
				if countersByName["Ethernet2"].SpeedBps != 2_000_000_000 {
					t.Fatalf("Ethernet2 SpeedBps = %d, want %d", countersByName["Ethernet2"].SpeedBps, int64(2_000_000_000))
				}
				if countersByName["loopback0"].SpeedBps != 0 {
					t.Fatalf("loopback0 SpeedBps = %d, want 0", countersByName["loopback0"].SpeedBps)
				}
				if !isZeroPrometheusEnrichment(result.Enrichment) {
					t.Fatalf("Enrichment = %#v, want zero value", result.Enrichment)
				}
			},
		},
		{
			name: "partial results leave missing metrics nil",
			device: domain.Device{
				ID:     uuid.New(),
				IP:     "192.0.2.11",
				Vendor: "default",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
			newClient: func() *scriptedPerformanceClient {
				return newPartialMetricsClient(registry)
			},
			assert: func(t *testing.T, result PerformanceResult, client *scriptedPerformanceClient, _ []ctorCall) {
				t.Helper()

				if result.Err != nil {
					t.Fatalf("Err = %v, want nil", result.Err)
				}
				if result.Metrics.CPUPercent != nil {
					t.Fatalf("CPUPercent = %v, want nil", *result.Metrics.CPUPercent)
				}
				if result.Metrics.MemPercent != nil {
					t.Fatalf("MemPercent = %v, want nil", *result.Metrics.MemPercent)
				}
				if result.Metrics.TempCelsius != nil {
					t.Fatalf("TempCelsius = %v, want nil", *result.Metrics.TempCelsius)
				}
				assertFloatPtrEqual(t, result.Metrics.UptimeSecs, 1234, "UptimeSecs")
				if client.closeCalls != 1 {
					t.Fatalf("close calls = %d, want 1", client.closeCalls)
				}
			},
		},
		{
			name: "connect failure returns error without fabricated data",
			device: domain.Device{
				ID:     uuid.New(),
				IP:     "192.0.2.12",
				Vendor: "default",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
			newClient: func() *scriptedPerformanceClient {
				return &scriptedPerformanceClient{connectErr: errors.New("connect failed")}
			},
			assert: func(t *testing.T, result PerformanceResult, client *scriptedPerformanceClient, _ []ctorCall) {
				t.Helper()

				if result.Err == nil {
					t.Fatal("expected error")
				}
				if result.Metrics.CPUPercent != nil || result.Metrics.MemPercent != nil || result.Metrics.TempCelsius != nil || result.Metrics.UptimeSecs != nil {
					t.Fatalf("Metrics = %#v, want zero value pointers", result.Metrics)
				}
				if len(result.Counters) != 0 {
					t.Fatalf("counter count = %d, want 0", len(result.Counters))
				}
				if client.closeCalls != 0 {
					t.Fatalf("close calls = %d, want 0", client.closeCalls)
				}
			},
		},
		{
			name: "all SNMP reads failing returns error instead of no data success",
			device: domain.Device{
				ID:     uuid.New(),
				IP:     "192.0.2.14",
				Vendor: "default",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
			newClient: func() *scriptedPerformanceClient {
				return &scriptedPerformanceClient{
					getErr:      errors.New("get timeout"),
					bulkWalkErr: errors.New("walk timeout"),
				}
			},
			assert: func(t *testing.T, result PerformanceResult, client *scriptedPerformanceClient, _ []ctorCall) {
				t.Helper()

				if result.Err == nil {
					t.Fatal("Err = nil, want all-read failure")
				}
				if result.Metrics.CPUPercent != nil || result.Metrics.MemPercent != nil || result.Metrics.TempCelsius != nil || result.Metrics.UptimeSecs != nil {
					t.Fatalf("Metrics = %#v, want zero value pointers", result.Metrics)
				}
				if len(result.Counters) != 0 {
					t.Fatalf("counter count = %d, want 0", len(result.Counters))
				}
				if len(client.bulkWalkCalls) != 0 {
					t.Fatalf("BulkWalk calls = %v, want none after uptime probe failure", client.bulkWalkCalls)
				}
				if client.closeCalls != 1 {
					t.Fatalf("close calls = %d, want 1", client.closeCalls)
				}
			},
		},
		{
			name: "enrichment stays zero value",
			device: domain.Device{
				ID:     uuid.New(),
				IP:     "192.0.2.13",
				Vendor: "default",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
			newClient: func() *scriptedPerformanceClient {
				return newPartialMetricsClient(registry)
			},
			assert: func(t *testing.T, result PerformanceResult, _ *scriptedPerformanceClient, _ []ctorCall) {
				t.Helper()

				if !isZeroPrometheusEnrichment(result.Enrichment) {
					t.Fatalf("Enrichment = %#v, want zero value", result.Enrichment)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.newClient()
			var calls []ctorCall

			collector := NewPerformanceCollector(registry, func(target string, creds domain.SNMPCredentials, gotTimeout time.Duration, gotRetries int) (SNMPClient, error) {
				calls = append(calls, ctorCall{
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
			if result.Metrics.DeviceID != tt.device.ID {
				t.Fatalf("Metrics.DeviceID = %s, want %s", result.Metrics.DeviceID, tt.device.ID)
			}
			if !result.Metrics.CollectedAt.Equal(collectedAt.UTC()) {
				t.Fatalf("Metrics.CollectedAt = %s, want %s", result.Metrics.CollectedAt, collectedAt.UTC())
			}

			tt.assert(t, result, client, calls)
		})
	}
}

func TestInstrumentedSNMPBulkWalkClient_DebugLogsFailedOperation(t *testing.T) {
	logs := captureCollectorDebugLogs(t)
	client := instrumentedSNMPBulkWalkClient{
		delegate:  &scriptedPerformanceClient{bulkWalkErr: errors.New("timeout waiting for response")},
		collector: "performance",
	}

	_, err := client.BulkWalk(".1.3.6.1.2.1.2")

	if err == nil {
		t.Fatal("expected bulk walk error")
	}
	output := logs.String()
	if !strings.Contains(output, "DEBUG snmp collector operation collector=performance operation=bulk_walk result=timeout") {
		t.Fatalf("debug output missing failed SNMP operation: %q", output)
	}
	if strings.Contains(output, ".1.3.6.1.2.1.2") {
		t.Fatalf("debug output should not include raw OID: %q", output)
	}
}

func TestPerformanceCollectorPoll_RecordsSysUpTimeEarlyExitMetrics(t *testing.T) {
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}
	metrics := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	client := &scriptedPerformanceClient{
		getErr: errors.New("get timeout"),
	}
	collector := NewPerformanceCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return client, nil
	})

	result := collector.Poll(context.Background(), domain.Device{
		ID:     uuid.New(),
		IP:     "192.0.2.30",
		Vendor: "default",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}, time.Second, 1)

	if result.Err == nil {
		t.Fatal("Err = nil, want sysUpTime probe failure")
	}
	if len(client.bulkWalkCalls) != 0 {
		t.Fatalf("BulkWalk calls = %v, want none after sysUpTime early exit", client.bulkWalkCalls)
	}

	body := string(metrics.MarshalPrometheus())
	assertContainsCollectorMetric(t, body, `theia_snmp_collector_operations_total{collector="performance",operation="sysuptime_probe",result="timeout"} 1`)
	assertContainsCollectorMetric(t, body, `theia_snmp_collector_operation_seconds_count{collector="performance",operation="sysuptime_probe",result="timeout"} 1`)
	assertContainsCollectorMetric(t, body, `theia_snmp_collector_early_exit_total{collector="performance",reason="sysuptime_probe_failed"} 1`)
	if strings.Contains(body, `operation="bulk_walk"`) {
		t.Fatalf("metrics output unexpectedly recorded bulk_walk after sysUpTime early exit:\n%s", body)
	}
}

func TestPerformanceCollectorPoll_RecordsBulkWalkMetricsAfterSysUpTimeSuccess(t *testing.T) {
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}
	metrics := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	client := newMetricsClient(registry, true)
	collector := NewPerformanceCollector(registry, func(string, domain.SNMPCredentials, time.Duration, int) (SNMPClient, error) {
		return client, nil
	})

	result := collector.Poll(context.Background(), domain.Device{
		ID:     uuid.New(),
		IP:     "192.0.2.31",
		Vendor: "default",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}, time.Second, 1)

	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if len(client.bulkWalkCalls) == 0 {
		t.Fatal("BulkWalk calls = 0, want full performance poll attempts")
	}

	body := string(metrics.MarshalPrometheus())
	assertContainsCollectorMetric(t, body, `theia_snmp_collector_operations_total{collector="performance",operation="sysuptime_probe",result="success"} 1`)
	assertContainsCollectorMetric(t, body, `theia_snmp_collector_operation_seconds_count{collector="performance",operation="sysuptime_probe",result="success"} 1`)
	if !strings.Contains(body, `theia_snmp_collector_operations_total{collector="performance",operation="bulk_walk",result="success"}`) {
		t.Fatalf("metrics output missing successful bulk_walk counter\n%s", body)
	}
	if !strings.Contains(body, `theia_snmp_collector_operation_seconds_count{collector="performance",operation="bulk_walk",result="success"}`) {
		t.Fatalf("metrics output missing successful bulk_walk histogram\n%s", body)
	}
}

func assertContainsCollectorMetric(t *testing.T, body, needle string) {
	t.Helper()
	if !strings.Contains(body, needle) {
		t.Fatalf("metrics output missing %q\n%s", needle, body)
	}
}

func newMetricsClient(registry *vendor.Registry, includeTemperature bool) *scriptedPerformanceClient {
	perfOIDs := registry.ResolvePerformanceOIDs("default")
	cpuOID := perfOIDs.CPUOID
	if cpuOID == "" {
		cpuOID = snmp.OidHrProcessorLoad
	}

	client := &scriptedPerformanceClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysUpTime: {
				{Name: snmp.OidSysUpTime, Value: uint32(123400)},
			},
		},
		bulkWalkResponses: map[string][]gosnmp.SnmpPDU{
			cpuOID: {
				{Name: cpuOID + ".1", Value: int(20)},
				{Name: cpuOID + ".2", Value: int(40)},
			},
			snmp.OidHrStorageType: {
				{Name: snmp.OidHrStorageType + ".1", Value: snmp.OidHrStorageRam},
			},
			snmp.OidHrStorageAllocUnits: {
				{Name: snmp.OidHrStorageAllocUnits + ".1", Value: int64(1)},
			},
			snmp.OidHrStorageSize: {
				{Name: snmp.OidHrStorageSize + ".1", Value: int64(200)},
			},
			snmp.OidHrStorageUsed: {
				{Name: snmp.OidHrStorageUsed + ".1", Value: int64(100)},
			},
			snmp.OidIfName: {
				{Name: snmp.OidIfName + ".1", Value: "ether1"},
				{Name: snmp.OidIfName + ".2", Value: "Ethernet2"},
				{Name: snmp.OidIfName + ".3", Value: "loopback0"},
			},
			snmp.OidIfHCInOctets: {
				{Name: snmp.OidIfHCInOctets + ".1", Value: uint64(111)},
				{Name: snmp.OidIfHCInOctets + ".2", Value: uint64(222)},
				{Name: snmp.OidIfHCInOctets + ".3", Value: uint64(333)},
			},
			snmp.OidIfHCOutOctets: {
				{Name: snmp.OidIfHCOutOctets + ".1", Value: uint64(444)},
				{Name: snmp.OidIfHCOutOctets + ".2", Value: uint64(555)},
				{Name: snmp.OidIfHCOutOctets + ".3", Value: uint64(666)},
			},
		},
	}

	if includeTemperature {
		client.bulkWalkResponses[snmp.OidEntPhySensorType] = []gosnmp.SnmpPDU{
			{Name: snmp.OidEntPhySensorType + ".1", Value: int64(8)},
		}
		client.bulkWalkResponses[snmp.OidEntPhySensorValue] = []gosnmp.SnmpPDU{
			{Name: snmp.OidEntPhySensorValue + ".1", Value: int64(48)},
		}
	}

	return client
}

func newPartialMetricsClient(registry *vendor.Registry) *scriptedPerformanceClient {
	client := newMetricsClient(registry, false)
	delete(client.bulkWalkResponses, snmp.OidHrStorageType)
	delete(client.bulkWalkResponses, snmp.OidHrStorageAllocUnits)
	delete(client.bulkWalkResponses, snmp.OidHrStorageSize)
	delete(client.bulkWalkResponses, snmp.OidHrStorageUsed)

	perfOIDs := registry.ResolvePerformanceOIDs("default")
	cpuOID := perfOIDs.CPUOID
	if cpuOID == "" {
		cpuOID = snmp.OidHrProcessorLoad
	}
	delete(client.bulkWalkResponses, cpuOID)

	return client
}

func isZeroPrometheusEnrichment(enrichment PrometheusEnrichment) bool {
	return enrichment.DeviceID == uuid.Nil &&
		enrichment.Hostname == "" &&
		enrichment.ProbeReachable == nil &&
		len(enrichment.Alerts) == 0 &&
		enrichment.CollectedAt.IsZero() &&
		enrichment.Error == ""
}
