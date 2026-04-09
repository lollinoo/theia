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
// TestMigration000012_DefaultRole (CRED-04)
// ---------------------------------------------------------------------------
// Verifies the behavior introduced by migration 000012:
//   (a) The credential_profiles table has a role column that defaults to 'Admin'
//       — inserting a row without specifying role yields role='Admin'.
//   (b) The device_credential_profiles join table exists with the expected columns.
//
// Because RunMigrations always runs all migrations to the latest version,
// we verify these invariants on the final migrated schema rather than
// replaying a partial migration.
func TestMigration000012_DefaultRole(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// (a) Insert a credential_profiles row without specifying role and verify
	//     that the DEFAULT 'Admin' constraint fills it in.
	profileID := "00000000-0000-0000-0000-000000000001"
	_, err := db.Exec(
		`INSERT INTO credential_profiles (id, name, description, username, port, auth_method, encrypted_secret, created_at, updated_at)
		 VALUES (?, 'role-default-test', '', 'admin', 22, 'password', '', datetime('now'), datetime('now'))`,
		profileID,
	)
	if err != nil {
		t.Fatalf("CRED-04: inserting credential_profiles row without role: %v", err)
	}

	var role string
	err = db.QueryRow(`SELECT role FROM credential_profiles WHERE id = ?`, profileID).Scan(&role)
	if err != nil {
		t.Fatalf("CRED-04: querying role from credential_profiles: %v", err)
	}
	if role != "Admin" {
		t.Errorf("CRED-04: expected role='Admin' (DEFAULT), got %q", role)
	}

	// (b) Verify device_credential_profiles join table exists with expected columns
	//     by performing a SELECT that references all required columns.
	_, err = db.Exec(
		`SELECT device_id, profile_id, is_winbox, created_at FROM device_credential_profiles LIMIT 0`,
	)
	if err != nil {
		t.Fatalf("CRED-04: device_credential_profiles table missing or has wrong columns: %v", err)
	}
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
