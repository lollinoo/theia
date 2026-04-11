package sqlite

import (
	"database/sql"
	"testing"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openSharedTestDB(t.Name())
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	return db
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return setupTestDB(t)
}
