package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActivateOptionalRestoreArtifactsNoOptionalSourcesIsNoOp(t *testing.T) {
	missingStagingDir := filepath.Join(t.TempDir(), "missing-staging")

	if err := activateOptionalRestoreArtifacts(restoreMarker{}, missingStagingDir); err != nil {
		t.Fatalf("activateOptionalRestoreArtifacts() error = %v", err)
	}
}

func TestActivateOptionalRestoreArtifactsMissingOptionalSourcesAreNoOp(t *testing.T) {
	root := t.TempDir()
	stagingDir := filepath.Join(root, ".restore-staging")
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		t.Fatalf("creating staging dir: %v", err)
	}
	marker := restoreMarker{
		StagedBackups:    filepath.Join(stagingDir, "backups"),
		StagedKnownHosts: filepath.Join(stagingDir, "known_hosts"),
		DeviceBackupDir:  filepath.Join(root, "device-backups"),
		KnownHostsPath:   filepath.Join(root, "known_hosts"),
	}

	if err := activateOptionalRestoreArtifacts(marker, stagingDir); err != nil {
		t.Fatalf("activateOptionalRestoreArtifacts() error = %v", err)
	}
}

func TestActivateOptionalRestoreArtifactsReplacesBackupsAndKnownHosts(t *testing.T) {
	root := t.TempDir()
	stagingDir := filepath.Join(root, ".restore-staging")
	stagedBackups := filepath.Join(stagingDir, "backups")
	stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")
	deviceBackupDir := filepath.Join(root, "device-backups")
	knownHostsPath := filepath.Join(root, "known_hosts")

	if err := os.MkdirAll(filepath.Join(stagedBackups, "router"), 0o700); err != nil {
		t.Fatalf("creating staged backups: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(deviceBackupDir, "router"), 0o700); err != nil {
		t.Fatalf("creating live backups: %v", err)
	}
	writeRegularFile(t, filepath.Join(stagedBackups, "router", "config.rsc"), "restored-backup")
	writeRegularFile(t, filepath.Join(deviceBackupDir, "router", "config.rsc"), "live-backup")
	writeRegularFile(t, stagedKnownHosts, "restored-known-hosts")
	writeRegularFile(t, knownHostsPath, "live-known-hosts")

	marker := restoreMarker{
		StagedBackups:    stagedBackups,
		StagedKnownHosts: stagedKnownHosts,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
	}

	if err := activateOptionalRestoreArtifacts(marker, stagingDir); err != nil {
		t.Fatalf("activateOptionalRestoreArtifacts() error = %v", err)
	}

	backupBytes, err := os.ReadFile(filepath.Join(deviceBackupDir, "router", "config.rsc"))
	if err != nil {
		t.Fatalf("reading restored backup: %v", err)
	}
	if string(backupBytes) != "restored-backup" {
		t.Fatalf("restored backup = %q, want restored-backup", string(backupBytes))
	}
	knownHostsBytes, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("reading restored known_hosts: %v", err)
	}
	if string(knownHostsBytes) != "restored-known-hosts" {
		t.Fatalf("restored known_hosts = %q, want restored-known-hosts", string(knownHostsBytes))
	}
}

func TestActivateOptionalRestoreArtifactsRejectsUnsafeSources(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, root string, stagingDir string) restoreMarker
		wantErr string
	}{
		{
			name: "backup dir symlink",
			setup: func(t *testing.T, root string, stagingDir string) restoreMarker {
				t.Helper()
				targetDir := filepath.Join(root, "backup-target")
				if err := os.Mkdir(targetDir, 0o700); err != nil {
					t.Fatalf("creating backup target: %v", err)
				}
				if err := os.Symlink(targetDir, filepath.Join(stagingDir, "backups")); err != nil {
					t.Fatalf("creating backup symlink: %v", err)
				}
				return restoreMarker{
					StagedBackups:   filepath.Join(stagingDir, "backups"),
					DeviceBackupDir: filepath.Join(root, "device-backups"),
				}
			},
			wantErr: "validate staged backup dir: staged backup dir must not be a symlink",
		},
		{
			name: "known hosts directory",
			setup: func(t *testing.T, root string, stagingDir string) restoreMarker {
				t.Helper()
				if err := os.Mkdir(filepath.Join(stagingDir, "known_hosts"), 0o700); err != nil {
					t.Fatalf("creating staged known_hosts directory: %v", err)
				}
				return restoreMarker{
					StagedKnownHosts: filepath.Join(stagingDir, "known_hosts"),
					KnownHostsPath:   filepath.Join(root, "known_hosts"),
				}
			},
			wantErr: "validate staged known_hosts: staged known_hosts must be a regular file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			stagingDir := filepath.Join(root, ".restore-staging")
			if err := os.MkdirAll(stagingDir, 0o700); err != nil {
				t.Fatalf("creating staging dir: %v", err)
			}
			marker := tt.setup(t, root, stagingDir)

			err := activateOptionalRestoreArtifacts(marker, stagingDir)
			if err == nil {
				t.Fatalf("activateOptionalRestoreArtifacts() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("activateOptionalRestoreArtifacts() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
