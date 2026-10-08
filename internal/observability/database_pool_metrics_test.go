package observability

import (
	"database/sql"
	"net/http/httptest"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestMetricsHandlerReportsDatabasePoolsWithoutQueries(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://unused/unused")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(7)
	pools := []DatabasePool{{Name: "primary", DB: db}, {Name: "unconfigured"}}
	handler := Handler(pools...)
	pools[0].Name = "changed"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	if response.Code != 200 {
		t.Fatalf("status=%d", response.Code)
	}
	body := response.Body.String()
	for _, metric := range []string{
		`theia_db_pool_max_open_connections{pool="primary"} 7`,
		`theia_db_pool_open_connections{pool="primary"} 0`,
		`theia_db_pool_in_use_connections{pool="primary"} 0`,
		`theia_db_pool_idle_connections{pool="primary"} 0`,
		`theia_db_pool_wait_total{pool="primary"} 0`,
		`theia_db_pool_wait_seconds_total{pool="primary"} 0`,
	} {
		assertContainsMetric(t, body, metric)
	}
	if db.Stats().OpenConnections != 0 {
		t.Fatal("scrape opened a database connection")
	}
}
