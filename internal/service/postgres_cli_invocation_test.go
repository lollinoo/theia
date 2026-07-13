package service

// This file exercises postgres cli invocation behavior so refactors preserve the documented contract.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPostgresDumpUsesSafeConnInfoAndPasswordEnv(t *testing.T) {
	const dsn = "postgres://theia:strong-password@localhost:5432/theia?sslmode=disable"
	destPath := filepath.Join(t.TempDir(), "theia.dump")
	pgDumpExecuted := false

	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		if name != "pg_dump" {
			return nil, fmt.Errorf("unexpected command %s", name)
		}
		if commandArgsEqual(args, "--version") {
			return []byte("pg_dump (PostgreSQL) 18.0\n"), nil
		}
		if commandEnvValue(env, "PGPASSWORD") != "strong-password" {
			t.Fatal("pg_dump PGPASSWORD env does not match DSN password")
		}
		if got := commandFlagValue(args, "--file"); got != destPath {
			t.Fatalf("pg_dump --file = %q, want %q", got, destPath)
		}
		connInfo := commandFlagValue(args, "--dbname")
		if strings.Contains(connInfo, "password") || strings.Contains(connInfo, "strong-password") {
			t.Fatalf("pg_dump conninfo leaked password material: %q", connInfo)
		}
		if strings.Contains(connInfo, "postgres://") {
			t.Fatalf("pg_dump conninfo should not use raw URL dsn: %q", connInfo)
		}
		if !commandArgExists(args, "--format=custom") {
			t.Fatalf("pg_dump args = %v, want custom format", args)
		}
		pgDumpExecuted = true
		return nil, nil
	})

	if err := runPostgresDump(context.Background(), dsn, destPath); err != nil {
		t.Fatalf("runPostgresDump() error = %v", err)
	}
	if !pgDumpExecuted {
		t.Fatal("pg_dump was not executed")
	}
}

func TestValidatePostgresDumpArchiveUsesPgRestoreList(t *testing.T) {
	dumpPath := filepath.Join(t.TempDir(), "database.dump")
	pgRestoreListed := false

	stubExternalCommands(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "pg_restore" {
			return nil, fmt.Errorf("unexpected command %s", name)
		}
		if commandArgsEqual(args, "--version") {
			return []byte("pg_restore (PostgreSQL) 18.0\n"), nil
		}
		if !commandArgsEqual(args, "--list", dumpPath) {
			return nil, fmt.Errorf("unexpected pg_restore args: %v", args)
		}
		pgRestoreListed = true
		return []byte("archive listing"), nil
	})

	if err := validatePostgresDumpArchive(context.Background(), dumpPath); err != nil {
		t.Fatalf("validatePostgresDumpArchive() error = %v", err)
	}
	if !pgRestoreListed {
		t.Fatal("pg_restore --list was not executed")
	}
}

func TestRunPostgresRestoreCleansSchemaBeforeRestore(t *testing.T) {
	const dsn = "postgres://theia:strong-password@localhost:5432/theia?sslmode=disable"
	stagedDump := filepath.Join(t.TempDir(), "database.dump")
	if err := os.WriteFile(stagedDump, []byte("dump"), 0o600); err != nil {
		t.Fatalf("writing staged dump: %v", err)
	}

	originalTerminate := terminatePostgresConnections
	terminateCalled := false
	terminatePostgresConnections = func(ctx context.Context, gotDSN string) error {
		terminateCalled = true
		if gotDSN != dsn {
			t.Fatalf("terminatePostgresConnections dsn = %q, want %q", gotDSN, dsn)
		}
		return nil
	}
	t.Cleanup(func() { terminatePostgresConnections = originalTerminate })

	cleanSchemaExecuted := false
	restoreExecuted := false
	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_restore":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_restore (PostgreSQL) 18.0\n"), nil
			}
			if !cleanSchemaExecuted {
				t.Fatal("pg_restore executed before schema cleanup")
			}
			if got := args[len(args)-1]; got != stagedDump {
				t.Fatalf("pg_restore target = %q, want %q", got, stagedDump)
			}
			restoreExecuted = true
			return nil, nil
		case "psql":
			if commandArgsEqual(args, "--version") {
				return []byte("psql (PostgreSQL) 18.0\n"), nil
			}
			if commandEnvValue(env, "PGPASSWORD") != "strong-password" {
				t.Fatal("psql PGPASSWORD env does not match DSN password")
			}
			connInfo := commandFlagValue(args, "--dbname")
			if strings.Contains(connInfo, "password") || strings.Contains(connInfo, "strong-password") {
				t.Fatalf("psql conninfo leaked password material: %q", connInfo)
			}
			command := commandFlagValue(args, "--command")
			if !strings.Contains(command, "DROP SCHEMA IF EXISTS public CASCADE") ||
				!strings.Contains(command, "CREATE SCHEMA public") {
				t.Fatalf("psql schema cleanup command = %q, want drop/create public schema", command)
			}
			cleanSchemaExecuted = true
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command %s", name)
		}
	})

	if err := runPostgresRestore(context.Background(), dsn, stagedDump); err != nil {
		t.Fatalf("runPostgresRestore() error = %v", err)
	}
	if !terminateCalled {
		t.Fatal("terminatePostgresConnections was not called")
	}
	if !cleanSchemaExecuted {
		t.Fatal("schema cleanup was not executed")
	}
	if !restoreExecuted {
		t.Fatal("pg_restore was not executed")
	}
}
