package api

// This file exercises vendor handler behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/vendor"
)

// --- Mock VendorConfigRepo ---

type mockVendorConfigRepo struct {
	records map[string]*domain.VendorConfigRecord
}

func newMockVendorConfigRepo() *mockVendorConfigRepo {
	return &mockVendorConfigRepo{records: make(map[string]*domain.VendorConfigRecord)}
}

func (r *mockVendorConfigRepo) GetAll() ([]domain.VendorConfigRecord, error) {
	var result []domain.VendorConfigRecord
	for _, rec := range r.records {
		result = append(result, *rec)
	}
	return result, nil
}

func (r *mockVendorConfigRepo) GetByName(name string) (*domain.VendorConfigRecord, error) {
	rec, ok := r.records[name]
	if !ok {
		return nil, fmt.Errorf("vendor config not found: %s", name)
	}
	cp := *rec
	return &cp, nil
}

func (r *mockVendorConfigRepo) Upsert(record *domain.VendorConfigRecord) error {
	r.records[record.Name] = record
	return nil
}

func buildTestVendorRegistryWithVendor(name string) *vendor.Registry {
	defaultCfg := vendor.DBVendorRecord{
		Name: "default",
		ConfigJSON: `{
			"vendor": {"name": "default", "display_name": "Generic"},
			"detection": {},
			"backup": {"supported": false}
		}`,
	}
	records := []vendor.DBVendorRecord{defaultCfg}
	if name != "" && name != "default" {
		records = append(records, vendor.DBVendorRecord{
			Name: name,
			ConfigJSON: fmt.Sprintf(`{
				"vendor": {"name": %q, "display_name": %q},
				"detection": {"sys_object_id_prefixes": ["1.3.6.1.4.1.99999"]},
				"backup": {"supported": true}
			}`, name, name),
		})
	}
	reg, err := vendor.LoadRegistryFromDB(records)
	if err != nil {
		panic(fmt.Sprintf("buildTestVendorRegistryWithVendor: %v", err))
	}
	return reg
}

func newTestVendorHandler(t *testing.T, vendorName string) *VendorHandler {
	t.Helper()
	registry := buildTestVendorRegistryWithVendor(vendorName)
	configRepo := newMockVendorConfigRepo()
	return NewVendorHandler(registry, configRepo)
}

func TestVendorHandlerListVendors(t *testing.T) {
	handler := newTestVendorHandler(t, "mikrotik")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vendors", nil)
	rec := httptest.NewRecorder()
	handler.HandleListVendors(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	dataRaw, ok := resp["data"]
	if !ok {
		t.Fatal("expected 'data' key in response")
	}

	var entries []json.RawMessage
	if err := json.Unmarshal(dataRaw, &entries); err != nil {
		t.Fatalf("failed to unmarshal data array: %v", err)
	}
	// Should have at least default + mikrotik
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 vendors, got %d", len(entries))
	}
}

func TestVendorHandlerGetVendor_HappyPath(t *testing.T) {
	handler := newTestVendorHandler(t, "mikrotik")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vendors/mikrotik", nil)
	rec := httptest.NewRecorder()
	handler.HandleGetVendor(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp["data"]; !ok {
		t.Fatal("expected 'data' key in response")
	}
}

func TestVendorHandlerGetVendor_NotFound(t *testing.T) {
	handler := newTestVendorHandler(t, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vendors/nonexistent", nil)
	rec := httptest.NewRecorder()
	handler.HandleGetVendor(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestVendorHandlerUpdateVendor_HappyPath(t *testing.T) {
	handler := newTestVendorHandler(t, "mikrotik")

	updateBody := `{
		"vendor": {"name": "mikrotik", "display_name": "MikroTik Updated"},
		"detection": {"sys_object_id_prefixes": ["1.3.6.1.4.1.99999"]},
		"backup": {"supported": true}
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/vendors/mikrotik", strings.NewReader(updateBody))
	rec := httptest.NewRecorder()
	handler.HandleUpdateVendor(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp["data"]; !ok {
		t.Fatal("expected 'data' key in response")
	}
}

// Silence unused import warnings.
var _ = time.Now
