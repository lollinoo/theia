package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// SupportedSchemaVersion is the newest migration embedded in this release.
func SupportedSchemaVersion() int {
	entries, _ := migrationsFS.ReadDir("migrations")
	latest := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		version, err := strconv.Atoi(strings.SplitN(entry.Name(), "_", 2)[0])
		if err == nil && version > latest {
			latest = version
		}
	}
	return latest
}

// RequireCurrentSchema is read-only. Managed HTTP startup never applies migrations.
func RequireCurrentSchema(ctx context.Context, db *sql.DB) error {
	var version int
	var dirty bool
	if err := db.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		return fmt.Errorf("database requires the maintenance migrate command before HTTP startup")
	}
	if dirty || version != SupportedSchemaVersion() {
		return fmt.Errorf("database schema is not ready for this release; run maintenance migrate with application writes stopped")
	}
	return nil
}
