package domain

import "strings"

// IsVirtualNoIPDevice reports whether the device is a virtual placeholder
// without an IP address and therefore must stay unmonitored.
func IsVirtualNoIPDevice(device Device) bool {
	return device.DeviceType == DeviceTypeVirtual && strings.TrimSpace(device.IP) == ""
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
