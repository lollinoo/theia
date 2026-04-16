package domain

import "strings"

// NormalizeVirtualNoIPDevice enforces the invariant that virtual devices
// without an IP are inert canvas nodes: they keep an unknown status and do not
// participate in live metrics collection.
func NormalizeVirtualNoIPDevice(device *Device) bool {
	if device == nil {
		return false
	}
	if device.DeviceType != DeviceTypeVirtual || device.IP != "" {
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
