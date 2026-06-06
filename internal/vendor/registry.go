package vendor

// This file defines registry vendor metadata loading and registry contracts.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Registry holds all loaded vendor configs and provides matching/resolution.
type Registry struct {
	mu       sync.RWMutex
	vendors  []VendorConfig // non-default vendors
	fallback VendorConfig   // the "default" vendor
}

// LoadRegistry reads all YAML files from the given directory and builds a Registry.
// A file named "default.yaml" is required and used as the fallback.
func LoadRegistry(dir string) (*Registry, error) {
	return LoadRegistryFromYAML(dir)
}

// LoadRegistryFromYAML reads all YAML files from the given directory and builds a Registry.
func LoadRegistryFromYAML(dir string) (*Registry, error) {
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

// LoadRegistryFromDB builds a Registry from database records.
func LoadRegistryFromDB(records []DBVendorRecord) (*Registry, error) {
	reg := &Registry{}
	foundDefault := false

	for _, rec := range records {
		var cfg VendorConfig
		if err := json.Unmarshal([]byte(rec.ConfigJSON), &cfg); err != nil {
			return nil, fmt.Errorf("parsing vendor config %q: %w", rec.Name, err)
		}

		if cfg.Vendor.Name == "default" {
			reg.fallback = cfg
			foundDefault = true
		} else {
			reg.vendors = append(reg.vendors, cfg)
		}
	}

	if !foundDefault {
		return nil, fmt.Errorf("no default vendor config found in DB")
	}

	return reg, nil
}

// DBVendorRecord is the minimal record needed to build a registry from DB rows.
type DBVendorRecord struct {
	Name       string
	ConfigJSON string
}

// ExportConfig serializes a vendor config to JSON.
func (r *Registry) ExportConfig(vendorName string) (json.RawMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if vendorName == "default" || vendorName == "" {
		data, err := json.Marshal(r.fallback)
		return data, err
	}
	if cfg := r.getByName(vendorName); cfg != nil {
		data, err := json.Marshal(cfg)
		return data, err
	}
	return nil, fmt.Errorf("vendor %q not found", vendorName)
}

// ExportAllConfigs returns a map of vendor name -> JSON config for all vendors.
func (r *Registry) ExportAllConfigs() (map[string]json.RawMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]json.RawMessage)

	data, err := json.Marshal(r.fallback)
	if err != nil {
		return nil, err
	}
	result["default"] = data

	for i := range r.vendors {
		data, err := json.Marshal(&r.vendors[i])
		if err != nil {
			return nil, err
		}
		result[r.vendors[i].Vendor.Name] = data
	}
	return result, nil
}

// UpdateConfig updates a vendor config from JSON and reloads it in the registry.
func (r *Registry) UpdateConfig(vendorName string, configJSON []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var cfg VendorConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return fmt.Errorf("invalid vendor config JSON: %w", err)
	}

	if vendorName == "default" || vendorName == "" {
		r.fallback = cfg
		return nil
	}

	for i := range r.vendors {
		if r.vendors[i].Vendor.Name == vendorName {
			r.vendors[i] = cfg
			return nil
		}
	}

	// Not found — add as new
	r.vendors = append(r.vendors, cfg)
	return nil
}

// GetAllVendorNames returns all vendor names (including default).
func (r *Registry) GetAllVendorNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := []string{"default"}
	for _, v := range r.vendors {
		names = append(names, v.Vendor.Name)
	}
	return names
}

// Match finds the best vendor config for a device based on its sysObjectID and sysDescr.
// Returns the default config if no vendor matches.
func (r *Registry) Match(sysObjectID, sysDescr string) VendorConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	r.mu.RLock()
	defer r.mu.RUnlock()
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
// Composes the result from the three per-tier resolver methods so that
// all merge semantics are defined in one place per tier.
// Kept for backward compatibility; canonical callers should prefer
// ResolvePerformanceOIDs / ResolveOperationalOIDs / ResolveStaticOIDs.
func (r *Registry) ResolveSNMPConfig(vendorName string) SNMPConfig {
	return SNMPConfig{
		Static:      r.ResolveStaticOIDs(vendorName),
		Operational: r.ResolveOperationalOIDs(vendorName),
		Performance: r.ResolvePerformanceOIDs(vendorName),
	}
}

// ResolveStaticOIDs returns merged StaticOIDs for a vendor.
// Vendor-specific values override default tier-by-tier; missing values
// fall back to default.
func (r *Registry) ResolveStaticOIDs(vendorName string) StaticOIDs {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := r.fallback.SNMP.Static
	if cfg := r.getByName(vendorName); cfg != nil {
		if cfg.SNMP.Static.SoftwareVersionOID != "" {
			result.SoftwareVersionOID = cfg.SNMP.Static.SoftwareVersionOID
		}
	}
	return result
}

// ResolveOperationalOIDs returns merged OperationalOIDs for a vendor.
// Standard MIB OIDs live in default.yaml per D-11; vendor overrides
// apply only when the vendor genuinely diverges from the standard.
func (r *Registry) ResolveOperationalOIDs(vendorName string) OperationalOIDs {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := r.fallback.SNMP.Operational
	if cfg := r.getByName(vendorName); cfg != nil {
		if cfg.SNMP.Operational.SysUpTimeOID != "" {
			result.SysUpTimeOID = cfg.SNMP.Operational.SysUpTimeOID
		}
		if cfg.SNMP.Operational.IfOperStatusOID != "" {
			result.IfOperStatusOID = cfg.SNMP.Operational.IfOperStatusOID
		}
	}
	return result
}

// ResolvePerformanceOIDs returns merged PerformanceOIDs for a vendor.
// Vendor-specific OIDs and scale factors override default; missing
// vendor values fall back to default tier-by-tier.
func (r *Registry) ResolvePerformanceOIDs(vendorName string) PerformanceOIDs {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := r.fallback.SNMP.Performance
	if cfg := r.getByName(vendorName); cfg != nil {
		if cfg.SNMP.Performance.CPUOID != "" {
			result.CPUOID = cfg.SNMP.Performance.CPUOID
		}
		if cfg.SNMP.Performance.MemoryUsedOID != "" {
			result.MemoryUsedOID = cfg.SNMP.Performance.MemoryUsedOID
		}
		if cfg.SNMP.Performance.MemoryTotalOID != "" {
			result.MemoryTotalOID = cfg.SNMP.Performance.MemoryTotalOID
		}
		if cfg.SNMP.Performance.TemperatureOID != "" {
			result.TemperatureOID = cfg.SNMP.Performance.TemperatureOID
		}
		if cfg.SNMP.Performance.TemperatureScale != 0 {
			result.TemperatureScale = cfg.SNMP.Performance.TemperatureScale
		}
	}
	return result
}

// ResolveBackupConfig returns the backup config for a vendor.
// Vendor-specific config takes precedence; falls back to default.
func (r *Registry) ResolveBackupConfig(vendorName string) BackupConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if cfg := r.getByName(vendorName); cfg != nil && cfg.Backup.Supported {
		return cfg.Backup
	}
	return r.fallback.Backup
}

// GetDisplayName returns the display name for a vendor (e.g., "MikroTik", "Cisco").
// Returns the default display name for unknown vendors.
func (r *Registry) GetDisplayName(vendorName string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if cfg := r.getByName(vendorName); cfg != nil {
		return cfg.Vendor.DisplayName
	}
	return r.fallback.Vendor.DisplayName
}

// VendorCount returns the total number of loaded vendors (including default).
func (r *Registry) VendorCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.vendors) + 1
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
