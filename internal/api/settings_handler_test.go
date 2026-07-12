package api

// This file exercises settings handler behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
)

// failingSettingsRepo always returns an error for all operations.
type failingSettingsRepo struct{}

func (f *failingSettingsRepo) Get(key string) (string, error) { return "", errMock }

func (f *failingSettingsRepo) Set(key, value string) error { return errMock }

func (f *failingSettingsRepo) GetAll() (map[string]string, error) { return nil, errMock }

func TestSettingsHandlerGetAll(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rec := httptest.NewRecorder()

	h.HandleGetAll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Data map[string]string `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := resp.Data[domain.SettingPollingInterval]; !ok {
		t.Fatal("expected response to contain polling_interval_seconds key")
	}
}

func TestSettingsHandlerGetAll_OmitsLegacyBridgeSecret(t *testing.T) {
	repo := newMockSettingsRepo()
	repo.settings[domain.SettingBridgeSecret] = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	h := NewSettingsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rec := httptest.NewRecorder()

	h.HandleGetAll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "0123456789abcdef") {
		t.Fatalf("settings response leaked bridge secret: %s", rec.Body.String())
	}

	var resp settingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp.Data[domain.SettingBridgeSecret]; ok {
		t.Fatal("expected legacy bridge_secret to be omitted from settings data")
	}
	if resp.Meta != nil {
		t.Fatalf("expected no secret metadata for legacy bridge_secret, got %+v", resp.Meta)
	}
}

func TestSettingsHandlerGetAll_RepoError(t *testing.T) {
	h := NewSettingsHandler(&failingSettingsRepo{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rec := httptest.NewRecorder()

	h.HandleGetAll(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestSettingsHandlerUpdate_HappyPath(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"30"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/polling_interval_seconds", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	got, _ := repo.Get(domain.SettingPollingInterval)
	if got != "30" {
		t.Fatalf("expected polling_interval_seconds=30, got %s", got)
	}
}

func TestSettingsHandler_NetworkProbePorts_NormalizesValidList(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":" 22,8291,443 "}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingNetworkProbePorts, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	got, _ := repo.Get(domain.SettingNetworkProbePorts)
	if got != "22,8291,443" {
		t.Fatalf("expected network_probe_ports=22,8291,443, got %s", got)
	}
}

func TestSettingsHandler_NetworkProbePorts_RejectsInvalidList(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"22,65536"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingNetworkProbePorts, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), domain.SettingNetworkProbePorts) {
		t.Fatalf("expected error mentioning %s, got: %s", domain.SettingNetworkProbePorts, rec.Body.String())
	}
}

func TestSettingsHandlerUpdate_PerDeviceGrafanaDashboardURLAllowed(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)
	deviceID := "550e8400-e29b-41d4-a716-446655440000"
	key := "grafana_dashboard_url:" + deviceID

	body := `{"value":"https://grafana.example/d/router?var-device=edge-01"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+key, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	got, _ := repo.Get(key)
	if got != "https://grafana.example/d/router?var-device=edge-01" {
		t.Fatalf("expected legacy per-device grafana URL to persist, got %q", got)
	}
}

func TestSettingsHandlerUpdateRejectsGrafanaDashboardConfig(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)
	body := `{"value":"{\"profiles\":[]}"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingGrafanaDashboardConfig, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
	value, err := repo.Get(domain.SettingGrafanaDashboardConfig)
	if err != nil {
		t.Fatalf("get Grafana config: %v", err)
	}
	if value != "{}" {
		t.Fatalf("Grafana config changed through generic settings endpoint: %q", value)
	}
}

func TestSettingsHandlerGet_LegacyBridgeSecretRejected(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/"+domain.SettingBridgeSecret, nil)
	rec := httptest.NewRecorder()

	h.HandleGet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSettingsHandlerUpdate_LegacyBridgeSecretRejected(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingBridgeSecret, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
	got, _ := repo.Get(domain.SettingBridgeSecret)
	if got != "" {
		t.Fatal("expected legacy bridge_secret not to be stored")
	}
}

func TestSettingsHandlerUpdate_DebugLogsSanitizedChange(t *testing.T) {
	logs := captureAPIDebugLogs(t)
	repo := newMockSettingsRepo()
	repo.settings[domain.SettingPrometheusURL] = "http://old-prometheus.example/api"
	h := NewSettingsHandler(repo)

	body := `{"value":"http://new-prometheus.example/api"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingPrometheusURL, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	output := logs.String()
	if !strings.Contains(output, "DEBUG settings changed key=prometheus_url previous=<set> new=<set> affects=prometheus") {
		t.Fatalf("debug output missing sanitized settings change: %q", output)
	}
	if strings.Contains(output, "old-prometheus.example") || strings.Contains(output, "new-prometheus.example") {
		t.Fatalf("debug output leaked URL value: %q", output)
	}
}

func TestSettingsHandlerUpdate_EmptyKey(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"test"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSettingsHandlerUpdate_MalformedJSON(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/polling_interval_seconds", strings.NewReader(`{invalid`))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSettingsHandlerUpdate_InvalidTimezone(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"Not/A/Timezone"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/timezone", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSettingsHandler_UnknownKey_PUT_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"anything"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/unknown_key", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unknown setting key") {
		t.Errorf("expected error about unknown setting key, got: %s", rec.Body.String())
	}
}

func TestSettingsHandler_NumericSetting_InvalidValue_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"abc"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingPollingInterval, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "must be a valid integer") {
		t.Errorf("expected error about invalid integer, got: %s", rec.Body.String())
	}
}

func TestSettingsHandler_BoundedIntegerSettings_ValidBoundariesAccepted(t *testing.T) {
	tests := []struct {
		name string
		key  string
		min  string
		max  string
	}{
		{name: "polling interval", key: domain.SettingPollingInterval, min: "5", max: "3600"},
		{name: "legacy snmp worker pool", key: domain.SettingSNMPWorkerPoolSize, min: "1", max: "128"},
		{name: "performance worker pool", key: domain.SettingSNMPWorkerPoolPerformance, min: "1", max: "128"},
		{name: "operational worker pool", key: domain.SettingSNMPWorkerPoolOperational, min: "1", max: "128"},
		{name: "static worker pool", key: domain.SettingSNMPWorkerPoolStatic, min: "1", max: "128"},
		{name: "snmp timeout", key: domain.SettingSNMPTimeout, min: "1", max: "120"},
		{name: "snmp retries", key: domain.SettingSNMPRetries, min: "0", max: "10"},
		{name: "performance counter timeout", key: domain.SettingSNMPPerformanceCounterTimeoutMillis, min: "100", max: "30000"},
		{name: "performance counter retries", key: domain.SettingSNMPPerformanceCounterRetries, min: "0", max: "10"},
		{name: "essential workers", key: domain.SettingPollingEssentialWorkers, min: "1", max: "256"},
		{name: "workers per site", key: domain.SettingPollingMaxWorkersPerSite, min: "1", max: "256"},
		{name: "workers per subnet", key: domain.SettingPollingMaxWorkersPerSubnet, min: "1", max: "256"},
		{name: "workers per device", key: domain.SettingPollingMaxWorkersPerDevice, min: "1", max: "32"},
		{name: "inflight per profile", key: domain.SettingPollingMaxInflightPerProfile, min: "1", max: "256"},
		{name: "essential timeout", key: domain.SettingPollingEssentialTimeoutMillis, min: "100", max: "30000"},
		{name: "essential retries", key: domain.SettingPollingEssentialRetries, min: "0", max: "10"},
		{name: "websocket coalesce", key: domain.SettingPollingWebSocketCoalesceMS, min: "50", max: "5000"},
		{name: "persistence batch", key: domain.SettingPollingPersistenceBatchMS, min: "100", max: "10000"},
		{name: "instance retention", key: domain.SettingInstanceBackupRetentionCount, min: "1", max: "365"},
		{name: "device retention", key: domain.SettingDeviceBackupRetentionCount, min: "1", max: "365"},
		{name: "bridge port", key: domain.SettingBridgePort, min: "1", max: "65535"},
	}

	for _, tt := range tests {
		for _, value := range []string{tt.min, tt.max} {
			t.Run(tt.name+" "+value, func(t *testing.T) {
				repo := newMockSettingsRepo()
				h := NewSettingsHandler(repo)
				req := httptest.NewRequest(
					http.MethodPut,
					"/api/v1/settings/"+tt.key,
					strings.NewReader(`{"value":" `+value+` "}`),
				)
				rec := httptest.NewRecorder()

				h.HandleUpdate(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
				}
				got, _ := repo.Get(tt.key)
				if got != strings.TrimSpace(value) {
					t.Fatalf("expected %s=%s, got %s", tt.key, strings.TrimSpace(value), got)
				}
			})
		}
	}
}

func TestSettingsHandler_BoundedIntegerSettings_OutOfRangeRejected(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "below polling interval", key: domain.SettingPollingInterval, value: "4", want: "polling_interval_seconds must be between 5 and 3600"},
		{name: "above polling interval", key: domain.SettingPollingInterval, value: "3601", want: "polling_interval_seconds must be between 5 and 3600"},
		{name: "negative snmp retries", key: domain.SettingSNMPRetries, value: "-1", want: "snmp_retries must be between 0 and 10"},
		{name: "excess snmp retries", key: domain.SettingSNMPRetries, value: "11", want: "snmp_retries must be between 0 and 10"},
		{name: "short performance counter timeout", key: domain.SettingSNMPPerformanceCounterTimeoutMillis, value: "99", want: "snmp_performance_counter_timeout_ms must be between 100 and 30000"},
		{name: "excess performance counter retries", key: domain.SettingSNMPPerformanceCounterRetries, value: "11", want: "snmp_performance_counter_retries must be between 0 and 10"},
		{name: "negative essential retries", key: domain.SettingPollingEssentialRetries, value: "-1", want: "polling_essential_retries must be between 0 and 10"},
		{name: "excess essential retries", key: domain.SettingPollingEssentialRetries, value: "11", want: "polling_essential_retries must be between 0 and 10"},
		{name: "oversized essential workers", key: domain.SettingPollingEssentialWorkers, value: "257", want: "polling_essential_workers must be between 1 and 256"},
		{name: "oversized workers per device", key: domain.SettingPollingMaxWorkersPerDevice, value: "33", want: "polling_max_workers_per_device must be between 1 and 32"},
		{name: "oversized bridge port", key: domain.SettingBridgePort, value: "65536", want: "bridge_port must be between 1 and 65535"},
		{name: "zero retention", key: domain.SettingInstanceBackupRetentionCount, value: "0", want: "instance_backup_retention_count must be between 1 and 365"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockSettingsRepo()
			h := NewSettingsHandler(repo)
			req := httptest.NewRequest(
				http.MethodPut,
				"/api/v1/settings/"+tt.key,
				strings.NewReader(`{"value":"`+tt.value+`"}`),
			)
			rec := httptest.NewRecorder()

			h.HandleUpdate(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Fatalf("expected error containing %q, got: %s", tt.want, rec.Body.String())
			}
		})
	}
}

func TestSettingsHandler_BoundedIntegerSettings_RejectFloats(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/settings/"+domain.SettingPollingEssentialWorkers,
		strings.NewReader(`{"value":"1.5"}`),
	)
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "polling_essential_workers must be a valid integer") {
		t.Fatalf("expected integer error, got: %s", rec.Body.String())
	}
}

func TestSettingsHandler_BoundedFloatSetting_RangeAndNonFiniteValidation(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantStatus int
		wantError  string
	}{
		{name: "minimum", value: " 1.0 ", wantStatus: http.StatusOK},
		{name: "maximum", value: "5.0", wantStatus: http.StatusOK},
		{name: "below minimum", value: "0.99", wantStatus: http.StatusBadRequest, wantError: "polling_capacity_safety_margin must be between 1.0 and 5.0"},
		{name: "above maximum", value: "5.01", wantStatus: http.StatusBadRequest, wantError: "polling_capacity_safety_margin must be between 1.0 and 5.0"},
		{name: "NaN", value: "NaN", wantStatus: http.StatusBadRequest, wantError: "polling_capacity_safety_margin must be a valid float"},
		{name: "positive infinity", value: "+Inf", wantStatus: http.StatusBadRequest, wantError: "polling_capacity_safety_margin must be a valid float"},
		{name: "negative infinity", value: "-Inf", wantStatus: http.StatusBadRequest, wantError: "polling_capacity_safety_margin must be a valid float"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockSettingsRepo()
			h := NewSettingsHandler(repo)
			req := httptest.NewRequest(
				http.MethodPut,
				"/api/v1/settings/"+domain.SettingPollingCapacitySafetyMargin,
				strings.NewReader(`{"value":"`+tt.value+`"}`),
			)
			rec := httptest.NewRecorder()

			h.HandleUpdate(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantError != "" && !strings.Contains(rec.Body.String(), tt.wantError) {
				t.Fatalf("expected error containing %q, got: %s", tt.wantError, rec.Body.String())
			}
		})
	}
}

func TestSettingsHandler_PollingPolicyIntegerSetting_ValidValue_200(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"32"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingPollingEssentialWorkers, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	got, _ := repo.Get(domain.SettingPollingEssentialWorkers)
	if got != "32" {
		t.Fatalf("expected polling_essential_workers=32, got %s", got)
	}
}

func TestSettingsHandler_PollingPolicyIntegerSetting_InvalidValue_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"abc"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingPollingEssentialWorkers, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "must be a valid integer") {
		t.Errorf("expected error about invalid integer, got: %s", rec.Body.String())
	}
}

func TestSettingsHandler_PollingPolicyFloatSetting_ValidValue_200(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"1.75"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingPollingCapacitySafetyMargin, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	got, _ := repo.Get(domain.SettingPollingCapacitySafetyMargin)
	if got != "1.75" {
		t.Fatalf("expected polling_capacity_safety_margin=1.75, got %s", got)
	}
}

func TestSettingsHandler_PollingPolicyFloatSetting_InvalidValue_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"abc"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingPollingCapacitySafetyMargin, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "must be a valid float") {
		t.Errorf("expected error about invalid float, got: %s", rec.Body.String())
	}
}

func TestSettingsHandler_PollingPolicyBooleanSetting_ValidValue_200(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"true"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingPollingForceOverCapacity, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	got, _ := repo.Get(domain.SettingPollingForceOverCapacity)
	if got != "true" {
		t.Fatalf("expected polling_force_over_capacity=true, got %s", got)
	}
}

func TestSettingsHandler_PollingPolicyBooleanSetting_InvalidValue_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"sometimes"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingPollingForceOverCapacity, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "must be a valid boolean") {
		t.Errorf("expected error about invalid boolean, got: %s", rec.Body.String())
	}
}

func TestSettingsHandler_URLSetting_InvalidScheme_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"ftp://example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingPrometheusURL, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "must be a valid http/https URL") {
		t.Errorf("expected error about http/https URL, got: %s", rec.Body.String())
	}
}

func TestSettingsHandler_URLSetting_EmptyClears_200(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":""}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingPrometheusURL, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsHandler_TopologyDiscoveryMode_ValidValue_200(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"bootstrap_once"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingTopologyDiscoveryDefaultMode, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	got, _ := repo.Get(domain.SettingTopologyDiscoveryDefaultMode)
	if got != "bootstrap_once" {
		t.Fatalf("expected topology_discovery_default_mode=bootstrap_once, got %s", got)
	}
}

func TestSettingsHandler_TopologyDiscoveryMode_InvalidValue_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"cdp_only"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingTopologyDiscoveryDefaultMode, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsHandler_IntervalSetting_InvalidValue_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"7"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingInstanceBackupIntervalHours, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "must be one of: 0, 6, 12, 24, 48, 168") {
		t.Errorf("expected error about allowed intervals, got: %s", rec.Body.String())
	}
}

func TestSettingsHandler_IntervalSetting_ValidValue_200(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"24"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingInstanceBackupIntervalHours, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsHandler_DeviceInterval_InvalidValue_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"3"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingDeviceBackupIntervalHours, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsHandler_Timezone_Invalid_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"Invalid/Zone"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingTimezone, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSettingsHandler_Timezone_Valid_200(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"America/New_York"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingTimezone, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsHandler_GET_KnownKey_200(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/"+domain.SettingPollingInterval, nil)
	rec := httptest.NewRecorder()

	h.HandleGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsHandler_GET_UnknownKey_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/unknown_key", nil)
	rec := httptest.NewRecorder()

	h.HandleGet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unknown setting key") {
		t.Errorf("expected error about unknown setting key, got: %s", rec.Body.String())
	}
}

func TestSettingsHandler_BridgePort_ValidInteger_200(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"8080"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingBridgePort, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	got, _ := repo.Get(domain.SettingBridgePort)
	if got != "8080" {
		t.Fatalf("expected bridge_port=8080, got %s", got)
	}
}

func TestSettingsHandler_BridgePort_InvalidString_400(t *testing.T) {
	repo := newMockSettingsRepo()
	h := NewSettingsHandler(repo)

	body := `{"value":"abc"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/"+domain.SettingBridgePort, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "must be a valid integer") {
		t.Errorf("expected error about invalid integer, got: %s", rec.Body.String())
	}
}

func TestSettingsHandler_BridgePort_Default_InDefaultSettings(t *testing.T) {
	defaults := domain.DefaultSettings()
	val, ok := defaults[domain.SettingBridgePort]
	if !ok {
		t.Fatal("expected DefaultSettings() to contain bridge_port key")
	}
	if val != "1337" {
		t.Fatalf("expected bridge_port default to be '1337', got %q", val)
	}
}
