package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

type fakeStatusProvider struct{ status string }

func (f fakeStatusProvider) Status() string { return f.status }

func TestHealthHandlerHealth(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	poller := fakeStatusProvider{status: "running"}
	h := NewHealthHandler(db, poller)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	h.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Status     string            `json:"status"`
		Version    map[string]string `json:"version"`
		Components map[string]string `json:"components"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Fatalf("expected status=ok, got %s", resp.Status)
	}
	if resp.Version["version"] == "" {
		t.Fatal("expected version field in health response")
	}
	if resp.Components["db"] != "ok" {
		t.Fatalf("expected db=ok, got %s", resp.Components["db"])
	}
	if resp.Components["db_dialect"] != "sqlite" {
		t.Fatalf("expected db_dialect=sqlite, got %s", resp.Components["db_dialect"])
	}
	if resp.Components["snmp_poller"] != "running" {
		t.Fatalf("expected snmp_poller=running, got %s", resp.Components["snmp_poller"])
	}
}

func TestHealthHandlerHealth_DBDown(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	// Close DB before calling health to simulate DB down
	db.Close()

	poller := fakeStatusProvider{status: "stopped"}
	h := NewHealthHandler(db, poller)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	h.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Status     string            `json:"status"`
		Version    map[string]string `json:"version"`
		Components map[string]string `json:"components"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "degraded" {
		t.Fatalf("expected status=degraded, got %s", resp.Status)
	}
	if resp.Components["db"] != "error" {
		t.Fatalf("expected db=error, got %s", resp.Components["db"])
	}
	if resp.Components["db_dialect"] != "sqlite" {
		t.Fatalf("expected db_dialect=sqlite, got %s", resp.Components["db_dialect"])
	}
}

func TestHealthHandlerHealth_NilPoller(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	h := NewHealthHandler(db, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	h.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Status     string            `json:"status"`
		Version    map[string]string `json:"version"`
		Components map[string]string `json:"components"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Components["snmp_poller"] != "stopped" {
		t.Fatalf("expected snmp_poller=stopped when poller is nil, got %s", resp.Components["snmp_poller"])
	}
	if resp.Components["db_dialect"] != "sqlite" {
		t.Fatalf("expected db_dialect=sqlite, got %s", resp.Components["db_dialect"])
	}
}
