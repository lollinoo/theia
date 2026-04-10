package api

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

func (f *failingSettingsRepo) Get(key string) (string, error)         { return "", errMock }
func (f *failingSettingsRepo) Set(key, value string) error            { return errMock }
func (f *failingSettingsRepo) GetAll() (map[string]string, error)     { return nil, errMock }

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
