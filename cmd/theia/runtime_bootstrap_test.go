package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type stubRuntimeStopper struct {
	name  string
	stops *[]string
}

func (s stubRuntimeStopper) Stop() {
	*s.stops = append(*s.stops, s.name)
}

type stubRuntimeServer struct {
	listenErr error
}

func (s stubRuntimeServer) ListenAndServe() error {
	return s.listenErr
}

func (stubRuntimeServer) Shutdown(context.Context) error {
	return nil
}

func TestRuntimeBootstrapRunWrapsLoadConfigError(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	original := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { loadRuntimeConfig = original })

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want wrapped load config error")
	}
	if got, want := err.Error(), "load config: boom"; got != want {
		t.Fatalf("Run() error = %q, want %q", got, want)
	}
}

func TestRuntimeBootstrapStopRuntimeStopsChildrenInReverseOrder(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	var order []string
	children := runtimeChildren{
		stubRuntimeStopper{name: "pipeline", stops: &order},
		stubRuntimeStopper{name: "instance-backups", stops: &order},
		stubRuntimeStopper{name: "device-backups", stops: &order},
	}

	bootstrap.stopRuntime(children)

	if got, want := fmt.Sprint(order), "[device-backups instance-backups pipeline]"; got != want {
		t.Fatalf("stop order = %s, want %s", got, want)
	}
}

func TestRuntimeBootstrapServeTreatsServerClosedAsSuccess(t *testing.T) {
	bootstrap := &runtimeBootstrap{}

	if err := bootstrap.serve(stubRuntimeServer{listenErr: http.ErrServerClosed}); err != nil {
		t.Fatalf("serve() error = %v, want nil", err)
	}
	if err := bootstrap.serve(stubRuntimeServer{}); err != nil {
		t.Fatalf("serve() unexpected error = %v", err)
	}

	boom := errors.New("boom")
	if err := bootstrap.serve(stubRuntimeServer{listenErr: boom}); !reflect.DeepEqual(err, boom) {
		t.Fatalf("serve() error = %v, want %v", err, boom)
	}
}

func TestRuntimeBootstrapRunHardensBackupDirForPostgres(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()
	backupTarget := filepath.Join(runtimeDir, "backup-target")
	if err := os.Mkdir(backupTarget, 0o700); err != nil {
		t.Fatalf("Mkdir(backupTarget): %v", err)
	}
	backupLink := filepath.Join(runtimeDir, "backup-link")
	if err := os.Symlink(backupTarget, backupLink); err != nil {
		t.Fatalf("Symlink(backupLink): %v", err)
	}

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DBDriver:   "postgres",
			DBDSN:      "postgres://user:pass@127.0.0.1:1/theia?sslmode=disable",
			DBPath:     filepath.Join(runtimeDir, "theia.db"),
			DataDir:    runtimeDir,
			ListenAddr: ":0",
			LogLevel:   "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	originalBackupDir, hadBackupDir := os.LookupEnv("THEIA_BACKUP_DIR")
	if err := os.Setenv("THEIA_BACKUP_DIR", backupLink); err != nil {
		t.Fatalf("Setenv(THEIA_BACKUP_DIR): %v", err)
	}
	t.Cleanup(func() {
		if hadBackupDir {
			_ = os.Setenv("THEIA_BACKUP_DIR", originalBackupDir)
		} else {
			_ = os.Unsetenv("THEIA_BACKUP_DIR")
		}
	})

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want backup directory hardening error")
	}
	if got, want := err.Error(), fmt.Sprintf("prepare backup directory %s: ensure private dir: path is a symlink", backupLink); got != want {
		t.Fatalf("Run() error = %q, want %q", got, want)
	}
}

func TestRuntimeBootstrapRunTightensExistingKnownHostsFile(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	if err := os.WriteFile(knownHostsPath, []byte("127.0.0.1 ssh-ed25519 AAAA\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(known_hosts): %v", err)
	}

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DBDriver:   "postgres",
			DBDSN:      "postgres://user:pass@127.0.0.1:1/theia?sslmode=disable",
			DBPath:     filepath.Join(runtimeDir, "theia.db"),
			DataDir:    runtimeDir,
			ListenAddr: ":0",
			LogLevel:   "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want database connect error")
	}
	if got, want := err.Error(), "connect to database: failed to connect to `host=127.0.0.1 user=user database=theia`: dial error (dial tcp 127.0.0.1:1: connect: connection refused)"; got != want {
		t.Fatalf("Run() error = %q, want %q", got, want)
	}

	info, statErr := os.Stat(knownHostsPath)
	if statErr != nil {
		t.Fatalf("Stat(known_hosts): %v", statErr)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("known_hosts mode = %04o, want %04o", got, 0o600)
	}
}
