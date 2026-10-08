package api

import (
	"encoding/json"
	"github.com/lollinoo/theia/internal/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsHandlerRetiresUnusedPersistenceBatch(t *testing.T) {
	repo := newMockSettingsRepo()
	key := domain.SettingPollingPersistenceBatchMS
	repo.settings[key] = "1500"
	handler := NewSettingsHandler(repo)
	all := httptest.NewRecorder()
	handler.HandleGetAll(all, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	var response settingsResponse
	if err := json.Unmarshal(all.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if _, ok := response.Data[key]; ok {
		t.Fatal("obsolete stored setting is still advertised")
	}
	if _, ok := domain.DefaultSettings()[key]; ok {
		t.Fatal("obsolete setting is still seeded")
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, "/api/v1/settings/"+key, strings.NewReader(`{"value":"2000"}`))
		if method == http.MethodGet {
			handler.HandleGet(recorder, request)
		} else {
			handler.HandleUpdate(recorder, request)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s obsolete setting = %d, want 400", method, recorder.Code)
		}
	}
	if repo.settings[key] != "1500" {
		t.Fatal("retirement mutated the old stored value")
	}
}
