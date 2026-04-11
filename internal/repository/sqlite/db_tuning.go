package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	sqlite3driver "github.com/mattn/go-sqlite3"
)

const (
	sqliteBusyTimeoutMS      = 5000
	sqliteWriteRetryAttempts = 3
	sqliteWriteRetryDelay    = 50 * time.Millisecond
	dbConnMaxIdleTime        = 5 * time.Minute
)

// PrimaryDSN returns the default SQLite DSN used by the main application.
// WAL + NORMAL keeps reads concurrent while reducing fsync pressure, and
// immediate transactions fail or wait at BEGIN rather than halfway through a write.
func PrimaryDSN(path string) string {
	return fmt.Sprintf(
		"%s?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=%d&_foreign_keys=on&_txlock=immediate",
		path,
		sqliteBusyTimeoutMS,
	)
}

func OpenPrimaryDB(driver, path, dsn string) (*sql.DB, Dialect, error) {
	dialect, err := NormalizeDialect(driver)
	if err != nil {
		return nil, "", err
	}

	var db *sql.DB
	switch dialect {
	case DialectSQLite:
		db, err = sql.Open("sqlite3", PrimaryDSN(path))
	case DialectPostgres:
		if strings.TrimSpace(dsn) == "" {
			return nil, "", fmt.Errorf("db_dsn is required when db_driver=postgres")
		}
		db, err = sql.Open("pgx", dsn)
	default:
		return nil, "", fmt.Errorf("unsupported database driver %q", dialect)
	}
	if err != nil {
		return nil, "", err
	}

	rememberDBDialect(db, dialect)
	return db, dialect, nil
}

// ConfigureSQLiteDB bounds the connection pool so large probe waves do not fan
// out into an unbounded number of competing SQLite writer connections.
func ConfigureSQLiteDB(db *sql.DB) {
	ConfigureDB(db)
}

func ConfigureDB(db *sql.DB) {
	maxConns := runtime.GOMAXPROCS(0) * 2
	idleConns := maxConns

	switch detectDialectFromDB(db) {
	case DialectPostgres:
		maxConns = runtime.GOMAXPROCS(0) * 4
		switch {
		case maxConns < 8:
			maxConns = 8
		case maxConns > 48:
			maxConns = 48
		}
		idleConns = maxConns / 2
		if idleConns < 4 {
			idleConns = 4
		}
	default:
		switch {
		case maxConns < 4:
			maxConns = 4
		case maxConns > 16:
			maxConns = 16
		}
		idleConns = maxConns
	}

	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(idleConns)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(dbConnMaxIdleTime)
}

func withSQLiteBusyRetry(fn func() error) error {
	delay := sqliteWriteRetryDelay
	var err error

	for attempt := 0; attempt < sqliteWriteRetryAttempts; attempt++ {
		err = fn()
		if err == nil || !isSQLiteBusyError(err) {
			return err
		}
		if attempt == sqliteWriteRetryAttempts-1 {
			return err
		}

		time.Sleep(delay)
		delay *= 2
	}

	return err
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}

	var sqliteErr sqlite3driver.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code == sqlite3driver.ErrBusy || sqliteErr.Code == sqlite3driver.ErrLocked
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "database schema is locked")
}
