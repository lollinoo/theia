package main

// This file exercises runtime paths behavior so refactors preserve the documented contract.

import (
	"path/filepath"
	"testing"

	"github.com/lollinoo/theia/internal/config"
)

func TestResolveRuntimePaths_DataDirSetsAppDataAndDefaultArtifacts(t *testing.T) {
	cfg := &config.Config{
		DataDir: filepath.Join("/srv", "theia"),
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
