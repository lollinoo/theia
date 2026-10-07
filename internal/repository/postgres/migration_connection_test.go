package postgres

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"
)

func migrationTestPool(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("THEIA_TEST_DB_DSN"))
	if dsn == "" {
		t.Skip("THEIA_TEST_DB_DSN is required for PostgreSQL repository tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func assertMigrationPoolAvailable(t *testing.T, db *sql.DB) {
	t.Helper()
	if got := db.Stats().InUse; got != 0 {
		t.Fatalf("migration retained %d connections", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("application pool unusable after migration: %v", err)
	}
}

func TestMigrationConnectionReleasedOnSuccessAndNoChange(t *testing.T) {
	db := migrationTestPool(t)
	// Check SQL migrations first: a leak here would starve Go-level migrations
	// when the application pool has only one connection.
	if err := runPostgresMigrations(db); err != nil {
		t.Fatal(err)
	}
	assertMigrationPoolAvailable(t, db)
	for i := 0; i < 3; i++ {
		if err := RunMigrations(db); err != nil {
			t.Fatal(err)
		}
		assertMigrationPoolAvailable(t, db)
	}
}

func TestMigrationConnectionReleasedOnDriverInitializationError(t *testing.T) {
	db := migrationTestPool(t)
	if _, err := db.Exec(`SET search_path TO theia_missing_migration_schema`); err != nil {
		t.Fatal(err)
	}
	if err := runPostgresMigrations(db); err == nil {
		t.Fatal("expected initialization failure without a current schema")
	}
	assertMigrationPoolAvailable(t, db)
	if _, err := db.Exec(`RESET search_path`); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationConnectionReleasedOnDirtyVersion(t *testing.T) {
	db := migrationTestPool(t)
	if err := runPostgresMigrations(db); err != nil {
		t.Fatal(err)
	}
	assertMigrationPoolAvailable(t, db)
	if _, err := db.Exec(`UPDATE schema_migrations SET dirty = TRUE`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`UPDATE schema_migrations SET dirty = FALSE`); err != nil {
			t.Errorf("restoring migration version: %v", err)
		}
	})
	if err := runPostgresMigrations(db); err == nil {
		t.Fatal("expected dirty migration failure")
	}
	assertMigrationPoolAvailable(t, db)
}
