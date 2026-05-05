package service

import (
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"os"
)

func createRestoreTestUnixSocket(t *testing.T, path string) func() {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating parent dir for %s: %v", path, err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("unix socket restore test unsupported on this platform: %v", err)
	}
	return func() {
		_ = listener.Close()
	}
}

func newShortRestoreTestDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "tr")
	if err != nil {
		t.Fatalf("MkdirTemp(short restore dir): %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func writeRestoreTestFile(t *testing.T, path string, contents string, mode os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating parent dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func readRestoreTestFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func assertRestorePathMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %04o, want %04o", path, got, want)
	}
}

func writeRestoreMarker(t *testing.T, markerPath string, marker restoreMarker) {
	t.Helper()

	data, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal restore marker: %v", err)
	}
	if err := os.WriteFile(markerPath, data, 0644); err != nil {
		t.Fatalf("write restore marker: %v", err)
	}
}

func TestNewRestoreMarkerUsesStableJSONFields(t *testing.T) {
	marker := newRestoreMarker(
		"/runtime/.restore-staging/theia.db",
		"/runtime/.restore-staging/backups",
		"/runtime/.restore-staging/known_hosts",
		"/runtime/theia.db",
		"/runtime/device-backups",
		"/runtime/known_hosts",
		"2026-04-20T00:00:00Z",
	)

	data, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("json.Marshal(marker): %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(marker): %v", err)
	}

	want := map[string]string{
		"staged_db":          "/runtime/.restore-staging/theia.db",
		"staged_backups":     "/runtime/.restore-staging/backups",
		"staged_known_hosts": "/runtime/.restore-staging/known_hosts",
		"db_path":            "/runtime/theia.db",
		"device_backup_dir":  "/runtime/device-backups",
		"known_hosts_path":   "/runtime/known_hosts",
		"timestamp":          "2026-04-20T00:00:00Z",
	}

	if len(got) != len(want) {
		t.Fatalf("marker field count = %d, want %d; marker=%v", len(got), len(want), got)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("marker[%q] = %q, want %q", key, got[key], wantValue)
		}
	}
}

func TestRestoreCoordinatorApplyPendingRestoreReturnsFalseWithoutMarker(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err != nil {
		t.Fatalf("ApplyPendingRestore() error = %v", err)
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsMalformedMarker(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	markerPath := filepath.Join(filepath.Dir(dbPath), ".theia-restore-pending")

	if err := os.WriteFile(markerPath, []byte("{not-json"), 0644); err != nil {
		t.Fatalf("WriteFile(%q): %v", markerPath, err)
	}

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want parse error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if !strings.Contains(err.Error(), "parse restore marker") {
		t.Fatalf("ApplyPendingRestore() error = %q, want substring %q", err.Error(), "parse restore marker")
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsMarkerPathMismatch(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	markerPath := filepath.Join(filepath.Dir(dbPath), ".theia-restore-pending")

	marker := restoreMarker{
		StagedDB:         filepath.Join(runtimeDir, ".restore-staging", "theia.db"),
		StagedBackups:    filepath.Join(runtimeDir, ".restore-staging", "backups"),
		StagedKnownHosts: filepath.Join(runtimeDir, ".restore-staging", "known_hosts"),
		DBPath:           filepath.Join(runtimeDir, "other.db"),
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("json.Marshal(marker): %v", err)
	}
	if err := os.WriteFile(markerPath, markerJSON, 0644); err != nil {
		t.Fatalf("WriteFile(%q): %v", markerPath, err)
	}

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want path mismatch error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if !strings.Contains(err.Error(), "restore marker targets do not match runtime paths") {
		t.Fatalf("ApplyPendingRestore() error = %q, want substring %q", err.Error(), "restore marker targets do not match runtime paths")
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsDeviceBackupDirMismatch(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	markerPath := filepath.Join(filepath.Dir(dbPath), ".theia-restore-pending")

	marker := restoreMarker{
		StagedDB:         filepath.Join(runtimeDir, ".restore-staging", "theia.db"),
		StagedBackups:    filepath.Join(runtimeDir, ".restore-staging", "backups"),
		StagedKnownHosts: filepath.Join(runtimeDir, ".restore-staging", "known_hosts"),
		DBPath:           dbPath,
		DeviceBackupDir:  filepath.Join(runtimeDir, "other-device-backups"),
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("json.Marshal(marker): %v", err)
	}
	if err := os.WriteFile(markerPath, markerJSON, 0644); err != nil {
		t.Fatalf("WriteFile(%q): %v", markerPath, err)
	}

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want path mismatch error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if !strings.Contains(err.Error(), "restore marker targets do not match runtime paths") {
		t.Fatalf("ApplyPendingRestore() error = %q, want substring %q", err.Error(), "restore marker targets do not match runtime paths")
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsKnownHostsPathMismatch(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	markerPath := filepath.Join(filepath.Dir(dbPath), ".theia-restore-pending")

	marker := restoreMarker{
		StagedDB:         filepath.Join(runtimeDir, ".restore-staging", "theia.db"),
		StagedBackups:    filepath.Join(runtimeDir, ".restore-staging", "backups"),
		StagedKnownHosts: filepath.Join(runtimeDir, ".restore-staging", "known_hosts"),
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   filepath.Join(runtimeDir, "other_known_hosts"),
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("json.Marshal(marker): %v", err)
	}
	if err := os.WriteFile(markerPath, markerJSON, 0644); err != nil {
		t.Fatalf("WriteFile(%q): %v", markerPath, err)
	}

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want path mismatch error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if !strings.Contains(err.Error(), "restore marker targets do not match runtime paths") {
		t.Fatalf("ApplyPendingRestore() error = %q, want substring %q", err.Error(), "restore marker targets do not match runtime paths")
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsIncompleteMarkerWithoutStagedDB(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	markerPath := filepath.Join(filepath.Dir(dbPath), ".theia-restore-pending")

	marker := restoreMarker{
		StagedBackups:    filepath.Join(runtimeDir, ".restore-staging", "backups"),
		StagedKnownHosts: filepath.Join(runtimeDir, ".restore-staging", "known_hosts"),
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("json.Marshal(marker): %v", err)
	}
	if err := os.WriteFile(markerPath, markerJSON, 0644); err != nil {
		t.Fatalf("WriteFile(%q): %v", markerPath, err)
	}

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want incomplete marker error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if !strings.Contains(err.Error(), "restore marker missing staged_db") {
		t.Fatalf("ApplyPendingRestore() error = %q, want substring %q", err.Error(), "restore marker missing staged_db")
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsStagedDBOutsideRuntimeStaging(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")
	externalStagingDir := filepath.Join(runtimeDir, "tampered-staging")
	externalStagedDB := filepath.Join(externalStagingDir, "theia.db")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, externalStagedDB, "staged-db", 0644)

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:        externalStagedDB,
		DBPath:          dbPath,
		DeviceBackupDir: deviceBackupDir,
		KnownHostsPath:  knownHostsPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want staged db path error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
	}
	if _, err := os.Stat(externalStagingDir); err != nil {
		t.Fatalf("external staged db parent should not be removed, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, externalStagedDB); got != "staged-db" {
		t.Fatalf("external staged db content = %q, want staged-db", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsSymlinkedRuntimeStagingDirBeforeDBActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	externalStagingDir := filepath.Join(runtimeDir, "external-staging")
	externalStagedDB := filepath.Join(externalStagingDir, "theia.db")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, externalStagedDB, "staged-db", 0644)
	if err := os.Symlink(externalStagingDir, stagingDir); err != nil {
		t.Fatalf("Symlink runtime staging dir: %v", err)
	}

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:        stagedDB,
		DBPath:          dbPath,
		DeviceBackupDir: deviceBackupDir,
		KnownHostsPath:  knownHostsPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want staging dir symlink error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
	}
	if _, err := os.Lstat(stagingDir); err != nil {
		t.Fatalf("runtime staging symlink should remain after preflight failure, lstat err = %v", err)
	}
	if _, err := os.Stat(externalStagingDir); err != nil {
		t.Fatalf("external staging dir should not be removed, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, externalStagedDB); got != "staged-db" {
		t.Fatalf("external staged db content = %q, want staged-db", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsSymlinkedStagedDBBeforeDBActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	externalStagedDB := filepath.Join(runtimeDir, "external-theia.db")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, externalStagedDB, "staged-db", 0644)
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", stagingDir, err)
	}
	if err := os.Symlink(externalStagedDB, stagedDB); err != nil {
		t.Fatalf("Symlink staged db: %v", err)
	}

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:        stagedDB,
		DBPath:          dbPath,
		DeviceBackupDir: deviceBackupDir,
		KnownHostsPath:  knownHostsPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want staged db symlink error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, externalStagedDB); got != "staged-db" {
		t.Fatalf("external staged db content = %q, want staged-db", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsSymlinkedStagedKnownHostsBeforeDBActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")
	externalKnownHosts := filepath.Join(runtimeDir, "external-known-hosts")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, externalKnownHosts, "staged-known-hosts", 0644)
	if err := os.Symlink(externalKnownHosts, stagedKnownHosts); err != nil {
		t.Fatalf("Symlink staged known_hosts: %v", err)
	}

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:         stagedDB,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want staged known_hosts symlink error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, externalKnownHosts); got != "staged-known-hosts" {
		t.Fatalf("external known_hosts content = %q, want staged-known-hosts", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsSymlinkedStagedBackupRootBeforeDBActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	externalBackups := filepath.Join(runtimeDir, "external-backups")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(externalBackups, "router.cfg"), "staged-backup", 0644)
	if err := os.Symlink(externalBackups, stagedBackups); err != nil {
		t.Fatalf("Symlink staged backup root: %v", err)
	}

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:        stagedDB,
		StagedBackups:   stagedBackups,
		DBPath:          dbPath,
		DeviceBackupDir: deviceBackupDir,
		KnownHostsPath:  knownHostsPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want staged backup root symlink error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, filepath.Join(externalBackups, "router.cfg")); got != "staged-backup" {
		t.Fatalf("external staged backup content = %q, want staged-backup", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsSocketInStagedBackupsBeforeDBActivation(t *testing.T) {
	runtimeDir := newShortRestoreTestDir(t)
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	socketPath := filepath.Join(stagedBackups, "backup.sock")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	closeSocket := createRestoreTestUnixSocket(t, socketPath)
	defer closeSocket()

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:        stagedDB,
		StagedBackups:   stagedBackups,
		DBPath:          dbPath,
		DeviceBackupDir: deviceBackupDir,
		KnownHostsPath:  knownHostsPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want staged backup socket error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsPostgresStagedDumpSymlinkBeforeDBActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDump := filepath.Join(stagingDir, "database.dump")
	externalDump := filepath.Join(runtimeDir, "external-database.dump")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, externalDump, "staged-postgres-dump", 0644)
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", stagingDir, err)
	}
	if err := os.Symlink(externalDump, stagedDump); err != nil {
		t.Fatalf("Symlink staged postgres dump: %v", err)
	}

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:        stagedDump,
		DBDriver:        "postgres",
		DBPath:          dbPath,
		DeviceBackupDir: deviceBackupDir,
		KnownHostsPath:  knownHostsPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinatorWithDSN(dbPath, "postgres://theia:secret@localhost:5432/theia?sslmode=disable", deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want staged postgres dump symlink error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, externalDump); got != "staged-postgres-dump" {
		t.Fatalf("external postgres dump content = %q, want staged-postgres-dump", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRevalidatesStagedDBBeforeSQLiteActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	stagedDB := filepath.Join(runtimeDir, ".restore-staging", "theia.db")
	externalStagedDB := filepath.Join(runtimeDir, "external-theia.db")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, externalStagedDB, "staged-db", 0644)
	if err := os.MkdirAll(filepath.Dir(stagedDB), 0755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(stagedDB), err)
	}
	if err := os.Symlink(externalStagedDB, stagedDB); err != nil {
		t.Fatalf("Symlink staged db: %v", err)
	}

	coordinator := NewRestoreCoordinator(dbPath, filepath.Join(runtimeDir, "device-backups"), filepath.Join(runtimeDir, "known_hosts"))

	err := coordinator.applySQLiteRestore(stagedDB)
	if err == nil {
		t.Fatal("applySQLiteRestore() error = nil, want staged db symlink error")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if got := readRestoreTestFile(t, externalStagedDB); got != "staged-db" {
		t.Fatalf("external staged db content = %q, want staged-db", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestorePreservesStagedDBAfterSQLiteActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	stagedDB := filepath.Join(runtimeDir, ".restore-staging", "theia.db")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, dbPath+"-wal", "live-wal", 0644)
	writeRestoreTestFile(t, dbPath+"-shm", "live-shm", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)

	coordinator := NewRestoreCoordinator(dbPath, filepath.Join(runtimeDir, "device-backups"), filepath.Join(runtimeDir, "known_hosts"))

	if err := coordinator.applySQLiteRestore(stagedDB); err != nil {
		t.Fatalf("applySQLiteRestore() error = %v", err)
	}

	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content = %q, want staged-db", got)
	}
	assertRestorePathMode(t, dbPath, 0600)
	if _, err := os.Stat(dbPath + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("wal file should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(dbPath + "-shm"); !os.IsNotExist(err) {
		t.Fatalf("shm file should be removed, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, stagedDB); got != "staged-db" {
		t.Fatalf("staged db content = %q, want staged-db", got)
	}
}

func TestReplaceFileForRestoreReplacesExistingFileAndRemovesScratchPaths(t *testing.T) {
	runtimeDir := t.TempDir()
	liveKnownHosts := filepath.Join(runtimeDir, "known_hosts")
	stagedKnownHosts := filepath.Join(runtimeDir, ".restore-staging", "known_hosts")

	writeRestoreTestFile(t, liveKnownHosts, "live-known-hosts", 0644)
	writeRestoreTestFile(t, stagedKnownHosts, "staged-known-hosts", 0644)
	writeRestoreTestFile(t, liveKnownHosts+".restore-old", "stale-old", 0644)
	writeRestoreTestFile(t, liveKnownHosts+".restore-tmp", "stale-tmp", 0644)

	if err := replaceFileForRestore(stagedKnownHosts, liveKnownHosts); err != nil {
		t.Fatalf("replaceFileForRestore() error = %v", err)
	}

	if got := readRestoreTestFile(t, liveKnownHosts); got != "staged-known-hosts" {
		t.Fatalf("known_hosts content = %q, want staged-known-hosts", got)
	}
	assertRestorePathMode(t, liveKnownHosts, 0600)
	if _, err := os.Stat(liveKnownHosts + ".restore-old"); !os.IsNotExist(err) {
		t.Fatalf("old scratch path should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(liveKnownHosts + ".restore-tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp scratch path should be removed, stat err = %v", err)
	}
}

func TestReplaceDirForRestoreReplacesExistingDirectoryAndRemovesScratchPaths(t *testing.T) {
	runtimeDir := newShortRestoreTestDir(t)
	liveBackups := filepath.Join(runtimeDir, "device-backups")
	stagedBackups := filepath.Join(runtimeDir, ".restore-staging", "backups")

	writeRestoreTestFile(t, filepath.Join(liveBackups, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, filepath.Join(liveBackups, "live-only.cfg"), "obsolete-live-backup", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "nested", "switch.cfg"), "staged-switch", 0644)
	writeRestoreTestFile(t, filepath.Join(liveBackups+".restore-old", "old.cfg"), "stale-old", 0644)
	writeRestoreTestFile(t, filepath.Join(liveBackups+".restore-tmp", "tmp.cfg"), "stale-tmp", 0644)

	if err := replaceDirForRestore(stagedBackups, liveBackups); err != nil {
		t.Fatalf("replaceDirForRestore() error = %v", err)
	}

	if got := readRestoreTestFile(t, filepath.Join(liveBackups, "router.cfg")); got != "staged-backup" {
		t.Fatalf("backup content = %q, want staged-backup", got)
	}
	if got := readRestoreTestFile(t, filepath.Join(liveBackups, "nested", "switch.cfg")); got != "staged-switch" {
		t.Fatalf("nested backup content = %q, want staged-switch", got)
	}
	if _, err := os.Stat(filepath.Join(liveBackups, "live-only.cfg")); !os.IsNotExist(err) {
		t.Fatalf("live-only backup should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(liveBackups + ".restore-old"); !os.IsNotExist(err) {
		t.Fatalf("old scratch path should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(liveBackups + ".restore-tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp scratch path should be removed, stat err = %v", err)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRevalidatesStagedKnownHostsBeforeActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	liveKnownHosts := filepath.Join(runtimeDir, "known_hosts")
	stagedKnownHosts := filepath.Join(runtimeDir, ".restore-staging", "known_hosts")
	externalKnownHosts := filepath.Join(runtimeDir, "external-known-hosts")

	writeRestoreTestFile(t, liveKnownHosts, "live-known-hosts", 0644)
	writeRestoreTestFile(t, externalKnownHosts, "staged-known-hosts", 0644)
	if err := os.MkdirAll(filepath.Dir(stagedKnownHosts), 0755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(stagedKnownHosts), err)
	}
	if err := os.Symlink(externalKnownHosts, stagedKnownHosts); err != nil {
		t.Fatalf("Symlink staged known_hosts: %v", err)
	}

	err := replaceFileForRestore(stagedKnownHosts, liveKnownHosts)
	if err == nil {
		t.Fatal("replaceFileForRestore() error = nil, want staged known_hosts symlink error")
	}
	if got := readRestoreTestFile(t, liveKnownHosts); got != "live-known-hosts" {
		t.Fatalf("known_hosts content = %q, want live-known-hosts", got)
	}
	if got := readRestoreTestFile(t, externalKnownHosts); got != "staged-known-hosts" {
		t.Fatalf("external known_hosts content = %q, want staged-known-hosts", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRevalidatesStagedBackupsBeforeActivation(t *testing.T) {
	runtimeDir := newShortRestoreTestDir(t)
	liveBackups := filepath.Join(runtimeDir, "device-backups")
	stagedBackups := filepath.Join(runtimeDir, ".restore-staging", "backups")
	socketPath := filepath.Join(stagedBackups, "backup.sock")

	writeRestoreTestFile(t, filepath.Join(liveBackups, "router.cfg"), "live-backup", 0644)
	closeSocket := createRestoreTestUnixSocket(t, socketPath)
	defer closeSocket()

	err := replaceDirForRestore(stagedBackups, liveBackups)
	if err == nil {
		t.Fatal("replaceDirForRestore() error = nil, want staged backup socket error")
	}
	if got := readRestoreTestFile(t, filepath.Join(liveBackups, "router.cfg")); got != "live-backup" {
		t.Fatalf("backup content = %q, want live-backup", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreTreatsChangedStagedBackupsFileAsRetryable(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)

	originalHook := restoreCoordinatorAfterDBActivationHook
	restoreCoordinatorAfterDBActivationHook = func() error {
		if err := os.RemoveAll(stagedBackups); err != nil {
			return err
		}
		return os.WriteFile(stagedBackups, []byte("not-a-directory"), 0644)
	}
	t.Cleanup(func() {
		restoreCoordinatorAfterDBActivationHook = originalHook
	})

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:        stagedDB,
		StagedBackups:   stagedBackups,
		DBPath:          dbPath,
		DeviceBackupDir: deviceBackupDir,
		KnownHostsPath:  knownHostsPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want changed staged backups validation error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content = %q, want staged-db after DB activation", got)
	}
	if got := readRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg")); got != "live-backup" {
		t.Fatalf("backup content = %q, want live-backup", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after retryable failure, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); err != nil {
		t.Fatalf("staging dir should remain after retryable failure, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, stagedDB); got != "staged-db" {
		t.Fatalf("staged db content after retry state restore = %q, want staged-db", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreDoesNotRepairRetryStateThroughUnsafeStagingDir(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")
	externalStagingDir := filepath.Join(runtimeDir, "external-staging")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, knownHostsPath, "live-known-hosts", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)
	writeRestoreTestFile(t, stagedKnownHosts, "staged-known-hosts", 0644)
	writeRestoreTestFile(t, filepath.Join(externalStagingDir, "backups", "router.cfg"), "external-backup", 0644)
	writeRestoreTestFile(t, filepath.Join(externalStagingDir, "known_hosts"), "external-known-hosts", 0644)

	originalHook := restoreCoordinatorAfterDBActivationHook
	restoreCoordinatorAfterDBActivationHook = func() error {
		if err := os.RemoveAll(stagingDir); err != nil {
			return err
		}
		return os.Symlink(externalStagingDir, stagingDir)
	}
	t.Cleanup(func() {
		restoreCoordinatorAfterDBActivationHook = originalHook
	})

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:         stagedDB,
		StagedBackups:    stagedBackups,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want changed staging dir validation error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content = %q, want staged-db after DB activation", got)
	}
	if got := readRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg")); got != "live-backup" {
		t.Fatalf("backup content = %q, want live-backup", got)
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "live-known-hosts" {
		t.Fatalf("known_hosts content = %q, want live-known-hosts", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after retryable failure, stat err = %v", err)
	}
	if info, err := os.Lstat(stagingDir); err != nil {
		t.Fatalf("staging symlink should remain after retryable failure, lstat err = %v", err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("staging path should remain a symlink after retryable failure, mode=%v", info.Mode())
	}
	if _, err := os.Lstat(filepath.Join(externalStagingDir, "theia.db")); !os.IsNotExist(err) {
		t.Fatalf("external staging target should not receive retry DB copy, lstat err = %v", err)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreTreatsDanglingStagedBackupsSymlinkAsRetryable(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)

	originalHook := restoreCoordinatorAfterDBActivationHook
	restoreCoordinatorAfterDBActivationHook = func() error {
		if err := os.RemoveAll(stagedBackups); err != nil {
			return err
		}
		return os.Symlink(filepath.Join(runtimeDir, "missing-backups"), stagedBackups)
	}
	t.Cleanup(func() {
		restoreCoordinatorAfterDBActivationHook = originalHook
	})

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:        stagedDB,
		StagedBackups:   stagedBackups,
		DBPath:          dbPath,
		DeviceBackupDir: deviceBackupDir,
		KnownHostsPath:  knownHostsPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want dangling staged backups symlink error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content = %q, want staged-db after DB activation", got)
	}
	if got := readRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg")); got != "live-backup" {
		t.Fatalf("backup content = %q, want live-backup", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after retryable failure, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); err != nil {
		t.Fatalf("staging dir should remain after retryable failure, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, stagedDB); got != "staged-db" {
		t.Fatalf("staged db content after retry state restore = %q, want staged-db", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreTreatsDanglingStagedKnownHostsSymlinkAsRetryable(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, knownHostsPath, "live-known-hosts", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, stagedKnownHosts, "staged-known-hosts", 0644)

	originalHook := restoreCoordinatorAfterDBActivationHook
	restoreCoordinatorAfterDBActivationHook = func() error {
		if err := os.Remove(stagedKnownHosts); err != nil {
			return err
		}
		return os.Symlink(filepath.Join(runtimeDir, "missing-known-hosts"), stagedKnownHosts)
	}
	t.Cleanup(func() {
		restoreCoordinatorAfterDBActivationHook = originalHook
	})

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:         stagedDB,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want dangling staged known_hosts symlink error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content = %q, want staged-db after DB activation", got)
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "live-known-hosts" {
		t.Fatalf("known_hosts content = %q, want live-known-hosts", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after retryable failure, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); err != nil {
		t.Fatalf("staging dir should remain after retryable failure, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, stagedDB); got != "staged-db" {
		t.Fatalf("staged db content after retry state restore = %q, want staged-db", got)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsStagedBackupsOutsideRuntimeStagingBeforeDBActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	externalBackups := filepath.Join(runtimeDir, "tampered-backups")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(externalBackups, "router.cfg"), "external-backup", 0644)

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:        stagedDB,
		StagedBackups:   externalBackups,
		DBPath:          dbPath,
		DeviceBackupDir: deviceBackupDir,
		KnownHostsPath:  knownHostsPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want staged backups path error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsWrongOptionalArtifactTypesBeforeDBActivation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, stagedBackups, stagedKnownHosts string)
	}{
		{
			name: "backups path is file",
			setup: func(t *testing.T, stagedBackups, stagedKnownHosts string) {
				writeRestoreTestFile(t, stagedBackups, "not-a-directory", 0644)
				writeRestoreTestFile(t, stagedKnownHosts, "staged-known-hosts", 0644)
			},
		},
		{
			name: "known_hosts path is directory",
			setup: func(t *testing.T, stagedBackups, stagedKnownHosts string) {
				writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)
				if err := os.MkdirAll(stagedKnownHosts, 0755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", stagedKnownHosts, err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtimeDir := t.TempDir()
			dbPath := filepath.Join(runtimeDir, "theia.db")
			deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
			knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
			stagingDir := filepath.Join(runtimeDir, ".restore-staging")
			stagedDB := filepath.Join(stagingDir, "theia.db")
			stagedBackups := filepath.Join(stagingDir, "backups")
			stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")
			markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

			writeRestoreTestFile(t, dbPath, "live-db", 0644)
			writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
			tc.setup(t, stagedBackups, stagedKnownHosts)

			writeRestoreMarker(t, markerPath, restoreMarker{
				StagedDB:         stagedDB,
				StagedBackups:    stagedBackups,
				StagedKnownHosts: stagedKnownHosts,
				DBPath:           dbPath,
				DeviceBackupDir:  deviceBackupDir,
				KnownHostsPath:   knownHostsPath,
				Timestamp:        time.Now().UTC().Format(time.RFC3339),
			})

			coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

			applied, err := coordinator.ApplyPendingRestore()
			if err == nil {
				t.Fatal("ApplyPendingRestore() error = nil, want optional artifact type error")
			}
			if applied {
				t.Fatal("ApplyPendingRestore() applied = true, want false")
			}
			if got := readRestoreTestFile(t, dbPath); got != "live-db" {
				t.Fatalf("db content = %q, want live-db", got)
			}
			if _, err := os.Stat(markerPath); err != nil {
				t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
			}
		})
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsStagedBackupSymlinkBeforeDBActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)
	if err := os.Symlink(filepath.Join(stagedBackups, "router.cfg"), filepath.Join(stagedBackups, "router-link.cfg")); err != nil {
		t.Fatalf("Symlink staged backup: %v", err)
	}

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:        stagedDB,
		StagedBackups:   stagedBackups,
		DBPath:          dbPath,
		DeviceBackupDir: deviceBackupDir,
		KnownHostsPath:  knownHostsPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want staged backup symlink error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreSwapsDBAndOptionalArtifacts(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, dbPath+"-wal", "live-wal", 0644)
	writeRestoreTestFile(t, dbPath+"-shm", "live-shm", 0644)
	writeRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, knownHostsPath, "live-known-hosts", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)
	writeRestoreTestFile(t, stagedKnownHosts, "staged-known-hosts", 0644)

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:         stagedDB,
		StagedBackups:    stagedBackups,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err != nil {
		t.Fatalf("ApplyPendingRestore() error = %v", err)
	}
	if !applied {
		t.Fatal("ApplyPendingRestore() applied = false, want true")
	}

	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content = %q, want staged-db", got)
	}
	assertRestorePathMode(t, dbPath, 0600)
	if got := readRestoreTestFile(t, dbPath+".pre-restore.bak"); got != "live-db" {
		t.Fatalf("pre-restore backup content = %q, want live-db", got)
	}
	if _, err := os.Stat(dbPath + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("wal file should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(dbPath + "-shm"); !os.IsNotExist(err) {
		t.Fatalf("shm file should be removed, stat err = %v", err)
	}
	if got := readRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg")); got != "staged-backup" {
		t.Fatalf("backup content = %q, want staged-backup", got)
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "staged-known-hosts" {
		t.Fatalf("known_hosts content = %q, want staged-known-hosts", got)
	}
	assertRestorePathMode(t, knownHostsPath, 0600)
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("restore marker should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("staging dir should be removed, stat err = %v", err)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreIsIdempotentAfterSuccess(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, knownHostsPath, "live-known-hosts", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)
	writeRestoreTestFile(t, stagedKnownHosts, "staged-known-hosts", 0644)

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:         stagedDB,
		StagedBackups:    stagedBackups,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err != nil {
		t.Fatalf("first ApplyPendingRestore() error = %v", err)
	}
	if !applied {
		t.Fatal("first ApplyPendingRestore() applied = false, want true")
	}

	applied, err = coordinator.ApplyPendingRestore()
	if err != nil {
		t.Fatalf("second ApplyPendingRestore() error = %v", err)
	}
	if applied {
		t.Fatal("second ApplyPendingRestore() applied = true, want false")
	}

	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content after second apply = %q, want staged-db", got)
	}
	if got := readRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg")); got != "staged-backup" {
		t.Fatalf("backup content after second apply = %q, want staged-backup", got)
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "staged-known-hosts" {
		t.Fatalf("known_hosts after second apply = %q, want staged-known-hosts", got)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("restore marker should stay absent after second apply, stat err = %v", err)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreKeepsRetryStateWhenBackupReplacementFails(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, knownHostsPath, "live-known-hosts", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)
	if err := os.Chmod(filepath.Join(stagedBackups, "router.cfg"), 0); err != nil {
		t.Fatalf("chmod staged backup unreadable: %v", err)
	}
	writeRestoreTestFile(t, stagedKnownHosts, "staged-known-hosts", 0644)

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:         stagedDB,
		StagedBackups:    stagedBackups,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want backup replacement error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}

	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content = %q, want staged-db after DB swap", got)
	}
	if got := readRestoreTestFile(t, dbPath+".pre-restore.bak"); got != "live-db" {
		t.Fatalf("pre-restore backup content = %q, want live-db", got)
	}
	if got := readRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg")); got != "live-backup" {
		t.Fatalf("backup content = %q, want preserved live-backup", got)
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "live-known-hosts" {
		t.Fatalf("known_hosts content = %q, want preserved live-known-hosts", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain for retry, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); err != nil {
		t.Fatalf("staging dir should remain for retry, stat err = %v", err)
	}
	if _, err := os.Stat(stagedDB); err != nil {
		t.Fatalf("staged db should remain available for retry, stat err = %v", err)
	}

	if err := os.Chmod(filepath.Join(stagedBackups, "router.cfg"), 0644); err != nil {
		t.Fatalf("restore staged backup readability: %v", err)
	}

	applied, err = coordinator.ApplyPendingRestore()
	if err != nil {
		t.Fatalf("ApplyPendingRestore() retry error = %v", err)
	}
	if !applied {
		t.Fatal("ApplyPendingRestore() retry applied = false, want true")
	}

	if got := readRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg")); got != "staged-backup" {
		t.Fatalf("backup content after retry = %q, want staged-backup", got)
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "staged-known-hosts" {
		t.Fatalf("known_hosts content after retry = %q, want staged-known-hosts", got)
	}
	if got := readRestoreTestFile(t, dbPath+".pre-restore.bak"); got != "live-db" {
		t.Fatalf("pre-restore backup content after retry = %q, want original live-db", got)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("restore marker should be removed after retry, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("staging dir should be removed after retry, stat err = %v", err)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreKeepsRetryStateWhenKnownHostsReplacementFails(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, knownHostsPath, "live-known-hosts", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)
	writeRestoreTestFile(t, stagedKnownHosts, "staged-known-hosts", 0644)
	if err := os.Chmod(stagedKnownHosts, 0); err != nil {
		t.Fatalf("chmod staged known_hosts unreadable: %v", err)
	}

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:         stagedDB,
		StagedBackups:    stagedBackups,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want known_hosts replacement error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content = %q, want staged-db after DB swap", got)
	}
	if got := readRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg")); got != "staged-backup" {
		t.Fatalf("backup content = %q, want staged-backup", got)
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "live-known-hosts" {
		t.Fatalf("known_hosts content = %q, want preserved live-known-hosts", got)
	}
	if _, err := os.Stat(stagedDB); err != nil {
		t.Fatalf("staged db should remain available for retry, stat err = %v", err)
	}

	if err := os.Chmod(stagedKnownHosts, 0644); err != nil {
		t.Fatalf("restore staged known_hosts readability: %v", err)
	}

	applied, err = coordinator.ApplyPendingRestore()
	if err != nil {
		t.Fatalf("ApplyPendingRestore() retry error = %v", err)
	}
	if !applied {
		t.Fatal("ApplyPendingRestore() retry applied = false, want true")
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "staged-known-hosts" {
		t.Fatalf("known_hosts content after retry = %q, want staged-known-hosts", got)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("restore marker should be removed after retry, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("staging dir should be removed after retry, stat err = %v", err)
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRejectsStagedKnownHostsOutsideRuntimeStagingBeforeDBActivation(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackups := filepath.Join(stagingDir, "backups")
	blockedDir := filepath.Join(runtimeDir, ".blocked-known-hosts")
	stagedKnownHosts := filepath.Join(blockedDir, "known_hosts")
	markerPath := filepath.Join(runtimeDir, ".theia-restore-pending")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, knownHostsPath, "live-known-hosts", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackups, "router.cfg"), "staged-backup", 0644)
	if err := os.MkdirAll(blockedDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", blockedDir, err)
	}
	if err := os.Chmod(blockedDir, 0); err != nil {
		t.Fatalf("chmod blocked known_hosts dir: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chmod(blockedDir, 0755)
	})

	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:         stagedDB,
		StagedBackups:    stagedBackups,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	})

	coordinator := NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want staged known_hosts path error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := readRestoreTestFile(t, dbPath); got != "live-db" {
		t.Fatalf("db content = %q, want live-db", got)
	}
	if got := readRestoreTestFile(t, filepath.Join(deviceBackupDir, "router.cfg")); got != "live-backup" {
		t.Fatalf("backup content = %q, want live-backup", got)
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "live-known-hosts" {
		t.Fatalf("known_hosts content = %q, want preserved live-known-hosts", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain after preflight failure, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); err != nil {
		t.Fatalf("runtime staging dir should remain after preflight failure, stat err = %v", err)
	}
}
