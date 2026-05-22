package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/service"
)

func TestUserSettingsHandlerGetReturnsOwnSettingsWithoutRawSecret(t *testing.T) {
	user := testAPIUser("alice", false, "account:manage")
	fake := &fakeUserSettingsService{
		settings: &service.UserSettingsResult{
			User: service.UserSettingsUser{
				ID:          user.User.User.ID,
				Username:    "alice",
				Email:       "alice@example.test",
				DisplayName: "Alice",
			},
			Preferences: service.UserSettingsPreferences{Timezone: "Europe/Rome", Locale: "it-IT", BridgePort: 1444},
			Bridge: service.UserSettingsBridge{
				Configured: true,
				Credential: &service.BridgeCredentialMetadata{
					ID:           uuid.New(),
					SecretPrefix: "theia_bridge_public",
					Status:       "active",
					CreatedAt:    time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	handler := NewUserSettingsHandler(fake, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/me", nil)
	req = req.WithContext(withAuthenticatedUser(req.Context(), user))
	rec := httptest.NewRecorder()
	handler.HandleMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "theia_bridge_raw") || strings.Contains(body, `"secret"`) {
		t.Fatalf("settings response leaked raw secret field: %s", body)
	}
	var parsed service.UserSettingsResult
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if parsed.Preferences.BridgePort != 1444 || !parsed.Bridge.Configured {
		t.Fatalf("settings response = %+v", parsed)
	}
}

func TestUserSettingsHandlerPatchRejectsPrivilegedFields(t *testing.T) {
	handler := NewUserSettingsHandler(&fakeUserSettingsService{}, "")
	user := testAPIUser("alice", false, "account:manage")
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings/me", strings.NewReader(`{"display_name":"Alice","role":"admin"}`))
	req = req.WithContext(withAuthenticatedUser(req.Context(), user))
	rec := httptest.NewRecorder()

	handler.HandleMe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserSettingsHandlerSecretReturnedOnlyOnMutation(t *testing.T) {
	user := testAPIUser("alice", false, "account:manage")
	fake := &fakeUserSettingsService{
		settings: &service.UserSettingsResult{
			User:        service.UserSettingsUser{ID: user.User.User.ID, Username: "alice"},
			Preferences: service.UserSettingsPreferences{Timezone: "UTC", Locale: "en-US", BridgePort: 1337},
		},
		secret: &service.BridgeSecretResult{
			Credential: service.BridgeCredentialMetadata{ID: uuid.New(), SecretPrefix: "theia_bridge_public", Status: "active"},
			Secret:     "theia_bridge_public.raw-secret",
			ShownOnce:  true,
		},
	}
	handler := NewUserSettingsHandler(fake, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/bridge/secret", nil)
	req = req.WithContext(withAuthenticatedUser(req.Context(), user))
	rec := httptest.NewRecorder()
	handler.HandleBridgeSecret(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "theia_bridge_public.raw-secret") {
		t.Fatalf("secret mutation response did not include one-time raw secret: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/settings/bridge", nil)
	req = req.WithContext(withAuthenticatedUser(req.Context(), user))
	handler.HandleBridge(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET bridge status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "raw-secret") || strings.Contains(rec.Body.String(), `"secret"`) {
		t.Fatalf("bridge metadata response leaked secret: %s", rec.Body.String())
	}
}

func TestUserSettingsHandlerDuplicateIdentifierReturnsConflict(t *testing.T) {
	handler := NewUserSettingsHandler(&fakeUserSettingsService{updateErr: service.ErrDuplicateUserIdentifier}, "")
	user := testAPIUser("alice", false, "account:manage")
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings/me", strings.NewReader(`{"username":"taken"}`))
	req = req.WithContext(withAuthenticatedUser(req.Context(), user))
	rec := httptest.NewRecorder()

	handler.HandleMe(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserSettingsHandlerPatchClearsBridgePortOverride(t *testing.T) {
	fake := &fakeUserSettingsService{}
	handler := NewUserSettingsHandler(fake, "")
	user := testAPIUser("alice", false, "account:manage")
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings/me", strings.NewReader(`{"bridge_port_override":null}`))
	req = req.WithContext(withAuthenticatedUser(req.Context(), user))
	rec := httptest.NewRecorder()

	handler.HandleMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !fake.lastUpdate.ClearBridgePortOverride || fake.lastUpdate.BridgePortOverride != nil {
		t.Fatalf("update input = %+v, want cleared bridge port override", fake.lastUpdate)
	}
}

func TestUserSettingsHandlerConnectorDownloadRequiresConfiguredBridgeSecret(t *testing.T) {
	handler := NewUserSettingsHandler(&fakeUserSettingsService{}, "")
	user := testAPIUser("alice", false, "account:manage")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/bridge/connector/download/linux/amd64", nil)
	req = req.WithContext(withAuthenticatedUser(req.Context(), user))
	rec := httptest.NewRecorder()

	handler.HandleConnectorDownload(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserSettingsHandlerConnectorConfigUsesForwardedBrowserURL(t *testing.T) {
	handler := NewUserSettingsHandler(&fakeUserSettingsService{}, "")
	user := testAPIUser("alice", false, "account:manage")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/bridge/connector/config", nil)
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("X-Forwarded-Host", "localhost:3000")
	req = req.WithContext(withAuthenticatedUser(req.Context(), user))
	rec := httptest.NewRecorder()

	handler.HandleConnectorConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var parsed struct {
		Config map[string]interface{} `json:"config"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := parsed.Config["theia_base_url"]; got != "http://localhost:3000" {
		t.Fatalf("theia_base_url = %v, want http://localhost:3000", got)
	}
	if got := parsed.Config["theia_origin"]; got != "http://localhost:3000" {
		t.Fatalf("theia_origin = %v, want http://localhost:3000", got)
	}
}

func TestUserSettingsHandlerConnectorConfigListsDownloadAvailability(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "winbox-bridge-linux-amd64"), []byte("bridge"), 0o600); err != nil {
		t.Fatalf("write bridge binary: %v", err)
	}
	handler := NewUserSettingsHandler(&fakeUserSettingsService{}, dir)
	user := testAPIUser("alice", false, "account:manage")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/bridge/connector/config", nil)
	req = req.WithContext(withAuthenticatedUser(req.Context(), user))
	rec := httptest.NewRecorder()

	handler.HandleConnectorConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var parsed struct {
		Downloads []struct {
			Label     string `json:"label"`
			OS        string `json:"os"`
			Arch      string `json:"arch"`
			URL       string `json:"url"`
			Available bool   `json:"available"`
		} `json:"downloads"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	downloads := make(map[string]struct {
		label     string
		url       string
		available bool
	})
	for _, target := range parsed.Downloads {
		downloads[target.OS+"/"+target.Arch] = struct {
			label     string
			url       string
			available bool
		}{label: target.Label, url: target.URL, available: target.Available}
	}
	linux := downloads["linux/amd64"]
	if linux.label != "Linux x64" || linux.url != "/api/v1/settings/bridge/connector/download/linux/amd64" || !linux.available {
		t.Fatalf("linux/amd64 download = %+v", linux)
	}
	windows := downloads["windows/amd64"]
	if windows.label != "Windows x64" || windows.available {
		t.Fatalf("windows/amd64 download = %+v, want unavailable Windows x64", windows)
	}
	mac := downloads["darwin/amd64"]
	if mac.label != "macOS Intel" || mac.available {
		t.Fatalf("darwin/amd64 download = %+v, want unavailable macOS Intel", mac)
	}
}

type fakeUserSettingsService struct {
	settings   *service.UserSettingsResult
	secret     *service.BridgeSecretResult
	updateErr  error
	lastUpdate service.UpdateUserSettingsInput
}

func (f *fakeUserSettingsService) GetSettings(context.Context, *service.AuthenticatedUser) (*service.UserSettingsResult, error) {
	if f.settings == nil {
		return &service.UserSettingsResult{
			User:        service.UserSettingsUser{ID: uuid.New(), Username: "alice"},
			Preferences: service.UserSettingsPreferences{Timezone: "UTC", Locale: "en-US", BridgePort: 1337},
		}, nil
	}
	return f.settings, nil
}

func (f *fakeUserSettingsService) UpdateSettings(_ context.Context, _ *service.AuthenticatedUser, input service.UpdateUserSettingsInput) (*service.UserSettingsResult, error) {
	f.lastUpdate = input
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.GetSettings(context.Background(), nil)
}

func (f *fakeUserSettingsService) GenerateSecret(context.Context, *service.AuthenticatedUser) (*service.BridgeSecretResult, error) {
	return f.secret, nil
}

func (f *fakeUserSettingsService) RotateSecret(context.Context, *service.AuthenticatedUser, string) (*service.BridgeSecretResult, error) {
	return f.secret, nil
}

func (f *fakeUserSettingsService) RevokeSecret(context.Context, *service.AuthenticatedUser, string) (*service.BridgeCredentialMetadata, error) {
	if f.secret == nil {
		return nil, errors.New("missing secret")
	}
	return &f.secret.Credential, nil
}

func (f *fakeUserSettingsService) RecordConnectorDownload(context.Context, *service.AuthenticatedUser, string, string, string) error {
	return nil
}
