package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultNetworkProbePorts is the global fallback order for network reachability probes.
var DefaultNetworkProbePorts = []int{22, 8291, 80, 443}

// NormalizeProbePorts validates ports, removes duplicates, and preserves first-seen order.
func NormalizeProbePorts(ports []int) ([]int, error) {
	if len(ports) == 0 {
		return nil, nil
	}

	seen := make(map[int]bool, len(ports))
	normalized := make([]int, 0, len(ports))
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("probe port %d must be between 1 and 65535", port)
		}
		if seen[port] {
			continue
		}
		seen[port] = true
		normalized = append(normalized, port)
	}
	return normalized, nil
}

// ParseProbePortsCSV parses a comma-separated probe port list.
func ParseProbePortsCSV(value string) ([]int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	tokens := strings.Split(trimmed, ",")
	ports := make([]int, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			return nil, fmt.Errorf("probe ports must not contain empty values")
		}
		port, err := strconv.Atoi(token)
		if err != nil {
			return nil, fmt.Errorf("probe port %q must be a valid integer", token)
		}
		ports = append(ports, port)
	}
	return NormalizeProbePorts(ports)
}

// FormatProbePortsCSV formats a probe port list for storage.
func FormatProbePortsCSV(ports []int) string {
	normalized, err := NormalizeProbePorts(ports)
	if err != nil || len(normalized) == 0 {
		return ""
	}

	parts := make([]string, 0, len(normalized))
	for _, port := range normalized {
		parts = append(parts, strconv.Itoa(port))
	}
	return strings.Join(parts, ",")
}

// ResolveProbePorts returns the first non-empty valid override in address, device, global order.
func ResolveProbePorts(addressPorts, devicePorts, globalPorts []int) []int {
	for _, ports := range [][]int{addressPorts, devicePorts, globalPorts} {
		normalized, err := NormalizeProbePorts(ports)
		if err == nil && len(normalized) > 0 {
			return append([]int(nil), normalized...)
		}
	}
	return append([]int(nil), DefaultNetworkProbePorts...)
}

// CoerceNetworkProbePortsCSV parses persisted probe ports or falls back to defaults.
func CoerceNetworkProbePortsCSV(value string) []int {
	ports, err := ParseProbePortsCSV(value)
	if err != nil || len(ports) == 0 {
		return append([]int(nil), DefaultNetworkProbePorts...)
	}
	return ports
}
