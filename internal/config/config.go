package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds bootstrap configuration loaded from YAML / environment.
// Runtime settings (Prometheus URL, polling interval, etc.) are stored
// in the primary database settings table and managed via the API.
type Config struct {
	ListenAddr        string   `yaml:"listen_addr"`
	DBDSN             string   `yaml:"db_dsn"`
	DataDir           string   `yaml:"data_dir"`
	LogLevel          string   `yaml:"log_level"`
	BridgeBinariesDir string   `yaml:"bridge_binaries_dir"`
	DeploymentEnv     string   `yaml:"deployment_env"`
	OperatorToken     string   `yaml:"operator_token"`
	MetricsToken      string   `yaml:"metrics_token"`
	AllowedOrigins    []string `yaml:"allowed_origins"`
}

// defaults returns a Config with sensible default values.
func defaults() *Config {
	return &Config{
		ListenAddr: ":8080",
		DataDir:    "./data",
		LogLevel:   "info",
	}
}

// Load reads configuration from a YAML file and then applies environment
// variable overrides. Environment variables take precedence over the file.
//
// Supported env vars:
//   - THEIA_LISTEN_ADDR
//   - THEIA_DB_DSN
//   - THEIA_DATA_DIR
//   - THEIA_LOG_LEVEL
//   - THEIA_BRIDGE_BINARIES_DIR
//   - THEIA_DEPLOYMENT_ENV
//   - THEIA_OPERATOR_TOKEN
//   - THEIA_METRICS_TOKEN
//   - THEIA_ALLOWED_ORIGINS
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
	if v := os.Getenv("THEIA_OPERATOR_TOKEN"); v != "" {
		cfg.OperatorToken = v
	}
	if v := os.Getenv("THEIA_METRICS_TOKEN"); v != "" {
		cfg.MetricsToken = v
	}
	if v := os.Getenv("THEIA_ALLOWED_ORIGINS"); v != "" {
		cfg.AllowedOrigins = splitAllowedOrigins(v)
	}

	return cfg, nil
}

func splitAllowedOrigins(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	origins := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			origins = append(origins, field)
		}
	}
	return origins
}
