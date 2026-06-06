package snmp

// This file defines detector SNMP collection and device-detection behavior.

import (
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/vendor"
)

// DetectVendor uses the vendor registry to identify a device's vendor, device type,
// and hardware model from SNMP system info.
func DetectVendor(registry *vendor.Registry, sysObjectID, sysDescr string) (vendorName string, deviceType domain.DeviceType, hardwareModel string) {
	matched := registry.Match(sysObjectID, sysDescr)
	vendorName = matched.Vendor.Name

	dt := registry.ResolveDeviceType(vendorName, sysDescr)
	deviceType = domain.DeviceType(dt)

	hardwareModel = registry.ExtractModel(vendorName, sysDescr)

	return vendorName, deviceType, hardwareModel
}
