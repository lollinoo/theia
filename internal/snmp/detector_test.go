package snmp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/vendor"
)

// testRegistry creates a vendor registry with default + selected vendor profiles for testing.
func testRegistry(t *testing.T) *vendor.Registry {
	t.Helper()
	dir := t.TempDir()

	defaultYAML := `
vendor:
  name: default
  display_name: Generic SNMP

detection:
  sys_object_id_prefixes: []

device_type_rules:
  - match:
      sys_descr_contains: "Router"
    type: router
  - match:
      sys_descr_contains: "Switch"
    type: switch
  - match:
      sys_descr_contains: "AP"
    type: ap
  - match:
      sys_descr_contains: "Access Point"
    type: ap
  - type: unknown

metrics:
  prometheus:
    cpu: 'hrProcessorLoad{%[1]s=~"%[2]s"}'
    memory: 'hrStorageUsed{%[1]s=~"%[2]s"}'
    temperature: 'entPhySensorValue{%[1]s=~"%[2]s"}'
    uptime: 'sysUpTime{%[1]s=~"%[2]s"}'

snmp:
  cpu_oid: ".1.3.6.1.2.1.25.3.3.1.2"
  temperature_oid: ".1.3.6.1.2.1.99.1.1.1.4"
  temperature_scale: 1.0
`
	mikrotikYAML := `
vendor:
  name: mikrotik
  display_name: MikroTik

detection:
  sys_object_id_prefixes:
    - "1.3.6.1.4.1.14988"
  sys_descr_patterns:
    - "RouterOS"
    - "SwOS"

device_type_rules:
  - match:
      sys_descr_contains: "RouterOS"
    type: router
  - match:
      sys_descr_contains: "SwOS"
    type: switch
  - type: unknown

model_extraction:
  sys_descr_regex: "(?:RouterOS|SwOS)\\s+(\\S+)"
  capture_group: 1

metrics:
  prometheus:
    cpu: 'mtxrHlCpuLoad{%[1]s=~"%[2]s"}'

snmp:
  static:
    software_version_oid: ".1.3.6.1.4.1.14988.1.1.4.4.0"
  performance:
    temperature_oid: ".1.3.6.1.4.1.14988.1.1.3.10.0"
    temperature_scale: 0.1
`
	ubiquitiYAML := `
vendor:
  name: ubiquiti
  display_name: Ubiquiti

detection:
  sys_object_id_prefixes:
    - "1.3.6.1.4.1.41112"
  sys_descr_patterns:
    - "airMAX"

device_type_rules:
  - match:
      sys_descr_contains: "Access Point"
    type: ap
  - match:
      sys_descr_contains: "Router"
    type: router
  - match:
      sys_descr_contains: "CPE"
    type: router
  - type: unknown

model_extraction:
  sys_descr_regex: "(?i)(?:Access Point|Router CPE)\\s+(.+?)\\s+[0-9]+(?:\\.[0-9]+)+"
  capture_group: 1
`

	os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644)
	os.WriteFile(filepath.Join(dir, "mikrotik.yaml"), []byte(mikrotikYAML), 0644)
	os.WriteFile(filepath.Join(dir, "ubiquiti.yaml"), []byte(ubiquitiYAML), 0644)

	reg, err := vendor.LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	return reg
}

func TestDetectVendor(t *testing.T) {
	reg := testRegistry(t)

	tests := []struct {
		name        string
		sysObjectID string
		sysDescr    string
		wantVendor  string
		wantType    domain.DeviceType
	}{
		// MikroTik
		{
			name:        "MikroTik Router",
			sysObjectID: "1.3.6.1.4.1.14988.1",
			sysDescr:    "RouterOS RB5009",
			wantVendor:  "mikrotik",
			wantType:    domain.DeviceTypeRouter,
		},
		{
			name:        "MikroTik Switch",
			sysObjectID: ".1.3.6.1.4.1.14988.1",
			sysDescr:    "SwOS CRS326",
			wantVendor:  "mikrotik",
			wantType:    domain.DeviceTypeSwitch,
		},
		// Unknown vendor — falls back to default with generic rules
		{
			name:        "Generic Router via Descr",
			sysObjectID: "1.3.6.1.4.1.9999",
			sysDescr:    "Generic Linux Router",
			wantVendor:  "default",
			wantType:    domain.DeviceTypeRouter,
		},
		{
			name:        "Generic Switch via Descr",
			sysObjectID: "1.3.6.1.4.1.9999",
			sysDescr:    "Managed Switch 24-port",
			wantVendor:  "default",
			wantType:    domain.DeviceTypeSwitch,
		},
		{
			name:        "Ubiquiti Sector AP",
			sysObjectID: "1.3.6.1.4.1.41112.1.6",
			sysDescr:    "airMAX Wireless Access Point Rocket Prism 5AC Gen2 8.7.4",
			wantVendor:  "ubiquiti",
			wantType:    domain.DeviceTypeAP,
		},
		{
			name:        "Ubiquiti CPE",
			sysObjectID: "1.3.6.1.4.1.41112.1.10",
			sysDescr:    "airMAX Wireless Router CPE LiteBeam 5AC Gen2 8.7.4",
			wantVendor:  "ubiquiti",
			wantType:    domain.DeviceTypeRouter,
		},
		{
			name:        "Unknown device",
			sysObjectID: "1.3.6.1.4.1.9999",
			sysDescr:    "Linux web server",
			wantVendor:  "default",
			wantType:    domain.DeviceTypeUnknown,
		},
		{
			name:        "Empty fields",
			sysObjectID: "",
			sysDescr:    "",
			wantVendor:  "default",
			wantType:    domain.DeviceTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVendor, gotType, _ := DetectVendor(reg, tt.sysObjectID, tt.sysDescr)
			if gotVendor != tt.wantVendor {
				t.Errorf("DetectVendor() vendor = %q, want %q", gotVendor, tt.wantVendor)
			}
			if gotType != tt.wantType {
				t.Errorf("DetectVendor() type = %v, want %v", gotType, tt.wantType)
			}
		})
	}
}
