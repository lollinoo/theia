package sqlite

import "testing"

func TestRebindQuery_Postgres(t *testing.T) {
	query := `UPDATE devices SET hostname = ?, ip = ? WHERE id = ?`
	got := rebindQuery(DialectPostgres, query)
	want := `UPDATE devices SET hostname = $1, ip = $2 WHERE id = $3`
	if got != want {
		t.Fatalf("rebindQuery() = %q, want %q", got, want)
	}
}

func TestRebindQuery_SQLiteUnchanged(t *testing.T) {
	query := `SELECT * FROM links WHERE source_device_id = ? AND target_device_id = ?`
	got := rebindQuery(DialectSQLite, query)
	if got != query {
		t.Fatalf("rebindQuery() = %q, want unchanged %q", got, query)
	}
}
