package main

import (
	"os"
	"path/filepath"
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
