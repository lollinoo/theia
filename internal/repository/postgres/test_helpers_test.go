package postgres

// This file exercises test helpers behavior so refactors preserve the documented contract.

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
)

var testKey = []byte("test-encryption-key-32-bytes!!!!")
var testKeyring = mustKeyringForTests()

func mustKeyringForTests() *crypto.Keyring {
	keyring, err := crypto.NewKeyring("test-key", map[string]string{
		"test-key": "test-encryption-key-32-bytes!!!!",
	})
	if err != nil {
		panic(err)
	}
	return keyring
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("THEIA_TEST_DB_DSN"))
	if dsn == "" {
		t.Skip("THEIA_TEST_DB_DSN is required for PostgreSQL repository tests")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ConfigureDB(db)

	// Clear application data before migrations restore required runtime defaults.
	resetTestDB(t, db)
	if err := RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	return db
}

func TestSetupTestDBResetsDataAndRestoresDefaults(t *testing.T) {
	db := setupTestDB(t)
	if _, err := NewCanvasMapRepo(db).GetDefault(); err != nil {
		t.Fatalf("default map missing after setup: %v", err)
	}
	settings := NewSettingsRepo(db)
	if err := settings.Set("test.fixture.custom", "dirty"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(domain.SettingPollingInterval, "999"); err != nil {
		t.Fatal(err)
	}

	reset := setupTestDB(t)
	if count := importTestCount(t, reset, "SELECT COUNT(*) FROM settings WHERE key=$1", "test.fixture.custom"); count != 0 {
		t.Fatal("previous fixture data survived setup")
	}
	if _, err := NewCanvasMapRepo(reset).GetDefault(); err != nil {
		t.Fatalf("default map missing after reset: %v", err)
	}
	if value, err := NewSettingsRepo(reset).Get(domain.SettingPollingInterval); err != nil || value != domain.DefaultSettings()[domain.SettingPollingInterval] {
		t.Fatalf("default setting not restored: value=%q error=%v", value, err)
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return setupTestDB(t)
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return setupTestDB(t)
}

func resetTestDB(t *testing.T, db *sql.DB) {
	t.Helper()

	rows, err := db.Query(`
		SELECT c.relname
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'p')
			AND n.nspname = current_schema()
			AND c.relname <> 'schema_migrations'
		ORDER BY c.relname`)
	if err != nil {
		t.Fatalf("listing test tables: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Fatalf("scanning test table: %v", err)
		}
		tables = append(tables, quotePostgresIdentifier(tableName))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating test tables: %v", err)
	}
	if len(tables) == 0 {
		return
	}

	query := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", "))
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("truncating test tables: %v", err)
	}
}

func quotePostgresIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
