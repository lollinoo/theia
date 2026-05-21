package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/vendor"
)

const deprecatedHrDeviceStatusCPUOID = ".1.3.6.1.2.1.25.3.2.1.5"

func loadBootstrapVendorRegistry() (*vendor.Registry, string, error) {
	if envVendors := os.Getenv("THEIA_VENDORS_DIR"); envVendors != "" {
		registry, err := vendor.LoadRegistryFromYAML(envVendors)
		if err != nil {
			return nil, envVendors, err
		}
		return registry, envVendors, nil
	}

	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		return nil, "", err
	}
	return registry, "", nil
}

// seedVendorConfigs seeds vendor configs into the DB from YAML if not already present.
func seedVendorConfigs(yamlRegistry *vendor.Registry, repo *postgres.VendorConfigRepo) {
	configs, err := yamlRegistry.ExportAllConfigs()
	if err != nil {
		log.Printf("Warning: failed to export YAML configs for seeding: %v", err)
		return
	}

	for name, configJSON := range configs {
		existing, err := repo.GetByName(name)
		if err != nil {
			log.Printf("Warning: failed to check vendor %s in DB: %v", name, err)
			continue
		}
		displayName := yamlRegistry.GetDisplayName(name)
		if name == "default" {
			displayName = "Generic / Default"
		}

		record := &domain.VendorConfigRecord{Name: name, DisplayName: displayName, ConfigJSON: string(configJSON)}
		if existing != nil {
			mergedJSON, changed, mergeErr := mergeVendorConfigDefaults([]byte(existing.ConfigJSON), configJSON)
			if mergeErr != nil {
				log.Printf("Warning: failed to merge vendor %s defaults from YAML: %v", name, mergeErr)
				continue
			}
			if !changed {
				continue
			}
			record.CreatedAt = existing.CreatedAt
			record.ConfigJSON = string(mergedJSON)
		}

		if err := repo.Upsert(record); err != nil {
			log.Printf("Warning: failed to seed vendor %s: %v", name, err)
		} else if existing != nil {
			log.Printf("Synced vendor config defaults: %s", name)
		} else {
			log.Printf("Seeded vendor config: %s", name)
		}
	}
}

func mergeVendorConfigDefaults(existingJSON, defaultsJSON []byte) ([]byte, bool, error) {
	var existingCfg vendor.VendorConfig
	if err := json.Unmarshal(existingJSON, &existingCfg); err != nil {
		return nil, false, fmt.Errorf("unmarshal existing config: %w", err)
	}

	var defaultCfg vendor.VendorConfig
	if err := json.Unmarshal(defaultsJSON, &defaultCfg); err != nil {
		return nil, false, fmt.Errorf("unmarshal default config: %w", err)
	}

	changed := mergeMissingVendorConfigFields(&existingCfg, defaultCfg)
	if !changed {
		return existingJSON, false, nil
	}

	mergedJSON, err := json.Marshal(existingCfg)
	if err != nil {
		return nil, false, fmt.Errorf("marshal merged config: %w", err)
	}
	return mergedJSON, true, nil
}

func mergeMissingVendorConfigFields(dst *vendor.VendorConfig, defaults vendor.VendorConfig) bool {
	if dst == nil {
		return false
	}

	changed := false
	if dst.SNMP.Static.SoftwareVersionOID == "" && defaults.SNMP.Static.SoftwareVersionOID != "" {
		dst.SNMP.Static.SoftwareVersionOID = defaults.SNMP.Static.SoftwareVersionOID
		changed = true
	}
	if shouldSyncCPUOID(dst.SNMP.Performance.CPUOID, defaults.SNMP.Performance.CPUOID) {
		dst.SNMP.Performance.CPUOID = defaults.SNMP.Performance.CPUOID
		changed = true
	}

	return changed
}

func shouldSyncCPUOID(existing string, defaultOID string) bool {
	if defaultOID == "" {
		return false
	}
	return existing == "" || existing == deprecatedHrDeviceStatusCPUOID
}

// loadRegistryFromDB builds a vendor registry from DB records.
func loadRegistryFromDB(repo *postgres.VendorConfigRepo) (*vendor.Registry, error) {
	records, err := repo.GetAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	var dbRecords []vendor.DBVendorRecord
	for _, rec := range records {
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(rec.ConfigJSON), &raw); err != nil {
			log.Printf("Warning: invalid vendor config JSON for %s, skipping: %v", rec.Name, err)
			continue
		}
		dbRecords = append(dbRecords, vendor.DBVendorRecord{
			Name:       rec.Name,
			ConfigJSON: rec.ConfigJSON,
		})
	}

	if len(dbRecords) == 0 {
		log.Printf("Warning: all DB vendor records failed JSON validation, falling back to YAML registry")
		return nil, nil
	}

	return vendor.LoadRegistryFromDB(dbRecords)
}
