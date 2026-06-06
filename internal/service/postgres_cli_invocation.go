package service

// This file defines postgres cli invocation service behavior and domain orchestration rules.

import (
	"context"
	"fmt"
	"strings"
)

// runPostgresDump writes a custom-format PostgreSQL dump using redacted CLI invocation errors.
func runPostgresDump(ctx context.Context, dbDSN string, destPath string) error {
	if strings.TrimSpace(dbDSN) == "" {
		return fmt.Errorf("postgres backup requires db_dsn")
	}
	if err := ensureSupportedPostgresCLITools(ctx, "pg_dump"); err != nil {
		return err
	}
	conn, err := postgresCLIConnInfo(dbDSN)
	if err != nil {
		return fmt.Errorf("build postgres conninfo: %w", err)
	}
	if _, err := runExternalCommandWithEnv(
		ctx,
		conn.env,
		"pg_dump",
		"--format=custom",
		"--no-owner",
		"--no-privileges",
		"--file", destPath,
		"--dbname", conn.connInfo,
	); err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return nil
}

// validatePostgresDumpArchive checks that pg_restore can inspect a generated dump archive.
func validatePostgresDumpArchive(ctx context.Context, dumpPath string) error {
	if err := ensureSupportedPostgresCLITools(ctx, "pg_restore"); err != nil {
		return err
	}
	if _, err := runExternalCommand(ctx, "pg_restore", "--list", dumpPath); err != nil {
		return fmt.Errorf("validating postgres dump: %w", err)
	}
	return nil
}

// runPostgresRestore replaces the public schema from a validated staged dump.
func runPostgresRestore(ctx context.Context, dbDSN string, stagedDB string) error {
	if strings.TrimSpace(dbDSN) == "" {
		return fmt.Errorf("postgres restore requires db_dsn")
	}
	if err := ensureSupportedPostgresCLITools(ctx, "pg_restore", "psql"); err != nil {
		return err
	}
	conn, err := postgresCLIConnInfo(dbDSN)
	if err != nil {
		return fmt.Errorf("build postgres conninfo: %w", err)
	}
	if err := terminatePostgresConnections(ctx, dbDSN); err != nil {
		return err
	}
	if err := validateStagedDBFile(stagedDB); err != nil {
		return err
	}
	if _, err := runExternalCommandWithEnv(
		ctx,
		conn.env,
		"psql",
		"--set", "ON_ERROR_STOP=1",
		"--single-transaction",
		"--dbname", conn.connInfo,
		"--command", "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;",
	); err != nil {
		return fmt.Errorf("clean postgres schema before restore: %w", err)
	}
	if _, err := runExternalCommandWithEnv(
		ctx,
		conn.env,
		"pg_restore",
		"--no-owner",
		"--no-privileges",
		"--single-transaction",
		"--exit-on-error",
		"--dbname", conn.connInfo,
		stagedDB,
	); err != nil {
		return fmt.Errorf("restore postgres database: %w", err)
	}
	return nil
}
