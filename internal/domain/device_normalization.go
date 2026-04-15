package domain

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
