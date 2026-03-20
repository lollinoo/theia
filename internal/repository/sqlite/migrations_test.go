package sqlite

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// openTestDB opens a fresh in-memory SQLite database without running migrations.
// Tests can control when (and if) RunMigrations is called.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ---------------------------------------------------------------------------
// TestMigrations (DEBT-05)
// ---------------------------------------------------------------------------
// Verifies that golang-migrate versioned migrations create the schema_migrations
// table with a version > 0 and dirty = false. Before the fix (inline string
// migrations with no version tracking), schema_migrations did not exist.
func TestMigrations(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// schema_migrations must exist and have a valid version
	var version int
	var dirty bool
	err := db.QueryRow(`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty)
	if err != nil {
		t.Fatalf("DEBT-05: schema_migrations table does not exist or is empty -- "+
			"migration system has no version tracking: %v", err)
	}

	if version <= 0 {
		t.Fatalf("DEBT-05: schema_migrations version is %d -- expected > 0", version)
	}

	if dirty {
		t.Fatalf("DEBT-05: schema_migrations dirty flag is true -- expected false")
	}

	t.Logf("schema_migrations: version=%d dirty=%v", version, dirty)
}

// ---------------------------------------------------------------------------
// TestLegacyTableDrop (DEBT-06)
// ---------------------------------------------------------------------------
// Verifies that legacy tables (config_backups, ssh_credentials) are dropped
// after running migrations. Before the fix (no migration 000006), these
// tables remained in the database.
func TestLegacyTableDrop(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// Check that legacy tables are dropped
	legacyTables := []string{"config_backups", "ssh_credentials"}
	for _, tableName := range legacyTables {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			tableName,
		).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query sqlite_master for %s: %v", tableName, err)
		}
		if count != 0 {
			t.Fatalf("DEBT-06: legacy table %s still exists after migrations -- "+
				"expected it to be dropped by migration 000006", tableName)
		}
	}
}
