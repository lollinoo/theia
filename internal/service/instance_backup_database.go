package service

// This file defines instance backup database backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"context"
	"fmt"
	"path/filepath"
)

type databaseBackupArtifact struct {
	tempPath         string
	archiveEntryName string
	migrationVersion int
}

// backupDatabase creates a logical dump of the live PostgreSQL database.
func (s *InstanceBackupService) backupDatabase(ctx context.Context, backupSubDir string) (databaseBackupArtifact, error) {
	return s.backupPostgresDatabase(ctx, filepath.Join(backupSubDir, postgresArchiveDBEntry+".tmp"))
}

// backupPostgresDatabase dumps PostgreSQL and records the migration version for the manifest.
func (s *InstanceBackupService) backupPostgresDatabase(ctx context.Context, destPath string) (databaseBackupArtifact, error) {
	if err := runPostgresDump(ctx, s.dbDSN, destPath); err != nil {
		return databaseBackupArtifact{}, err
	}

	migrationVersion, err := s.readCurrentMigrationVersion(ctx)
	if err != nil {
		return databaseBackupArtifact{}, fmt.Errorf("reading migration version: %w", err)
	}

	return databaseBackupArtifact{
		tempPath:         destPath,
		archiveEntryName: postgresArchiveDBEntry,
		migrationVersion: migrationVersion,
	}, nil
}

// readCurrentMigrationVersion reads the live schema migration version from PostgreSQL.
func (s *InstanceBackupService) readCurrentMigrationVersion(ctx context.Context) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database connection unavailable")
	}

	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT version FROM schema_migrations").Scan(&version); err != nil {
		return 0, fmt.Errorf("querying migration version: %w", err)
	}
	return version, nil
}

// validatePostgresDump delegates dump validation to pg_restore inspection.
func (s *InstanceBackupService) validatePostgresDump(ctx context.Context, dumpPath string) error {
	return validatePostgresDumpArchive(ctx, dumpPath)
}
