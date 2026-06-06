package api

// This file exercises oversized body behavior so refactors preserve the documented contract.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOversizedBody_AllMutationHandlers verifies that every mutation handler
// returns 413 when the request body exceeds the MaxBytesReader limit.
// This proves SEC-03 gap closure: all handlers use decodeJSON, not raw json.NewDecoder.
func TestOversizedBody_AllMutationHandlers(t *testing.T) {
	// Build an oversized but valid JSON body (>1MB)
	bigJSON := `{"data":"` + strings.Repeat("a", 2<<20) + `"}`

	// Construct minimal handler instances with nil/mock dependencies.
	// We only need the handler to reach the decodeJSON call -- the oversized body
	// triggers MaxBytesError before any service/repo call happens.

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
	}{
		// DeviceHandler mutations
		{"DeviceHandler.HandleCreate", http.MethodPost, "/api/v1/devices", (&DeviceHandler{}).HandleCreate},
		{"DeviceHandler.HandleUpdate", http.MethodPut, "/api/v1/devices/00000000-0000-0000-0000-000000000001", (&DeviceHandler{}).HandleUpdate},
		{"DeviceHandler.HandleBatchAdd", http.MethodPost, "/api/v1/devices/batch", (&DeviceHandler{}).HandleBatchAdd},
		// LinkHandler mutations
		{"LinkHandler.HandleCreate", http.MethodPost, "/api/v1/links", (&LinkHandler{}).HandleCreate},
		{"LinkHandler.HandleUpdate", http.MethodPut, "/api/v1/links/00000000-0000-0000-0000-000000000001", (&LinkHandler{}).HandleUpdate},
		// SettingsHandler mutations
		{"SettingsHandler.HandleUpdate", http.MethodPut, "/api/v1/settings/poll_interval", (&SettingsHandler{}).HandleUpdate},
		// CredentialProfileHandler mutations
		{"CredentialProfileHandler.HandleCreate", http.MethodPost, "/api/v1/credential-profiles", (&CredentialProfileHandler{}).HandleCreate},
		{"CredentialProfileHandler.HandleUpdate", http.MethodPut, "/api/v1/credential-profiles/00000000-0000-0000-0000-000000000001", (&CredentialProfileHandler{}).HandleUpdate},
		{"CredentialProfileHandler.HandleTest", http.MethodPost, "/api/v1/credential-profiles/00000000-0000-0000-0000-000000000001/test", (&CredentialProfileHandler{}).HandleTest},
		// SNMPProfileHandler mutations
		{"SNMPProfileHandler.HandleCreate", http.MethodPost, "/api/v1/snmp-profiles", (&SNMPProfileHandler{}).HandleCreate},
		{"SNMPProfileHandler.HandleUpdate", http.MethodPut, "/api/v1/snmp-profiles/00000000-0000-0000-0000-000000000001", (&SNMPProfileHandler{}).HandleUpdate},
		// PositionHandler mutations
		{"PositionHandler.HandleSaveAll", http.MethodPut, "/api/v1/positions", (&PositionHandler{}).HandleSaveAll},
		// BackupHandler mutations
		{"BackupHandler.HandleBulkDownload", http.MethodPost, "/api/v1/backups/bulk-download", (&BackupHandler{}).HandleBulkDownload},
		// VendorHandler mutations
		{"VendorHandler.HandleUpdateVendor", http.MethodPut, "/api/v1/vendors/test-vendor", (&VendorHandler{}).HandleUpdateVendor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(bigJSON))
			req.Body = http.MaxBytesReader(rec, req.Body, 1<<20) // 1MB limit

			tt.handler(rec, req)

			if rec.Code != http.StatusRequestEntityTooLarge {
				t.Errorf("expected 413, got %d", rec.Code)
			}
		})
	}
}
