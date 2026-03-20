package snmp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/gosnmp/gosnmp"
)

// OIDs for Discovery
const (
	OidSysDescr    = ".1.3.6.1.2.1.1.1.0"
	OidSysObjectID = ".1.3.6.1.2.1.1.2.0"
	OidSysName     = ".1.3.6.1.2.1.1.5.0"

	OidIfTable       = ".1.3.6.1.2.1.2.2"
	OidIfIndex       = ".1.3.6.1.2.1.2.2.1.1"
	OidIfDescr       = ".1.3.6.1.2.1.2.2.1.2"
	OidIfType        = ".1.3.6.1.2.1.2.2.1.3"
	OidIfSpeed       = ".1.3.6.1.2.1.2.2.1.5"
	OidIfAdminStatus = ".1.3.6.1.2.1.2.2.1.7"
	OidIfOperStatus  = ".1.3.6.1.2.1.2.2.1.8"

	OidIfXTable    = ".1.3.6.1.2.1.31.1.1"
	OidIfName      = ".1.3.6.1.2.1.31.1.1.1.1"
	OidIfHighSpeed = ".1.3.6.1.2.1.31.1.1.1.15"

	OidLLDPRemChassisId = ".1.0.8802.1.1.2.1.4.1.1.5"
	OidLLDPRemPortId    = ".1.0.8802.1.1.2.1.4.1.1.7"
	OidLLDPRemSysName   = ".1.0.8802.1.1.2.1.4.1.1.9"

	OidCDPDeviceID = ".1.3.6.1.4.1.9.9.23.1.2.1.1.6"
	OidCDPPortID   = ".1.3.6.1.4.1.9.9.23.1.2.1.1.7"

	// OIDs for live device metrics (HOST-RESOURCES-MIB, ENTITY-SENSOR-MIB)
	OidSysUpTime           = ".1.3.6.1.2.1.1.3.0"
	OidHrProcessorLoad     = ".1.3.6.1.2.1.25.3.2.1.5"
	OidHrStorageType       = ".1.3.6.1.2.1.25.2.3.1.2"
	OidHrStorageAllocUnits = ".1.3.6.1.2.1.25.2.3.1.4"
	OidHrStorageSize       = ".1.3.6.1.2.1.25.2.3.1.5"
	OidHrStorageUsed       = ".1.3.6.1.2.1.25.2.3.1.6"
	OidHrStorageRam        = ".1.3.6.1.2.1.25.2.1.2" // hrStorageRam type OID
	OidEntPhySensorType    = ".1.3.6.1.2.1.99.1.1.1.1"
	OidEntPhySensorValue   = ".1.3.6.1.2.1.99.1.1.1.4"
)

// DiscoveryResult holds the aggregated data from an SNMP discovery walk.
type DiscoveryResult struct {
	SysName       string
	SysDescr      string
	SysObjectID   string
	HardwareModel string
	Vendor        string
	DeviceType    domain.DeviceType
	Interfaces    []domain.Interface
	Neighbors     []NeighborInfo
}

// NeighborInfo represents a discovered LLDP or CDP neighbor.
type NeighborInfo struct {
	RemoteChassisID string
	RemotePortID    string
	RemoteSysName   string
	LocalIfIndex    int // Note: the internal model uses LocalIfName, but discovery gathers index first. We map it later.
	LocalIfName     string
	Protocol        domain.DiscoveryProtocol
}

// ClientInterface masks the underlying Client for easier mocking in tests.
type ClientInterface interface {
	Get(oids []string) ([]gosnmp.SnmpPDU, error)
	BulkWalk(rootOid string) ([]gosnmp.SnmpPDU, error)
}

// DiscoverDevice gathers all required details from a network device via SNMP.
// The vendor registry is used for device type detection, model extraction, and vendor identification.
func DiscoverDevice(client ClientInterface, registry *vendor.Registry) (*DiscoveryResult, error) {
	res := &DiscoveryResult{}

	// 1. Get System Info
	pduList, err := client.Get([]string{OidSysName, OidSysDescr, OidSysObjectID})
	if err != nil {
		return nil, fmt.Errorf("getting sys info: %w", err)
	}

	for _, pdu := range pduList {
		switch pdu.Name {
		case OidSysName:
			res.SysName = stringFromPDU(pdu)
		case OidSysDescr:
			res.SysDescr = stringFromPDU(pdu)
		case OidSysObjectID:
			res.SysObjectID = stringFromPDU(pdu)
		}
	}

	// Use vendor registry for detection
	res.Vendor, res.DeviceType, res.HardwareModel = DetectVendor(registry, res.SysObjectID, res.SysDescr)

	// 2. Walk ifTable & ifXTable to populate Interfaces
	res.Interfaces = discoverInterfaces(client)

	// Map interface index to interface name for neighbor association
	ifIndexToName := make(map[int]string)
	for _, intf := range res.Interfaces {
		ifIndexToName[intf.IfIndex] = intf.IfName
	}

	// 3. Walk LLDP & CDP to discover neighbors
	res.Neighbors = discoverNeighbors(client, ifIndexToName)

	return res, nil
}

// discoverInterfaces fetches basic ifTable and ifXTable metrics and merges them.
func discoverInterfaces(client ClientInterface) []domain.Interface {
	ifMap := make(map[int]*domain.Interface)

	// Walk ifTable
	ifTablePDUs, err := client.BulkWalk(OidIfTable)
	if err == nil {
		for _, pdu := range ifTablePDUs {
			// Extract ifIndex from the last part of the OID
			parts := strings.Split(pdu.Name, ".")
			if len(parts) < 2 {
				continue
			}
			indexStr := parts[len(parts)-1]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				continue
			}

			if _, ok := ifMap[index]; !ok {
				ifMap[index] = &domain.Interface{IfIndex: index}
			}

			if matchOIDColumn(pdu.Name, OidIfDescr) {
				ifMap[index].IfDescr = stringFromPDU(pdu)
				ifMap[index].IfName = ifMap[index].IfDescr // default ifName to ifDescr
			} else if matchOIDColumn(pdu.Name, OidIfSpeed) {
				if val, ok := pdu.Value.(uint); ok {
					ifMap[index].Speed = int64(val)
				} else if val, ok := pdu.Value.(uint32); ok {
					ifMap[index].Speed = int64(val)
				}
			} else if matchOIDColumn(pdu.Name, OidIfAdminStatus) {
				ifMap[index].AdminStatus = statusString(pdu.Value)
			} else if matchOIDColumn(pdu.Name, OidIfOperStatus) {
				ifMap[index].OperStatus = statusString(pdu.Value)
			}
		}
	}

	// Walk ifXTable to get 64-bit ifHighSpeed and the true ifName (often shorter, e.g. "eth0" vs "Ethernet0")
	ifXTablePDUs, err := client.BulkWalk(OidIfXTable)
	if err == nil {
		for _, pdu := range ifXTablePDUs {
			parts := strings.Split(pdu.Name, ".")
			if len(parts) < 2 {
				continue
			}
			indexStr := parts[len(parts)-1]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				continue
			}

			if _, ok := ifMap[index]; !ok {
				continue
			}

			if matchOIDColumn(pdu.Name, OidIfName) {
				ifMap[index].IfName = stringFromPDU(pdu)
			} else if matchOIDColumn(pdu.Name, OidIfHighSpeed) {
				if val, ok := pdu.Value.(uint); ok && val > 0 {
					// ifHighSpeed is in megabits/sec
					ifMap[index].Speed = int64(val) * 1_000_000
				} else if val, ok := pdu.Value.(uint32); ok && val > 0 {
					ifMap[index].Speed = int64(val) * 1_000_000
				}
			}
		}
	}

	var interfaces []domain.Interface
	for _, intf := range ifMap {
		interfaces = append(interfaces, *intf)
	}
	return interfaces
}

// discoverNeighbors fetches LLDP and CDP remote parameters.
func discoverNeighbors(client ClientInterface, ifIndexToName map[int]string) []NeighborInfo {
	var neighbors []NeighborInfo
	lldpMap := make(map[string]*NeighborInfo)

	// Fetch LLDP Neighbors
	lldpRemChassisIdPDUs, _ := client.BulkWalk(OidLLDPRemChassisId)
	for _, pdu := range lldpRemChassisIdPDUs {
		indexStr := extractLLDPIndex(pdu.Name, OidLLDPRemChassisId)
		if indexStr == "" {
			continue
		}
		if _, ok := lldpMap[indexStr]; !ok {
			lldpMap[indexStr] = &NeighborInfo{Protocol: domain.DiscoveryProtocolLLDP}
		}
		lldpMap[indexStr].RemoteChassisID = stringFromPDU(pdu)
		// Try to extract LocalIfIndex from the first part of the LLDP index (timeMark.localPortNum.index)
		parts := strings.Split(indexStr, ".")
		if len(parts) >= 2 {
			if localIfIndex, err := strconv.Atoi(parts[1]); err == nil {
				lldpMap[indexStr].LocalIfIndex = localIfIndex
				lldpMap[indexStr].LocalIfName = ifIndexToName[localIfIndex]
			}
		}
	}

	lldpRemPortIdPDUs, _ := client.BulkWalk(OidLLDPRemPortId)
	for _, pdu := range lldpRemPortIdPDUs {
		indexStr := extractLLDPIndex(pdu.Name, OidLLDPRemPortId)
		if n, ok := lldpMap[indexStr]; ok {
			n.RemotePortID = stringFromPDU(pdu)
		}
	}

	lldpRemSysNamePDUs, _ := client.BulkWalk(OidLLDPRemSysName)
	for _, pdu := range lldpRemSysNamePDUs {
		indexStr := extractLLDPIndex(pdu.Name, OidLLDPRemSysName)
		if n, ok := lldpMap[indexStr]; ok {
			n.RemoteSysName = stringFromPDU(pdu)
		}
	}

	for _, n := range lldpMap {
		neighbors = append(neighbors, *n)
	}

	// CDP discovery could go here analogously, utilizing OidCDPDeviceID and OidCDPPortID
	// Simplified CDP tracking:
	cdpMap := make(map[string]*NeighborInfo)
	cdpDeviceIDPDUs, _ := client.BulkWalk(OidCDPDeviceID)
	for _, pdu := range cdpDeviceIDPDUs {
		indexStr := extractCDPIndex(pdu.Name, OidCDPDeviceID)
		if indexStr == "" {
			continue
		}
		if _, ok := cdpMap[indexStr]; !ok {
			cdpMap[indexStr] = &NeighborInfo{Protocol: domain.DiscoveryProtocolCDP}
		}
		cdpMap[indexStr].RemoteSysName = stringFromPDU(pdu)
		
		// CDP index typically looks like localIfIndex.cdpCacheDeviceIndex
		parts := strings.Split(indexStr, ".")
		if len(parts) >= 1 {
			if localIfIndex, err := strconv.Atoi(parts[0]); err == nil {
				cdpMap[indexStr].LocalIfIndex = localIfIndex
				cdpMap[indexStr].LocalIfName = ifIndexToName[localIfIndex]
			}
		}
	}

	cdpPortIDPDUs, _ := client.BulkWalk(OidCDPPortID)
	for _, pdu := range cdpPortIDPDUs {
		indexStr := extractCDPIndex(pdu.Name, OidCDPPortID)
		if n, ok := cdpMap[indexStr]; ok {
			n.RemotePortID = stringFromPDU(pdu)
		}
	}

	for _, n := range cdpMap {
		neighbors = append(neighbors, *n)
	}

	return neighbors
}

// --- Helpers ---

// matchOIDColumn checks if a full OID belongs to a given column OID prefix.
// It ensures the prefix is followed by a dot (instance separator), preventing
// ambiguous matches like .1.3.6.1.2.1.31.1.1.1.1 matching .1.3.6.1.2.1.31.1.1.1.15.
func matchOIDColumn(fullOID, columnOID string) bool {
	return strings.HasPrefix(fullOID, columnOID+".")
}

func stringFromPDU(pdu gosnmp.SnmpPDU) string {
	switch pdu.Type {
	case gosnmp.OctetString:
		if bytes, ok := pdu.Value.([]byte); ok {
			// Handle some devices returning null-terminated strings
			return strings.TrimRight(string(bytes), "\x00")
		}
	case gosnmp.ObjectIdentifier:
		if str, ok := pdu.Value.(string); ok {
			return str
		}
	}
	return fmt.Sprintf("%v", pdu.Value)
}

func statusString(val interface{}) string {
	// IF-MIB specifies 1=up, 2=down, 3=testing
	switch v := val.(type) {
	case int:
		switch v {
		case 1:
			return "up"
		case 2:
			return "down"
		case 3:
			return "testing"
		}
	case uint:
		switch v {
		case 1:
			return "up"
		case 2:
			return "down"
		case 3:
			return "testing"
		}
	}
	return "unknown"
}

func extractLLDPIndex(oid, prefix string) string {
	return strings.TrimPrefix(oid, prefix+".")
}

func extractCDPIndex(oid, prefix string) string {
	return strings.TrimPrefix(oid, prefix+".")
}

// PollDeviceMetrics collects live CPU, memory, uptime, and temperature metrics
// directly from a device via SNMP. Uses vendor-resolved SNMP config for
// temperature OID and scale. Returns nil pointers for metrics that are
// not available on the target device.
func PollDeviceMetrics(client ClientInterface, snmpCfg vendor.SNMPConfig) (cpuPercent, memPercent, uptimeSecs, tempCelsius *float64) {
	// Uptime — sysUpTime is in hundredths of seconds (TimeTicks)
	if pdus, err := client.Get([]string{OidSysUpTime}); err == nil {
		for _, pdu := range pdus {
			if pdu.Name == OidSysUpTime {
				if v := uint32FromPDU(pdu); v > 0 {
					secs := float64(v) / 100.0
					uptimeSecs = &secs
				}
			}
		}
	}

	// CPU — average of all hrProcessorLoad entries (one per CPU core)
	cpuOID := OidHrProcessorLoad
	if snmpCfg.CPUOID != "" {
		cpuOID = snmpCfg.CPUOID
	}
	if pdus, err := client.BulkWalk(cpuOID); err == nil && len(pdus) > 0 {
		var sum float64
		count := 0
		for _, pdu := range pdus {
			if matchOIDColumn(pdu.Name, cpuOID) {
				v := int64FromPDU(pdu)
				if v >= 0 {
					sum += float64(v)
					count++
				}
			}
		}
		if count > 0 {
			avg := sum / float64(count)
			cpuPercent = &avg
		}
	}

	// Memory — hrStorage table, find the RAM entry
	memPercent = pollMemoryPercent(client)

	// Temperature — try vendor-specific OID first, fall back to entity sensor MIB
	tempOID := snmpCfg.TemperatureOID
	tempScale := snmpCfg.TemperatureScale
	if tempOID != "" {
		if pdus, err := client.Get([]string{tempOID}); err == nil {
			for _, pdu := range pdus {
				if pdu.Name == tempOID {
					if v := int64FromPDU(pdu); v > 0 {
						c := float64(v)
						if tempScale > 0 {
							c *= tempScale
						}
						tempCelsius = &c
					}
				}
			}
		}
	}
	if tempCelsius == nil {
		tempCelsius = pollEntitySensorTemp(client)
	}

	return
}

// pollMemoryPercent looks up the hrStorageRam entry and returns used/total*100.
func pollMemoryPercent(client ClientInterface) *float64 {
	types := make(map[int]string)
	units := make(map[int]int64)
	sizes := make(map[int]int64)
	used := make(map[int]int64)

	for _, pdu := range bulkWalkSafe(client, OidHrStorageType) {
		if idx := lastOIDIndex(pdu.Name, OidHrStorageType); idx >= 0 {
			types[idx] = stringFromPDU(pdu)
		}
	}
	for _, pdu := range bulkWalkSafe(client, OidHrStorageAllocUnits) {
		if idx := lastOIDIndex(pdu.Name, OidHrStorageAllocUnits); idx >= 0 {
			units[idx] = int64FromPDU(pdu)
		}
	}
	for _, pdu := range bulkWalkSafe(client, OidHrStorageSize) {
		if idx := lastOIDIndex(pdu.Name, OidHrStorageSize); idx >= 0 {
			sizes[idx] = int64FromPDU(pdu)
		}
	}
	for _, pdu := range bulkWalkSafe(client, OidHrStorageUsed) {
		if idx := lastOIDIndex(pdu.Name, OidHrStorageUsed); idx >= 0 {
			used[idx] = int64FromPDU(pdu)
		}
	}

	for idx, typeOID := range types {
		// hrStorageRam OID, may appear with or without leading dot
		if typeOID != OidHrStorageRam && typeOID != strings.TrimPrefix(OidHrStorageRam, ".") {
			continue
		}
		allocUnits := units[idx]
		total := sizes[idx]
		usedVal := used[idx]
		if allocUnits <= 0 || total <= 0 {
			continue
		}
		pct := float64(usedVal) / float64(total) * 100.0
		return &pct
	}
	return nil
}

// pollEntitySensorTemp returns the highest Celsius reading from ENTITY-SENSOR-MIB.
func pollEntitySensorTemp(client ClientInterface) *float64 {
	sensorTypes := make(map[int]int64)
	for _, pdu := range bulkWalkSafe(client, OidEntPhySensorType) {
		if idx := lastOIDIndex(pdu.Name, OidEntPhySensorType); idx >= 0 {
			sensorTypes[idx] = int64FromPDU(pdu)
		}
	}

	var maxTemp *float64
	for _, pdu := range bulkWalkSafe(client, OidEntPhySensorValue) {
		idx := lastOIDIndex(pdu.Name, OidEntPhySensorValue)
		if idx < 0 {
			continue
		}
		if sensorTypes[idx] != 8 { // 8 = celsius
			continue
		}
		v := int64FromPDU(pdu)
		if v <= 0 {
			continue
		}
		c := float64(v)
		if maxTemp == nil || c > *maxTemp {
			maxTemp = &c
		}
	}
	return maxTemp
}

// --- Numeric helpers ---

func bulkWalkSafe(client ClientInterface, oid string) []gosnmp.SnmpPDU {
	pdus, _ := client.BulkWalk(oid)
	return pdus
}

func lastOIDIndex(oid, prefix string) int {
	suffix := strings.TrimPrefix(oid, prefix+".")
	if suffix == oid {
		return -1
	}
	// Only accept simple integer indices (no dots)
	if strings.Contains(suffix, ".") {
		return -1
	}
	idx, err := strconv.Atoi(suffix)
	if err != nil {
		return -1
	}
	return idx
}

func uint32FromPDU(pdu gosnmp.SnmpPDU) uint32 {
	switch v := pdu.Value.(type) {
	case uint32:
		return v
	case uint:
		return uint32(v)
	case int:
		if v >= 0 {
			return uint32(v)
		}
	case int32:
		if v >= 0 {
			return uint32(v)
		}
	}
	return 0
}

func int64FromPDU(pdu gosnmp.SnmpPDU) int64 {
	switch v := pdu.Value.(type) {
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		return int64(v)
	}
	return 0
}
