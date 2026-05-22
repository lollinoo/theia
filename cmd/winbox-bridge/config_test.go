package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// --- Config: defaults ---

func TestConfigDefaultConfig_WinBoxPathEmpty(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.WinBoxPath != "" {
		t.Errorf("expected WinBoxPath='', got %q", cfg.WinBoxPath)
	}
}

func TestConfigDefaultConfig_ListenPort1337(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ListenPort != 1337 {
		t.Errorf("expected ListenPort=1337, got %d", cfg.ListenPort)
	}
}

func TestConfigDefaultConfig_TheiaOrigin(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.TheiaOrigin != "http://localhost:3000" {
		t.Errorf("expected TheiaOrigin='http://localhost:3000', got %q", cfg.TheiaOrigin)
	}
}

func TestConfigDefaultConfig_TheiaBaseURL(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.TheiaBaseURL != "http://localhost:3000" {
		t.Errorf("expected TheiaBaseURL='http://localhost:3000', got %q", cfg.TheiaBaseURL)
	}
	if cfg.BridgeSecret != "" {
		t.Fatalf("DefaultConfig should not auto-generate BridgeSecret, got %q", cfg.BridgeSecret)
	}
}

// --- Config: configFilePath ---

func TestConfigFilePath_EndsWithExpectedSuffix(t *testing.T) {
	path, err := configFilePath()
	if err != nil {
		t.Skipf("configFilePath() error (may not have home dir in CI): %v", err)
	}
	suffix := filepath.Join("winbox-bridge", "config.json")
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
	if len(path) < len(suffix) || path[len(path)-len(suffix):] != suffix {
		t.Errorf("expected path ending in %q, got %q", suffix, path)
	}
}

// --- Config: round-trip via saveConfigTo / loadConfigFrom ---

func TestConfigRoundTrip_AllFieldsPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := Config{
		WinBoxPath:   "/usr/bin/winbox",
		ListenPort:   9999,
		TheiaOrigin:  "http://theia.example.com:8080",
		TheiaBaseURL: "http://theia.example.com:8080",
		BridgeSecret: "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899",
		LogLevel:     "debug",
	}

	if err := saveConfigTo(original, path); err != nil {
		t.Fatalf("saveConfigTo: %v", err)
	}

	loaded, err := loadConfigFrom(path)
	if err != nil {
		t.Fatalf("loadConfigFrom: %v", err)
	}

	if loaded != original {
		t.Errorf("round-trip mismatch")
	}
}

func TestConfigLoadConfigFrom_MissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	cfg, err := loadConfigFrom(path)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}

	defaults := DefaultConfig()
	if cfg != defaults {
		t.Errorf("expected defaults for missing file:\n  want: %+v\n  got:  %+v", defaults, cfg)
	}
}

func TestConfigLoadConfigFrom_CorruptJSONReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte("not valid json }{"), 0o600); err != nil {
		t.Fatalf("setup: write corrupt file: %v", err)
	}

	cfg, err := loadConfigFrom(path)
	// Should return defaults and a non-nil error (parse error)
	if err == nil {
		t.Error("expected non-nil error for corrupt JSON, got nil")
	}
	defaults := DefaultConfig()
	if cfg != defaults {
		t.Errorf("expected defaults on corrupt JSON:\n  want: %+v\n  got:  %+v", defaults, cfg)
	}
}

// --- Config: file permissions ---

func TestConfigSaveConfigTo_CreatesParentDirWith0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not report POSIX directory permission bits from os.Stat")
	}

	base := t.TempDir()
	// Use a subdirectory that doesn't exist yet
	subDir := filepath.Join(base, "newsubdir")
	path := filepath.Join(subDir, "config.json")

	if err := saveConfigTo(DefaultConfig(), path); err != nil {
		t.Fatalf("saveConfigTo: %v", err)
	}

	info, err := os.Stat(subDir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	got := info.Mode().Perm()
	if got != 0o700 {
		t.Errorf("expected dir perm 0o700, got %04o", got)
	}
}

func TestConfigSaveConfigTo_WritesFileWith0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not report POSIX file permission bits from os.Stat")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := saveConfigTo(DefaultConfig(), path); err != nil {
		t.Fatalf("saveConfigTo: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	got := info.Mode().Perm()
	if got != 0o600 {
		t.Errorf("expected file perm 0o600, got %04o", got)
	}
}

// --- Config: JSON field names ---

func TestConfigJSONFieldNames(t *testing.T) {
	cfg := Config{
		WinBoxPath:   "/some/path",
		ListenPort:   1234,
		TheiaOrigin:  "http://test.local",
		TheiaBaseURL: "http://test.local",
		BridgeSecret: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"winbox_path", "listen_port", "theia_origin", "theia_base_url", "bridge_secret", "log_level"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected JSON key %q not found in marshaled output", key)
		}
	}
}

// TestConfigLoadConfigFrom_MissingLogLevelUsesDefault verifies that a config file
// written by an older binary (without the log_level field) loads with the default
// "info" value rather than an empty string.
func TestConfigLoadConfigFrom_MissingLogLevelUsesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Simulate an old config file that predates the log_level field.
	old := `{"winbox_path":"","listen_port":1337,"theia_origin":"http://localhost:3000","bridge_secret":""}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatalf("setup: write old config: %v", err)
	}

	cfg, err := loadConfigFrom(path)
	if err != nil {
		t.Fatalf("loadConfigFrom: %v", err)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel=%q for missing log_level field, got %q", "info", cfg.LogLevel)
	}
	if cfg.TheiaBaseURL != "http://localhost:3000" {
		t.Errorf("expected TheiaBaseURL default for old config, got %q", cfg.TheiaBaseURL)
	}
}
