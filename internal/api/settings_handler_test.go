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
