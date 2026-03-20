package vendor

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestVendorConfigUnmarshal(t *testing.T) {
	raw := `
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
  sys_descr_regex: "RouterOS\\s+(\\S+)"
  capture_group: 1

metrics:
  prometheus:
    cpu: "mtxrHlCpuLoad{%label%}"
    memory: "100 * (1 - mtxrHlFreeMemory{%label%} / mtxrHlTotalMemory{%label%})"
    temperature: "mtxrHlTemperature{%label%}"
    uptime: "mtxrHlUpTime{%label%} / 100"

snmp:
  temperature_oid: ".1.3.6.1.4.1.14988.1.1.3.10.0"
  temperature_scale: 0.1
`
	var cfg VendorConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if cfg.Vendor.Name != "mikrotik" {
		t.Errorf("expected vendor name 'mikrotik', got %q", cfg.Vendor.Name)
	}
	if cfg.Vendor.DisplayName != "MikroTik" {
		t.Errorf("expected display name 'MikroTik', got %q", cfg.Vendor.DisplayName)
	}
	if len(cfg.Detection.SysObjectIDPrefixes) != 1 {
		t.Fatalf("expected 1 OID prefix, got %d", len(cfg.Detection.SysObjectIDPrefixes))
	}
	if cfg.Detection.SysObjectIDPrefixes[0] != "1.3.6.1.4.1.14988" {
		t.Errorf("unexpected OID prefix: %s", cfg.Detection.SysObjectIDPrefixes[0])
	}
	if len(cfg.DeviceTypeRules) != 3 {
		t.Fatalf("expected 3 device type rules, got %d", len(cfg.DeviceTypeRules))
	}
	if cfg.DeviceTypeRules[0].Type != "router" {
		t.Errorf("expected first rule type 'router', got %q", cfg.DeviceTypeRules[0].Type)
	}
	if cfg.ModelExtraction.SysDescrRegex != `RouterOS\s+(\S+)` {
		t.Errorf("unexpected regex: %s", cfg.ModelExtraction.SysDescrRegex)
	}
	if cfg.Metrics.Prometheus.CPU != "mtxrHlCpuLoad{%label%}" {
		t.Errorf("unexpected cpu query: %s", cfg.Metrics.Prometheus.CPU)
	}
	if cfg.SNMP.TemperatureOID != ".1.3.6.1.4.1.14988.1.1.3.10.0" {
		t.Errorf("expected temp OID, got %q", cfg.SNMP.TemperatureOID)
	}
	if cfg.SNMP.TemperatureScale != 0.1 {
		t.Errorf("expected temp scale 0.1, got %f", cfg.SNMP.TemperatureScale)
	}
}
