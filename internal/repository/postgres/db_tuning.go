package postgres

// This file defines db tuning persistence behavior, ordering guarantees, and not-found conventions.

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const dbConnMaxIdleTime = 5 * time.Minute

// OpenPrimaryDB opens the PostgreSQL database used by the main application.
func OpenPrimaryDB(dsn string) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("db_dsn is required")
	}
	return sql.Open("pgx", dsn)
}

// ConfigureDB bounds the PostgreSQL connection pool for mixed API and collector load.
func ConfigureDB(db *sql.DB) {
	_ = ConfigureDBWithLimits(db, 16, 8)
}

// ConfigureDBWithLimits applies a validated deployment connection budget.
// MaxIdleConns may be zero to release every connection when it becomes idle.
func ConfigureDBWithLimits(db *sql.DB, maxOpenConns, maxIdleConns int) error {
	if maxOpenConns <= 0 || maxIdleConns < 0 || maxIdleConns > maxOpenConns {
		return fmt.Errorf("invalid database pool limits: open=%d idle=%d", maxOpenConns, maxIdleConns)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(dbConnMaxIdleTime)
	return nil
}
