package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/instance"
)

func TestProtectedInstanceBackupRecoversOriginalSecretsAndRejectsTruncation(t *testing.T) {
	state, err := instance.Generate(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := age.GenerateX25519Identity()
	state.RecoveryRecipient = identity.Recipient().String()
	stateJSON, _ := json.Marshal(state)
	dir := t.TempDir()
	dump := filepath.Join(dir, "database.dump")
	if err := os.WriteFile(dump, []byte("test-database-content"), 0600); err != nil {
		t.Fatal(err)
	}
	hash, _ := computeFileHashContext(context.Background(), dump)
	keys, _ := state.Keyring()
	manifest := backupManifest{Version: 2, DBEntryName: postgresArchiveDBEntry, DBSHA256: hash, Encryption: &backupManifestEncryption{ActiveKeyID: keys.ActiveKeyID(), RequiredKeyIDs: keys.KeyIDs()}}
	manifestJSON, _ := json.Marshal(manifest)
	archive := filepath.Join(dir, "backup.age")
	svc := &InstanceBackupService{operations: newInstanceBackupOperationTracker(), backupLimits: DefaultBackupArchiveLimits}
	_, temporary, err := svc.createArchiveWithProtection(context.Background(), archive, databaseBackupArtifact{tempPath: dump, archiveEntryName: postgresArchiveDBEntry}, nil, nil, manifestJSON, &manifest, uuid.New(), stateJSON)
	if err != nil {
		t.Fatal(err)
	}
	for _, opener := range []age.Identity{identity, temporary} {
		recovered, err := ExtractProtectedInstanceBackup(context.Background(), archive, t.TempDir(), DefaultRestoreArchiveLimits, opener)
		if err != nil {
			t.Fatal(err)
		}
		if recovered.CredentialKeys[state.ActiveKeyID].Secret != state.CredentialKeys[state.ActiveKeyID].Secret {
			t.Fatal("credential encryption key lost")
		}
	}
	info, _ := os.Stat(archive)
	if err := os.Truncate(archive, info.Size()-1); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractProtectedInstanceBackup(context.Background(), archive, t.TempDir(), DefaultRestoreArchiveLimits, identity); err == nil {
		t.Fatal("truncated encrypted backup accepted")
	}
}

func TestManagedBackupNeverSkipsMissingDeviceFiles(t *testing.T) {
	state, _ := instance.Generate(time.Now())
	identity, _ := age.GenerateX25519Identity()
	state.RecoveryRecipient = identity.Recipient().String()
	stateJSON, _ := json.Marshal(state)
	dir := t.TempDir()
	dump := filepath.Join(dir, "database.dump")
	if err := os.WriteFile(dump, []byte("dump"), 0600); err != nil {
		t.Fatal(err)
	}
	svc := &InstanceBackupService{operations: newInstanceBackupOperationTracker(), backupLimits: DefaultBackupArchiveLimits}
	_, _, err := svc.createArchiveWithProtection(context.Background(), filepath.Join(dir, "backup.age"), databaseBackupArtifact{tempPath: dump, archiveEntryName: postgresArchiveDBEntry}, []archiveSourceFile{{archiveName: "backups/missing", diskPath: filepath.Join(dir, "missing")}}, nil, []byte("{}"), &backupManifest{}, uuid.New(), stateJSON)
	if err == nil {
		t.Fatal("incomplete managed instance archive accepted")
	}
}
