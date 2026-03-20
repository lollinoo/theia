# Multi-Vendor Hub Conversion Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Convert MikroTik Theia into "Theia" — a vendor-agnostic network monitoring hub where vendors are defined via YAML configuration files, not hardcoded logic.

**Architecture:** A `vendors/` directory holds one YAML file per vendor. At startup, Theia loads all vendor definitions into an in-memory registry. During SNMP discovery, each device is matched to a vendor by sysObjectID prefix or sysDescr pattern. Metrics queries (both Prometheus and direct SNMP) are resolved through the vendor config with automatic fallback to `default.yaml` (standard MIBs). MikroTik becomes just another vendor file with no special treatment in code.

**Tech Stack:** Go 1.24, React 18 + TypeScript, SQLite, gosnmp, Prometheus HTTP API, YAML (gopkg.in/yaml.v3)

---

## Decision Log

| # | Decision | Alternatives Considered | Why |
|---|----------|------------------------|-----|
| 1 | Future-proof architecture, not building vendor configs now | Active multi-vendor support | No non-MikroTik devices to test against yet |
| 2 | Configuration-driven vendor definitions (YAML) | Hardcoded Go plugins, database-stored | No code changes to add vendors, version-controllable |
| 3 | One YAML file per vendor in `vendors/` directory | Single vendors file, database storage | Scales naturally, easy to contribute |
| 4 | Rename to "Theia" | "Theia Hub", "NetTheia" | Clean and simple |
| 5 | Generic SNMP baseline as default fallback | Reject unknown devices | Compatibility first |
| 6 | Vendor matching via sysObjectID prefix + sysDescr patterns | Manual vendor assignment | Standard SNMP fields, no extra config |
| 7 | Metrics resolution: vendor query -> default fallback | Vendor-only | Minimal YAML required, graceful degradation |
| 8 | Vendor as string field on Device, not FK | FK to vendor table | Loose coupling, vendor YAML can be added/removed |
| 9 | Re-detect vendor on every poll cycle | Only first discovery | New vendor configs picked up on restart |
| 10 | Frontend changes cosmetic only | Vendor-specific UI panels | YAGNI |

---

## Task 1: Add YAML dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add gopkg.in/yaml.v3**

```bash
cd /home/azmin/projects/mikrotik-theia && go get gopkg.in/yaml.v3
```

**Step 2: Verify**

```bash
grep "yaml.v3" go.mod
```
Expected: `gopkg.in/yaml.v3 v3.x.x`

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add gopkg.in/yaml.v3 dependency for vendor config system"
```

---

## Task 2: Create vendor YAML schema structs

**Files:**
- Create: `internal/vendor/schema.go`
- Test: `internal/vendor/schema_test.go`

**Step 1: Write the test**

```go
// internal/vendor/schema_test.go
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
		t.Errorf("unexpected temp OID: %s", cfg.SNMP.TemperatureOID)
	}
	if cfg.SNMP.TemperatureScale != 0.1 {
		t.Errorf("expected temp scale 0.1, got %f", cfg.SNMP.TemperatureScale)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./internal/vendor/ -v -run TestVendorConfigUnmarshal
```
Expected: FAIL — package does not exist yet

**Step 3: Write the implementation**

```go
// internal/vendor/schema.go
package vendor

// VendorConfig represents a vendor definition loaded from YAML.
type VendorConfig struct {
	Vendor          VendorInfo       `yaml:"vendor"`
	Detection       Detection        `yaml:"detection"`
	DeviceTypeRules []DeviceTypeRule `yaml:"device_type_rules"`
	ModelExtraction ModelExtraction  `yaml:"model_extraction"`
	Metrics         MetricsConfig    `yaml:"metrics"`
	SNMP            SNMPConfig       `yaml:"snmp"`
}

// VendorInfo holds vendor identity.
type VendorInfo struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"display_name"`
}

// Detection defines how to match a device to this vendor.
type Detection struct {
	SysObjectIDPrefixes []string `yaml:"sys_object_id_prefixes"`
	SysDescrPatterns    []string `yaml:"sys_descr_patterns"`
}

// DeviceTypeRule maps a sysDescr match to a device type.
type DeviceTypeRule struct {
	Match *DeviceTypeMatch `yaml:"match,omitempty"`
	Type  string           `yaml:"type"`
}

// DeviceTypeMatch defines the condition for a device type rule.
type DeviceTypeMatch struct {
	SysDescrContains string `yaml:"sys_descr_contains"`
}

// ModelExtraction defines how to extract a hardware model from sysDescr.
type ModelExtraction struct {
	SysDescrRegex string `yaml:"sys_descr_regex"`
	CaptureGroup  int    `yaml:"capture_group"`
}

// MetricsConfig holds both Prometheus and SNMP metric definitions.
type MetricsConfig struct {
	Prometheus PrometheusMetrics `yaml:"prometheus"`
}

// PrometheusMetrics holds PromQL query templates per metric type.
// The placeholder %label% is replaced at query time with
// labelName=~"labelValue" for the target device.
type PrometheusMetrics struct {
	CPU         string `yaml:"cpu"`
	Memory      string `yaml:"memory"`
	Temperature string `yaml:"temperature"`
	Uptime      string `yaml:"uptime"`
}

// SNMPConfig holds vendor-specific SNMP OIDs and scale factors.
type SNMPConfig struct {
	TemperatureOID   string  `yaml:"temperature_oid"`
	TemperatureScale float64 `yaml:"temperature_scale"`
	CPUOID           string  `yaml:"cpu_oid"`
	MemoryUsedOID    string  `yaml:"memory_used_oid"`
	MemoryTotalOID   string  `yaml:"memory_total_oid"`
}
```

**Step 4: Run test to verify it passes**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./internal/vendor/ -v -run TestVendorConfigUnmarshal
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/vendor/schema.go internal/vendor/schema_test.go
git commit -m "feat(vendor): add YAML schema structs for vendor definition files"
```

---

## Task 3: Create vendor registry with loading and matching

**Files:**
- Create: `internal/vendor/registry.go`
- Test: `internal/vendor/registry_test.go`

**Step 1: Write the tests**

```go
// internal/vendor/registry_test.go
package vendor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryLoadAndMatch(t *testing.T) {
	// Create temp vendor directory with two YAML files
	dir := t.TempDir()

	defaultYAML := `
vendor:
  name: default
  display_name: Generic SNMP

detection:
  sys_object_id_prefixes: []

metrics:
  prometheus:
    cpu: 'avg by (%[1]s) (hrProcessorLoad{%[1]s=~"%[2]s"})'
    memory: 'avg by (%[1]s) (100 * hrStorageUsed{hrStorageDescr=~"(?i)physical memory|main memory",%[1]s=~"%[2]s"} / hrStorageSize{hrStorageDescr=~"(?i)physical memory|main memory",%[1]s=~"%[2]s"})'
    temperature: 'max by (%[1]s) (entPhySensorValue{entPhySensorType="8",%[1]s=~"%[2]s"})'
    uptime: '(hrSystemUptime{%[1]s=~"%[2]s"} or sysUpTime{%[1]s=~"%[2]s"}) / 100'

snmp:
  cpu_oid: ".1.3.6.1.2.1.25.3.2.1.5"
  memory_used_oid: ".1.3.6.1.2.1.25.2.3.1.6"
  memory_total_oid: ".1.3.6.1.2.1.25.2.3.1.5"
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

	// Test: MikroTik device matched by OID prefix
	v := reg.Match("1.3.6.1.4.1.14988.1.1.2", "RouterOS RB5009UG+S+")
	if v.Vendor.Name != "mikrotik" {
		t.Errorf("expected mikrotik, got %q", v.Vendor.Name)
	}

	// Test: Unknown device falls back to default
	v = reg.Match("1.3.6.1.4.1.9.1.2500", "Cisco IOS whatever")
	if v.Vendor.Name != "default" {
		t.Errorf("expected default, got %q", v.Vendor.Name)
	}

	// Test: sysDescr pattern fallback (no OID match)
	v = reg.Match("1.3.6.1.4.1.99999", "RouterOS something")
	if v.Vendor.Name != "mikrotik" {
		t.Errorf("expected mikrotik via sysDescr pattern, got %q", v.Vendor.Name)
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

snmp:
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

	// MikroTik has custom CPU, falls back to default for memory/temp/uptime
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

	// Unknown vendor resolves to all defaults
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
		t.Errorf("expected router from default rules, got %q", dt)
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
	// CPU OID falls back to default
	if s.CPUOID != ".1.3.6.1.2.1.25.3.2.1.5" {
		t.Errorf("expected default cpu OID, got %q", s.CPUOID)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./internal/vendor/ -v -run "TestRegistry|TestResolve|TestExtract"
```
Expected: FAIL — LoadRegistry not defined

**Step 3: Write the implementation**

```go
// internal/vendor/registry.go
package vendor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Registry holds all loaded vendor configs and provides matching/resolution.
type Registry struct {
	vendors  []VendorConfig // non-default vendors
	fallback VendorConfig   // the "default" vendor
}

// LoadRegistry reads all YAML files from the given directory and builds a Registry.
// A file named "default.yaml" is required and used as the fallback.
func LoadRegistry(dir string) (*Registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading vendor directory %s: %w", dir, err)
	}

	reg := &Registry{}
	foundDefault := false

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		var cfg VendorConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}

		if cfg.Vendor.Name == "default" {
			reg.fallback = cfg
			foundDefault = true
		} else {
			reg.vendors = append(reg.vendors, cfg)
		}
	}

	if !foundDefault {
		return nil, fmt.Errorf("default.yaml not found in %s", dir)
	}

	return reg, nil
}

// Match finds the best vendor config for a device based on its sysObjectID and sysDescr.
// Returns the default config if no vendor matches.
func (r *Registry) Match(sysObjectID, sysDescr string) VendorConfig {
	oid := strings.TrimPrefix(sysObjectID, ".")

	// Phase 1: OID prefix match (most reliable)
	for _, v := range r.vendors {
		for _, prefix := range v.Detection.SysObjectIDPrefixes {
			if strings.HasPrefix(oid, prefix) {
				return v
			}
		}
	}

	// Phase 2: sysDescr pattern match (fallback)
	descrLower := strings.ToLower(sysDescr)
	for _, v := range r.vendors {
		for _, pattern := range v.Detection.SysDescrPatterns {
			if strings.Contains(descrLower, strings.ToLower(pattern)) {
				return v
			}
		}
	}

	return r.fallback
}

// ResolvePrometheusMetrics returns merged Prometheus query templates for a vendor.
// Vendor-specific queries take precedence; missing ones fall back to default.
func (r *Registry) ResolvePrometheusMetrics(vendorName string) PrometheusMetrics {
	vendorCfg := r.getByName(vendorName)
	result := r.fallback.Metrics.Prometheus // start with defaults

	if vendorCfg != nil {
		pm := vendorCfg.Metrics.Prometheus
		if pm.CPU != "" {
			result.CPU = pm.CPU
		}
		if pm.Memory != "" {
			result.Memory = pm.Memory
		}
		if pm.Temperature != "" {
			result.Temperature = pm.Temperature
		}
		if pm.Uptime != "" {
			result.Uptime = pm.Uptime
		}
	}

	return result
}

// ResolveDeviceType evaluates device type rules for a vendor against sysDescr.
// Falls back to default rules if the vendor has none or no rule matches.
func (r *Registry) ResolveDeviceType(vendorName, sysDescr string) string {
	descrLower := strings.ToLower(sysDescr)

	// Try vendor-specific rules first
	if cfg := r.getByName(vendorName); cfg != nil && len(cfg.DeviceTypeRules) > 0 {
		if dt := evaluateRules(cfg.DeviceTypeRules, descrLower); dt != "" {
			return dt
		}
	}

	// Fall back to default rules
	if dt := evaluateRules(r.fallback.DeviceTypeRules, descrLower); dt != "" {
		return dt
	}

	return "unknown"
}

// ExtractModel applies the vendor's model extraction regex to sysDescr.
// Returns "Unknown" if no regex is configured or no match is found.
func (r *Registry) ExtractModel(vendorName, sysDescr string) string {
	cfg := r.getByName(vendorName)
	if cfg == nil || cfg.ModelExtraction.SysDescrRegex == "" {
		return "Unknown"
	}

	re, err := regexp.Compile(cfg.ModelExtraction.SysDescrRegex)
	if err != nil {
		return "Unknown"
	}

	matches := re.FindStringSubmatch(sysDescr)
	group := cfg.ModelExtraction.CaptureGroup
	if group < 0 || group >= len(matches) {
		return "Unknown"
	}

	return matches[group]
}

// ResolveSNMPConfig returns merged SNMP config for a vendor.
// Vendor-specific OIDs take precedence; missing ones fall back to default.
func (r *Registry) ResolveSNMPConfig(vendorName string) SNMPConfig {
	result := r.fallback.SNMP // start with defaults

	if cfg := r.getByName(vendorName); cfg != nil {
		if cfg.SNMP.TemperatureOID != "" {
			result.TemperatureOID = cfg.SNMP.TemperatureOID
		}
		if cfg.SNMP.TemperatureScale != 0 {
			result.TemperatureScale = cfg.SNMP.TemperatureScale
		}
		if cfg.SNMP.CPUOID != "" {
			result.CPUOID = cfg.SNMP.CPUOID
		}
		if cfg.SNMP.MemoryUsedOID != "" {
			result.MemoryUsedOID = cfg.SNMP.MemoryUsedOID
		}
		if cfg.SNMP.MemoryTotalOID != "" {
			result.MemoryTotalOID = cfg.SNMP.MemoryTotalOID
		}
	}

	return result
}

// GetDisplayName returns the display name for a vendor (e.g., "MikroTik", "Cisco").
// Returns "Generic" for unknown vendors.
func (r *Registry) GetDisplayName(vendorName string) string {
	if cfg := r.getByName(vendorName); cfg != nil {
		return cfg.Vendor.DisplayName
	}
	return r.fallback.Vendor.DisplayName
}

// getByName looks up a vendor config by name (excluding default).
func (r *Registry) getByName(name string) *VendorConfig {
	if name == "default" || name == "" {
		return nil
	}
	for i := range r.vendors {
		if r.vendors[i].Vendor.Name == name {
			return &r.vendors[i]
		}
	}
	return nil
}

// evaluateRules walks device type rules in order; first match wins.
func evaluateRules(rules []DeviceTypeRule, descrLower string) string {
	for _, rule := range rules {
		if rule.Match == nil {
			// Unconditional fallback rule
			return rule.Type
		}
		if rule.Match.SysDescrContains != "" {
			if strings.Contains(descrLower, strings.ToLower(rule.Match.SysDescrContains)) {
				return rule.Type
			}
		}
	}
	return ""
}
```

**Step 4: Run tests**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./internal/vendor/ -v
```
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/vendor/registry.go internal/vendor/registry_test.go
git commit -m "feat(vendor): add registry with YAML loading, vendor matching, and metrics resolution"
```

---

## Task 4: Create vendor YAML definition files

**Files:**
- Create: `vendors/default.yaml`
- Create: `vendors/mikrotik.yaml`

**Step 1: Create default.yaml (standard MIB baseline)**

```yaml
# vendors/default.yaml
# Generic SNMP device — standard MIB baseline.
# All devices that don't match a specific vendor config use these queries.

vendor:
  name: default
  display_name: Generic SNMP

detection:
  sys_object_id_prefixes: []
  sys_descr_patterns: []

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
  - match:
      sys_descr_contains: "wireless ap"
    type: ap
  - type: unknown

model_extraction:
  sys_descr_regex: ""
  capture_group: 0

metrics:
  prometheus:
    cpu: 'avg by (%[1]s) (hrProcessorLoad{%[1]s=~"%[2]s"})'
    memory: 'avg by (%[1]s) (100 * hrStorageUsed{hrStorageDescr=~"(?i)physical memory|main memory",%[1]s=~"%[2]s"} / hrStorageSize{hrStorageDescr=~"(?i)physical memory|main memory",%[1]s=~"%[2]s"})'
    temperature: 'max by (%[1]s) (entPhySensorValue{entPhySensorType="8",%[1]s=~"%[2]s"})'
    uptime: '(hrSystemUptime{%[1]s=~"%[2]s"} or sysUpTime{%[1]s=~"%[2]s"}) / 100'

snmp:
  cpu_oid: ".1.3.6.1.2.1.25.3.2.1.5"
  memory_used_oid: ".1.3.6.1.2.1.25.2.3.1.6"
  memory_total_oid: ".1.3.6.1.2.1.25.2.3.1.5"
  temperature_oid: ".1.3.6.1.2.1.99.1.1.1.4"
  temperature_scale: 1.0
```

**Step 2: Create mikrotik.yaml**

```yaml
# vendors/mikrotik.yaml
# MikroTik RouterOS and SwOS devices.

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
    cpu: 'avg by (%[1]s) (mtxrHlCpuLoad{%[1]s=~"%[2]s"} or hrProcessorLoad{%[1]s=~"%[2]s"})'
    memory: '(avg by (%[1]s) (100 * (1 - mtxrHlFreeMemory{%[1]s=~"%[2]s"} / on(%[1]s) mtxrHlTotalMemory{%[1]s=~"%[2]s"}))) or (avg by (%[1]s) (100 * hrStorageUsed{hrStorageDescr=~"(?i)physical memory|main memory",%[1]s=~"%[2]s"} / hrStorageSize{hrStorageDescr=~"(?i)physical memory|main memory",%[1]s=~"%[2]s"}))'
    temperature: 'mtxrHlTemperature{%[1]s=~"%[2]s"} or max by (%[1]s) (entPhySensorValue{entPhySensorType="8",%[1]s=~"%[2]s"})'
    uptime: '(hrSystemUptime{%[1]s=~"%[2]s"} or sysUpTime{%[1]s=~"%[2]s"}) / 100'

snmp:
  temperature_oid: ".1.3.6.1.4.1.14988.1.1.3.10.0"
  temperature_scale: 0.1
```

**Step 3: Verify YAML loads correctly**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./internal/vendor/ -v -run TestRegistryLoadAndMatch
```

But since tests use temp dirs, let's add a quick sanity test that loads from the real vendors/ dir:

Add to registry_test.go:
```go
func TestLoadRealVendors(t *testing.T) {
	// Skip if vendors/ doesn't exist (CI without vendor files)
	vendorDir := filepath.Join("..", "..", "vendors")
	if _, err := os.Stat(vendorDir); os.IsNotExist(err) {
		t.Skip("vendors/ directory not found")
	}

	reg, err := LoadRegistry(vendorDir)
	if err != nil {
		t.Fatalf("LoadRegistry failed on real vendors: %v", err)
	}

	// Verify MikroTik is loaded
	v := reg.Match("1.3.6.1.4.1.14988.1.1.2", "RouterOS RB5009")
	if v.Vendor.Name != "mikrotik" {
		t.Errorf("expected mikrotik, got %q", v.Vendor.Name)
	}
}
```

**Step 4: Run test**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./internal/vendor/ -v -run TestLoadRealVendors
```
Expected: PASS

**Step 5: Commit**

```bash
git add vendors/default.yaml vendors/mikrotik.yaml internal/vendor/registry_test.go
git commit -m "feat(vendor): add default and mikrotik vendor YAML definition files"
```

---

## Task 5: Add Vendor field to domain model and database

**Files:**
- Modify: `internal/domain/device.go:83-102` — add Vendor field
- Modify: `internal/repository/sqlite/migrations.go:99-102` — add vendor column
- Modify: `internal/repository/sqlite/device_repo.go` — include vendor in CRUD
- Modify: `frontend/src/types/api.ts:35-50` — add vendor field

**Step 1: Add Vendor field to domain model**

In `internal/domain/device.go`, add `Vendor` field to the Device struct after `HardwareModel`:

```go
// Add after line 93 (HardwareModel field):
Vendor          string            `json:"vendor"`
```

**Step 2: Add vendor column migration**

In `internal/repository/sqlite/migrations.go`, add to the migrations slice (after the existing ALTER TABLE statements):

```go
`ALTER TABLE devices ADD COLUMN vendor TEXT NOT NULL DEFAULT 'default'`,
```

**Step 3: Update device_repo.go**

Update the `Create`, `scan`, and `Update` methods to include the `vendor` column in SQL queries. The exact changes depend on the current SQL statements — add `vendor` alongside `hardware_model` in INSERT/UPDATE/SELECT.

**Step 4: Update frontend types**

In `frontend/src/types/api.ts`, add to the Device interface:

```typescript
vendor: string;
```

And add to parseDevicesResponse:

```typescript
vendor: readString(attributes, 'vendor', 'default'),
```

**Step 5: Run tests**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./internal/repository/sqlite/ -v -run TestDevice
```

**Step 6: Commit**

```bash
git add internal/domain/device.go internal/repository/sqlite/migrations.go internal/repository/sqlite/device_repo.go frontend/src/types/api.ts
git commit -m "feat: add vendor field to device domain model, database schema, and frontend types"
```

---

## Task 6: Refactor detector.go to use vendor registry

**Files:**
- Modify: `internal/snmp/detector.go` — replace hardcoded logic with registry call
- Modify: `internal/snmp/discovery.go:98-100` — pass registry for detection and model extraction
- Modify: `internal/snmp/detector_test.go` — update tests

**Step 1: Refactor detector.go**

Replace the entire `DetectDeviceType` function. The function now accepts a vendor registry and returns both vendor name and device type:

```go
package snmp

import (
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/vendor"
)

// DetectVendorAndType identifies the vendor and device type from sysObjectID and sysDescr
// using the vendor registry. Falls back to "default" vendor with heuristic type detection.
func DetectVendorAndType(registry *vendor.Registry, sysObjectID, sysDescr string) (vendorName string, deviceType domain.DeviceType) {
	cfg := registry.Match(sysObjectID, sysDescr)
	vendorName = cfg.Vendor.Name
	dt := registry.ResolveDeviceType(vendorName, sysDescr)
	return vendorName, domain.DeviceType(dt)
}
```

**Step 2: Update discovery.go**

Modify `DiscoverDevice` to accept a `*vendor.Registry` parameter:

- Line 78: `func DiscoverDevice(client ClientInterface, registry *vendor.Registry) (*DiscoveryResult, error) {`
- Line 98: Replace `res.DeviceType = DetectDeviceType(res.SysObjectID, res.SysDescr)` with:
  ```go
  res.Vendor, res.DeviceType = DetectVendorAndType(registry, res.SysObjectID, res.SysDescr)
  res.HardwareModel = registry.ExtractModel(res.Vendor, res.SysDescr)
  ```
- Add `Vendor string` field to `DiscoveryResult` struct (line 51-59)
- Remove the `extractHardwareModel` function (lines 337-347)
- Remove the old `DetectDeviceType` function from detector.go

**Step 3: Update PollDeviceMetrics to use vendor config for temperature**

In `discovery.go`, modify `PollDeviceMetrics` signature:
```go
func PollDeviceMetrics(client ClientInterface, snmpCfg vendor.SNMPConfig) (cpuPercent, memPercent, uptimeSecs, tempCelsius *float64) {
```

Replace the MikroTik temperature OID block (lines 387-400) with:
```go
if snmpCfg.TemperatureOID != "" {
    if pdus, err := client.Get([]string{snmpCfg.TemperatureOID}); err == nil {
        for _, pdu := range pdus {
            if pdu.Name == snmpCfg.TemperatureOID {
                if v := int64FromPDU(pdu); v > 0 {
                    scale := snmpCfg.TemperatureScale
                    if scale == 0 {
                        scale = 1.0
                    }
                    c := float64(v) * scale
                    tempCelsius = &c
                }
            }
        }
    }
}
if tempCelsius == nil {
    tempCelsius = pollEntitySensorTemp(client)
}
```

**Step 4: Update tests**

Update `detector_test.go` and `discovery_test.go` to construct a vendor registry from test YAML and pass it to the refactored functions.

**Step 5: Run tests**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./internal/snmp/ -v
```

**Step 6: Commit**

```bash
git add internal/snmp/detector.go internal/snmp/discovery.go internal/snmp/detector_test.go internal/snmp/discovery_test.go
git commit -m "refactor(snmp): replace hardcoded device detection with vendor registry"
```

---

## Task 7: Refactor Prometheus client to use vendor-resolved queries

**Files:**
- Modify: `internal/metrics/prometheus.go:43-130` — accept query templates instead of hardcoded queries
- Modify: `internal/metrics/prometheus_test.go`

**Step 1: Refactor QueryDeviceMetrics**

The method should accept query templates instead of building them internally. Add a new method or modify the existing one:

```go
// QueryDeviceMetricsWithQueries fetches device metrics using vendor-resolved PromQL templates.
// Each template uses fmt.Sprintf format with %[1]s = labelName and %[2]s = target matcher.
func (c *PromClient) QueryDeviceMetricsWithQueries(
	ctx context.Context,
	labelName string,
	labelValues []string,
	queries PrometheusQueries,
) (map[string]domain.DeviceMetrics, error) {
```

Where `PrometheusQueries` is:
```go
type PrometheusQueries struct {
	CPU         string
	Memory      string
	Temperature string
	Uptime      string
}
```

The existing `QueryDeviceMetrics` method can be kept as a backward-compatible wrapper that uses default queries, or removed entirely if all callers are updated.

**Step 2: Update QueryDeviceMetrics to use template-based queries**

Replace the hardcoded query strings (lines 59-107) with `fmt.Sprintf(queries.CPU, labelName, targets)` etc.

**Step 3: Update metrics_collector.go to pass vendor-resolved queries**

This will be done in Task 8.

**Step 4: Run tests**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./internal/metrics/ -v
```

**Step 5: Commit**

```bash
git add internal/metrics/prometheus.go internal/metrics/prometheus_test.go
git commit -m "refactor(metrics): accept vendor-resolved PromQL templates in Prometheus client"
```

---

## Task 8: Wire vendor registry into workers and main.go

**Files:**
- Modify: `internal/worker/metrics_collector.go` — add registry field, use it for query resolution
- Modify: `cmd/theia/main.go` — initialize registry, pass to workers, rename log messages
- Modify: `internal/service/device_service.go` — pass registry to discovery, set vendor on device

**Step 1: Update MetricsCollector**

Add `vendorRegistry *vendor.Registry` field to MetricsCollector struct. Update `NewMetricsCollector` to accept it. In `buildSnapshot`, resolve vendor-specific Prometheus queries per device:

```go
// For each device, look up its vendor and get the right queries
vendorName := device.Vendor
if vendorName == "" {
    vendorName = "default"
}
promMetrics := c.vendorRegistry.ResolvePrometheusMetrics(vendorName)
```

Group devices by vendor for batched querying, or query per-vendor-group if they use different query templates.

**Step 2: Update DeviceService**

Add `vendorRegistry *vendor.Registry` to DeviceService. Pass it to `snmp.DiscoverDevice`. After discovery, set `fresh.Vendor = result.Vendor`.

**Step 3: Update main.go**

```go
// After config load, before creating services:
vendorRegistry, err := vendor.LoadRegistry("vendors")
if err != nil {
    log.Fatalf("Failed to load vendor definitions: %v", err)
}
log.Printf("Loaded %d vendor definitions", len(vendorRegistry.vendors)+1)
```

Pass registry to DeviceService and MetricsCollector constructors.

Change line 132:
```go
log.Printf("Theia starting on %s", cfg.ListenAddr)
```

**Step 4: Update SNMP poll func to use vendor config**

In `newSNMPMetricsPollFunc`, resolve the device's vendor SNMP config:

```go
snmpCfg := vendorRegistry.ResolveSNMPConfig(device.Vendor)
cpu, mem, uptime, temp := snmp.PollDeviceMetrics(client, snmpCfg)
```

This requires the poll func to have access to the device's vendor, so the `SNMPPollFunc` signature may need to change:
```go
type SNMPPollFunc func(target string, creds domain.SNMPCredentials, vendor string) (domain.DeviceMetrics, error)
```

**Step 5: Run all tests**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./... -v
```

**Step 6: Commit**

```bash
git add internal/worker/metrics_collector.go internal/service/device_service.go cmd/theia/main.go
git commit -m "feat: wire vendor registry into metrics collector, device service, and application startup"
```

---

## Task 9: Update frontend branding and vendor display

**Files:**
- Modify: `frontend/index.html:6` — title to "Theia"
- Modify: `frontend/package.json:2` — name to "theia-frontend"
- Modify: `frontend/src/components/SNMPProfileManager.tsx:100` — remove MikroTik placeholder
- Modify: `frontend/src/components/DeviceCard.tsx` — add vendor badge
- Modify: `frontend/src/types/api.ts` — (already done in Task 5)

**Step 1: Update index.html title**

```html
<title>Theia</title>
```

**Step 2: Update package.json name**

```json
"name": "theia-frontend",
```

**Step 3: Update SNMPProfileManager placeholder**

```tsx
placeholder="e.g. Office SNMPv3"
```

**Step 4: Add vendor badge to DeviceCard**

In `DeviceCard.tsx`, add a small vendor label in the header section, after the device icon:

```tsx
{data.device.vendor && data.device.vendor !== 'default' && (
  <span className="ml-1 rounded bg-accent/10 px-1.5 py-0.5 text-[9px] font-medium uppercase tracking-wider text-accent/70">
    {data.device.vendor}
  </span>
)}
```

Add `vendor` to the memo comparison function.

**Step 5: Run frontend build**

```bash
cd /home/azmin/projects/mikrotik-theia/frontend && npm run build
```

**Step 6: Commit**

```bash
git add frontend/index.html frontend/package.json frontend/src/components/SNMPProfileManager.tsx frontend/src/components/DeviceCard.tsx
git commit -m "feat(frontend): rename to Theia, add vendor badge to device cards"
```

---

## Task 10: Update remaining MikroTik references

**Files:**
- Modify: `cmd/theia/main.go:132` — (already done in Task 8)
- Modify: `go.mod:1` — consider renaming module (optional, breaking change)
- Review: `docker-compose.yml`, `CHANGELOG.md`, etc. — update display references

**Step 1: Update go.mod module name (optional)**

If desired, rename `github.com/lollinoo/theia` to `github.com/azmin/theia`. This requires updating ALL import paths across every Go file. This can be done with a find-and-replace:

```bash
# Only do this if you want to rename the Go module
find . -name "*.go" -exec sed -i 's|github.com/lollinoo/theia|github.com/azmin/theia|g' {} +
sed -i 's|github.com/lollinoo/theia|github.com/azmin/theia|g' go.mod
```

**Note:** This is optional and can be deferred. It's a breaking change for anyone importing the module. Given this is dev stage, it's safe to do now.

**Step 2: Update docker-compose service names (cosmetic)**

Replace any `mikrotik-theia` references in docker-compose.yml with `theia`.

**Step 3: Commit**

```bash
git add -A
git commit -m "chore: rename project references from MikroTik Theia to Theia"
```

---

## Task 11: Remove dead code

**Files:**
- Modify: `internal/snmp/discovery.go` — remove `OidMtxrTemperature` constant, `extractHardwareModel` function
- Modify: `internal/snmp/detector.go` — remove old `DetectDeviceType` function (if not already removed in Task 6)

**Step 1: Clean up**

Remove:
- `OidMtxrTemperature` constant from `discovery.go:47`
- `extractHardwareModel` function from `discovery.go:337-347`
- Old `DetectDeviceType` function from `detector.go` (entire file becomes the new `DetectVendorAndType`)

**Step 2: Run tests**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./... -v
```

**Step 3: Commit**

```bash
git add internal/snmp/detector.go internal/snmp/discovery.go
git commit -m "refactor: remove dead MikroTik-specific code replaced by vendor config"
```

---

## Task 12: End-to-end verification

**Step 1: Build backend**

```bash
cd /home/azmin/projects/mikrotik-theia && go build ./cmd/theia/
```
Expected: Success

**Step 2: Build frontend**

```bash
cd /home/azmin/projects/mikrotik-theia/frontend && npm run build
```
Expected: Success

**Step 3: Run all Go tests**

```bash
cd /home/azmin/projects/mikrotik-theia && go test ./... -v
```
Expected: All pass

**Step 4: Verify Docker test environment**

```bash
cd /home/azmin/projects/mikrotik-theia && docker compose up -d
```
Verify existing MikroTik snmpd simulators are discovered and matched to `vendors/mikrotik.yaml`.

**Step 5: Manual smoke test**

- Open Theia UI
- Verify devices show vendor badge
- Verify metrics (CPU, memory, temp, uptime) still populate
- Verify link metrics still work
- Verify title says "Theia" not "MikroTik Theia"
