package service

import (
	"path/filepath"
	"testing"
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
