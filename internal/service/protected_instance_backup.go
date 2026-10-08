package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"filippo.io/age"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/repository/postgres"
)

// ExtractProtectedInstanceBackup authenticates the entire age stream before its
// contents are usable. Destination must be a fresh private staging directory.
func ExtractProtectedInstanceBackup(ctx context.Context, archivePath, destination string, limits RestoreArchiveLimits, identities ...age.Identity) (*instance.State, error) {
	if err := validateRestoreArchiveFile(archivePath, limits); err != nil {
		return nil, err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader, err := age.Decrypt(f, identities...)
	if err != nil {
		return nil, fmt.Errorf("open encrypted instance backup: %w", err)
	}
	if err := extractArchiveReader(ctx, reader, destination, limits); err != nil {
		return nil, err
	}
	manifest, err := readRestoreManifest(destination)
	if err != nil {
		return nil, err
	}
	if manifest.Version != 2 {
		return nil, fmt.Errorf("encrypted instance backup requires manifest version 2")
	}
	state, err := (instance.Store{Path: filepath.Join(destination, "instance-secrets.json")}).Load()
	if err != nil {
		return nil, fmt.Errorf("backup lacks valid protected instance secrets: %w", err)
	}
	keys, err := state.Keyring()
	if err != nil {
		return nil, err
	}
	if err := validateRestoreManifestEncryptionKey(manifest, keys); err != nil {
		return nil, err
	}
	entry, err := manifestDatabaseEntryName(manifest)
	if err != nil {
		return nil, err
	}
	hash, err := computeFileHashContext(ctx, filepath.Join(destination, entry))
	if err != nil {
		return nil, err
	}
	if hash != manifest.DBSHA256 {
		return nil, fmt.Errorf("database checksum mismatch")
	}
	count := 0
	if err := filepath.WalkDir(filepath.Join(destination, "backups"), func(path string, item os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !item.IsDir() {
			count++
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if count != manifest.BackupFileCount {
		return nil, fmt.Errorf("device backup file count does not match manifest")
	}
	return state, nil
}

func (s *InstanceBackupService) verifyProtectedBackup(ctx context.Context, path string, identity age.Identity) error {
	staging, err := os.MkdirTemp("", "theia-backup-check-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	limits := RestoreArchiveLimits{MaxCompressedBytes: 1 << 40, MaxTotalBytes: s.BackupArchiveLimits().MaxTotalBytes, MaxEntryBytes: s.BackupArchiveLimits().MaxEntryBytes, MaxFileEntries: s.BackupArchiveLimits().MaxFileEntries}
	state, err := ExtractProtectedInstanceBackup(ctx, path, staging, limits, identity)
	if err != nil {
		return err
	}
	keys, err := state.Keyring()
	if err != nil {
		return err
	}
	return VerifyIsolatedPostgresDump(ctx, filepath.Join(staging, postgresArchiveDBEntry), keys)
}

// VerifyIsolatedPostgresDump actually restores the dump into a fresh local cluster,
// runs schema/data migrations, and decrypts every stored sensitive credential.
// It never connects to the live database or requires CREATEDB on an external server.
func VerifyIsolatedPostgresDump(ctx context.Context, dumpPath string, keys *crypto.Keyring) error {
	return withIsolatedPostgres(ctx, func(dsn string) error {
		if err := runPostgresRestore(ctx, dsn, dumpPath); err != nil {
			return err
		}
		db, err := postgres.OpenPrimaryDB(dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := postgres.RunMigrations(db, keys); err != nil {
			return err
		}
		return VerifyStoredCredentials(ctx, db, keys, true)
	})
}
