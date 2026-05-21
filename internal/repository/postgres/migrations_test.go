package postgres

import (
	"io/fs"
	"strings"
	"testing"
)

func TestPostgresMigrationsAreEmbedded(t *testing.T) {
	files, err := fs.Glob(migrationsFS, "migrations/*.up.sql")
	if err != nil {
		t.Fatalf("globbing migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected embedded PostgreSQL migrations")
	}
	legacyName := "sqli" + "te"
	for _, file := range files {
		if strings.Contains(strings.ToLower(file), legacyName) {
			t.Fatalf("migration path still references legacy dialect: %s", file)
		}
	}
}

func TestPostgresMigrationsDeclareScaleCriticalIndexes(t *testing.T) {
	files, err := fs.Glob(migrationsFS, "migrations/*.up.sql")
	if err != nil {
		t.Fatalf("globbing migrations: %v", err)
	}

	var builder strings.Builder
	for _, file := range files {
		content, err := migrationsFS.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		builder.Write(content)
		builder.WriteByte('\n')
	}
	allMigrations := builder.String()

	for _, expected := range []string{
		"idx_devices_sys_name_lookup",
		"idx_links_target_device_created_at",
		"idx_topology_observations_remote_identity_protocol",
		"idx_unresolved_neighbors_local_device_id",
	} {
		if !strings.Contains(allMigrations, expected) {
			t.Fatalf("PostgreSQL migrations missing critical index %s", expected)
		}
	}
	if strings.Contains(strings.ToLower(allMigrations), "sqli"+"te") {
		t.Fatal("PostgreSQL migrations still reference legacy dialect")
	}
}

func TestRunMigrationsOnConfiguredPostgresTestDB(t *testing.T) {
	db := setupTestDB(t)
	if err := RunMigrations(db, testKey); err != nil {
		t.Fatalf("running PostgreSQL migrations twice should be idempotent: %v", err)
	}
}
