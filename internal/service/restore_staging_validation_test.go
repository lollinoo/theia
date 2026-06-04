package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRestoreStagingLayoutAcceptsExpectedArtifacts(t *testing.T) {
	stateDir := t.TempDir()
	stagingDir := filepath.Join(stateDir, ".restore-staging")
	if err := os.MkdirAll(filepath.Join(stagingDir, "backups", "router"), 0o700); err != nil {
		t.Fatalf("creating staging backups: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, postgresArchiveDBEntry), []byte("dump"), 0o600); err != nil {
		t.Fatalf("writing staged db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "backups", "router", "config.rsc"), []byte("backup"), 0o600); err != nil {
		t.Fatalf("writing staged backup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "known_hosts"), []byte("host key"), 0o600); err != nil {
		t.Fatalf("writing staged known_hosts: %v", err)
	}

	marker := newRestoreMarker(
		filepath.Join(stagingDir, postgresArchiveDBEntry),
		filepath.Join(stagingDir, "backups"),
		filepath.Join(stagingDir, "known_hosts"),
		stateDir,
		filepath.Join(stateDir, "device-backups"),
		filepath.Join(stateDir, "known_hosts"),
		"2026-04-23T00:00:00Z",
	)

	if err := validateRestoreStagingLayout(marker, stagingDir); err != nil {
		t.Fatalf("validateRestoreStagingLayout() error = %v", err)
	}
}

func TestValidateRestoreStagingLayoutRejectsUnsafeArtifacts(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, stagingDir string) restoreMarker
		wantErr string
	}{
		{
			name: "wrong db path",
			setup: func(t *testing.T, stagingDir string) restoreMarker {
				t.Helper()
				writeRegularFile(t, filepath.Join(stagingDir, postgresArchiveDBEntry), "dump")
				return restoreMarker{StagedDB: filepath.Join(stagingDir, "other.dump")}
			},
			wantErr: "restore marker staged db path does not match runtime staging path",
		},
		{
			name: "staged db directory",
			setup: func(t *testing.T, stagingDir string) restoreMarker {
				t.Helper()
				if err := os.Mkdir(filepath.Join(stagingDir, postgresArchiveDBEntry), 0o700); err != nil {
					t.Fatalf("creating staged db directory: %v", err)
				}
				return restoreMarker{StagedDB: filepath.Join(stagingDir, postgresArchiveDBEntry)}
			},
			wantErr: "staged db must be a regular file",
		},
		{
			name: "staged backup symlink",
			setup: func(t *testing.T, stagingDir string) restoreMarker {
				t.Helper()
				writeRegularFile(t, filepath.Join(stagingDir, postgresArchiveDBEntry), "dump")
				targetDir := filepath.Join(t.TempDir(), "backups-target")
				if err := os.Mkdir(targetDir, 0o700); err != nil {
					t.Fatalf("creating backup symlink target: %v", err)
				}
				if err := os.Symlink(targetDir, filepath.Join(stagingDir, "backups")); err != nil {
					t.Fatalf("creating backup symlink: %v", err)
				}
				return restoreMarker{
					StagedDB:      filepath.Join(stagingDir, postgresArchiveDBEntry),
					StagedBackups: filepath.Join(stagingDir, "backups"),
				}
			},
			wantErr: "staged backup dir must not be a symlink",
		},
		{
			name: "staged known hosts directory",
			setup: func(t *testing.T, stagingDir string) restoreMarker {
				t.Helper()
				writeRegularFile(t, filepath.Join(stagingDir, postgresArchiveDBEntry), "dump")
				if err := os.Mkdir(filepath.Join(stagingDir, "known_hosts"), 0o700); err != nil {
					t.Fatalf("creating known_hosts directory: %v", err)
				}
				return restoreMarker{
					StagedDB:         filepath.Join(stagingDir, postgresArchiveDBEntry),
					StagedKnownHosts: filepath.Join(stagingDir, "known_hosts"),
				}
			},
			wantErr: "staged known_hosts must be a regular file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stagingDir := filepath.Join(t.TempDir(), ".restore-staging")
			if err := os.MkdirAll(stagingDir, 0o700); err != nil {
				t.Fatalf("creating staging dir: %v", err)
			}
			marker := tt.setup(t, stagingDir)

			err := validateRestoreStagingLayout(marker, stagingDir)
			if err == nil {
				t.Fatalf("validateRestoreStagingLayout() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateRestoreStagingLayout() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func writeRegularFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
