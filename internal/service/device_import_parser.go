package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// DeviceImportMaxFileBytes is the maximum accepted file-SD upload size.
	DeviceImportMaxFileBytes = 2 << 20
	// DeviceImportMaxTargets is the maximum number of raw targets in one upload.
	DeviceImportMaxTargets = 5000
)

// ErrDeviceImportLimitExceeded reports an upload size or raw-target count limit.
var ErrDeviceImportLimitExceeded = errors.New("device import limit exceeded")

// DeviceImportMode controls target validation for an import preview.
type DeviceImportMode string

const (
	// DeviceImportModePrometheus imports Prometheus instance label values.
	DeviceImportModePrometheus DeviceImportMode = "prometheus"
	// DeviceImportModePrometheusFallback imports Prometheus targets with SNMP fallback.
	DeviceImportModePrometheusFallback DeviceImportMode = "prometheus_snmp_fallback"
	// DeviceImportModeSNMP imports targets for direct SNMP polling.
	DeviceImportModeSNMP DeviceImportMode = "snmp"
)

// ParsedDeviceImportFile is the ordered, label-blind preview of a file-SD upload.
type ParsedDeviceImportFile struct {
	Targets     []ParsedDeviceImportTarget
	Diagnostics []DeviceImportGroupDiagnostic
}

// DeviceImportTargetLocation identifies a target by zero-based group and item indexes.
type DeviceImportTargetLocation struct {
	GroupIndex int
	ItemIndex  int
}

// DeviceImportGroupDiagnostic reports a malformed file-SD group without rejecting other groups.
type DeviceImportGroupDiagnostic struct {
	GroupIndex int
	Message    string
}

// ParsedDeviceImportTarget contains only endpoint data derived from a targets entry.
type ParsedDeviceImportTarget struct {
	GroupIndex      int
	ItemIndex       int
	RawTarget       string
	CanonicalHost   string
	ExplicitPort    *uint16
	DuplicateOf     *DeviceImportTargetLocation
	ValidationError string
}

// ParsePrometheusFileSD parses one Prometheus file-SD document without retaining labels.
func ParsePrometheusFileSD(input []byte, mode DeviceImportMode) (ParsedDeviceImportFile, error) {
	var result ParsedDeviceImportFile
	if len(input) > DeviceImportMaxFileBytes {
		return result, fmt.Errorf("%w: file exceeds %d bytes", ErrDeviceImportLimitExceeded, DeviceImportMaxFileBytes)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(input))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return result, errors.New("device import file must contain one non-empty YAML document")
		}
		return result, fmt.Errorf("decode device import YAML: %w", err)
	}

	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return result, fmt.Errorf("decode trailing device import YAML: %w", err)
		}
		return result, errors.New("device import file must contain exactly one YAML document")
	}

	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return result, errors.New("device import file must contain one non-empty YAML document")
	}
	root := document.Content[0]
	if root.Kind != yaml.SequenceNode {
		return result, errors.New("device import YAML root must be a sequence")
	}
	if len(root.Content) == 0 {
		return result, errors.New("device import YAML root sequence must not be empty")
	}

	seenHosts := make(map[string]DeviceImportTargetLocation)
	rawTargetCount := 0
	for groupIndex, group := range root.Content {
		if group.Kind != yaml.MappingNode {
			result.Diagnostics = append(result.Diagnostics, DeviceImportGroupDiagnostic{
				GroupIndex: groupIndex,
				Message:    "file-SD group must be a mapping",
			})
			continue
		}

		var targetNodes []*yaml.Node
		targetsKeyCount := 0
		for contentIndex := 0; contentIndex+1 < len(group.Content); contentIndex += 2 {
			key := group.Content[contentIndex]
			value := group.Content[contentIndex+1]
			if key.Kind != yaml.ScalarNode || key.Value != "targets" {
				continue
			}
			targetsKeyCount++
			targetNodes = append(targetNodes, value)
			if value.Kind == yaml.SequenceNode {
				rawTargetCount += len(value.Content)
				if rawTargetCount > DeviceImportMaxTargets {
					return ParsedDeviceImportFile{}, fmt.Errorf(
						"%w: file contains more than %d raw targets",
						ErrDeviceImportLimitExceeded,
						DeviceImportMaxTargets,
					)
				}
			}
		}

		if targetsKeyCount == 0 {
			result.Diagnostics = append(result.Diagnostics, DeviceImportGroupDiagnostic{
				GroupIndex: groupIndex,
				Message:    "file-SD group is missing targets",
			})
			continue
		}
		if targetsKeyCount != 1 {
			result.Diagnostics = append(result.Diagnostics, DeviceImportGroupDiagnostic{
				GroupIndex: groupIndex,
				Message:    "file-SD group has repeated targets keys; exactly one is required",
			})
			continue
		}
		if targetNodes[0].Kind != yaml.SequenceNode {
			result.Diagnostics = append(result.Diagnostics, DeviceImportGroupDiagnostic{
				GroupIndex: groupIndex,
				Message:    "file-SD group targets must be a sequence",
			})
			continue
		}

		for itemIndex, node := range targetNodes[0].Content {
			target := ParsedDeviceImportTarget{
				GroupIndex: groupIndex,
				ItemIndex:  itemIndex,
			}
			if node.Kind != yaml.ScalarNode || node.ShortTag() != "!!str" {
				target.ValidationError = "target must be a string scalar"
				result.Targets = append(result.Targets, target)
				continue
			}

			target.RawTarget = strings.TrimSpace(node.Value)
			target.CanonicalHost, target.ExplicitPort, target.ValidationError = canonicalImportTarget(target.RawTarget, mode)
			if target.ValidationError == "" {
				if first, duplicate := seenHosts[target.CanonicalHost]; duplicate {
					firstCopy := first
					target.DuplicateOf = &firstCopy
				} else {
					seenHosts[target.CanonicalHost] = DeviceImportTargetLocation{
						GroupIndex: groupIndex,
						ItemIndex:  itemIndex,
					}
				}
			}
			result.Targets = append(result.Targets, target)
		}
	}

	return result, nil
}

func canonicalImportTarget(rawTarget string, mode DeviceImportMode) (string, *uint16, string) {
	if rawTarget == "" {
		return "", nil, "target host must not be empty"
	}

	host, port, validationError := splitCanonicalImportEndpoint(rawTarget)
	if validationError != "" {
		return "", nil, validationError
	}
	if (mode == DeviceImportModePrometheus || mode == DeviceImportModePrometheusFallback) && len(rawTarget) > 255 {
		return host, port, "Prometheus target exceeds 255 characters"
	}
	if mode == DeviceImportModeSNMP && port != nil && *port != 161 {
		return host, port, "direct SNMP target port must be 161"
	}
	return host, port, ""
}

func splitCanonicalImportEndpoint(rawTarget string) (string, *uint16, string) {
	if address, err := netip.ParseAddr(rawTarget); err == nil {
		return address.String(), nil, ""
	}

	host := rawTarget
	var explicitPort *uint16
	if strings.Contains(rawTarget, ":") {
		parsedHost, portText, err := net.SplitHostPort(rawTarget)
		if err != nil {
			return "", nil, "target endpoint is malformed"
		}
		port, err := strconv.ParseUint(portText, 10, 16)
		if err != nil || port == 0 {
			return "", nil, "target port must be between 1 and 65535"
		}
		host = parsedHost
		portValue := uint16(port)
		explicitPort = &portValue
	}

	if address, err := netip.ParseAddr(host); err == nil {
		return address.String(), explicitPort, ""
	}
	if !isValidImportHostname(host) {
		return "", nil, "target host must be a valid IP address or RFC 1123 hostname"
	}
	return strings.ToLower(host), explicitPort, ""
}

// isValidImportHostname mirrors Theia's create-device RFC 1123 hostname rules.
func isValidImportHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		hasLetter := false
		for index, character := range label {
			switch {
			case (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z'):
				hasLetter = true
			case character >= '0' && character <= '9':
				// Digits are allowed, but a hostname label cannot be entirely numeric.
			case character == '-' && index > 0 && index < len(label)-1:
				// Hyphens are allowed only inside a label.
			default:
				return false
			}
		}
		if !hasLetter {
			return false
		}
	}
	return true
}
