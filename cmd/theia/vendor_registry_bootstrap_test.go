package main

import (
	"database/sql"
	"encoding/json"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/vendor"
)

func openVendorConfigTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	return db
}

func TestLoadRegistryFromDB_FallsBackWhenAllRecordsInvalid(t *testing.T) {
	db := openVendorConfigTestDB(t)
	repo := sqlite.NewVendorConfigRepo(db)

	if err := repo.Upsert(&domain.VendorConfigRecord{
		Name:        "default",
		DisplayName: "Generic / Default",
		ConfigJSON:  "{not-json}",
	}); err != nil {
		t.Fatalf("upserting invalid vendor config: %v", err)
	}

	registry, err := loadRegistryFromDB(repo)
	if err != nil {
		t.Fatalf("loadRegistryFromDB() error = %v, want nil", err)
	}
	if registry != nil {
		t.Fatalf("loadRegistryFromDB() registry = %#v, want nil fallback signal", registry)
	}
}

func TestSeedVendorConfigs_SyncsMissingDefaultsWithoutOverwritingCustomizations(t *testing.T) {
	db := openVendorConfigTestDB(t)
	repo := sqlite.NewVendorConfigRepo(db)

	yamlRegistry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}

	currentJSON, err := yamlRegistry.ExportConfig("mikrotik")
	if err != nil {
		t.Fatalf("ExportConfig(mikrotik) error = %v", err)
	}

	var stale vendor.VendorConfig
	if err := json.Unmarshal(currentJSON, &stale); err != nil {
		t.Fatalf("json.Unmarshal(currentJSON) error = %v", err)
	}
	stale.SNMP.Static.SoftwareVersionOID = ""
	stale.Metrics.Prometheus.CPU = "custom_cpu_query"

	staleJSON, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("json.Marshal(stale) error = %v", err)
	}

	if err := repo.Upsert(&domain.VendorConfigRecord{
		Name:        "mikrotik",
		DisplayName: "MikroTik",
		ConfigJSON:  string(staleJSON),
	}); err != nil {
		t.Fatalf("repo.Upsert(stale) error = %v", err)
	}

	seedVendorConfigs(yamlRegistry, repo)

	record, err := repo.GetByName("mikrotik")
	if err != nil {
		t.Fatalf("repo.GetByName(mikrotik) error = %v", err)
	}
	if record == nil {
		t.Fatal("repo.GetByName(mikrotik) returned nil")
	}

	var merged vendor.VendorConfig
	if err := json.Unmarshal([]byte(record.ConfigJSON), &merged); err != nil {
		t.Fatalf("json.Unmarshal(merged) error = %v", err)
	}

	if merged.SNMP.Static.SoftwareVersionOID != ".1.3.6.1.4.1.14988.1.1.4.4.0" {
		t.Fatalf("SoftwareVersionOID = %q, want %q", merged.SNMP.Static.SoftwareVersionOID, ".1.3.6.1.4.1.14988.1.1.4.4.0")
	}
	if merged.Metrics.Prometheus.CPU != "custom_cpu_query" {
		t.Fatalf("CPU query = %q, want preserved custom value", merged.Metrics.Prometheus.CPU)
	}
}
