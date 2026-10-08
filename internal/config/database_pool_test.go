package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDatabasePoolConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, openEnv, idleEnv, wantError string
		wantOpen, wantIdle                      int
	}{
		{name: "defaults", wantOpen: 16, wantIdle: 8},
		{name: "yaml", yaml: "db_max_open_conns: 7\ndb_max_idle_conns: 2\n", wantOpen: 7, wantIdle: 2},
		{name: "environment", yaml: "db_max_open_conns: 7\ndb_max_idle_conns: 2\n", openEnv: "12", idleEnv: "0", wantOpen: 12},
		{name: "zero open", openEnv: "0", wantError: "THEIA_DB_MAX_OPEN_CONNS"},
		{name: "negative open", yaml: "db_max_open_conns: -1\n", wantError: "db_max_open_conns"},
		{name: "negative idle", idleEnv: "-1", wantError: "THEIA_DB_MAX_IDLE_CONNS"},
		{name: "idle exceeds open", yaml: "db_max_open_conns: 3\ndb_max_idle_conns: 4\n", wantError: "db_max_idle_conns"},
		{name: "malformed open", openEnv: "many", wantError: "THEIA_DB_MAX_OPEN_CONNS"},
		{name: "malformed idle", idleEnv: "many", wantError: "THEIA_DB_MAX_IDLE_CONNS"},
		{name: "overflow", openEnv: "999999999999999999999999", wantError: "THEIA_DB_MAX_OPEN_CONNS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("THEIA_DB_MAX_OPEN_CONNS", tc.openEnv)
			t.Setenv("THEIA_DB_MAX_IDLE_CONNS", tc.idleEnv)
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error=%v, want %s", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DBMaxOpenConns != tc.wantOpen || cfg.DBMaxIdleConns != tc.wantIdle {
				t.Fatalf("open=%d idle=%d", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns)
			}
		})
	}
}

func TestDownloadLeaseLimitRejectsProbeSlotOverflow(t *testing.T) {
	t.Setenv("THEIA_BULK_DOWNLOAD_MAX_CONCURRENT_GLOBAL", strconv.Itoa(int(^uint(0)>>1)))
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil || !strings.Contains(err.Error(), "lease probe") {
		t.Fatalf("error=%v, want lease connection overflow rejection", err)
	}
}
