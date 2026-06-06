package service

// This file defines device discovery support service behavior and domain orchestration rules.

import (
	"strings"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
)

func dedupePreferredDiscoveredNeighbors(neighbors []snmp.NeighborInfo) []snmp.NeighborInfo {
	result := make([]snmp.NeighborInfo, 0, len(neighbors))
	for _, candidate := range neighbors {
		if shouldDropDiscoveredNeighbor(candidate, neighbors) {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func shouldDropDiscoveredNeighbor(candidate snmp.NeighborInfo, neighbors []snmp.NeighborInfo) bool {
	candidateRemote := normalizeDiscoveredNeighborIdentity(candidate.RemoteSysName)
	if candidateRemote == "" {
		candidateRemote = normalizeDiscoveredNeighborIdentity(candidate.RemoteChassisID)
	}
	if candidateRemote == "" {
		return false
	}

	candidateIsCompletePhysical := isCompletePhysicalDiscoveredNeighbor(candidate)
	candidateHasPhysicalRemotePort := isLikelyPhysicalDiscoveredInterface(candidate.RemotePortID)
	for _, other := range neighbors {
		otherRemote := normalizeDiscoveredNeighborIdentity(other.RemoteSysName)
		if otherRemote == "" {
			otherRemote = normalizeDiscoveredNeighborIdentity(other.RemoteChassisID)
		}
		if otherRemote != candidateRemote {
			continue
		}
		if compareDiscoveredNeighborPreference(other, candidate) <= 0 {
			continue
		}
		if isCompletePhysicalDiscoveredNeighbor(other) {
			if candidateIsCompletePhysical {
				continue
			}
			return true
		}
		if candidateHasPhysicalRemotePort {
			continue
		}
		if isLikelyPhysicalDiscoveredInterface(other.RemotePortID) {
			return true
		}
	}
	return false
}

func isCompletePhysicalDiscoveredNeighbor(neighbor snmp.NeighborInfo) bool {
	return discoveredPhysicalInterfaceAnchor(neighbor.LocalIfName) != "" && discoveredPhysicalInterfaceAnchor(neighbor.RemotePortID) != ""
}

func compareDiscoveredNeighborPreference(candidate, existing snmp.NeighborInfo) int {
	candidateScore := discoveredNeighborPreferenceScore(candidate)
	existingScore := discoveredNeighborPreferenceScore(existing)
	if candidateScore > existingScore {
		return 1
	}
	if candidateScore < existingScore {
		return -1
	}
	return 0
}

func discoveredNeighborPreferenceScore(neighbor snmp.NeighborInfo) int {
	score := 0
	if neighbor.Protocol == domain.DiscoveryProtocolLLDP {
		score += 100
	}
	if isLikelyPhysicalDiscoveredInterface(neighbor.LocalIfName) {
		score += 50
	}
	if isLikelyPhysicalDiscoveredInterface(neighbor.RemotePortID) {
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

func discoveredPhysicalInterfaceAnchor(name string) string {
	normalized := normalizeDiscoveredNeighborIdentity(name)
	if normalized == "" {
		return ""
	}
	return extractDiscoveredPhysicalInterfaceAnchor(normalized)
}

func isLikelyPhysicalDiscoveredInterface(name string) bool {
	normalized := normalizeDiscoveredNeighborIdentity(name)
	if normalized == "" {
		return false
	}
	return extractDiscoveredPhysicalInterfaceAnchor(normalized) != ""
}

func extractDiscoveredPhysicalInterfaceAnchor(normalized string) string {
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
			if discoveredHasDigit(anchor) {
				return anchor
			}
		}
	}

	shortPrefixes := []string{"gi", "te", "fo", "port"}
	for _, prefix := range shortPrefixes {
		if strings.HasPrefix(normalized, prefix) && discoveredHasDigit(normalized) {
			return normalized
		}
	}

	return ""
}

func discoveredHasDigit(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func normalizeDiscoveredNeighborIdentity(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
