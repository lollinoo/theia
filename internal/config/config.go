package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds bootstrap configuration loaded from YAML / environment.
// Runtime settings (Prometheus URL, polling interval, etc.) are stored
// in the SQLite settings table and managed via the API.
type Config struct {
	ListenAddr        string `yaml:"listen_addr"`
	DBPath            string `yaml:"db_path"`
	LogLevel          string `yaml:"log_level"`
	BridgeBinariesDir string `yaml:"bridge_binaries_dir"`
}

// defaults returns a Config with sensible default values.
func defaults() *Config {
	return &Config{
		ListenAddr: ":8080",
		DBPath:     "./data/theia.db",
		LogLevel:   "info",
	}
}

// Load reads configuration from a YAML file and then applies environment
// variable overrides. Environment variables take precedence over the file.
//
// Supported env vars:
//   - THEIA_LISTEN_ADDR
//   - THEIA_DB_PATH
//   - THEIA_LOG_LEVEL
//   - THEIA_BRIDGE_BINARIES_DIR
func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// Config file not found — proceed with defaults + env overrides
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	// Environment variable overrides
	if v := os.Getenv("THEIA_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("THEIA_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("THEIA_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("THEIA_BRIDGE_BINARIES_DIR"); v != "" {
		cfg.BridgeBinariesDir = v
	}

	return cfg, nil
}
