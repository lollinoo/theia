package main

import (
	"path/filepath"
	"testing"

	"github.com/lollinoo/theia/internal/config"
)

func TestResolveRuntimePaths_DBPathSetsAppDataAndDefaultArtifacts(t *testing.T) {
	cfg := &config.Config{
		DBPath: filepath.Join("/srv", "theia", "theia.db"),
	}

	got := resolveRuntimePaths(cfg)
	want := runtimePaths{
		appDataDir:        filepath.Join("/srv", "theia"),
		backupDir:         filepath.Join("/srv", "theia", "backups"),
		knownHostsPath:    filepath.Join("/srv", "theia", "known_hosts"),
		instanceBackupDir: filepath.Join("/srv", "theia", "instance-backups"),
	}

	if got != want {
		t.Fatalf("resolveRuntimePaths() = %#v, want %#v", got, want)
	}
}

func TestResolveRuntimePaths_DataDirWinsAndEnvOverridesArtifactDirs(t *testing.T) {
	t.Setenv("THEIA_BACKUP_DIR", filepath.Join("/env", "device-backups"))
	t.Setenv("THEIA_INSTANCE_BACKUP_DIR", filepath.Join("/env", "instance-backups"))

	cfg := &config.Config{
		DBPath:  filepath.Join("/srv", "db", "theia.db"),
		DataDir: filepath.Join("/srv", "runtime"),
	}

	got := resolveRuntimePaths(cfg)
	want := runtimePaths{
		appDataDir:        filepath.Join("/srv", "runtime"),
		backupDir:         filepath.Join("/env", "device-backups"),
		knownHostsPath:    filepath.Join("/srv", "runtime", "known_hosts"),
		instanceBackupDir: filepath.Join("/env", "instance-backups"),
	}

	if got != want {
		t.Fatalf("resolveRuntimePaths() = %#v, want %#v", got, want)
	}
}
