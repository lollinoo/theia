package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/gosnmp/gosnmp"
	_ "github.com/mattn/go-sqlite3"

	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
)

func openVendorConfigTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	return db
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

func TestLoadRegistryFromDB_FallsBackWhenAllRecordsInvalid(t *testing.T) {
	db := openVendorConfigTestDB(t)
	repo := sqlite.NewVendorConfigRepo(db)

	if err := repo.Upsert(&domain.VendorConfigRecord{
		Name:        "default",
		DisplayName: "Generic / Default",
		ConfigJSON:  "{not-json}",
	}); err != nil {
		t.Fatalf("upserting invalid vendor config: %v", err)
	}

	registry, err := loadRegistryFromDB(repo)
	if err != nil {
		t.Fatalf("loadRegistryFromDB() error = %v, want nil", err)
	}
	if registry != nil {
		t.Fatalf("loadRegistryFromDB() registry = %#v, want nil fallback signal", registry)
	}
}

func TestApplyPendingRestore_KeepsLiveArtifactsWhenBackupReplacementFails(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "theia.db")
	liveBackupDir := filepath.Join(tempDir, "backups")
	knownHostsPath := filepath.Join(tempDir, "known_hosts")
	stagingDir := filepath.Join(tempDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackupDir := filepath.Join(stagingDir, "backups")
	stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, filepath.Join(liveBackupDir, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, knownHostsPath, "live-known-hosts", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackupDir, "router.cfg"), "staged-backup", 0644)
	if err := os.Chmod(filepath.Join(stagedBackupDir, "router.cfg"), 0); err != nil {
		t.Fatalf("chmod staged backup unreadable: %v", err)
	}
	writeRestoreTestFile(t, stagedKnownHosts, "staged-known-hosts", 0644)

	markerPath := filepath.Join(tempDir, ".theia-restore-pending")
	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:         stagedDB,
		StagedBackups:    stagedBackupDir,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  liveBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	})

	if applied := applyPendingRestore(dbPath); applied {
		t.Fatal("applyPendingRestore() = true, want false when backup replacement fails")
	}

	if got := readRestoreTestFile(t, filepath.Join(liveBackupDir, "router.cfg")); got != "live-backup" {
		t.Fatalf("live backup content = %q, want preserved live-backup", got)
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "live-known-hosts" {
		t.Fatalf("known_hosts content = %q, want preserved live-known-hosts", got)
	}
	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content = %q, want staged-db after DB swap", got)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain for retry, stat error: %v", err)
	}
	if _, err := os.Stat(stagingDir); err != nil {
		t.Fatalf("staging dir should remain for retry, stat error: %v", err)
	}
}

func TestApplyPendingRestore_CleansUpAfterSuccessfulArtifactSwap(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "theia.db")
	liveBackupDir := filepath.Join(tempDir, "backups")
	knownHostsPath := filepath.Join(tempDir, "known_hosts")
	stagingDir := filepath.Join(tempDir, ".restore-staging")
	stagedDB := filepath.Join(stagingDir, "theia.db")
	stagedBackupDir := filepath.Join(stagingDir, "backups")
	stagedKnownHosts := filepath.Join(stagingDir, "known_hosts")

	writeRestoreTestFile(t, dbPath, "live-db", 0644)
	writeRestoreTestFile(t, filepath.Join(liveBackupDir, "router.cfg"), "live-backup", 0644)
	writeRestoreTestFile(t, knownHostsPath, "live-known-hosts", 0644)
	writeRestoreTestFile(t, stagedDB, "staged-db", 0644)
	writeRestoreTestFile(t, filepath.Join(stagedBackupDir, "router.cfg"), "staged-backup", 0644)
	writeRestoreTestFile(t, stagedKnownHosts, "staged-known-hosts", 0644)

	markerPath := filepath.Join(tempDir, ".theia-restore-pending")
	writeRestoreMarker(t, markerPath, restoreMarker{
		StagedDB:         stagedDB,
		StagedBackups:    stagedBackupDir,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  liveBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	})

	if applied := applyPendingRestore(dbPath); !applied {
		t.Fatal("applyPendingRestore() = false, want true on successful restore")
	}

	if got := readRestoreTestFile(t, dbPath); got != "staged-db" {
		t.Fatalf("db content = %q, want staged-db", got)
	}
	if got := readRestoreTestFile(t, filepath.Join(liveBackupDir, "router.cfg")); got != "staged-backup" {
		t.Fatalf("live backup content = %q, want staged-backup", got)
	}
	if got := readRestoreTestFile(t, knownHostsPath); got != "staged-known-hosts" {
		t.Fatalf("known_hosts content = %q, want staged-known-hosts", got)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("restore marker should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("staging dir should be removed, stat err = %v", err)
	}
}

type stubSettingsRepo struct {
	values map[string]string
}

func (r stubSettingsRepo) Get(key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", errors.New("setting not found")
}

func (r stubSettingsRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r stubSettingsRepo) GetAll() (map[string]string, error) {
	cloned := make(map[string]string, len(r.values))
	for key, value := range r.values {
		cloned[key] = value
	}
	return cloned, nil
}

type fakeCollectorSNMPClient struct{}

func (fakeCollectorSNMPClient) Connect() error { return nil }
func (fakeCollectorSNMPClient) Close() error   { return nil }
func (fakeCollectorSNMPClient) Get([]string) ([]gosnmp.SnmpPDU, error) {
	return nil, nil
}
func (fakeCollectorSNMPClient) BulkWalk(string) ([]gosnmp.SnmpPDU, error) {
	return nil, nil
}

func TestMainSNMPRuntimeHelpersRemainConstructibleAfterPipelineCutover(t *testing.T) {
	t.Run("uses caller timeout and retries when settings are invalid", func(t *testing.T) {
		var (
			gotTimeout time.Duration
			gotRetries int
		)

		original := newCollectorSNMPClient
		newCollectorSNMPClient = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
			gotTimeout = timeout
			gotRetries = retries
			return fakeCollectorSNMPClient{}, nil
		}
		t.Cleanup(func() { newCollectorSNMPClient = original })

		factory := newCollectorSNMPClientFunc(stubSettingsRepo{
			values: map[string]string{
				domain.SettingSNMPTimeout: "bad-timeout",
				domain.SettingSNMPRetries: "bad-retries",
			},
		})
		client, err := factory("10.0.0.1", domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c: &domain.SNMPv2cCredentials{
				Community: "public",
			},
		}, 12*time.Second, 4)
		if err != nil {
			t.Fatalf("factory() error = %v", err)
		}
		if client == nil {
			t.Fatal("factory() returned nil client")
		}
		if gotTimeout != 12*time.Second {
			t.Fatalf("timeout = %v, want caller timeout 12s", gotTimeout)
		}
		if gotRetries != 4 {
			t.Fatalf("retries = %d, want caller retries 4", gotRetries)
		}
	})

	t.Run("defaults when caller inputs are invalid and settings are missing", func(t *testing.T) {
		var (
			gotTimeout time.Duration
			gotRetries int
		)

		original := newCollectorSNMPClient
		newCollectorSNMPClient = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
			gotTimeout = timeout
			gotRetries = retries
			return fakeCollectorSNMPClient{}, nil
		}
		t.Cleanup(func() { newCollectorSNMPClient = original })

		factory := newCollectorSNMPClientFunc(stubSettingsRepo{
			values: map[string]string{
				domain.SettingSNMPTimeout: "-1",
				domain.SettingSNMPRetries: "nope",
			},
		})
		client, err := factory("10.0.0.1", domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c: &domain.SNMPv2cCredentials{
				Community: "public",
			},
		}, 0, -1)
		if err != nil {
			t.Fatalf("factory() error = %v", err)
		}
		if client == nil {
			t.Fatal("factory() returned nil client")
		}
		if gotTimeout != 10*time.Second {
			t.Fatalf("timeout = %v, want 10s fallback", gotTimeout)
		}
		if gotRetries != 2 {
			t.Fatalf("retries = %d, want 2 fallback", gotRetries)
		}
	})

	t.Run("parses settings overrides", func(t *testing.T) {
		var (
			gotTimeout time.Duration
			gotRetries int
		)

		original := newCollectorSNMPClient
		newCollectorSNMPClient = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
			gotTimeout = timeout
			gotRetries = retries
			return fakeCollectorSNMPClient{}, nil
		}
		t.Cleanup(func() { newCollectorSNMPClient = original })

		factory := newCollectorSNMPClientFunc(stubSettingsRepo{
			values: map[string]string{
				domain.SettingSNMPTimeout: "9",
				domain.SettingSNMPRetries: "3",
			},
		})
		client, err := factory("10.0.0.2", domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c: &domain.SNMPv2cCredentials{
				Community: "public",
			},
		}, 12*time.Second, 4)
		if err != nil {
			t.Fatalf("factory() error = %v", err)
		}
		if client == nil {
			t.Fatal("factory() returned nil client")
		}
		if gotTimeout != 9*time.Second {
			t.Fatalf("timeout = %v, want 9s", gotTimeout)
		}
		if gotRetries != 3 {
			t.Fatalf("retries = %d, want 3", gotRetries)
		}
	})
}

type stubDeviceSource struct{}

func (stubDeviceSource) GetDevices() ([]domain.Device, error) {
	return nil, nil
}

func TestWirePollRescheduler_AttachesSchedulerToDeviceService(t *testing.T) {
	deviceService := service.NewDeviceService(nil, nil, nil, nil, nil)
	sched := scheduler.NewScheduler(stubDeviceSource{}, nil)

	wirePollRescheduler(deviceService, sched)

	field := reflect.ValueOf(deviceService).Elem().FieldByName("pollRescheduler")
	if !field.IsValid() {
		t.Fatal("pollRescheduler field missing on DeviceService")
	}
	if field.IsNil() {
		t.Fatal("pollRescheduler field is nil after wirePollRescheduler")
	}

	attached := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	attachedScheduler, ok := attached.(*scheduler.Scheduler)
	if !ok {
		t.Fatalf("pollRescheduler concrete type = %T, want *scheduler.Scheduler", attached)
	}
	if attachedScheduler != sched {
		t.Fatalf("attached scheduler = %p, want %p", attachedScheduler, sched)
	}
}
