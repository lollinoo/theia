package api

// This file exercises grafana dashboard handler behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestGrafanaDashboardHandlerCreateProfileStoresDefaultTemplate(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewGrafanaDashboardHandler(repo)

	body := `{"name":"RouterBoard shared","url_template":"https://grafana.example/d/router?var-device={{hostname}}","variable_source":"hostname","is_default":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grafana/dashboard-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleProfiles(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp grafanaDashboardConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.Profiles) != 1 {
		t.Fatalf("expected one profile, got %#v", resp.Data.Profiles)
	}
	profile := resp.Data.Profiles[0]
	if profile.Name != "RouterBoard shared" || profile.URLTemplate != "https://grafana.example/d/router?var-device={{hostname}}" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	if resp.Data.DefaultProfileID != profile.ID {
		t.Fatalf("expected profile to be default, got default=%q profile=%q", resp.Data.DefaultProfileID, profile.ID)
	}
}

func TestGrafanaDashboardHandlerDeviceOverrideCanUseCustomURLWithoutGlobalGrafanaURL(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewGrafanaDashboardHandler(repo)
	deviceID := uuid.New()

	body := `{"profile_id":null,"custom_url":"https://grafana.example/d/router?var-device=edge-01"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/grafana/device-overrides/"+deviceID.String(), strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleDeviceOverride(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp grafanaDashboardConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	override, ok := resp.Data.DeviceOverrides[deviceID.String()]
	if !ok {
		t.Fatalf("expected override for device %s, got %#v", deviceID, resp.Data.DeviceOverrides)
	}
	if override.CustomURL != "https://grafana.example/d/router?var-device=edge-01" {
		t.Fatalf("unexpected override: %#v", override)
	}
}
