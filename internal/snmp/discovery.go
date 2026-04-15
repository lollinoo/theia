package snmp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/vendor"
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

	OidIfXTable      = ".1.3.6.1.2.1.31.1.1"
	OidIfName        = ".1.3.6.1.2.1.31.1.1.1.1"
	OidIfHCInOctets  = ".1.3.6.1.2.1.31.1.1.1.6"
	OidIfHCOutOctets = ".1.3.6.1.2.1.31.1.1.1.10"
	OidIfHighSpeed   = ".1.3.6.1.2.1.31.1.1.1.15"

	OidLLDPRemChassisId = ".1.0.8802.1.1.2.1.4.1.1.5"
	OidLLDPRemPortId    = ".1.0.8802.1.1.2.1.4.1.1.7"
	OidLLDPRemSysName   = ".1.0.8802.1.1.2.1.4.1.1.9"

	// OidLLDPLocPortIfIndex maps the LLDP local port number (lldpLocPortNum) to
	// the corresponding IF-MIB ifIndex. On most vendors (including MikroTik)
	// the LLDP port numbering scheme is independent from the IF-MIB ifIndex
	// numbering, so a direct lookup of lldpPortNum in ifIndexToName would fail.
	// Walking this OID builds the bridge between the two numbering schemes.
	OidLLDPLocPortIfIndex = ".1.0.8802.1.1.2.1.3.7.1.3"

	// OidLLDPLocPortId is the local port identifier string from lldpLocPortTable.
	// When lldpLocPortIdSubtype is interfaceName (7) or interfaceAlias (5), this
	// OID directly contains the interface name, making it a reliable last-resort
	// fallback for LocalIfName when the two-step ifIndex lookup fails.
	OidLLDPLocPortId = ".1.0.8802.1.1.2.1.3.7.1.4"

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

// InterfaceCounter holds raw 64-bit octet counters for a single interface.
type InterfaceCounter struct {
	IfIndex   int
	IfName    string
	InOctets  uint64
	OutOctets uint64
}

// PollOperationalStatus collects sysUpTime and per-interface ifOperStatus
// values using vendor-resolved operational OIDs. Missing fields remain partial;
// transport/query failures return an error.
func PollOperationalStatus(client ClientInterface, operationalOIDs vendor.OperationalOIDs) (uptimeSecs *float64, statuses map[string]string, err error) {
	uptimeOID := operationalOIDs.SysUpTimeOID
	if uptimeOID == "" {
		uptimeOID = OidSysUpTime
	}

	statusOID := operationalOIDs.IfOperStatusOID
	if statusOID == "" {
		statusOID = OidIfOperStatus
	}

	pdus, err := client.Get([]string{uptimeOID})
	if err != nil {
		return nil, nil, fmt.Errorf("getting sysUpTime: %w", err)
	}
	for _, pdu := range pdus {
		if pdu.Name != uptimeOID {
			continue
		}
		if v := uint32FromPDU(pdu); v > 0 {
			secs := float64(v) / 100.0
			uptimeSecs = &secs
			break
		}
	}

	ifNames := make(map[int]string)
	for _, pdu := range bulkWalkSafe(client, OidIfName) {
		if idx := lastOIDIndex(pdu.Name, OidIfName); idx >= 0 {
			ifNames[idx] = stringFromPDU(pdu)
		}
	}
	if len(ifNames) == 0 {
		for _, pdu := range bulkWalkSafe(client, OidIfDescr) {
			if idx := lastOIDIndex(pdu.Name, OidIfDescr); idx >= 0 {
				ifNames[idx] = stringFromPDU(pdu)
			}
		}
	}

	statusPDUs, err := client.BulkWalk(statusOID)
	if err != nil {
		return nil, nil, fmt.Errorf("walking ifOperStatus: %w", err)
	}
	for _, pdu := range statusPDUs {
		idx := lastOIDIndex(pdu.Name, statusOID)
		if idx < 0 {
			continue
		}
		ifName := ifNames[idx]
		if ifName == "" {
			continue
		}
		if statuses == nil {
			statuses = make(map[string]string)
		}
		statuses[ifName] = statusString(pdu.Value)
	}

	return uptimeSecs, statuses, nil
}

// PollInterfaceCounters walks ifHCInOctets and ifHCOutOctets (64-bit counters)
// and returns raw counter values per interface. The caller is responsible for
// computing rates by comparing two successive polls.
func PollInterfaceCounters(client ClientInterface) []InterfaceCounter {
	// Build ifIndex→ifName map from ifXTable (or ifDescr as fallback)
	ifNames := make(map[int]string)
	for _, pdu := range bulkWalkSafe(client, OidIfName) {
		if idx := lastOIDIndex(pdu.Name, OidIfName); idx >= 0 {
			ifNames[idx] = stringFromPDU(pdu)
		}
	}
	if len(ifNames) == 0 {
		for _, pdu := range bulkWalkSafe(client, OidIfDescr) {
			if idx := lastOIDIndex(pdu.Name, OidIfDescr); idx >= 0 {
				ifNames[idx] = stringFromPDU(pdu)
			}
		}
	}

	inOctets := make(map[int]uint64)
	for _, pdu := range bulkWalkSafe(client, OidIfHCInOctets) {
		if idx := lastOIDIndex(pdu.Name, OidIfHCInOctets); idx >= 0 {
			inOctets[idx] = uint64FromPDU(pdu)
		}
	}

	outOctets := make(map[int]uint64)
	for _, pdu := range bulkWalkSafe(client, OidIfHCOutOctets) {
		if idx := lastOIDIndex(pdu.Name, OidIfHCOutOctets); idx >= 0 {
			outOctets[idx] = uint64FromPDU(pdu)
		}
	}

	var counters []InterfaceCounter
	for idx, name := range ifNames {
		counters = append(counters, InterfaceCounter{
			IfIndex:   idx,
			IfName:    name,
			InOctets:  inOctets[idx],
			OutOctets: outOctets[idx],
		})
	}
	return counters
}

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
// LLDP is treated as canonical when both protocols describe the same physical
// connection. CDP is only used to fill gaps when LLDP data is absent or
// incomplete for a given local/remote interface pair.
func discoverNeighbors(client ClientInterface, ifIndexToName map[int]string) []NeighborInfo {
	var neighbors []NeighborInfo
	lldpMap := make(map[string]*NeighborInfo)

	// Build lldpPortNum -> IF-MIB ifIndex mapping from lldpLocPortIfIndex.
	// On MikroTik (and most vendors), LLDP local port numbers differ from
	// IF-MIB ifIndex values. This map bridges the two numbering schemes.
	lldpPortNumToIfIndex := make(map[int]int)
	lldpLocPortIfIndexPDUs, _ := client.BulkWalk(OidLLDPLocPortIfIndex)
	for _, pdu := range lldpLocPortIfIndexPDUs {
		// OID format: .1.0.8802.1.1.2.1.3.7.1.3.<lldpLocPortNum>
		suffix := strings.TrimPrefix(pdu.Name, OidLLDPLocPortIfIndex+".")
		if suffix == pdu.Name {
			continue
		}
		portNum, err := strconv.Atoi(suffix)
		if err != nil {
			continue
		}
		ifIndex := int(int64FromPDU(pdu))
		if ifIndex > 0 {
			lldpPortNumToIfIndex[portNum] = ifIndex
		}
	}

	// Build lldpPortNum -> port ID string from lldpLocPortId.
	// When lldpLocPortIdSubtype is interfaceName (7) or interfaceAlias (5),
	// this directly contains the interface name — used as a last-resort fallback
	// when the two-step ifIndex lookup above yields an empty result.
	lldpPortNumToPortId := make(map[int]string)
	lldpLocPortIdPDUs, _ := client.BulkWalk(OidLLDPLocPortId)
	for _, pdu := range lldpLocPortIdPDUs {
		// OID format: .1.0.8802.1.1.2.1.3.7.1.4.<lldpLocPortNum>
		suffix := strings.TrimPrefix(pdu.Name, OidLLDPLocPortId+".")
		if suffix == pdu.Name {
			continue
		}
		portNum, err := strconv.Atoi(suffix)
		if err != nil {
			continue
		}
		if s := stringFromPDU(pdu); s != "" {
			lldpPortNumToPortId[portNum] = s
		}
	}

	// Fetch LLDP Neighbors
	lldpRemChassisIdPDUs, _ := client.BulkWalk(OidLLDPRemChassisId)
	for _, pdu := range lldpRemChassisIdPDUs {
		indexStr := extractLLDPIndex(pdu.Name, OidLLDPRemChassisId)
		if indexStr == pdu.Name {
			continue
		}
		if _, ok := lldpMap[indexStr]; !ok {
			lldpMap[indexStr] = &NeighborInfo{Protocol: domain.DiscoveryProtocolLLDP}
		}
		lldpMap[indexStr].RemoteChassisID = stringFromPDU(pdu)
		// Extract localPortNum from the LLDP index (format: timeMark.localPortNum.remIndex)
		// then do a two-step resolution: lldpPortNum -> ifIndex -> ifName.
		parts := strings.Split(indexStr, ".")
		if len(parts) >= 2 {
			if localPortNum, err := strconv.Atoi(parts[1]); err == nil {
				lldpMap[indexStr].LocalIfIndex = localPortNum
				// Two-step resolution: lldpPortNum -> ifIndex -> ifName
				if ifIdx, ok := lldpPortNumToIfIndex[localPortNum]; ok {
					lldpMap[indexStr].LocalIfIndex = ifIdx
					lldpMap[indexStr].LocalIfName = ifIndexToName[ifIdx]
				} else {
					// Fallback: treat lldpPortNum as ifIndex directly
					// (works on devices where the numbering happens to match)
					lldpMap[indexStr].LocalIfName = ifIndexToName[localPortNum]
				}
				// Last-resort fallback: use lldpLocPortId string directly.
				// On devices where lldpLocPortIdSubtype is interfaceName (7) or
				// interfaceAlias (5), this OID holds the interface name verbatim
				// and is mandatory per the LLDP MIB — so it is always present.
				if lldpMap[indexStr].LocalIfName == "" {
					if portIdStr, ok := lldpPortNumToPortId[localPortNum]; ok {
						lldpMap[indexStr].LocalIfName = portIdStr
					}
				}
			}
		}
	}

	lldpRemPortIdPDUs, _ := client.BulkWalk(OidLLDPRemPortId)
	for _, pdu := range lldpRemPortIdPDUs {
		indexStr := extractLLDPIndex(pdu.Name, OidLLDPRemPortId)
		if indexStr == pdu.Name {
			continue
		}
		if n, ok := lldpMap[indexStr]; ok {
			n.RemotePortID = stringFromPDU(pdu)
		}
	}

	lldpRemSysNamePDUs, _ := client.BulkWalk(OidLLDPRemSysName)
	for _, pdu := range lldpRemSysNamePDUs {
		indexStr := extractLLDPIndex(pdu.Name, OidLLDPRemSysName)
		if indexStr == pdu.Name {
			continue
		}
		if n, ok := lldpMap[indexStr]; ok {
			n.RemoteSysName = stringFromPDU(pdu)
		}
	}

	// CDP discovery could go here analogously, utilizing OidCDPDeviceID and OidCDPPortID
	// Simplified CDP tracking:
	cdpMap := make(map[string]*NeighborInfo)
	cdpDeviceIDPDUs, _ := client.BulkWalk(OidCDPDeviceID)
	for _, pdu := range cdpDeviceIDPDUs {
		indexStr := extractCDPIndex(pdu.Name, OidCDPDeviceID)
		if indexStr == pdu.Name {
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

	lldpByKey := make(map[string]*NeighborInfo, len(lldpMap))
	lldpByLocalIf := make(map[string][]*NeighborInfo, len(lldpMap))
	for _, n := range lldpMap {
		key := neighborMergeKey(*n)
		lldpByKey[key] = n
		localKey := strings.ToLower(strings.TrimSpace(n.LocalIfName))
		lldpByLocalIf[localKey] = append(lldpByLocalIf[localKey], n)
	}

	for _, n := range cdpMap {
		key := neighborMergeKey(*n)
		if existing, ok := lldpByKey[key]; ok {
			mergeCDPIntoLLDP(existing, *n)
			continue
		}
		if existing := findLLDPGapFillCandidate(lldpByLocalIf, *n); existing != nil {
			mergeCDPIntoLLDP(existing, *n)
			continue
		}
		neighbors = append(neighbors, *n)
	}

	for _, n := range lldpMap {
		neighbors = append(neighbors, *n)
	}

	return dedupePreferredNeighbors(neighbors)
}

func findLLDPGapFillCandidate(lldpByLocalIf map[string][]*NeighborInfo, cdp NeighborInfo) *NeighborInfo {
	localKey := strings.ToLower(strings.TrimSpace(cdp.LocalIfName))
	candidates := lldpByLocalIf[localKey]
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if candidate.RemotePortID == "" || candidate.RemoteSysName == "" {
			return candidate
		}
	}
	return nil
}

func neighborMergeKey(neighbor NeighborInfo) string {
	return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%s|%s", neighbor.LocalIfName, neighbor.RemotePortID)))
}

func mergeCDPIntoLLDP(existing *NeighborInfo, cdp NeighborInfo) {
	if existing == nil {
		return
	}
	if existing.RemoteSysName == "" {
		existing.RemoteSysName = cdp.RemoteSysName
	}
	if existing.RemotePortID == "" {
		existing.RemotePortID = cdp.RemotePortID
	}
	if existing.LocalIfName == "" {
		existing.LocalIfName = cdp.LocalIfName
	}
	if existing.LocalIfIndex == 0 {
		existing.LocalIfIndex = cdp.LocalIfIndex
	}
	if existing.RemoteChassisID == "" {
		existing.RemoteChassisID = cdp.RemoteChassisID
	}
}

func dedupePreferredNeighbors(neighbors []NeighborInfo) []NeighborInfo {
	result := make([]NeighborInfo, 0, len(neighbors))
	for _, candidate := range neighbors {
		if shouldDropPreferredNeighbor(candidate, neighbors) {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func shouldDropPreferredNeighbor(candidate NeighborInfo, neighbors []NeighborInfo) bool {
	candidateRemote := normalizeNeighborIdentity(candidate.RemoteSysName)
	if candidateRemote == "" {
		candidateRemote = normalizeNeighborIdentity(candidate.RemoteChassisID)
	}
	if candidateRemote == "" {
		return false
	}

	candidateIsCompletePhysical := isCompletePhysicalNeighbor(candidate)
	for _, other := range neighbors {
		otherRemote := normalizeNeighborIdentity(other.RemoteSysName)
		if otherRemote == "" {
			otherRemote = normalizeNeighborIdentity(other.RemoteChassisID)
		}
		if otherRemote != candidateRemote {
			continue
		}
		if !isCompletePhysicalNeighbor(other) {
			continue
		}
		if compareNeighborPreference(other, candidate) <= 0 {
			continue
		}
		if candidateIsCompletePhysical {
			continue
		}
		return true
	}
	return false
}

func isCompletePhysicalNeighbor(neighbor NeighborInfo) bool {
	return physicalInterfaceAnchor(neighbor.LocalIfName) != "" && physicalInterfaceAnchor(neighbor.RemotePortID) != ""
}

func compareNeighborPreference(candidate, existing NeighborInfo) int {
	candidateScore := neighborPreferenceScore(candidate)
	existingScore := neighborPreferenceScore(existing)
	if candidateScore > existingScore {
		return 1
	}
	if candidateScore < existingScore {
		return -1
	}
	return 0
}

func neighborPreferenceScore(neighbor NeighborInfo) int {
	score := 0
	if strings.EqualFold(string(neighbor.Protocol), string(domain.DiscoveryProtocolLLDP)) {
		score += 100
	}
	if isLikelyPhysicalInterface(neighbor.LocalIfName) {
		score += 50
	}
	if isLikelyPhysicalInterface(neighbor.RemotePortID) {
		score += 40
	}
	if neighbor.LocalIfName != "" {
		score += 20
	}
	if neighbor.RemotePortID != "" {
		score += 20
	}
	if neighbor.RemoteSysName != "" {
		score += 10
	}
	if neighbor.RemoteChassisID != "" {
		score += 5
	}
	return score
}

func physicalInterfaceAnchor(name string) string {
	normalized := normalizeNeighborIdentity(name)
	if normalized == "" {
		return ""
	}
	anchor := extractPhysicalInterfaceAnchor(normalized)
	if anchor == "" {
		return ""
	}
	return anchor
}

func isLikelyPhysicalInterface(name string) bool {
	normalized := normalizeNeighborIdentity(name)
	if normalized == "" {
		return false
	}
	return extractPhysicalInterfaceAnchor(normalized) != ""
}

func extractPhysicalInterfaceAnchor(normalized string) string {
	virtualHints := []string{
		"vlan", "vrf", "vpn", "bridge", "br-", "bond", "loopback", "lo",
		"gre", "eoip", "wg", "wireguard", "pppoe", "ppp", "sstp", "ovpn",
		"l2tp", "vxlan", "veth", "tap", "tun",
	}
	for _, hint := range virtualHints {
		if strings.Contains(normalized, hint) {
			return ""
		}
	}

	physicalPatterns := []string{
		"ether", "eth", "sfp-sfpplus", "sfp", "qsfp", "ens", "eno", "enp",
		"gigabitethernet", "tengigabitethernet", "fastethernet", "ge-", "xe-", "et-",
	}
	for _, pattern := range physicalPatterns {
		if idx := strings.Index(normalized, pattern); idx >= 0 {
			anchor := normalized[idx:]
			for i, r := range anchor {
				if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '/') {
					anchor = anchor[:i]
					break
				}
			}
			anchor = strings.Trim(anchor, "- /")
			if hasDigit(anchor) {
				return anchor
			}
		}
	}

	shortPrefixes := []string{"gi", "te", "fo", "port"}
	for _, prefix := range shortPrefixes {
		if strings.HasPrefix(normalized, prefix) && hasDigit(normalized) {
			return normalized
		}
	}

	return ""
}

func hasDigit(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func normalizeNeighborIdentity(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
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
// directly from a device via SNMP. Uses vendor-resolved performance OIDs for
// temperature OID and scale. Returns nil pointers for metrics that are
// not available on the target device.
func PollDeviceMetrics(client ClientInterface, perfOIDs vendor.PerformanceOIDs) (cpuPercent, memPercent, uptimeSecs, tempCelsius *float64) {
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
	if perfOIDs.CPUOID != "" {
		cpuOID = perfOIDs.CPUOID
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
	tempOID := perfOIDs.TemperatureOID
	tempScale := perfOIDs.TemperatureScale
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

func uint64FromPDU(pdu gosnmp.SnmpPDU) uint64 {
	switch v := pdu.Value.(type) {
	case uint64:
		return v
	case uint:
		return uint64(v)
	case uint32:
		return uint64(v)
	case int:
		if v >= 0 {
			return uint64(v)
		}
	case int64:
		if v >= 0 {
			return uint64(v)
		}
	}
	return 0
}
