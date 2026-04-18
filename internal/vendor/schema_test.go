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
  static:
    software_version_oid: ".1.3.6.1.4.1.14988.1.1.4.4"
  operational: {}
  performance:
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
	// New nested shape assertions (replaces the old flat field assertions)
	if cfg.SNMP.Static.SoftwareVersionOID != ".1.3.6.1.4.1.14988.1.1.4.4" {
		t.Errorf("expected software version OID, got %q", cfg.SNMP.Static.SoftwareVersionOID)
	}
	if cfg.SNMP.Performance.TemperatureOID != ".1.3.6.1.4.1.14988.1.1.3.10.0" {
		t.Errorf("expected temp OID, got %q", cfg.SNMP.Performance.TemperatureOID)
	}
	if cfg.SNMP.Performance.TemperatureScale != 0.1 {
		t.Errorf("expected temp scale 0.1, got %f", cfg.SNMP.Performance.TemperatureScale)
	}
}

// TestVendorConfigUnmarshal_NestedSNMPGroups verifies that the three-tiered
// SNMP structure (static, operational, performance) unmarshals correctly from
// YAML, and that missing sub-sections produce zero-value structs (no panic).
func TestVendorConfigUnmarshal_NestedSNMPGroups(t *testing.T) {
	t.Run("all_three_groups_populated", func(t *testing.T) {
		raw := `
vendor:
  name: testvendor
  display_name: Test Vendor

snmp:
  static: {}
  operational:
    sys_uptime_oid: ".1.3.6.1.2.1.1.3.0"
    if_oper_status_oid: ".1.3.6.1.2.1.2.2.1.8"
  performance:
    cpu_oid: ".1.3.6.1.2.1.25.3.2.1.5"
    memory_used_oid: ".1.3.6.1.2.1.25.2.3.1.6"
    memory_total_oid: ".1.3.6.1.2.1.25.2.3.1.5"
    temperature_oid: ".1.3.6.1.2.1.99.1.1.1.4"
    temperature_scale: 1.0
`
		var cfg VendorConfig
		if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		// Static group — placeholder, zero value
		_ = cfg.SNMP.Static // must be accessible without panic

		// Operational group
		if cfg.SNMP.Operational.SysUpTimeOID != ".1.3.6.1.2.1.1.3.0" {
			t.Errorf("expected sysUpTime OID, got %q", cfg.SNMP.Operational.SysUpTimeOID)
		}
		if cfg.SNMP.Operational.IfOperStatusOID != ".1.3.6.1.2.1.2.2.1.8" {
			t.Errorf("expected ifOperStatus OID, got %q", cfg.SNMP.Operational.IfOperStatusOID)
		}

		// Performance group
		if cfg.SNMP.Performance.CPUOID != ".1.3.6.1.2.1.25.3.2.1.5" {
			t.Errorf("expected cpu OID, got %q", cfg.SNMP.Performance.CPUOID)
		}
		if cfg.SNMP.Performance.MemoryUsedOID != ".1.3.6.1.2.1.25.2.3.1.6" {
			t.Errorf("expected memory used OID, got %q", cfg.SNMP.Performance.MemoryUsedOID)
		}
		if cfg.SNMP.Performance.MemoryTotalOID != ".1.3.6.1.2.1.25.2.3.1.5" {
			t.Errorf("expected memory total OID, got %q", cfg.SNMP.Performance.MemoryTotalOID)
		}
		if cfg.SNMP.Performance.TemperatureOID != ".1.3.6.1.2.1.99.1.1.1.4" {
			t.Errorf("expected temperature OID, got %q", cfg.SNMP.Performance.TemperatureOID)
		}
		if cfg.SNMP.Performance.TemperatureScale != 1.0 {
			t.Errorf("expected temperature scale 1.0, got %f", cfg.SNMP.Performance.TemperatureScale)
		}
	})

	t.Run("missing_snmp_performance_section_no_panic", func(t *testing.T) {
		// T-39-05: missing snmp.performance should produce zero-value struct, not panic
		raw := `
vendor:
  name: testvendor
  display_name: Test Vendor

snmp:
  static: {}
  operational:
    sys_uptime_oid: ".1.3.6.1.2.1.1.3.0"
`
		var cfg VendorConfig
		if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		// Performance should be zero value — no panic
		if cfg.SNMP.Performance.CPUOID != "" {
			t.Errorf("expected empty CPUOID for missing performance section, got %q", cfg.SNMP.Performance.CPUOID)
		}
		if cfg.SNMP.Performance.TemperatureOID != "" {
			t.Errorf("expected empty TemperatureOID for missing performance section, got %q", cfg.SNMP.Performance.TemperatureOID)
		}
		if cfg.SNMP.Performance.TemperatureScale != 0 {
			t.Errorf("expected 0 TemperatureScale for missing performance section, got %f", cfg.SNMP.Performance.TemperatureScale)
		}
	})

	t.Run("no_snmp_section_at_all_no_panic", func(t *testing.T) {
		// Completely missing snmp section should not panic
		raw := `
vendor:
  name: testvendor
  display_name: Test Vendor
`
		var cfg VendorConfig
		if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		// All tiers zero value — no panic
		_ = cfg.SNMP.Static
		if cfg.SNMP.Operational.SysUpTimeOID != "" {
			t.Errorf("expected empty SysUpTimeOID, got %q", cfg.SNMP.Operational.SysUpTimeOID)
		}
		if cfg.SNMP.Performance.CPUOID != "" {
			t.Errorf("expected empty CPUOID, got %q", cfg.SNMP.Performance.CPUOID)
		}
	})
}
