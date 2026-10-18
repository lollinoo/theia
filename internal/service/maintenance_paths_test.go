package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/instance"
)

func TestMaintenanceRejectsControlAndRecoveryStorageInsideRestoredPaths(t *testing.T) {
	root := t.TempDir()
	m := Maintenance{StatePath: filepath.Join(root, "control", "state.json"), DataDir: filepath.Join(root, "data"), DeviceBackupDir: filepath.Join(root, "data", "backups"), BackupDir: filepath.Join(root, "data", "archives")}
	if err := m.validatePaths(); err != nil {
		t.Fatal(err)
	}
	m.StatePath = filepath.Join(root, "data", "secrets.json")
	if err := m.validatePaths(); err == nil {
		t.Fatal("instance secrets could be replaced by restore")
	}
	m.StatePath = filepath.Join(root, "control", "state.json")
	m.BackupDir = filepath.Join(m.DeviceBackupDir, "archives")
	if err := m.validatePaths(); err == nil {
		t.Fatal("restore could delete retained recovery archives")
	}
}

func TestMaintenanceRejectsEmptyStatePath(t *testing.T) {
	root := t.TempDir()
	m := Maintenance{
		StatePath:       "",
		DataDir:         filepath.Join(root, "data"),
		DeviceBackupDir: filepath.Join(root, "data", "backups"),
		BackupDir:       filepath.Join(root, "data", "archives"),
	}
	if err := m.validatePaths(); err == nil {
		t.Fatal("validatePaths() error = nil, want empty StatePath rejected")
	}
	m.StatePath = "   "
	if err := m.validatePaths(); err == nil {
		t.Fatal("validatePaths() error = nil, want whitespace StatePath rejected")
	}
}

func TestMaintenanceResumeValidatesPaths(t *testing.T) {
	root := t.TempDir()
	m := Maintenance{
		StatePath:       "",
		DataDir:         filepath.Join(root, "data"),
		DeviceBackupDir: filepath.Join(root, "data", "backups"),
		BackupDir:       filepath.Join(root, "data", "archives"),
	}
	if err := m.Resume(context.Background()); err == nil {
		t.Fatal("Resume() error = nil, want validatePaths failure")
	}
}

func TestMaintenanceResumeInterruptedPreparingPhase(t *testing.T) {
	root := t.TempDir()
	store := instance.Store{Path: filepath.Join(root, "control", "state.json")}
	state, err := instance.Generate(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(state); err != nil {
		t.Fatal(err)
	}
	m := &Maintenance{
		StatePath:       store.Path,
		DataDir:         filepath.Join(root, "data"),
		DeviceBackupDir: filepath.Join(root, "data", "backups"),
		BackupDir:       filepath.Join(root, "data", "archives"),
		ReleaseTag:      "v1.0.0",
	}
	op := &MaintenanceOperation{
		ID:                 uuid.NewString(),
		Action:             "migrate",
		Phase:              "preparing",
		Attempt:            1,
		CreatedAt:          time.Now().UTC(),
		OriginalReleaseTag: "v0.9.0",
	}
	if err := m.persist(op, op.Phase); err != nil {
		t.Fatal(err)
	}

	if err := m.Resume(context.Background()); err != nil {
		t.Fatalf("Resume() failed: %v", err)
	}

	after, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if after.Phase != "rolled_back" {
		t.Fatalf("after.Phase = %s, want rolled_back", after.Phase)
	}
}

func TestMaintenanceVerifyCurrentStateValidations(t *testing.T) {
	m := &Maintenance{}
	if err := m.verifyCurrentState(context.Background(), nil); err == nil {
		t.Fatal("verifyCurrentState(nil) error = nil, want error for nil state")
	}

	state := &instance.State{}
	if err := m.verifyCurrentState(context.Background(), state); err == nil {
		t.Fatal("verifyCurrentState() error = nil, want error for unconfigured DB")
	}
}

func TestAbortPostgresUpgradePreflightDoesNotBurnAttempts(t *testing.T) {
	root := t.TempDir()
	store := instance.Store{Path: filepath.Join(root, "control", "state.json")}
	state, err := instance.Generate(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(state); err != nil {
		t.Fatal(err)
	}
	m := &Maintenance{
		StatePath:       store.Path,
		DataDir:         filepath.Join(root, "data"),
		DeviceBackupDir: filepath.Join(root, "data", "backups"),
		BackupDir:       filepath.Join(root, "data", "archives"),
		DBDSN:           "",
	}
	op := &MaintenanceOperation{
		ID:                  uuid.NewString(),
		Action:              "postgres_major",
		Phase:               "awaiting_database_cutover",
		Attempt:             1,
		CreatedAt:           time.Now().UTC(),
		SourcePostgresMajor: 17,
		TargetPostgresMajor: 18,
	}
	if err := m.persist(op, op.Phase); err != nil {
		t.Fatal(err)
	}

	err = m.AbortPostgresUpgrade(context.Background())
	if err == nil {
		t.Fatal("AbortPostgresUpgrade() error = nil, want preflight error for unconfigured DB")
	}

	after, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if after.Attempt != 1 {
		t.Fatalf("after.Attempt = %d, want 1 (preflight error should not burn recovery attempts)", after.Attempt)
	}
	if after.Phase != "awaiting_database_cutover" {
		t.Fatalf("after.Phase = %s, want awaiting_database_cutover (should not set operator_required on preflight error)", after.Phase)
	}
}

