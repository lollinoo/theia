package api

// This file exercises grafana dashboard handler behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type grafanaErrorSettingsRepo struct {
	getErr      error
	setCalls    int
	updateCalls int
}

func (r *grafanaErrorSettingsRepo) Get(string) (string, error) {
	return "", r.getErr
}

func (r *grafanaErrorSettingsRepo) Set(string, string) error {
	r.setCalls++
	return nil
}

func (r *grafanaErrorSettingsRepo) GetAll() (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *grafanaErrorSettingsRepo) Update(string, func(string) (string, error)) (string, error) {
	r.updateCalls++
	return "", r.getErr
}

type concurrentGrafanaSettingsRepo struct {
	updateMu sync.Mutex
	mu       sync.Mutex
	settings map[string]string
	reads    int
	readGate chan struct{}
}

func newConcurrentGrafanaSettingsRepo() *concurrentGrafanaSettingsRepo {
	return &concurrentGrafanaSettingsRepo{
		settings: domain.DefaultSettings(),
		readGate: make(chan struct{}),
	}
}

func (r *concurrentGrafanaSettingsRepo) Get(key string) (string, error) {
	r.mu.Lock()
	value, ok := r.settings[key]
	if key == domain.SettingGrafanaDashboardConfig {
		r.reads++
		if r.reads == 2 {
			close(r.readGate)
		}
	}
	r.mu.Unlock()
	if key == domain.SettingGrafanaDashboardConfig {
		<-r.readGate
	}
	if !ok {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	return value, nil
}

func (r *concurrentGrafanaSettingsRepo) Set(key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settings[key] = value
	return nil
}

func (r *concurrentGrafanaSettingsRepo) GetAll() (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	settings := make(map[string]string, len(r.settings))
	for key, value := range r.settings {
		settings[key] = value
	}
	return settings, nil
}

func (r *concurrentGrafanaSettingsRepo) Update(key string, update func(string) (string, error)) (string, error) {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()

	r.mu.Lock()
	current, ok := r.settings[key]
	r.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	next, err := update(current)
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	r.settings[key] = next
	r.mu.Unlock()
	return next, nil
}

func (r *concurrentGrafanaSettingsRepo) stored(key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.settings[key]
	if !ok {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	return value, nil
}

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

func TestGrafanaDashboardHandlerGetReturnsServerErrorOnConfigReadFailure(t *testing.T) {
	repo := &grafanaErrorSettingsRepo{getErr: errors.New("database unavailable")}
	h := NewGrafanaDashboardHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/grafana/dashboard-profiles", nil)
	rec := httptest.NewRecorder()

	h.HandleProfiles(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGrafanaDashboardHandlerMutationStopsOnConfigReadFailure(t *testing.T) {
	profileID := uuid.NewString()
	deviceID := uuid.NewString()
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		handle func(*GrafanaDashboardHandler, http.ResponseWriter, *http.Request)
	}{
		{
			name:   "create profile",
			method: http.MethodPost,
			path:   "/api/v1/grafana/dashboard-profiles",
			body:   `{"name":"RouterBoard shared","url_template":"https://grafana.example/d/router?var-device={{hostname}}","variable_source":"hostname","is_default":true}`,
			handle: (*GrafanaDashboardHandler).HandleProfiles,
		},
		{
			name:   "update profile",
			method: http.MethodPut,
			path:   "/api/v1/grafana/dashboard-profiles/" + profileID,
			body:   `{"name":"RouterBoard shared","url_template":"https://grafana.example/d/router?var-device={{hostname}}","variable_source":"hostname"}`,
			handle: (*GrafanaDashboardHandler).HandleProfile,
		},
		{
			name:   "delete profile",
			method: http.MethodDelete,
			path:   "/api/v1/grafana/dashboard-profiles/" + profileID,
			handle: (*GrafanaDashboardHandler).HandleProfile,
		},
		{
			name:   "update device override",
			method: http.MethodPut,
			path:   "/api/v1/grafana/device-overrides/" + deviceID,
			body:   `{"profile_id":null,"custom_url":"https://grafana.example/d/router?var-device=edge-01"}`,
			handle: (*GrafanaDashboardHandler).HandleDeviceOverride,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &grafanaErrorSettingsRepo{getErr: errors.New("database unavailable")}
			h := NewGrafanaDashboardHandler(repo)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			tt.handle(h, rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d; body=%s", rec.Code, rec.Body.String())
			}
			if repo.setCalls != 0 {
				t.Fatalf("expected no config mutation after read failure, got %d Set calls", repo.setCalls)
			}
			if repo.updateCalls != 1 {
				t.Fatalf("expected one atomic update attempt, got %d", repo.updateCalls)
			}
		})
	}
}

func TestGrafanaDashboardHandlerMissingConfigReturnsEmptyConfig(t *testing.T) {
	repo := &grafanaErrorSettingsRepo{getErr: fmt.Errorf("reading setting: %w", domain.ErrSettingNotFound)}
	h := NewGrafanaDashboardHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/grafana/dashboard-profiles", nil)
	rec := httptest.NewRecorder()

	h.HandleProfiles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp grafanaDashboardConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.Profiles) != 0 || len(resp.Data.DeviceOverrides) != 0 || resp.Data.DefaultProfileID != "" {
		t.Fatalf("expected empty config, got %#v", resp.Data)
	}
}

func TestGrafanaDashboardHandlerConcurrentProfileCreatesPreserveBothUpdates(t *testing.T) {
	repo := newConcurrentGrafanaSettingsRepo()
	h := NewGrafanaDashboardHandler(repo)

	bodies := []string{
		`{"name":"Router dashboard","url_template":"https://grafana.example/d/router?var-device={{hostname}}","variable_source":"hostname"}`,
		`{"name":"Switch dashboard","url_template":"https://grafana.example/d/switch?var-device={{hostname}}","variable_source":"hostname"}`,
	}
	recorders := make([]*httptest.ResponseRecorder, len(bodies))
	var wg sync.WaitGroup
	for i, body := range bodies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/grafana/dashboard-profiles", strings.NewReader(body))
			recorders[i] = httptest.NewRecorder()
			h.HandleProfiles(recorders[i], req)
		}()
	}
	wg.Wait()

	for i, recorder := range recorders {
		if recorder.Code != http.StatusCreated {
			t.Fatalf("request %d: expected 201, got %d; body=%s", i, recorder.Code, recorder.Body.String())
		}
	}
	raw, err := repo.stored(domain.SettingGrafanaDashboardConfig)
	if err != nil {
		t.Fatalf("get stored config: %v", err)
	}
	var config grafanaDashboardConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatalf("decode stored config: %v", err)
	}
	if len(config.Profiles) != 2 {
		t.Fatalf("expected both profiles to survive concurrent creates, got %#v", config.Profiles)
	}
}
