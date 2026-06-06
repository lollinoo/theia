package vendor

// This file defines embedded vendor metadata loading and registry contracts.

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultVendorFS holds the bundled vendor YAML files.
// These are embedded at compile time so no filesystem access is required at runtime.
//
//go:embed data/*.yaml
var DefaultVendorFS embed.FS

// LoadRegistryFromEmbedded builds a Registry from the embedded vendor YAML files.
// This is the preferred loader for production: no external files needed at runtime.
func LoadRegistryFromEmbedded() (*Registry, error) {
	entries, err := fs.ReadDir(DefaultVendorFS, "data")
	if err != nil {
		return nil, fmt.Errorf("reading embedded vendor data: %w", err)
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

		data, err := DefaultVendorFS.ReadFile("data/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading embedded vendor %s: %w", entry.Name(), err)
		}

		var cfg VendorConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing embedded vendor %s: %w", entry.Name(), err)
		}

		if cfg.Vendor.Name == "default" {
			reg.fallback = cfg
			foundDefault = true
		} else {
			reg.vendors = append(reg.vendors, cfg)
		}
	}

	if !foundDefault {
		return nil, fmt.Errorf("default.yaml not found in embedded vendor data")
	}

	return reg, nil
}
