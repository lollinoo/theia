package sqlite

import (
	"database/sql"
	"strconv"
	"strings"
)

type DB struct {
	raw     *sql.DB
	dialect Dialect
}

type Tx struct {
	raw     *sql.Tx
	dialect Dialect
}

func wrapDB(db *sql.DB) *DB {
	return &DB{raw: db, dialect: detectDialectFromDB(db)}
}

func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.raw.Exec(rebindQuery(db.dialect, query), args...)
}

func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.raw.Query(rebindQuery(db.dialect, query), args...)
}

func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.raw.QueryRow(rebindQuery(db.dialect, query), args...)
}

func (db *DB) Begin() (*Tx, error) {
	tx, err := db.raw.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{raw: tx, dialect: db.dialect}, nil
}

func (db *DB) Prepare(query string) (*sql.Stmt, error) {
	return db.raw.Prepare(rebindQuery(db.dialect, query))
}

func (tx *Tx) Exec(query string, args ...interface{}) (sql.Result, error) {
	return tx.raw.Exec(rebindQuery(tx.dialect, query), args...)
}

func (tx *Tx) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return tx.raw.Query(rebindQuery(tx.dialect, query), args...)
}

func (tx *Tx) QueryRow(query string, args ...interface{}) *sql.Row {
	return tx.raw.QueryRow(rebindQuery(tx.dialect, query), args...)
}

func (tx *Tx) Prepare(query string) (*sql.Stmt, error) {
	return tx.raw.Prepare(rebindQuery(tx.dialect, query))
}

func (tx *Tx) Commit() error {
	return tx.raw.Commit()
}

func (tx *Tx) Rollback() error {
	return tx.raw.Rollback()
}

func rebindQuery(dialect Dialect, query string) string {
	if dialect != DialectPostgres || !strings.Contains(query, "?") {
		return query
	}

	var builder strings.Builder
	builder.Grow(len(query) + 8)

	placeholder := 1
	for _, r := range query {
		if r == '?' {
			builder.WriteByte('$')
			builder.WriteString(strconv.Itoa(placeholder))
			placeholder++
			continue
		}
		builder.WriteRune(r)
	}

	return builder.String()
}
