package snmp

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/vendor"
)

// testDiscoveryRegistry creates a vendor registry suitable for discovery tests.
// Reuses the helper from detector_test.go (same package).
var testDiscoveryRegistry = testRegistry

func TestEmbeddedDefaultCPUOIDUsesHostResourcesProcessorLoad(t *testing.T) {
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}

	perfOIDs := registry.ResolvePerformanceOIDs("default")
	if OidHrProcessorLoad != ".1.3.6.1.2.1.25.3.3.1.2" {
		t.Fatalf("OidHrProcessorLoad = %q, want HOST-RESOURCES-MIB hrProcessorLoad", OidHrProcessorLoad)
	}
	if perfOIDs.CPUOID != OidHrProcessorLoad {
		t.Fatalf("embedded default cpu_oid = %q, want %q", perfOIDs.CPUOID, OidHrProcessorLoad)
	}
}

// MockClient implements ClientInterface for testing
type MockClient struct {
	GetFunc      func(oids []string) ([]gosnmp.SnmpPDU, error)
	BulkWalkFunc func(rootOid string) ([]gosnmp.SnmpPDU, error)
}

func (m *MockClient) Get(oids []string) ([]gosnmp.SnmpPDU, error) {
	if m.GetFunc != nil {
		return m.GetFunc(oids)
	}
	return nil, nil
}

func (m *MockClient) BulkWalk(rootOid string) ([]gosnmp.SnmpPDU, error) {
	if m.BulkWalkFunc != nil {
		return m.BulkWalkFunc(rootOid)
	}
	return nil, nil
}

func TestPollEssentialMetricsDoesNotWalkTemperatureOrCounters(t *testing.T) {
	t.Parallel()

	var getOIDs []string
	var walkedRoots []string
	client := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			getOIDs = append(getOIDs, oids...)
			return []gosnmp.SnmpPDU{
				{Name: OidSysUpTime, Type: gosnmp.TimeTicks, Value: uint32(12000)},
			}, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			walkedRoots = append(walkedRoots, rootOid)
			return nil, nil
		},
	}

	result := PollEssentialMetrics(client, vendor.PerformanceOIDs{
		CPUOID:           ".1.3.6.1.4.1.9999.1.0",
		MemoryUsedOID:    ".1.3.6.1.4.1.9999.2.0",
		MemoryTotalOID:   ".1.3.6.1.4.1.9999.3.0",
		TemperatureOID:   ".1.3.6.1.4.1.9999.4.0",
		TemperatureScale: 1,
	})

	if result.Uptime == nil || result.Uptime.State != "ok" {
		t.Fatalf("uptime = %#v, want ok", result.Uptime)
	}
	for _, oid := range getOIDs {
		if oid == ".1.3.6.1.4.1.9999.4.0" {
			t.Fatalf("essential helper requested temperature oid %q", oid)
		}
	}
	if len(walkedRoots) != 0 {
		t.Fatalf("essential helper walked roots %v, want none", walkedRoots)
	}
}

func TestPollEssentialMetricsScalarOIDsCanComplete(t *testing.T) {
	var walkedRoots []string
	client := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			var pdus []gosnmp.SnmpPDU
			for _, oid := range oids {
				switch oid {
				case OidSysUpTime:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysUpTime, Type: gosnmp.TimeTicks, Value: uint32(12300)})
				case ".1.3.6.1.4.1.9999.1.0":
					pdus = append(pdus, gosnmp.SnmpPDU{Name: ".1.3.6.1.4.1.9999.1.0", Type: gosnmp.Integer, Value: 25})
				case ".1.3.6.1.4.1.9999.2.7":
					pdus = append(pdus, gosnmp.SnmpPDU{Name: ".1.3.6.1.4.1.9999.2.7", Type: gosnmp.Integer, Value: 40})
				case ".1.3.6.1.4.1.9999.3.7":
					pdus = append(pdus, gosnmp.SnmpPDU{Name: ".1.3.6.1.4.1.9999.3.7", Type: gosnmp.Integer, Value: 80})
				}
			}
			return pdus, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			walkedRoots = append(walkedRoots, rootOid)
			return nil, nil
		},
	}

	result := PollEssentialMetrics(client, vendor.PerformanceOIDs{
		CPUOID:         ".1.3.6.1.4.1.9999.1.0",
		MemoryUsedOID:  ".1.3.6.1.4.1.9999.2.7",
		MemoryTotalOID: ".1.3.6.1.4.1.9999.3.7",
	})

	assertEssentialField(t, result.Uptime, "ok", 123)
	assertEssentialField(t, result.CPU, "ok", 25)
	assertEssentialField(t, result.Memory, "ok", 50)
	if len(walkedRoots) != 0 {
		t.Fatalf("essential helper walked roots %v, want none", walkedRoots)
	}
}

func TestPollEssentialMetricsDefaultColumnRootsStayMissingWithoutWalks(t *testing.T) {
	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}

	var getOIDs []string
	var walkedRoots []string
	client := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			getOIDs = append(getOIDs, oids...)
			return []gosnmp.SnmpPDU{
				{Name: OidSysUpTime, Type: gosnmp.TimeTicks, Value: uint32(9000)},
			}, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			walkedRoots = append(walkedRoots, rootOid)
			return nil, nil
		},
	}

	result := PollEssentialMetrics(client, registry.ResolvePerformanceOIDs("default"))

	assertEssentialField(t, result.Uptime, "ok", 90)
	assertEssentialField(t, result.CPU, "missing", 0)
	assertEssentialField(t, result.Memory, "missing", 0)
	if len(walkedRoots) != 0 {
		t.Fatalf("essential helper walked roots %v, want none", walkedRoots)
	}
	for _, oid := range getOIDs {
		switch oid {
		case OidHrProcessorLoad, OidHrStorageUsed, OidHrStorageSize:
			t.Fatalf("essential helper requested table column root %q", oid)
		}
	}
}

func TestPollEssentialMetricsMemoryGetErrorIsPreserved(t *testing.T) {
	client := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			if len(oids) == 2 {
				return nil, assertiveError("memory timeout")
			}
			return []gosnmp.SnmpPDU{
				{Name: OidSysUpTime, Type: gosnmp.TimeTicks, Value: uint32(9000)},
			}, nil
		},
	}

	result := PollEssentialMetrics(client, vendor.PerformanceOIDs{
		MemoryUsedOID:  ".1.3.6.1.4.1.9999.2.0",
		MemoryTotalOID: ".1.3.6.1.4.1.9999.3.0",
	})

	if result.Memory == nil || result.Memory.State != "error" || result.Memory.Error != "memory timeout" {
		t.Fatalf("memory = %#v, want error with message", result.Memory)
	}
}

func assertEssentialField(t *testing.T, field *EssentialMetricField, wantState string, wantValue float64) {
	t.Helper()
	if field == nil {
		t.Fatalf("field = nil, want state %q", wantState)
	}
	if field.State != wantState {
		t.Fatalf("field state = %q, want %q", field.State, wantState)
	}
	if wantState != "ok" {
		if field.Value != nil {
			t.Fatalf("field value = %v, want nil", *field.Value)
		}
		return
	}
	if field.Value == nil || *field.Value != wantValue {
		t.Fatalf("field value = %v, want %v", field.Value, wantValue)
	}
}

func TestPollOperationalStatus_Success(t *testing.T) {
	t.Parallel()

	client := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			if len(oids) != 1 || oids[0] != ".1.3.6.1.4.1.9999.1.0" {
				t.Fatalf("Get oids = %v, want [%q]", oids, ".1.3.6.1.4.1.9999.1.0")
			}
			return []gosnmp.SnmpPDU{
				{Name: ".1.3.6.1.4.1.9999.1.0", Type: gosnmp.TimeTicks, Value: uint32(32100)},
			}, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidIfName:
				return []gosnmp.SnmpPDU{
					{Name: OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
					{Name: OidIfName + ".2", Type: gosnmp.OctetString, Value: []byte("ether2")},
				}, nil
			case ".1.3.6.1.4.1.9999.2":
				return []gosnmp.SnmpPDU{
					{Name: ".1.3.6.1.4.1.9999.2.1", Type: gosnmp.Integer, Value: 1},
					{Name: ".1.3.6.1.4.1.9999.2.2", Type: gosnmp.Integer, Value: 2},
				}, nil
			default:
				return nil, nil
			}
		},
	}

	uptimeSecs, statuses, err := PollOperationalStatus(client, vendor.OperationalOIDs{
		SysUpTimeOID:    ".1.3.6.1.4.1.9999.1.0",
		IfOperStatusOID: ".1.3.6.1.4.1.9999.2",
	})
	if err != nil {
		t.Fatalf("PollOperationalStatus() error = %v", err)
	}
	if uptimeSecs == nil || *uptimeSecs != 321 {
		t.Fatalf("uptimeSecs = %v, want 321", uptimeSecs)
	}
	if len(statuses) != 2 {
		t.Fatalf("status count = %d, want 2", len(statuses))
	}
	if statuses["ether1"] != "up" {
		t.Fatalf("statuses[ether1] = %q, want %q", statuses["ether1"], "up")
	}
	if statuses["ether2"] != "down" {
		t.Fatalf("statuses[ether2] = %q, want %q", statuses["ether2"], "down")
	}
}

func TestPollOperationalStatus_UsesFallbackOIDsWhenEmpty(t *testing.T) {
	t.Parallel()

	var gotGetOIDs []string
	var walked []string

	client := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			gotGetOIDs = append([]string(nil), oids...)
			return []gosnmp.SnmpPDU{
				{Name: OidSysUpTime, Type: gosnmp.TimeTicks, Value: uint32(12300)},
			}, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			walked = append(walked, rootOid)
			switch rootOid {
			case OidIfName:
				return nil, nil
			case OidIfDescr:
				return []gosnmp.SnmpPDU{
					{Name: OidIfDescr + ".7", Type: gosnmp.OctetString, Value: []byte("port7")},
				}, nil
			case OidIfOperStatus:
				return []gosnmp.SnmpPDU{
					{Name: OidIfOperStatus + ".7", Type: gosnmp.Integer, Value: 3},
				}, nil
			default:
				return nil, nil
			}
		},
	}

	uptimeSecs, statuses, err := PollOperationalStatus(client, vendor.OperationalOIDs{})
	if err != nil {
		t.Fatalf("PollOperationalStatus() error = %v", err)
	}
	if len(gotGetOIDs) != 1 || gotGetOIDs[0] != OidSysUpTime {
		t.Fatalf("Get oids = %v, want [%q]", gotGetOIDs, OidSysUpTime)
	}
	if len(walked) != 3 || walked[0] != OidIfName || walked[1] != OidIfDescr || walked[2] != OidIfOperStatus {
		t.Fatalf("walked roots = %v, want [%q %q %q]", walked, OidIfName, OidIfDescr, OidIfOperStatus)
	}
	if uptimeSecs == nil || *uptimeSecs != 123 {
		t.Fatalf("uptimeSecs = %v, want 123", uptimeSecs)
	}
	if len(statuses) != 1 || statuses["port7"] != "testing" {
		t.Fatalf("statuses = %#v, want map[port7:testing]", statuses)
	}
}

func TestPollOperationalStatus_MissingFieldsStayPartial(t *testing.T) {
	t.Parallel()

	client := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			return nil, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidIfName:
				return nil, nil
			case OidIfDescr:
				return []gosnmp.SnmpPDU{
					{Name: OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("uplink")},
				}, nil
			case OidIfOperStatus:
				return []gosnmp.SnmpPDU{
					{Name: OidIfOperStatus + ".1", Type: gosnmp.Integer, Value: 1},
					{Name: OidIfOperStatus + ".99", Type: gosnmp.Integer, Value: 2},
				}, nil
			default:
				return nil, nil
			}
		},
	}

	uptimeSecs, statuses, err := PollOperationalStatus(client, vendor.OperationalOIDs{})
	if err != nil {
		t.Fatalf("PollOperationalStatus() error = %v", err)
	}
	if uptimeSecs != nil {
		t.Fatalf("uptimeSecs = %v, want nil", *uptimeSecs)
	}
	if len(statuses) != 1 {
		t.Fatalf("status count = %d, want 1", len(statuses))
	}
	if statuses["uplink"] != "up" {
		t.Fatalf("statuses[uplink] = %q, want %q", statuses["uplink"], "up")
	}
}

func TestPollOperationalStatus_QueryError(t *testing.T) {
	t.Parallel()

	client := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			return []gosnmp.SnmpPDU{
				{Name: OidSysUpTime, Type: gosnmp.TimeTicks, Value: uint32(100)},
			}, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			if rootOid == OidIfName {
				return nil, nil
			}
			if rootOid == OidIfDescr {
				return []gosnmp.SnmpPDU{
					{Name: OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("port1")},
				}, nil
			}
			if rootOid == OidIfOperStatus {
				return nil, assertiveError("bulk walk failed")
			}
			return nil, nil
		},
	}

	uptimeSecs, statuses, err := PollOperationalStatus(client, vendor.OperationalOIDs{})
	if err == nil {
		t.Fatal("expected error")
	}
	if uptimeSecs != nil {
		t.Fatalf("uptimeSecs = %v, want nil", *uptimeSecs)
	}
	if statuses != nil {
		t.Fatalf("statuses = %#v, want nil", statuses)
	}
}

type assertiveError string

func (e assertiveError) Error() string {
	return string(e)
}

func TestDiscoverDevice(t *testing.T) {
	reg := testDiscoveryRegistry(t)
	mock := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			var pdus []gosnmp.SnmpPDU
			for _, oid := range oids {
				switch oid {
				case OidSysName:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router1")})
				case OidSysDescr:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS RB5009")})
				case OidSysObjectID:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"})
				case ".1.3.6.1.4.1.14988.1.1.4.4.0":
					pdus = append(pdus, gosnmp.SnmpPDU{Name: ".1.3.6.1.4.1.14988.1.1.4.4.0", Type: gosnmp.OctetString, Value: []byte("7.22.1")})
				}
			}
			return pdus, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidIfTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
					{Name: OidIfOperStatus + ".1", Type: gosnmp.Integer, Value: 1}, // up
				}, nil
			case OidIfXTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("eth1")},
					{Name: OidIfHighSpeed + ".1", Type: gosnmp.Gauge32, Value: uint(1000)}, // 1 Gbps
				}, nil
			case OidLLDPLocPortIfIndex:
				// lldpPortNum 1 maps to ifIndex 1 — numbering happens to match on this test device
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPLocPortIfIndex + ".1", Type: gosnmp.Integer, Value: 1},
				}, nil
			case OidLLDPRemChassisId:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemChassisId + ".1000.1.1", Type: gosnmp.OctetString, Value: []byte("00:11:22:33:44:55")},
				}, nil
			case OidLLDPRemPortId:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemPortId + ".1000.1.1", Type: gosnmp.OctetString, Value: []byte("ether2")},
				}, nil
			case OidLLDPRemSysName:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemSysName + ".1000.1.1", Type: gosnmp.OctetString, Value: []byte("switch1")},
				}, nil
			}
			return nil, nil // Empty slice for CDP
		},
	}

	res, err := DiscoverDevice(mock, reg)
	if err != nil {
		t.Fatalf("DiscoverDevice returned error: %v", err)
	}

	if res.SysName != "router1" {
		t.Errorf("expected SysName router1, got %s", res.SysName)
	}
	if res.DeviceType != domain.DeviceTypeRouter {
		t.Errorf("expected DeviceTypeRouter, got %s", res.DeviceType)
	}
	if res.HardwareModel != "RB5009" {
		t.Errorf("expected model RB5009, got %s", res.HardwareModel)
	}
	if res.OSVersion != "7.22.1" {
		t.Errorf("expected OSVersion 7.22.1, got %s", res.OSVersion)
	}

	// Interfaces check
	if len(res.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(res.Interfaces))
	}
	iface := res.Interfaces[0]
	if iface.IfName != "eth1" { // Should be overridden by ifXTable name
		t.Errorf("expected ifName eth1, got %s", iface.IfName)
	}
	if iface.IfDescr != "ether1" {
		t.Errorf("expected ifDescr ether1, got %s", iface.IfDescr)
	}
	if iface.OperStatus != "up" {
		t.Errorf("expected OperStatus up, got %s", iface.OperStatus)
	}
	if iface.Speed != 1000_000_000 {
		t.Errorf("expected speed 1,000,000,000, got %d", iface.Speed)
	}

	// Neighbors check
	if len(res.Neighbors) != 1 {
		t.Fatalf("expected 1 neighbor, got %d", len(res.Neighbors))
	}
	nbr := res.Neighbors[0]
	if nbr.RemoteSysName != "switch1" {
		t.Errorf("expected neighbor sysName switch1, got %s", nbr.RemoteSysName)
	}
	if nbr.Protocol != domain.DiscoveryProtocolLLDP {
		t.Errorf("expected protocol LLDP, got %s", nbr.Protocol)
	}
	// 1000.1.1 means local port number = 1
	if nbr.LocalIfIndex != 1 {
		t.Errorf("expected localIfIndex 1, got %d", nbr.LocalIfIndex)
	}
	if nbr.LocalIfName != "eth1" { // Mapping should work via interfaces check
		t.Errorf("expected localIfName eth1, got %s", nbr.LocalIfName)
	}
}

func TestDiscoverDeviceWithPolicyRecordsCriticalNeighborDiscoveryFailures(t *testing.T) {
	tests := []struct {
		name         string
		policy       NeighborDiscoveryPolicy
		failOID      string
		wantProtocol domain.DiscoveryProtocol
	}{
		{
			name:         "lldp remote chassis failure",
			policy:       NeighborDiscoveryPolicy{LLDP: true},
			failOID:      OidLLDPRemChassisId,
			wantProtocol: domain.DiscoveryProtocolLLDP,
		},
		{
			name:         "cdp device id failure",
			policy:       NeighborDiscoveryPolicy{CDP: true},
			failOID:      OidCDPDeviceID,
			wantProtocol: domain.DiscoveryProtocolCDP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := testDiscoveryRegistry(t)
			mock := &MockClient{
				GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
					var pdus []gosnmp.SnmpPDU
					for _, oid := range oids {
						switch oid {
						case OidSysName:
							pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router1")})
						case OidSysDescr:
							pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS RB5009")})
						case OidSysObjectID:
							pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"})
						}
					}
					return pdus, nil
				},
				BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
					switch rootOid {
					case OidIfTable:
						return []gosnmp.SnmpPDU{
							{Name: OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
							{Name: OidIfOperStatus + ".1", Type: gosnmp.Integer, Value: 1},
						}, nil
					case OidIfXTable:
						return []gosnmp.SnmpPDU{
							{Name: OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
						}, nil
					case tt.failOID:
						return nil, assertiveError("neighbor walk timeout")
					default:
						return nil, nil
					}
				},
			}

			res, err := DiscoverDeviceWithPolicy(mock, reg, tt.policy)
			if err != nil {
				t.Fatalf("DiscoverDeviceWithPolicy returned error: %v", err)
			}
			if res.SysName != "router1" {
				t.Fatalf("SysName = %q, want router1", res.SysName)
			}
			if len(res.Interfaces) != 1 {
				t.Fatalf("interface count = %d, want 1", len(res.Interfaces))
			}
			if len(res.Neighbors) != 0 {
				t.Fatalf("neighbor count = %d, want 0", len(res.Neighbors))
			}

			failure := findNeighborDiscoveryFailure(res.NeighborDiscoveryFailures, tt.wantProtocol, tt.failOID)
			if failure == nil {
				t.Fatalf("expected critical %s failure for %s, got %#v", tt.wantProtocol, tt.failOID, res.NeighborDiscoveryFailures)
			}
			if !failure.Critical {
				t.Fatalf("failure Critical = false, want true")
			}
			if failure.Error != "neighbor walk timeout" {
				t.Fatalf("failure Error = %q, want neighbor walk timeout", failure.Error)
			}
		})
	}
}

func TestDiscoverDeviceWithPolicyRecordsAttemptedNeighborDiscoveryProtocols(t *testing.T) {
	tests := []struct {
		name   string
		policy NeighborDiscoveryPolicy
		want   []domain.DiscoveryProtocol
	}{
		{
			name:   "lldp and cdp",
			policy: NeighborDiscoveryPolicy{LLDP: true, CDP: true},
			want:   []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP, domain.DiscoveryProtocolCDP},
		},
		{
			name:   "lldp only",
			policy: NeighborDiscoveryPolicy{LLDP: true},
			want:   []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		},
		{
			name:   "cdp only",
			policy: NeighborDiscoveryPolicy{CDP: true},
			want:   []domain.DiscoveryProtocol{domain.DiscoveryProtocolCDP},
		},
		{
			name:   "off",
			policy: NeighborDiscoveryPolicy{},
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := DiscoverDeviceWithPolicy(minimalDiscoveryClient(), testDiscoveryRegistry(t), tt.policy)
			if err != nil {
				t.Fatalf("DiscoverDeviceWithPolicy returned error: %v", err)
			}
			if !discoveryProtocolsEqual(res.NeighborDiscoveryProtocols, tt.want) {
				t.Fatalf("NeighborDiscoveryProtocols = %v, want %v", res.NeighborDiscoveryProtocols, tt.want)
			}
		})
	}
}

func findNeighborDiscoveryFailure(failures []NeighborDiscoveryFailure, protocol domain.DiscoveryProtocol, oid string) *NeighborDiscoveryFailure {
	for i := range failures {
		if failures[i].Protocol == protocol && failures[i].OID == oid {
			return &failures[i]
		}
	}
	return nil
}

func discoveryProtocolsEqual(got, want []domain.DiscoveryProtocol) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func minimalDiscoveryClient() ClientInterface {
	return &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			var pdus []gosnmp.SnmpPDU
			for _, oid := range oids {
				switch oid {
				case OidSysName:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router1")})
				case OidSysDescr:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS RB5009")})
				case OidSysObjectID:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"})
				}
			}
			return pdus, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidIfTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
				}, nil
			case OidIfXTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
				}, nil
			default:
				return nil, nil
			}
		},
	}
}

func TestDiscoverSoftwareVersion_AppendsScalarInstanceSuffix(t *testing.T) {
	t.Parallel()

	var getCalls []string
	client := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			if len(oids) != 1 {
				t.Fatalf("Get oids len = %d, want 1", len(oids))
			}
			getCalls = append(getCalls, oids[0])
			if oids[0] == ".1.3.6.1.4.1.14988.1.1.4.4.0" {
				return []gosnmp.SnmpPDU{
					{Name: ".1.3.6.1.4.1.14988.1.1.4.4.0", Type: gosnmp.OctetString, Value: []byte("7.22.1")},
				}, nil
			}
			return nil, assertiveError("no such object")
		},
	}

	got := discoverSoftwareVersion(client, vendor.StaticOIDs{
		SoftwareVersionOID: ".1.3.6.1.4.1.14988.1.1.4.4",
	})
	if got != "7.22.1" {
		t.Fatalf("discoverSoftwareVersion() = %q, want %q", got, "7.22.1")
	}
	if len(getCalls) != 2 || getCalls[0] != ".1.3.6.1.4.1.14988.1.1.4.4" || getCalls[1] != ".1.3.6.1.4.1.14988.1.1.4.4.0" {
		t.Fatalf("Get calls = %v, want fallback to scalar instance OID", getCalls)
	}
}

func TestParseLLDPNeighbors(t *testing.T) {
	reg := testDiscoveryRegistry(t)
	// Tests that LLDP neighbor parsing correctly extracts chassis ID, port ID, and system name
	mock := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			return []gosnmp.SnmpPDU{
				{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("core-sw")},
				{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS RB4011")},
				{Name: OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"},
			}, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidIfTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
					{Name: OidIfDescr + ".2", Type: gosnmp.OctetString, Value: []byte("ether2")},
				}, nil
			case OidIfXTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("eth1")},
					{Name: OidIfName + ".2", Type: gosnmp.OctetString, Value: []byte("eth2")},
				}, nil
			case OidLLDPRemChassisId:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemChassisId + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("AA:BB:CC:DD:EE:01")},
					{Name: OidLLDPRemChassisId + ".0.2.1", Type: gosnmp.OctetString, Value: []byte("AA:BB:CC:DD:EE:02")},
				}, nil
			case OidLLDPRemPortId:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemPortId + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("ge-0/0/0")},
					{Name: OidLLDPRemPortId + ".0.2.1", Type: gosnmp.OctetString, Value: []byte("ge-0/0/1")},
				}, nil
			case OidLLDPRemSysName:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemSysName + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("neighbor-sw1")},
					{Name: OidLLDPRemSysName + ".0.2.1", Type: gosnmp.OctetString, Value: []byte("neighbor-sw2")},
				}, nil
			}
			return nil, nil
		},
	}

	res, err := DiscoverDevice(mock, reg)
	if err != nil {
		t.Fatalf("DiscoverDevice returned error: %v", err)
	}

	// Filter LLDP neighbors
	var lldpNeighbors []NeighborInfo
	for _, n := range res.Neighbors {
		if n.Protocol == domain.DiscoveryProtocolLLDP {
			lldpNeighbors = append(lldpNeighbors, n)
		}
	}

	if len(lldpNeighbors) != 2 {
		t.Fatalf("expected 2 LLDP neighbors, got %d", len(lldpNeighbors))
	}

	// Build lookup by chassis ID
	nbrByChassisID := make(map[string]NeighborInfo)
	for _, n := range lldpNeighbors {
		nbrByChassisID[n.RemoteChassisID] = n
	}

	n1, ok := nbrByChassisID["AA:BB:CC:DD:EE:01"]
	if !ok {
		t.Fatal("neighbor with chassis AA:BB:CC:DD:EE:01 not found")
	}
	if n1.RemotePortID != "ge-0/0/0" {
		t.Errorf("expected RemotePortID ge-0/0/0, got %s", n1.RemotePortID)
	}
	if n1.RemoteSysName != "neighbor-sw1" {
		t.Errorf("expected RemoteSysName neighbor-sw1, got %s", n1.RemoteSysName)
	}
	if n1.LocalIfIndex != 1 {
		t.Errorf("expected LocalIfIndex 1, got %d", n1.LocalIfIndex)
	}

	n2, ok := nbrByChassisID["AA:BB:CC:DD:EE:02"]
	if !ok {
		t.Fatal("neighbor with chassis AA:BB:CC:DD:EE:02 not found")
	}
	if n2.RemotePortID != "ge-0/0/1" {
		t.Errorf("expected RemotePortID ge-0/0/1, got %s", n2.RemotePortID)
	}
	if n2.RemoteSysName != "neighbor-sw2" {
		t.Errorf("expected RemoteSysName neighbor-sw2, got %s", n2.RemoteSysName)
	}
	if n2.LocalIfIndex != 2 {
		t.Errorf("expected LocalIfIndex 2, got %d", n2.LocalIfIndex)
	}
}

func TestParseCDPNeighbors(t *testing.T) {
	reg := testDiscoveryRegistry(t)
	// Tests that CDP neighbor parsing correctly extracts device ID and port
	mock := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			return []gosnmp.SnmpPDU{
				{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("cisco-sw")},
				{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("Cisco IOS Software, Catalyst C2960")},
				{Name: OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.9.1.1208"},
			}, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidIfTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("GigabitEthernet0/1")},
				}, nil
			case OidIfXTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("Gi0/1")},
				}, nil
			case OidLLDPRemChassisId:
				return nil, nil // No LLDP
			case OidLLDPRemPortId:
				return nil, nil
			case OidLLDPRemSysName:
				return nil, nil
			case OidCDPDeviceID:
				return []gosnmp.SnmpPDU{
					{Name: OidCDPDeviceID + ".1.5", Type: gosnmp.OctetString, Value: []byte("remote-cisco-rtr.example.com")},
				}, nil
			case OidCDPPortID:
				return []gosnmp.SnmpPDU{
					{Name: OidCDPPortID + ".1.5", Type: gosnmp.OctetString, Value: []byte("GigabitEthernet0/2")},
				}, nil
			}
			return nil, nil
		},
	}

	res, err := DiscoverDevice(mock, reg)
	if err != nil {
		t.Fatalf("DiscoverDevice returned error: %v", err)
	}

	// Filter CDP neighbors
	var cdpNeighbors []NeighborInfo
	for _, n := range res.Neighbors {
		if n.Protocol == domain.DiscoveryProtocolCDP {
			cdpNeighbors = append(cdpNeighbors, n)
		}
	}

	if len(cdpNeighbors) != 1 {
		t.Fatalf("expected 1 CDP neighbor, got %d", len(cdpNeighbors))
	}

	nbr := cdpNeighbors[0]
	if nbr.RemoteSysName != "remote-cisco-rtr.example.com" {
		t.Errorf("expected CDP device ID remote-cisco-rtr.example.com, got %s", nbr.RemoteSysName)
	}
	if nbr.RemotePortID != "GigabitEthernet0/2" {
		t.Errorf("expected CDP port GigabitEthernet0/2, got %s", nbr.RemotePortID)
	}
	if nbr.LocalIfIndex != 1 {
		t.Errorf("expected LocalIfIndex 1, got %d", nbr.LocalIfIndex)
	}
	if nbr.LocalIfName != "Gi0/1" {
		t.Errorf("expected LocalIfName Gi0/1, got %s", nbr.LocalIfName)
	}
	if nbr.Protocol != domain.DiscoveryProtocolCDP {
		t.Errorf("expected protocol CDP, got %s", nbr.Protocol)
	}
}

func TestDiscoverNeighbors_PrefersLLDPOverCDPForSameConnection(t *testing.T) {
	neighbors := discoverNeighbors(&MockClient{
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidLLDPLocPortIfIndex:
				return []gosnmp.SnmpPDU{{Name: OidLLDPLocPortIfIndex + ".1", Type: gosnmp.Integer, Value: 1}}, nil
			case OidLLDPRemChassisId:
				return []gosnmp.SnmpPDU{{Name: OidLLDPRemChassisId + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("AA:BB:CC:DD:EE:FF")}}, nil
			case OidLLDPRemPortId:
				return []gosnmp.SnmpPDU{{Name: OidLLDPRemPortId + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("ether2")}}, nil
			case OidLLDPRemSysName:
				return []gosnmp.SnmpPDU{{Name: OidLLDPRemSysName + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("switch-b")}}, nil
			case OidCDPDeviceID:
				return []gosnmp.SnmpPDU{{Name: OidCDPDeviceID + ".1.7", Type: gosnmp.OctetString, Value: []byte("switch-b-cdp")}}, nil
			case OidCDPPortID:
				return []gosnmp.SnmpPDU{{Name: OidCDPPortID + ".1.7", Type: gosnmp.OctetString, Value: []byte("ether2")}}, nil
			default:
				return nil, nil
			}
		},
	}, map[int]string{1: "ether1"}, NeighborDiscoveryPolicy{LLDP: true, CDP: true})

	if len(neighbors) != 1 {
		t.Fatalf("expected 1 merged neighbor, got %d", len(neighbors))
	}

	nbr := neighbors[0]
	if nbr.Protocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected LLDP to remain canonical, got %s", nbr.Protocol)
	}
	if nbr.RemoteSysName != "switch-b" {
		t.Fatalf("expected LLDP sysName to win, got %q", nbr.RemoteSysName)
	}
	if nbr.RemotePortID != "ether2" {
		t.Fatalf("expected remote port ether2, got %q", nbr.RemotePortID)
	}
}

func TestDiscoverNeighbors_UsesCDPToFillMissingLLDPFields(t *testing.T) {
	neighbors := discoverNeighbors(&MockClient{
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidLLDPLocPortIfIndex:
				return []gosnmp.SnmpPDU{{Name: OidLLDPLocPortIfIndex + ".1", Type: gosnmp.Integer, Value: 1}}, nil
			case OidLLDPRemChassisId:
				return []gosnmp.SnmpPDU{{Name: OidLLDPRemChassisId + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("AA:BB:CC:DD:EE:FF")}}, nil
			case OidLLDPRemPortId:
				return nil, nil
			case OidLLDPRemSysName:
				return nil, nil
			case OidCDPDeviceID:
				return []gosnmp.SnmpPDU{{Name: OidCDPDeviceID + ".1.7", Type: gosnmp.OctetString, Value: []byte("switch-b")}}, nil
			case OidCDPPortID:
				return []gosnmp.SnmpPDU{{Name: OidCDPPortID + ".1.7", Type: gosnmp.OctetString, Value: []byte("ether2")}}, nil
			default:
				return nil, nil
			}
		},
	}, map[int]string{1: "ether1"}, NeighborDiscoveryPolicy{LLDP: true, CDP: true})

	if len(neighbors) != 1 {
		t.Fatalf("expected 1 merged neighbor, got %d", len(neighbors))
	}

	nbr := neighbors[0]
	if nbr.Protocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected merged neighbor to stay LLDP, got %s", nbr.Protocol)
	}
	if nbr.RemoteSysName != "switch-b" {
		t.Fatalf("expected CDP to fill missing remote sysName, got %q", nbr.RemoteSysName)
	}
	if nbr.RemotePortID != "ether2" {
		t.Fatalf("expected CDP to fill missing remote port, got %q", nbr.RemotePortID)
	}
}

func TestDiscoverNeighbors_PrefersPhysicalInterfacesWhenVirtualVariantsExist(t *testing.T) {
	neighbors := dedupePreferredNeighbors([]NeighborInfo{
		{
			RemoteSysName:   "border-botte",
			RemotePortID:    "VLAN-99-MGMT-ETH6",
			LocalIfName:     "",
			Protocol:        domain.DiscoveryProtocolLLDP,
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
		},
		{
			RemoteSysName:   "border-botte",
			RemotePortID:    "ether6-link_new_apparati",
			LocalIfName:     "ether2-verso-border-botte",
			Protocol:        domain.DiscoveryProtocolLLDP,
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
		},
		{
			RemoteSysName:   "border-botte",
			RemotePortID:    "ether6-link_new_apparati",
			LocalIfName:     "",
			Protocol:        domain.DiscoveryProtocolLLDP,
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
		},
	})

	if len(neighbors) != 1 {
		t.Fatalf("expected 1 preferred neighbor, got %d", len(neighbors))
	}

	nbr := neighbors[0]
	if nbr.LocalIfName != "ether2-verso-border-botte" {
		t.Fatalf("expected physical local interface to win, got %q", nbr.LocalIfName)
	}
	if nbr.RemotePortID != "ether6-link_new_apparati" {
		t.Fatalf("expected physical remote port to win, got %q", nbr.RemotePortID)
	}
}

func TestDiscoverNeighbors_PrefersPhysicalRemotePortWhenLocalInterfaceIsMissingOnAllVariants(t *testing.T) {
	neighbors := dedupePreferredNeighbors([]NeighborInfo{
		{
			RemoteSysName:   "gallitello",
			RemotePortID:    "br_eoip_radius_vlan/eoip_gallitello_uff",
			LocalIfName:     "",
			Protocol:        domain.DiscoveryProtocolLLDP,
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
		},
		{
			RemoteSysName:   "gallitello",
			RemotePortID:    "ether6-Link_Ufficio",
			LocalIfName:     "",
			Protocol:        domain.DiscoveryProtocolLLDP,
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
		},
	})

	if len(neighbors) != 1 {
		t.Fatalf("expected only the physical variant to remain, got %d neighbors", len(neighbors))
	}

	if neighbors[0].RemotePortID != "ether6-Link_Ufficio" {
		t.Fatalf("expected physical remote port to survive, got %q", neighbors[0].RemotePortID)
	}
}

// TestDiscoverDevice_LLDPLocPortIfIndex verifies that when lldpLocPortIfIndex maps
// lldpPortNum 3 to ifIndex 5, and ifIndex 5 has ifName "ether5", the discovered
// neighbor gets LocalIfName == "ether5" and LocalIfIndex == 5.
func TestDiscoverDevice_LLDPLocPortIfIndex(t *testing.T) {
	reg := testDiscoveryRegistry(t)
	mock := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			var pdus []gosnmp.SnmpPDU
			for _, oid := range oids {
				switch oid {
				case OidSysName:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router-lldp")})
				case OidSysDescr:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS RB5009")})
				case OidSysObjectID:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"})
				}
			}
			return pdus, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidIfTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfDescr + ".5", Type: gosnmp.OctetString, Value: []byte("ether5-descr")},
					{Name: OidIfOperStatus + ".5", Type: gosnmp.Integer, Value: 1},
				}, nil
			case OidIfXTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfName + ".5", Type: gosnmp.OctetString, Value: []byte("ether5")},
				}, nil
			case OidLLDPLocPortIfIndex:
				// lldpPortNum 3 maps to ifIndex 5
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPLocPortIfIndex + ".3", Type: gosnmp.Integer, Value: 5},
				}, nil
			case OidLLDPRemChassisId:
				// Index: timeMark=0, localPortNum=3, remoteIndex=1 => "0.3.1"
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemChassisId + ".0.3.1", Type: gosnmp.OctetString, Value: []byte("AA:BB:CC:DD:EE:FF")},
				}, nil
			case OidLLDPRemPortId:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemPortId + ".0.3.1", Type: gosnmp.OctetString, Value: []byte("eth0")},
				}, nil
			case OidLLDPRemSysName:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemSysName + ".0.3.1", Type: gosnmp.OctetString, Value: []byte("neighbor-a")},
				}, nil
			}
			return nil, nil
		},
	}

	res, err := DiscoverDevice(mock, reg)
	if err != nil {
		t.Fatalf("DiscoverDevice returned error: %v", err)
	}

	var lldpNeighbors []NeighborInfo
	for _, n := range res.Neighbors {
		if n.Protocol == domain.DiscoveryProtocolLLDP {
			lldpNeighbors = append(lldpNeighbors, n)
		}
	}
	if len(lldpNeighbors) != 1 {
		t.Fatalf("expected 1 LLDP neighbor, got %d", len(lldpNeighbors))
	}

	nbr := lldpNeighbors[0]
	if nbr.LocalIfName != "ether5" {
		t.Errorf("expected LocalIfName 'ether5' via lldpLocPortIfIndex two-step lookup, got %q", nbr.LocalIfName)
	}
	if nbr.LocalIfIndex != 5 {
		t.Errorf("expected LocalIfIndex 5, got %d", nbr.LocalIfIndex)
	}
}

// TestDiscoverDevice_LLDPLocPortIfIndex_Fallback verifies that when lldpLocPortIfIndex
// returns empty, the code falls back to treating lldpPortNum directly as ifIndex.
func TestDiscoverDevice_LLDPLocPortIfIndex_Fallback(t *testing.T) {
	reg := testDiscoveryRegistry(t)
	mock := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			var pdus []gosnmp.SnmpPDU
			for _, oid := range oids {
				switch oid {
				case OidSysName:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router-fallback")})
				case OidSysDescr:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS RB4011")})
				case OidSysObjectID:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"})
				}
			}
			return pdus, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidIfTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("eth1-descr")},
					{Name: OidIfOperStatus + ".1", Type: gosnmp.Integer, Value: 1},
				}, nil
			case OidIfXTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("eth1")},
				}, nil
			case OidLLDPLocPortIfIndex:
				// Empty — device does not support lldpLocPortIfIndex
				return nil, nil
			case OidLLDPRemChassisId:
				// Index: timeMark=0, localPortNum=1, remoteIndex=1 => "0.1.1"
				// lldpPortNum=1 happens to equal ifIndex=1 on this device
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemChassisId + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("11:22:33:44:55:66")},
				}, nil
			case OidLLDPRemPortId:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemPortId + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("eth0")},
				}, nil
			case OidLLDPRemSysName:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemSysName + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("neighbor-b")},
				}, nil
			}
			return nil, nil
		},
	}

	res, err := DiscoverDevice(mock, reg)
	if err != nil {
		t.Fatalf("DiscoverDevice returned error: %v", err)
	}

	var lldpNeighbors []NeighborInfo
	for _, n := range res.Neighbors {
		if n.Protocol == domain.DiscoveryProtocolLLDP {
			lldpNeighbors = append(lldpNeighbors, n)
		}
	}
	if len(lldpNeighbors) != 1 {
		t.Fatalf("expected 1 LLDP neighbor, got %d", len(lldpNeighbors))
	}

	nbr := lldpNeighbors[0]
	// Fallback: lldpPortNum=1 used directly as ifIndex=1 => "eth1"
	if nbr.LocalIfName != "eth1" {
		t.Errorf("expected LocalIfName 'eth1' via fallback lookup, got %q", nbr.LocalIfName)
	}
}

// TestDiscoverDevice_LLDPPartialNeighborData verifies that a neighbor with a
// malformed LLDP index (no dot — cannot parse localPortNum) results in
// LocalIfName="" without crashing, while a valid neighbor is still discovered.
func TestDiscoverDevice_LLDPPartialNeighborData(t *testing.T) {
	reg := testDiscoveryRegistry(t)
	mock := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			var pdus []gosnmp.SnmpPDU
			for _, oid := range oids {
				switch oid {
				case OidSysName:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router-partial")})
				case OidSysDescr:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("RouterOS")})
				case OidSysObjectID:
					pdus = append(pdus, gosnmp.SnmpPDU{Name: OidSysObjectID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.14988.1"})
				}
			}
			return pdus, nil
		},
		BulkWalkFunc: func(rootOid string) ([]gosnmp.SnmpPDU, error) {
			switch rootOid {
			case OidIfTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfDescr + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
				}, nil
			case OidIfXTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("ether1")},
				}, nil
			case OidLLDPLocPortIfIndex:
				return nil, nil
			case OidLLDPRemChassisId:
				return []gosnmp.SnmpPDU{
					// Valid neighbor: index "0.1.1"
					{Name: OidLLDPRemChassisId + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("AA:AA:AA:AA:AA:01")},
					// Malformed neighbor: single-component index (no dot — cannot split to get localPortNum)
					{Name: OidLLDPRemChassisId + ".99", Type: gosnmp.OctetString, Value: []byte("BB:BB:BB:BB:BB:02")},
				}, nil
			case OidLLDPRemPortId:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemPortId + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("eth0")},
					{Name: OidLLDPRemPortId + ".99", Type: gosnmp.OctetString, Value: []byte("eth1")},
				}, nil
			case OidLLDPRemSysName:
				return []gosnmp.SnmpPDU{
					{Name: OidLLDPRemSysName + ".0.1.1", Type: gosnmp.OctetString, Value: []byte("valid-neighbor")},
					{Name: OidLLDPRemSysName + ".99", Type: gosnmp.OctetString, Value: []byte("malformed-neighbor")},
				}, nil
			}
			return nil, nil
		},
	}

	res, err := DiscoverDevice(mock, reg)
	if err != nil {
		t.Fatalf("DiscoverDevice should not return error for partial LLDP data, got: %v", err)
	}

	var lldpNeighbors []NeighborInfo
	for _, n := range res.Neighbors {
		if n.Protocol == domain.DiscoveryProtocolLLDP {
			lldpNeighbors = append(lldpNeighbors, n)
		}
	}

	if len(lldpNeighbors) != 2 {
		t.Fatalf("expected 2 LLDP neighbors (valid + malformed), got %d", len(lldpNeighbors))
	}

	// Find the valid neighbor and verify LocalIfName is populated
	validFound := false
	malformedFound := false
	for _, n := range lldpNeighbors {
		switch n.RemoteChassisID {
		case "AA:AA:AA:AA:AA:01":
			validFound = true
			if n.LocalIfName != "ether1" {
				t.Errorf("valid neighbor: expected LocalIfName 'ether1', got %q", n.LocalIfName)
			}
		case "BB:BB:BB:BB:BB:02":
			malformedFound = true
			if n.LocalIfName != "" {
				t.Errorf("malformed neighbor: expected empty LocalIfName, got %q", n.LocalIfName)
			}
		}
	}
	if !validFound {
		t.Error("valid neighbor (AA:AA:AA:AA:AA:01) not found in results")
	}
	if !malformedFound {
		t.Error("malformed neighbor (BB:BB:BB:BB:BB:02) not found in results")
	}
}
