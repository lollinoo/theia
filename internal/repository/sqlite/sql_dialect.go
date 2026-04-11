package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
)

type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

var dbDialects sync.Map

func NormalizeDialect(value string) (Dialect, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(DialectSQLite):
		return DialectSQLite, nil
	case "postgres", "postgresql":
		return DialectPostgres, nil
	default:
		return "", fmt.Errorf("unsupported database driver %q", value)
	}
}

func rememberDBDialect(db *sql.DB, dialect Dialect) {
	if db != nil {
		dbDialects.Store(db, dialect)
	}
}

func detectDialectFromDB(db *sql.DB) Dialect {
	if db == nil {
		return DialectSQLite
	}
	if value, ok := dbDialects.Load(db); ok {
		if dialect, ok := value.(Dialect); ok {
			return dialect
		}
	}

	driverType := fmt.Sprintf("%T", db.Driver())
	switch {
	case strings.Contains(driverType, "sqlite3"):
		return DialectSQLite
	case strings.Contains(strings.ToLower(driverType), "pgx"),
		strings.Contains(strings.ToLower(driverType), "postgres"):
		return DialectPostgres
	default:
		return DialectSQLite
	}
}
