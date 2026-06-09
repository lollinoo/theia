package domain

// This file defines device normalization domain contracts and lifecycle invariants.

import (
	"sort"
	"strings"

	"github.com/google/uuid"
)

// IsVirtualWithIPDevice reports whether the device is virtual and has an IP
// address, which means it participates only in lightweight reachability
// probing rather than SNMP collection.
func IsVirtualWithIPDevice(device Device) bool {
	return device.DeviceType == DeviceTypeVirtual && strings.TrimSpace(device.IP) != ""
}

// IsVirtualNoIPDevice reports whether the device is a virtual placeholder
// without an IP address and therefore must stay unmonitored.
func IsVirtualNoIPDevice(device Device) bool {
	return device.DeviceType == DeviceTypeVirtual && strings.TrimSpace(device.IP) == ""
}

// NormalizeVirtualDevice enforces the runtime collection invariants for
// virtual nodes. Virtual nodes never use scalar metric collection; nodes with
// an IP are probed only for lightweight reachability, while nodes without an
// IP stay inert on the canvas.
func NormalizeVirtualDevice(device *Device) bool {
	if device == nil || device.DeviceType != DeviceTypeVirtual {
		return false
	}

	changed := false
	if device.MetricsSource != MetricsSourceNone {
		device.MetricsSource = MetricsSourceNone
		changed = true
	}
	if IsVirtualNoIPDevice(*device) && device.Status != DeviceStatusUnknown {
		device.Status = DeviceStatusUnknown
		changed = true
	}

	return changed
}

// NormalizeVirtualNoIPDevice enforces the invariant that virtual devices
// without an IP are inert canvas nodes: they keep an unknown status and do not
// participate in live metrics collection.
func NormalizeVirtualNoIPDevice(device *Device) bool {
	if device == nil {
		return false
	}
	if !IsVirtualNoIPDevice(*device) {
		return false
	}

	changed := false
	if device.Status != DeviceStatusUnknown {
		device.Status = DeviceStatusUnknown
		changed = true
	}
	if device.MetricsSource != MetricsSourceNone {
		device.MetricsSource = MetricsSourceNone
		changed = true
	}

	return changed
}

// NormalizeDeviceNotes trims user-entered notes and collapses blank values to nil.
func NormalizeDeviceNotes(notes *string) *string {
	if notes == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*notes)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

// NormalizeDeviceAddressValue canonicalizes address strings for comparisons.
func NormalizeDeviceAddressValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// NormalizeDeviceAddressRole returns a supported address role, defaulting to other.
func NormalizeDeviceAddressRole(role DeviceAddressRole) DeviceAddressRole {
	switch DeviceAddressRole(strings.ToLower(strings.TrimSpace(string(role)))) {
	case DeviceAddressRolePrimary:
		return DeviceAddressRolePrimary
	case DeviceAddressRoleManagement:
		return DeviceAddressRoleManagement
	case DeviceAddressRoleBackup:
		return DeviceAddressRoleBackup
	case DeviceAddressRoleMonitoring:
		return DeviceAddressRoleMonitoring
	case DeviceAddressRoleOther:
		return DeviceAddressRoleOther
	default:
		return DeviceAddressRoleOther
	}
}

// NormalizeDeviceAddresses trims, deduplicates, and synchronizes Device.IP with
// the primary address while preserving the legacy IP field as authoritative when set.
func NormalizeDeviceAddresses(device *Device) {
	if device == nil {
		return
	}

	device.IP = strings.TrimSpace(device.IP)
	legacyIP := device.IP
	seen := make(map[string]int, len(device.Addresses)+1)
	addresses := make([]DeviceAddress, 0, len(device.Addresses)+1)

	for _, address := range device.Addresses {
		address.Address = strings.TrimSpace(address.Address)
		if address.Address == "" {
			continue
		}
		normalized := NormalizeDeviceAddressValue(address.Address)
		if _, exists := seen[normalized]; exists {
			continue
		}
		address.Label = strings.TrimSpace(address.Label)
		address.Role = NormalizeDeviceAddressRole(address.Role)
		if device.ID != uuid.Nil && address.DeviceID == uuid.Nil {
			address.DeviceID = device.ID
		}
		seen[normalized] = len(addresses)
		addresses = append(addresses, address)
	}

	if legacyIP != "" {
		normalizedLegacy := NormalizeDeviceAddressValue(legacyIP)
		if index, exists := seen[normalizedLegacy]; exists {
			addresses[index].Address = legacyIP
			addresses[index].IsPrimary = true
			addresses[index].Role = DeviceAddressRolePrimary
			addresses[index].Priority = 0
			if device.ID != uuid.Nil && addresses[index].DeviceID == uuid.Nil {
				addresses[index].DeviceID = device.ID
			}
		} else {
			addresses = append([]DeviceAddress{{
				DeviceID:  device.ID,
				Address:   legacyIP,
				Label:     "Primary",
				Role:      DeviceAddressRolePrimary,
				IsPrimary: true,
				Priority:  0,
			}}, addresses...)
		}
	} else if len(addresses) > 0 {
		primaryIndex := firstPrimaryAddressIndex(addresses)
		if primaryIndex < 0 {
			primaryIndex = 0
		}
		device.IP = addresses[primaryIndex].Address
		addresses[primaryIndex].IsPrimary = true
		addresses[primaryIndex].Role = DeviceAddressRolePrimary
		addresses[primaryIndex].Priority = 0
	} else {
		device.Addresses = nil
		return
	}

	sort.SliceStable(addresses, func(i, j int) bool {
		if addresses[i].IsPrimary != addresses[j].IsPrimary {
			return addresses[i].IsPrimary
		}
		if addresses[i].Priority != addresses[j].Priority {
			return addresses[i].Priority < addresses[j].Priority
		}
		return NormalizeDeviceAddressValue(addresses[i].Address) < NormalizeDeviceAddressValue(addresses[j].Address)
	})

	primarySeen := false
	for i := range addresses {
		if device.ID != uuid.Nil && addresses[i].DeviceID == uuid.Nil {
			addresses[i].DeviceID = device.ID
		}
		if addresses[i].IsPrimary && !primarySeen {
			addresses[i].IsPrimary = true
			addresses[i].Role = DeviceAddressRolePrimary
			addresses[i].Priority = 0
			device.IP = addresses[i].Address
			primarySeen = true
			continue
		}
		addresses[i].IsPrimary = false
		if addresses[i].Role == DeviceAddressRolePrimary {
			addresses[i].Role = DeviceAddressRoleOther
		}
	}

	device.Addresses = addresses
}

func firstPrimaryAddressIndex(addresses []DeviceAddress) int {
	for i, address := range addresses {
		if address.IsPrimary || NormalizeDeviceAddressRole(address.Role) == DeviceAddressRolePrimary {
			return i
		}
	}
	return -1
}

// PrimaryAddress returns the current primary address for a device.
func PrimaryAddress(device Device) string {
	for _, address := range device.Addresses {
		if address.IsPrimary || NormalizeDeviceAddressRole(address.Role) == DeviceAddressRolePrimary {
			if strings.TrimSpace(address.Address) != "" {
				return strings.TrimSpace(address.Address)
			}
		}
	}
	return strings.TrimSpace(device.IP)
}

// AddressForRole returns the highest-priority address matching the requested role.
func AddressForRole(device Device, role DeviceAddressRole) string {
	role = NormalizeDeviceAddressRole(role)
	if role == DeviceAddressRolePrimary {
		return PrimaryAddress(device)
	}

	best := ""
	bestPriority := int(^uint(0) >> 1)
	for _, address := range device.Addresses {
		if NormalizeDeviceAddressRole(address.Role) != role {
			continue
		}
		trimmed := strings.TrimSpace(address.Address)
		if trimmed == "" {
			continue
		}
		if best == "" || address.Priority < bestPriority {
			best = trimmed
			bestPriority = address.Priority
		}
	}
	return best
}

// BackupAddress chooses the backup target, then management, then primary.
func BackupAddress(device Device) string {
	if address := AddressForRole(device, DeviceAddressRoleBackup); address != "" {
		return address
	}
	if address := AddressForRole(device, DeviceAddressRoleManagement); address != "" {
		return address
	}
	return PrimaryAddress(device)
}

// DeviceAddressValues returns all normalized device addresses in comparison order.
func DeviceAddressValues(device Device) []string {
	seen := make(map[string]struct{}, len(device.Addresses)+1)
	values := make([]string, 0, len(device.Addresses)+1)
	if primary := strings.TrimSpace(device.IP); primary != "" {
		normalized := NormalizeDeviceAddressValue(primary)
		seen[normalized] = struct{}{}
		values = append(values, primary)
	}
	for _, address := range device.Addresses {
		trimmed := strings.TrimSpace(address.Address)
		if trimmed == "" {
			continue
		}
		normalized := NormalizeDeviceAddressValue(trimmed)
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		values = append(values, trimmed)
	}
	return values
}

// DeviceAddressesEqual compares address collections by their normalized public fields.
func DeviceAddressesEqual(first, second []DeviceAddress) bool {
	if len(first) != len(second) {
		return false
	}
	for i := range first {
		if NormalizeDeviceAddressValue(first[i].Address) != NormalizeDeviceAddressValue(second[i].Address) {
			return false
		}
		if strings.TrimSpace(first[i].Label) != strings.TrimSpace(second[i].Label) {
			return false
		}
		if NormalizeDeviceAddressRole(first[i].Role) != NormalizeDeviceAddressRole(second[i].Role) {
			return false
		}
		if first[i].IsPrimary != second[i].IsPrimary {
			return false
		}
		if first[i].Priority != second[i].Priority {
			return false
		}
	}
	return true
}
