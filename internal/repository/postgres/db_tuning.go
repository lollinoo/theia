package postgres

import (
	"database/sql"
	"fmt"
	"runtime"
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
	maxConns := runtime.GOMAXPROCS(0) * 4
	switch {
	case maxConns < 8:
		maxConns = 8
	case maxConns > 48:
		maxConns = 48
	}

	idleConns := maxConns / 2
	if idleConns < 4 {
		idleConns = 4
	}

	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(idleConns)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(dbConnMaxIdleTime)
}

func withWriteRetry(fn func() error) error {
	return fn()
}
