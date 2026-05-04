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

func TestRebindQuery_PostgresSkipsSQLSyntaxQuestionMarks(t *testing.T) {
	query := "SELECT '?' AS literal, 'it''s ?' AS escaped_literal, \"question?column\", $tag$? inside dollar quote$tag$ FROM devices -- comment ?\nWHERE id = ? /* block ? */ AND hostname = ?"
	got := rebindQuery(DialectPostgres, query)
	want := "SELECT '?' AS literal, 'it''s ?' AS escaped_literal, \"question?column\", $tag$? inside dollar quote$tag$ FROM devices -- comment ?\nWHERE id = $1 /* block ? */ AND hostname = $2"
	if got != want {
		t.Fatalf("rebindQuery() = %q, want %q", got, want)
	}
}

func TestRebindQuery_PostgresPreservesJSONQuestionOperators(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "bare-question-operator",
			query: `SELECT metadata ? 'prometheus?' FROM devices WHERE id = ?`,
			want:  `SELECT metadata ? 'prometheus?' FROM devices WHERE id = $1`,
		},
		{
			name:  "any-question-operator",
			query: `SELECT metadata ?| array['prometheus?', 'snmp'] FROM devices WHERE id = ?`,
			want:  `SELECT metadata ?| array['prometheus?', 'snmp'] FROM devices WHERE id = $1`,
		},
		{
			name:  "all-question-operator",
			query: `SELECT metadata ?& array['prometheus', 'snmp?'] FROM devices WHERE id = ?`,
			want:  `SELECT metadata ?& array['prometheus', 'snmp?'] FROM devices WHERE id = $1`,
		},
		{
			name:  "operator-with-placeholder-rhs",
			query: `SELECT metadata ? ? FROM devices WHERE id = ?`,
			want:  `SELECT metadata ? $1 FROM devices WHERE id = $2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rebindQuery(DialectPostgres, tt.query)
			if got != tt.want {
				t.Fatalf("rebindQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRebindQuery_PostgresDoesNotTreatKeywordPlaceholdersAsJSONOperators(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "between",
			query: `SELECT * FROM metrics WHERE observed_at BETWEEN ? AND ?`,
			want:  `SELECT * FROM metrics WHERE observed_at BETWEEN $1 AND $2`,
		},
		{
			name:  "between-with-continuation",
			query: `SELECT * FROM metrics WHERE observed_at BETWEEN ? AND ? AND status = ?`,
			want:  `SELECT * FROM metrics WHERE observed_at BETWEEN $1 AND $2 AND status = $3`,
		},
		{
			name:  "like-escape",
			query: `SELECT * FROM devices WHERE hostname LIKE ? ESCAPE ?`,
			want:  `SELECT * FROM devices WHERE hostname LIKE $1 ESCAPE $2`,
		},
		{
			name:  "limit-offset",
			query: `SELECT * FROM devices ORDER BY hostname LIMIT ? OFFSET ?`,
			want:  `SELECT * FROM devices ORDER BY hostname LIMIT $1 OFFSET $2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rebindQuery(DialectPostgres, tt.query)
			if got != tt.want {
				t.Fatalf("rebindQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRebindQuery_PostgresDoesNotTreatProjectionPlaceholdersAsJSONOperators(t *testing.T) {
	query := `SELECT ? FROM devices WHERE id = ?`
	got := rebindQuery(DialectPostgres, query)
	want := `SELECT $1 FROM devices WHERE id = $2`
	if got != want {
		t.Fatalf("rebindQuery() = %q, want %q", got, want)
	}
}

func TestRebindQuery_PostgresNumbersOnlyBindablePlaceholders(t *testing.T) {
	query := `INSERT INTO audit_log (message, device_id, payload) VALUES ('why?', ?, ?)`
	got := rebindQuery(DialectPostgres, query)
	want := `INSERT INTO audit_log (message, device_id, payload) VALUES ('why?', $1, $2)`
	if got != want {
		t.Fatalf("rebindQuery() = %q, want %q", got, want)
	}
}
