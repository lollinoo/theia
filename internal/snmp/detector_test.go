package snmp

import (
	"testing"

	"github.com/azmin/mikrotik-theia/internal/domain"
)

func TestDetectDeviceType(t *testing.T) {
	tests := []struct {
		name        string
		sysObjectID string
		sysDescr    string
		expected    domain.DeviceType
	}{
		// MikroTik
		{
			name:        "MikroTik Router",
			sysObjectID: "1.3.6.1.4.1.14988.1",
			sysDescr:    "RouterOS RB5009",
			expected:    domain.DeviceTypeRouter,
		},
		{
			name:        "MikroTik Switch",
			sysObjectID: ".1.3.6.1.4.1.14988.1", // with leading dot
			sysDescr:    "SwOS CRS326",
			expected:    domain.DeviceTypeSwitch,
		},
		// Cisco
		{
			name:        "Cisco Switch Catalyst",
			sysObjectID: "1.3.6.1.4.1.9.1.1208", // typical catalyst
			sysDescr:    "Cisco IOS Software, C2960 Software",
			expected:    domain.DeviceTypeSwitch,
		},
		{
			name:        "Cisco Router ISR",
			sysObjectID: "1.3.6.1.4.1.9.1.2068",
			sysDescr:    "Cisco IOS Software, ISR Software",
			expected:    domain.DeviceTypeRouter,
		},
		// Ubiquiti
		{
			name:        "Ubiquiti EdgeRouter",
			sysObjectID: "1.3.6.1.4.1.41112.1.2",
			sysDescr:    "EdgeOS EdgeRouter 4",
			expected:    domain.DeviceTypeRouter,
		},
		{
			name:        "Ubiquiti AP",
			sysObjectID: "1.3.6.1.4.1.41112.1.6",
			sysDescr:    "UAP-AC-Pro",
			expected:    domain.DeviceTypeAP,
		},
		{
			name:        "Ubiquiti U6 AP",
			sysObjectID: "1.3.6.1.4.1.41112.1.6",
			sysDescr:    "U6-LR 6.5.28",
			expected:    domain.DeviceTypeAP,
		},
		{
			name:        "Ubiquiti Switch US-24",
			sysObjectID: "1.3.6.1.4.1.41112.1.3",
			sysDescr:    "US-24-250W, 6.2.14",
			expected:    domain.DeviceTypeSwitch,
		},
		{
			name:        "Ubiquiti UniFi Switch",
			sysObjectID: "1.3.6.1.4.1.41112.1.3",
			sysDescr:    "UniFi Switch 48 PoE",
			expected:    domain.DeviceTypeSwitch,
		},
		// Cisco extended
		{
			name:        "Cisco Switch C3750",
			sysObjectID: "1.3.6.1.4.1.9.1.516",
			sysDescr:    "Cisco IOS Software, C3750 Software",
			expected:    domain.DeviceTypeSwitch,
		},
		{
			name:        "Cisco Router ASR",
			sysObjectID: "1.3.6.1.4.1.9.1.1928",
			sysDescr:    "Cisco IOS XE Software, ASR1002-HX",
			expected:    domain.DeviceTypeRouter,
		},
		{
			name:        "Cisco Router C9000",
			sysObjectID: "1.3.6.1.4.1.9.1.2614",
			sysDescr:    "Cisco IOS XE Software, C9000 Series",
			expected:    domain.DeviceTypeRouter,
		},
		// Fallbacks
		{
			name:        "Generic Router via Descr",
			sysObjectID: "1.3.6.1.4.1.9999",
			sysDescr:    "Generic Linux Router",
			expected:    domain.DeviceTypeRouter,
		},
		{
			name:        "Generic Switch via Descr",
			sysObjectID: "1.3.6.1.4.1.9999",
			sysDescr:    "Managed Switch 24-port",
			expected:    domain.DeviceTypeSwitch,
		},
		{
			name:        "Generic AP via Descr",
			sysObjectID: "1.3.6.1.4.1.9999",
			sysDescr:    "802.11ac Access Point",
			expected:    domain.DeviceTypeAP,
		},
		{
			name:        "Unknown sysDescr returns Unknown",
			sysObjectID: "1.3.6.1.4.1.9999",
			sysDescr:    "Linux web server",
			expected:    domain.DeviceTypeUnknown,
		},
		{
			name:        "Empty sysDescr returns Unknown",
			sysObjectID: "",
			sysDescr:    "",
			expected:    domain.DeviceTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectDeviceType(tt.sysObjectID, tt.sysDescr)
			if got != tt.expected {
				t.Errorf("DetectDeviceType() = %v, want %v", got, tt.expected)
			}
		})
	}
}
