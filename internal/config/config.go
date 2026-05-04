package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds bootstrap configuration loaded from YAML / environment.
// Runtime settings (Prometheus URL, polling interval, etc.) are stored
// in the primary database settings table and managed via the API.
type Config struct {
	DBDriver          string `yaml:"db_driver"`
	ListenAddr        string `yaml:"listen_addr"`
	DBPath            string `yaml:"db_path"`
	DBDSN             string `yaml:"db_dsn"`
	DataDir           string `yaml:"data_dir"`
	LogLevel          string `yaml:"log_level"`
	BridgeBinariesDir string `yaml:"bridge_binaries_dir"`
	DeploymentEnv     string `yaml:"deployment_env"`
}

// defaults returns a Config with sensible default values.
func defaults() *Config {
	return &Config{
		DBDriver:   "postgres",
		ListenAddr: ":8080",
		DBPath:     "./data/theia.db",
		DataDir:    "./data",
		LogLevel:   "info",
	}
}

// Load reads configuration from a YAML file and then applies environment
// variable overrides. Environment variables take precedence over the file.
//
// Supported env vars:
//   - THEIA_DB_DRIVER
//   - THEIA_LISTEN_ADDR
//   - THEIA_DB_PATH
//   - THEIA_DB_DSN
//   - THEIA_DATA_DIR
//   - THEIA_LOG_LEVEL
//   - THEIA_BRIDGE_BINARIES_DIR
//   - THEIA_DEPLOYMENT_ENV
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
	if v := os.Getenv("THEIA_DB_DRIVER"); v != "" {
		cfg.DBDriver = v
	}
	if v := os.Getenv("THEIA_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("THEIA_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("THEIA_DB_DSN"); v != "" {
		cfg.DBDSN = v
	}
	if v := os.Getenv("THEIA_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("THEIA_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("THEIA_BRIDGE_BINARIES_DIR"); v != "" {
		cfg.BridgeBinariesDir = v
	}
	if v := os.Getenv("THEIA_DEPLOYMENT_ENV"); v != "" {
		cfg.DeploymentEnv = v
	}

	return cfg, nil
}
