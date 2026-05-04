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

	return rebindPostgresQuestionPlaceholders(query)
}

func rebindPostgresQuestionPlaceholders(query string) string {
	var builder strings.Builder
	builder.Grow(len(query) + 8)

	placeholder := 1
	for i := 0; i < len(query); {
		switch {
		case query[i] == '\'':
			i = copySingleQuotedSQL(&builder, query, i)
		case query[i] == '"':
			i = copyDoubleQuotedSQL(&builder, query, i)
		case query[i] == '-' && i+1 < len(query) && query[i+1] == '-':
			i = copyLineCommentSQL(&builder, query, i)
		case query[i] == '/' && i+1 < len(query) && query[i+1] == '*':
			i = copyBlockCommentSQL(&builder, query, i)
		case query[i] == '$':
			if tag, ok := readDollarQuoteTag(query, i); ok {
				i = copyDollarQuotedSQL(&builder, query, i, tag)
				continue
			}
			builder.WriteByte(query[i])
			i++
		case query[i] == '?':
			if isPostgresQuestionOperator(query, i) {
				builder.WriteByte(query[i])
				i++
				continue
			}
			builder.WriteByte('$')
			builder.WriteString(strconv.Itoa(placeholder))
			placeholder++
			i++
		default:
			builder.WriteByte(query[i])
			i++
		}
	}

	return builder.String()
}

func copySingleQuotedSQL(builder *strings.Builder, query string, start int) int {
	i := start + 1
	for i < len(query) {
		if query[i] == '\'' {
			if i+1 < len(query) && query[i+1] == '\'' {
				i += 2
				continue
			}
			i++
			break
		}
		i++
	}
	builder.WriteString(query[start:i])
	return i
}

func copyDoubleQuotedSQL(builder *strings.Builder, query string, start int) int {
	i := start + 1
	for i < len(query) {
		if query[i] == '"' {
			if i+1 < len(query) && query[i+1] == '"' {
				i += 2
				continue
			}
			i++
			break
		}
		i++
	}
	builder.WriteString(query[start:i])
	return i
}

func copyLineCommentSQL(builder *strings.Builder, query string, start int) int {
	i := start + 2
	for i < len(query) && query[i] != '\n' {
		i++
	}
	if i < len(query) {
		i++
	}
	builder.WriteString(query[start:i])
	return i
}

func copyBlockCommentSQL(builder *strings.Builder, query string, start int) int {
	i := start + 2
	for i+1 < len(query) {
		if query[i] == '*' && query[i+1] == '/' {
			i += 2
			builder.WriteString(query[start:i])
			return i
		}
		i++
	}
	builder.WriteString(query[start:])
	return len(query)
}

func readDollarQuoteTag(query string, start int) (string, bool) {
	if start+1 >= len(query) {
		return "", false
	}
	if query[start+1] == '$' {
		return "$$", true
	}
	if !isDollarQuoteTagStart(query[start+1]) {
		return "", false
	}

	i := start + 2
	for i < len(query) && isDollarQuoteTagContinue(query[i]) {
		i++
	}
	if i < len(query) && query[i] == '$' {
		return query[start : i+1], true
	}
	return "", false
}

func copyDollarQuotedSQL(builder *strings.Builder, query string, start int, tag string) int {
	contentStart := start + len(tag)
	if end := strings.Index(query[contentStart:], tag); end >= 0 {
		i := contentStart + end + len(tag)
		builder.WriteString(query[start:i])
		return i
	}
	builder.WriteString(query[start:])
	return len(query)
}

func isPostgresQuestionOperator(query string, index int) bool {
	if index+1 < len(query) && (query[index+1] == '|' || query[index+1] == '&') {
		return true
	}

	prev := previousNonSpaceIndex(query, index-1)
	if prev < 0 || !isPostgresExpressionEnd(query[prev]) {
		return false
	}

	next := nextNonSpaceIndex(query, index+1)
	if next < 0 || !isPostgresExpressionStart(query[next]) {
		return false
	}

	return true
}

func previousNonSpaceIndex(query string, index int) int {
	for index >= 0 && isSQLSpace(query[index]) {
		index--
	}
	return index
}

func nextNonSpaceIndex(query string, index int) int {
	for index < len(query) && isSQLSpace(query[index]) {
		index++
	}
	return index
}

func isPostgresExpressionEnd(b byte) bool {
	return isASCIIAlphaNumeric(b) || b == '_' || b == '\'' || b == '"' || b == ')' || b == ']' || b == '}'
}

func isPostgresExpressionStart(b byte) bool {
	return isASCIIAlphaNumeric(b) || b == '_' || b == '\'' || b == '"' || b == '$' || b == '(' || b == '[' || b == '{' || b == '?'
}

func isDollarQuoteTagStart(b byte) bool {
	return isASCIILetter(b) || b == '_'
}

func isDollarQuoteTagContinue(b byte) bool {
	return isASCIIAlphaNumeric(b) || b == '_'
}

func isASCIIAlphaNumeric(b byte) bool {
	return isASCIILetter(b) || ('0' <= b && b <= '9')
}

func isASCIILetter(b byte) bool {
	return ('A' <= b && b <= 'Z') || ('a' <= b && b <= 'z')
}

func isSQLSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}
