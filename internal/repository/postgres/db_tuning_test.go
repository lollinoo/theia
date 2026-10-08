package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/lollinoo/theia/internal/observability"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestConfigureDBUsesFixedConnectionBudget(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://unused/unused")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	ConfigureDB(db)
	if got := db.Stats().MaxOpenConnections; got != 16 {
		t.Fatalf("default open=%d", got)
	}
	runtime.GOMAXPROCS(32)
	ConfigureDB(db)
	if got := db.Stats().MaxOpenConnections; got != 16 {
		t.Fatalf("CPU changed pool size to %d", got)
	}
	if err := ConfigureDBWithLimits(db, 3, 0); err != nil {
		t.Fatal(err)
	}
	if got := db.Stats().MaxOpenConnections; got != 3 {
		t.Fatalf("configured open=%d", got)
	}
	for _, limits := range [][2]int{{0, 0}, {-1, 0}, {3, -1}, {3, 4}} {
		if err := ConfigureDBWithLimits(db, limits[0], limits[1]); err == nil {
			t.Fatalf("accepted limits=%v", limits)
		}
		if db.Stats().MaxOpenConnections != 3 {
			t.Fatal("invalid limits changed pool")
		}
	}
}

func TestConfiguredDBPoolEnforcesBudgetAndReportsWaits(t *testing.T) {
	db := setupTestDB(t)
	if err := ConfigureDBWithLimits(db, 3, 1); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if conn, err := db.Conn(ctx); !errors.Is(err, context.DeadlineExceeded) {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("extra connection error=%v, want budget wait deadline", err)
	}
	stats := db.Stats()
	if stats.InUse != 3 || stats.OpenConnections != 3 || stats.WaitCount != 1 || stats.WaitDuration <= 0 {
		t.Fatalf("unexpected pool stats: %+v", stats)
	}
	response := httptest.NewRecorder()
	observability.Handler(observability.DatabasePool{Name: "primary", DB: db}).ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	for _, sample := range []string{`theia_db_pool_in_use_connections{pool="primary"} 3`, `theia_db_pool_wait_total{pool="primary"} 1`} {
		if !strings.Contains(response.Body.String(), sample) {
			t.Fatalf("missing metric %s", sample)
		}
	}
}
