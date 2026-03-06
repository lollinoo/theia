package snmp

import (
	"fmt"
	"testing"

	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/gosnmp/gosnmp"
)

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

func TestDiscoverDevice(t *testing.T) {
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
					{Name: OidIfOperStatus + ".1", Type: gosnmp.Integer, Value: 1}, // up
				}, nil
			case OidIfXTable:
				return []gosnmp.SnmpPDU{
					{Name: OidIfName + ".1", Type: gosnmp.OctetString, Value: []byte("eth1")},
					{Name: OidIfHighSpeed + ".1", Type: gosnmp.Gauge32, Value: uint(1000)}, // 1 Gbps
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

	res, err := DiscoverDevice(mock)
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

func TestParseLLDPNeighbors(t *testing.T) {
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

	res, err := DiscoverDevice(mock)
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

	res, err := DiscoverDevice(mock)
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

func TestDiscoverDevice_Error(t *testing.T) {
	mockErr := &MockClient{
		GetFunc: func(oids []string) ([]gosnmp.SnmpPDU, error) {
			return nil, fmt.Errorf("timeout")
		},
	}

	_, err := DiscoverDevice(mockErr)
	if err == nil {
		t.Error("expected error but got nil")
	}
}
