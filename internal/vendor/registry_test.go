package vendor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRegistryLoadAndMatch(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := `
vendor:
  name: default
  display_name: Generic SNMP

detection:
  sys_object_id_prefixes: []

metrics:
  prometheus:
    cpu: 'hrProcessorLoad{%label%}'
    memory: 'hrStorageUsed{%label%}'
    temperature: 'entPhySensorValue{%label%}'
    uptime: 'sysUpTime{%label%}'

snmp:
  cpu_oid: ".1.3.6.1.2.1.25.3.2.1.5"
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

snmp:
  temperature_oid: ".1.3.6.1.4.1.14988.1.1.3.10.0"
  temperature_scale: 0.1
`

	os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644)
	os.WriteFile(filepath.Join(dir, "mikrotik.yaml"), []byte(mikrotikYAML), 0644)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}

	// MikroTik matched by OID prefix
	v := reg.Match("1.3.6.1.4.1.14988.1.1.2", "RouterOS RB5009UG+S+")
	if v.Vendor.Name != "mikrotik" {
		t.Errorf("expected mikrotik, got %q", v.Vendor.Name)
	}

	// Unknown device falls back to default
	v = reg.Match("1.3.6.1.4.1.9.1.2500", "Cisco IOS whatever")
	if v.Vendor.Name != "default" {
		t.Errorf("expected default, got %q", v.Vendor.Name)
	}

	// sysDescr pattern fallback
	v = reg.Match("1.3.6.1.4.1.99999", "RouterOS something")
	if v.Vendor.Name != "mikrotik" {
		t.Errorf("expected mikrotik via sysDescr, got %q", v.Vendor.Name)
	}
}

func TestResolveMetrics(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := `
vendor:
  name: default
  display_name: Generic SNMP

metrics:
  prometheus:
    cpu: "hrProcessorLoad{%label%}"
    memory: "hrStorageUsed{%label%}"
    temperature: "entPhySensorValue{%label%}"
    uptime: "sysUpTime{%label%}"
`
	mikrotikYAML := `
vendor:
  name: mikrotik
  display_name: MikroTik

detection:
  sys_object_id_prefixes:
    - "1.3.6.1.4.1.14988"

metrics:
  prometheus:
    cpu: "mtxrHlCpuLoad{%label%}"
`
	os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644)
	os.WriteFile(filepath.Join(dir, "mikrotik.yaml"), []byte(mikrotikYAML), 0644)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}

	m := reg.ResolvePrometheusMetrics("mikrotik")
	if m.CPU != "mtxrHlCpuLoad{%label%}" {
		t.Errorf("expected mikrotik cpu, got %q", m.CPU)
	}
	if m.Memory != "hrStorageUsed{%label%}" {
		t.Errorf("expected default memory, got %q", m.Memory)
	}
	if m.Temperature != "entPhySensorValue{%label%}" {
		t.Errorf("expected default temperature, got %q", m.Temperature)
	}

	m = reg.ResolvePrometheusMetrics("unknown_vendor")
	if m.CPU != "hrProcessorLoad{%label%}" {
		t.Errorf("expected default cpu, got %q", m.CPU)
	}
}

func TestResolveDeviceType(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := `
vendor:
  name: default
  display_name: Generic SNMP

device_type_rules:
  - match:
      sys_descr_contains: "router"
    type: router
  - match:
      sys_descr_contains: "switch"
    type: switch
  - match:
      sys_descr_contains: "access point"
    type: ap
  - type: unknown
`
	mikrotikYAML := `
vendor:
  name: mikrotik
  display_name: MikroTik

detection:
  sys_object_id_prefixes:
    - "1.3.6.1.4.1.14988"

device_type_rules:
  - match:
      sys_descr_contains: "RouterOS"
    type: router
  - match:
      sys_descr_contains: "SwOS"
    type: switch
  - type: unknown
`
	os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644)
	os.WriteFile(filepath.Join(dir, "mikrotik.yaml"), []byte(mikrotikYAML), 0644)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}

	if dt := reg.ResolveDeviceType("mikrotik", "RouterOS RB5009"); dt != "router" {
		t.Errorf("expected router, got %q", dt)
	}
	if dt := reg.ResolveDeviceType("mikrotik", "SwOS 2.14"); dt != "switch" {
		t.Errorf("expected switch, got %q", dt)
	}
	if dt := reg.ResolveDeviceType("default", "Some random router"); dt != "router" {
		t.Errorf("expected router, got %q", dt)
	}
}

func TestExtractModel(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := `
vendor:
  name: default
  display_name: Generic SNMP
`
	mikrotikYAML := `
vendor:
  name: mikrotik
  display_name: MikroTik

detection:
  sys_object_id_prefixes:
    - "1.3.6.1.4.1.14988"

model_extraction:
  sys_descr_regex: "RouterOS\\s+(\\S+)"
  capture_group: 1
`
	os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644)
	os.WriteFile(filepath.Join(dir, "mikrotik.yaml"), []byte(mikrotikYAML), 0644)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}

	if model := reg.ExtractModel("mikrotik", "RouterOS RB5009UG+S+ (stable)"); model != "RB5009UG+S+" {
		t.Errorf("expected RB5009UG+S+, got %q", model)
	}
	if model := reg.ExtractModel("default", "Some device"); model != "Unknown" {
		t.Errorf("expected Unknown, got %q", model)
	}
}

func TestResolveSNMPConfig(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := `
vendor:
  name: default
  display_name: Generic SNMP

snmp:
  temperature_oid: ".1.3.6.1.2.1.99.1.1.1.4"
  temperature_scale: 1.0
  cpu_oid: ".1.3.6.1.2.1.25.3.2.1.5"
`
	mikrotikYAML := `
vendor:
  name: mikrotik
  display_name: MikroTik

detection:
  sys_object_id_prefixes:
    - "1.3.6.1.4.1.14988"

snmp:
  temperature_oid: ".1.3.6.1.4.1.14988.1.1.3.10.0"
  temperature_scale: 0.1
`
	os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644)
	os.WriteFile(filepath.Join(dir, "mikrotik.yaml"), []byte(mikrotikYAML), 0644)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}

	s := reg.ResolveSNMPConfig("mikrotik")
	if s.TemperatureOID != ".1.3.6.1.4.1.14988.1.1.3.10.0" {
		t.Errorf("expected mikrotik temp OID, got %q", s.TemperatureOID)
	}
	if s.TemperatureScale != 0.1 {
		t.Errorf("expected 0.1 scale, got %f", s.TemperatureScale)
	}
	if s.CPUOID != ".1.3.6.1.2.1.25.3.2.1.5" {
		t.Errorf("expected default cpu OID, got %q", s.CPUOID)
	}
}

func TestLoadRealVendors(t *testing.T) {
	reg, err := LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded failed: %v", err)
	}

	v := reg.Match("1.3.6.1.4.1.14988.1.1.2", "RouterOS RB5009")
	if v.Vendor.Name != "mikrotik" {
		t.Errorf("expected mikrotik, got %q", v.Vendor.Name)
	}

	// Unknown device gets default
	v = reg.Match("1.3.6.1.4.1.9.1.2500", "Cisco IOS")
	if v.Vendor.Name != "default" {
		t.Errorf("expected default, got %q", v.Vendor.Name)
	}
}

// ---------------------------------------------------------------------------
// TestRegistryConcurrency (DEBT-09)
// ---------------------------------------------------------------------------
// Verifies that the Registry is safe for concurrent read and write access.
// Without mutex protection, the race detector will flag data races on the
// vendors slice and fallback struct fields.
//
// This test is designed to be run with -race. It will PASS without -race
// even on unprotected code, but FAIL with -race when the Registry lacks
// synchronization.
func TestRegistryConcurrency(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := `
vendor:
  name: default
  display_name: Generic SNMP

detection:
  sys_object_id_prefixes: []

metrics:
  prometheus:
    cpu: 'hrProcessorLoad{%label%}'
    memory: 'hrStorageUsed{%label%}'
    temperature: 'entPhySensorValue{%label%}'
    uptime: 'sysUpTime{%label%}'

snmp:
  cpu_oid: ".1.3.6.1.2.1.25.3.2.1.5"
  temperature_oid: ".1.3.6.1.2.1.99.1.1.1.4"
  temperature_scale: 1.0

backup:
  supported: false
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

backup:
  supported: true
  methods:
    - ssh
  ssh_commands:
    export_running: "/export"
`

	os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644)
	os.WriteFile(filepath.Join(dir, "mikrotik.yaml"), []byte(mikrotikYAML), 0644)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}

	var wg sync.WaitGroup

	// 10 reader goroutines doing concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = reg.Match("1.3.6.1.4.1.14988.1.1.2", "RouterOS RB5009")
				_ = reg.ResolveBackupConfig("mikrotik")
				_ = reg.ResolvePrometheusMetrics("mikrotik")
				_ = reg.ResolveSNMPConfig("mikrotik")
				_ = reg.ResolveDeviceType("mikrotik", "RouterOS RB5009")
				_ = reg.ExtractModel("mikrotik", "RouterOS RB5009")
				_ = reg.GetAllVendorNames()
				_ = reg.GetDisplayName("mikrotik")
				_ = reg.VendorCount()
			}
		}()
	}

	// 2 writer goroutines doing concurrent config updates
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				cfg := VendorConfig{
					Vendor: VendorInfo{Name: "mikrotik", DisplayName: "MikroTik Updated"},
					Detection: Detection{
						SysObjectIDPrefixes: []string{"1.3.6.1.4.1.14988"},
						SysDescrPatterns:    []string{"RouterOS"},
					},
					Backup: BackupConfig{Supported: true},
				}
				data, _ := json.Marshal(cfg)
				_ = reg.UpdateConfig("mikrotik", data)
			}
		}()
	}

	wg.Wait()

	// Verify registry is still functional after concurrent operations
	v := reg.Match("1.3.6.1.4.1.14988.1.1.2", "RouterOS RB5009")
	if v.Vendor.Name != "mikrotik" {
		t.Errorf("expected mikrotik after concurrent ops, got %q", v.Vendor.Name)
	}

	if reg.VendorCount() < 2 {
		t.Errorf("expected at least 2 vendors (default + mikrotik), got %d", reg.VendorCount())
	}
}
