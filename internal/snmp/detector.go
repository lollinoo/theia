package snmp

import (
	"strings"

	"github.com/azmin/mikrotik-theia/internal/domain"
)

// DetectDeviceType attempts to identify the device type from its sysObjectID and sysDescr.
func DetectDeviceType(sysObjectID string, sysDescr string) domain.DeviceType {
	// Clean OID string (remove leading dot if present)
	oid := strings.TrimPrefix(sysObjectID, ".")
	descLower := strings.ToLower(sysDescr)

	// 1. Enterprise OID Prefix matching (Most reliable)

	// MikroTik (1.3.6.1.4.1.14988)
	if strings.HasPrefix(oid, "1.3.6.1.4.1.14988") {
		if strings.Contains(descLower, "routeros") {
			return domain.DeviceTypeRouter
		}
		if strings.Contains(descLower, "swos") {
			return domain.DeviceTypeSwitch
		}
		return domain.DeviceTypeUnknown
	}

	// Cisco (1.3.6.1.4.1.9)
	if strings.HasPrefix(oid, "1.3.6.1.4.1.9") {
		if strings.Contains(descLower, "c9000") || strings.Contains(descLower, "isr") || strings.Contains(descLower, "asr") {
			return domain.DeviceTypeRouter
		}
		if strings.Contains(descLower, "catalyst") || strings.Contains(descLower, "c2960") || strings.Contains(descLower, "c3750") {
			return domain.DeviceTypeSwitch
		}
		// If it has multiple ports or acts as L2, fallback might catch it, but IOS is tricky without more context.
		// We'll mark string patterns below.
	}

	// Ubiquiti (1.3.6.1.4.1.41112)
	if strings.HasPrefix(oid, "1.3.6.1.4.1.41112") {
		if strings.Contains(descLower, "edgerouter") {
			return domain.DeviceTypeRouter
		}
		if strings.Contains(descLower, "us-") || strings.Contains(descLower, "unifi switch") {
			return domain.DeviceTypeSwitch
		}
		if strings.Contains(descLower, "uap") || strings.Contains(descLower, "u6") {
			return domain.DeviceTypeAP
		}
	}

	// 2. Heuristic string matching on sysDescr (Fallback)
	if strings.Contains(descLower, "router") {
		return domain.DeviceTypeRouter
	}
	if strings.Contains(descLower, "switch") {
		return domain.DeviceTypeSwitch
	}
	if strings.Contains(descLower, "access point") || strings.Contains(descLower, "wireless ap") {
		return domain.DeviceTypeAP
	}

	return domain.DeviceTypeUnknown
}
