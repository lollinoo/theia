package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var testKey = []byte("test-encryption-key-32-bytes!!!!")

func openSharedTestDB(name string) (*sql.DB, error) {
	sanitized := strings.NewReplacer("/", "_", " ", "_", "=", "_", ":", "_").Replace(name)
	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared&_foreign_keys=on&_busy_timeout=%d&_txlock=immediate",
		sanitized,
		sqliteBusyTimeoutMS,
	)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	ConfigureSQLiteDB(db)
	return db, nil
}
