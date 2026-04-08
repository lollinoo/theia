package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the persistent bridge configuration.
// Fields map directly to CLI flag defaults for backward compatibility.
type Config struct {
	WinBoxPath  string `json:"winbox_path"`
	ListenPort  int    `json:"listen_port"`
	TheiaOrigin string `json:"theia_origin"`
}

// DefaultConfig returns the config matching current CLI flag defaults.
func DefaultConfig() Config {
	return Config{
		WinBoxPath:  "",
		ListenPort:  1337,
		TheiaOrigin: "http://localhost:3000",
	}
}

// configFilePath returns the platform-appropriate path for config.json.
// Uses os.UserConfigDir(): Windows=%APPDATA%, Linux=~/.config, macOS=~/Library/Application Support.
func configFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return filepath.Join(dir, "winbox-bridge", "config.json"), nil
}

// loadConfigFrom reads config from the given path.
// Returns DefaultConfig if file is missing (os.IsNotExist).
// Returns DefaultConfig and a non-nil error if the file exists but cannot be parsed.
func loadConfigFrom(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return DefaultConfig(), fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// saveConfigTo writes config to the given path, creating the parent directory if needed.
// Directory is created with 0o700 (owner-only) and file with 0o600 (owner read/write).
func saveConfigTo(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// loadConfig reads config from the platform-default config file path.
// Returns DefaultConfig if the file does not exist or if the config dir is unavailable.
func loadConfig() (Config, error) {
	path, err := configFilePath()
	if err != nil {
		return DefaultConfig(), nil // degrade gracefully when no home dir
	}
	return loadConfigFrom(path)
}

// saveConfig writes config to the platform-default config file path.
func saveConfig(cfg Config) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	return saveConfigTo(cfg, path)
}
