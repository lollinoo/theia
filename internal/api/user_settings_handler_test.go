package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

type fakeUserSettingsService struct {
	settings  *service.UserSettingsResult
	secret    *service.BridgeSecretResult
	updateErr error
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

func (f *fakeUserSettingsService) UpdateSettings(context.Context, *service.AuthenticatedUser, service.UpdateUserSettingsInput) (*service.UserSettingsResult, error) {
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
