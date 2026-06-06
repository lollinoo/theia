package main

// This file exercises install behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstalledExecutablePathForUsesStableUserDataDirectory(t *testing.T) {
	tests := []struct {
		name         string
		goos         string
		homeDir      string
		localAppData string
		xdgDataHome  string
		want         string
	}{
		{
			name:         "windows",
			goos:         "windows",
			localAppData: filepath.Join("C:", "Users", "Alice", "AppData", "Local"),
			want: filepath.Join(
				"C:", "Users", "Alice", "AppData", "Local", "Theia", "WinBoxBridge",
				"winbox-bridge.exe",
			),
		},
		{
			name:    "macos",
			goos:    "darwin",
			homeDir: filepath.Join("Users", "alice"),
			want: filepath.Join(
				"Users", "alice", "Library", "Application Support", "Theia", "WinBoxBridge",
				"winbox-bridge",
			),
		},
		{
			name:        "linux xdg",
			goos:        "linux",
			homeDir:     filepath.Join("home", "alice"),
			xdgDataHome: filepath.Join("home", "alice", ".local", "share-custom"),
			want: filepath.Join(
				"home", "alice", ".local", "share-custom", "theia", "winbox-bridge",
				"winbox-bridge",
			),
		},
		{
			name:    "linux fallback",
			goos:    "linux",
			homeDir: filepath.Join("home", "alice"),
			want: filepath.Join(
				"home", "alice", ".local", "share", "theia", "winbox-bridge", "winbox-bridge",
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := installedExecutablePathFor(tt.goos, tt.homeDir, tt.localAppData, tt.xdgDataHome)
			if err != nil {
				t.Fatalf("installedExecutablePathFor returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSystemConnectorInstallerEnsureInstalledCopiesCurrentExecutableToStablePath(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "downloaded-bridge")
	target := filepath.Join(dir, "stable", "winbox-bridge")
	if err := os.WriteFile(source, []byte("bridge binary"), 0o700); err != nil {
		t.Fatalf("write source: %v", err)
	}
	installer := systemConnectorInstaller{
		currentExecutable: func() (string, error) { return source, nil },
		installedPath:     func() (string, error) { return target, nil },
	}

	status, err := installer.EnsureInstalled()
	if err != nil {
		t.Fatalf("EnsureInstalled returned error: %v", err)
	}
	if !status.Installed || status.InstalledPath != target || status.RunningFromInstalledPath {
		t.Fatalf("status = %#v", status)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "bridge binary" {
		t.Fatalf("target content = %q", got)
	}
}

func TestSystemConnectorInstallerEnsureInstalledMovesConfigBesideInstalledExecutable(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "downloaded-bridge")
	target := filepath.Join(dir, "stable", "winbox-bridge")
	legacyConfig := filepath.Join(dir, "legacy", "winbox-bridge", "config.json")
	if err := os.WriteFile(source, []byte("bridge binary"), 0o700); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyConfig), 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(legacyConfig, []byte(`{"listen_port":1444}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	installer := systemConnectorInstaller{
		currentExecutable: func() (string, error) { return source, nil },
		installedPath:     func() (string, error) { return target, nil },
		legacyConfigPath:  func() (string, error) { return legacyConfig, nil },
	}

	if _, err := installer.EnsureInstalled(); err != nil {
		t.Fatalf("EnsureInstalled returned error: %v", err)
	}

	installedConfig := filepath.Join(filepath.Dir(target), "config.json")
	got, err := os.ReadFile(installedConfig)
	if err != nil {
		t.Fatalf("read installed config: %v", err)
	}
	if string(got) != `{"listen_port":1444}` {
		t.Fatalf("installed config content = %q", got)
	}
	if _, err := os.Stat(legacyConfig); !os.IsNotExist(err) {
		t.Fatalf("legacy config still exists or returned unexpected error: %v", err)
	}
}

func TestSystemConnectorInstallerEnsureInstalledRepairsInvalidInstalledConfigFromLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "downloaded-bridge")
	target := filepath.Join(dir, "stable", "winbox-bridge")
	legacyConfig := filepath.Join(dir, "legacy", "winbox-bridge", "config.json")
	installedConfig := filepath.Join(filepath.Dir(target), "config.json")
	if err := os.WriteFile(source, []byte("bridge binary"), 0o700); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyConfig), 0o700); err != nil {
		t.Fatalf("create legacy config dir: %v", err)
	}
	saved := DefaultConfig()
	saved.WinBoxPath = "/opt/winbox"
	saved.ListenPort = 1444
	saved.TheiaOrigin = "http://theia.local:3000"
	saved.TheiaBaseURL = "http://theia.local:8080"
	saved.BridgeSecret = "theia_bridge_public.saved-secret"
	saved.LogLevel = "debug"
	if err := saveConfigTo(saved, legacyConfig); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(installedConfig), 0o700); err != nil {
		t.Fatalf("create install dir: %v", err)
	}
	if err := os.WriteFile(installedConfig, []byte("{"), 0o600); err != nil {
		t.Fatalf("write invalid installed config: %v", err)
	}
	installer := systemConnectorInstaller{
		currentExecutable: func() (string, error) { return source, nil },
		installedPath:     func() (string, error) { return target, nil },
		legacyConfigPath:  func() (string, error) { return legacyConfig, nil },
		currentConfig:     func() (Config, error) { return DefaultConfig(), nil },
	}

	if _, err := installer.EnsureInstalled(); err != nil {
		t.Fatalf("EnsureInstalled returned error: %v", err)
	}

	var got Config
	data, err := os.ReadFile(installedConfig)
	if err != nil {
		t.Fatalf("read installed config: %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse installed config: %v", err)
	}
	if got != saved {
		t.Fatalf("installed config = %+v, want saved config %+v", got, saved)
	}
	if _, err := os.Stat(legacyConfig); !os.IsNotExist(err) {
		t.Fatalf("legacy config still exists or returned unexpected error: %v", err)
	}
}

func TestSystemConnectorInstallerEnsureInstalledCreatesStableConfigWhenMissing(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "downloaded-bridge")
	target := filepath.Join(dir, "stable", "winbox-bridge")
	if err := os.WriteFile(source, []byte("bridge binary"), 0o700); err != nil {
		t.Fatalf("write source: %v", err)
	}
	installer := systemConnectorInstaller{
		currentExecutable: func() (string, error) { return source, nil },
		installedPath:     func() (string, error) { return target, nil },
		legacyConfigPath:  func() (string, error) { return filepath.Join(dir, "missing", "config.json"), nil },
		currentConfig: func() (Config, error) {
			cfg := DefaultConfig()
			cfg.ListenPort = 1555
			cfg.TheiaBaseURL = "http://localhost:8080"
			return cfg, nil
		},
	}

	status, err := installer.EnsureInstalled()
	if err != nil {
		t.Fatalf("EnsureInstalled returned error: %v", err)
	}
	if !status.InstalledConfigExists || !status.InstalledConfigValid || !status.InstallHealthy {
		t.Fatalf("status = %#v", status)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(target), "config.json"))
	if err != nil {
		t.Fatalf("read stable config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse stable config: %v", err)
	}
	if cfg.ListenPort != 1555 || cfg.TheiaBaseURL != "http://localhost:8080" {
		t.Fatalf("stable config was not written from current config: %+v", cfg)
	}
}

func TestSystemConnectorInstallerStatusReportsExecutableAndConfigHealth(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "downloaded-bridge")
	target := filepath.Join(dir, "stable", "winbox-bridge")
	if err := os.WriteFile(source, []byte("bridge binary"), 0o700); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatalf("create install dir: %v", err)
	}
	if err := os.WriteFile(target, []byte("bridge binary"), 0o700); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := saveConfigTo(DefaultConfig(), filepath.Join(filepath.Dir(target), "config.json")); err != nil {
		t.Fatalf("write config: %v", err)
	}
	installer := systemConnectorInstaller{
		currentExecutable: func() (string, error) { return source, nil },
		installedPath:     func() (string, error) { return target, nil },
	}

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.InstalledConfigPath != filepath.Join(filepath.Dir(target), "config.json") {
		t.Fatalf("InstalledConfigPath = %q", status.InstalledConfigPath)
	}
	if !status.Installed || !status.InstalledExecutableValid ||
		!status.InstalledConfigExists || !status.InstalledConfigValid || !status.InstallHealthy {
		t.Fatalf("status = %#v", status)
	}
}

func TestSystemConnectorInstallerStatusRejectsInvalidStableFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX executable bits")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "downloaded-bridge")
	target := filepath.Join(dir, "stable", "winbox-bridge")
	if err := os.WriteFile(source, []byte("bridge binary"), 0o700); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatalf("create install dir: %v", err)
	}
	if err := os.WriteFile(target, []byte(""), 0o600); err != nil {
		t.Fatalf("write invalid target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(target), "config.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	installer := systemConnectorInstaller{
		currentExecutable: func() (string, error) { return source, nil },
		installedPath:     func() (string, error) { return target, nil },
	}

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !status.Installed || !status.InstalledConfigExists {
		t.Fatalf("expected files to exist, got status = %#v", status)
	}
	if status.InstalledExecutableValid || status.InstalledConfigValid || status.InstallHealthy {
		t.Fatalf("invalid files reported healthy: %#v", status)
	}
}

func TestSystemConnectorInstallerStatusReportsMissingTarget(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "downloaded-bridge")
	target := filepath.Join(dir, "stable", "winbox-bridge")
	if err := os.WriteFile(source, []byte("bridge binary"), 0o700); err != nil {
		t.Fatalf("write source: %v", err)
	}
	installer := systemConnectorInstaller{
		currentExecutable: func() (string, error) { return source, nil },
		installedPath:     func() (string, error) { return target, nil },
	}

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.Installed || status.RunningFromInstalledPath || status.InstalledPath != target {
		t.Fatalf("status = %#v", status)
	}
}
