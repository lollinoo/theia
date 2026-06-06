package api

// This file exercises prometheus handler behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
)

func TestPrometheusHandlerHealth(t *testing.T) {
	// Spin up a fake Prometheus server that responds to /api/v1/query.
	fakeProm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result": []map[string]interface{}{
					{
						"metric": map[string]string{},
						"value":  []interface{}{1616000000, "1"},
					},
				},
			},
		})
	}))
	t.Cleanup(func() { fakeProm.Close() })

	repo := newMockSettingsRepo()
	repo.settings[domain.SettingPrometheusURL] = fakeProm.URL
	h := NewPrometheusHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prometheus/health", nil)
	rec := httptest.NewRecorder()

	h.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp prometheusHealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Available {
		t.Fatalf("expected available=true, got false; error=%s", resp.Error)
	}
	if !resp.Enabled {
		t.Fatal("expected enabled=true for configured Prometheus")
	}
	if resp.URL != fakeProm.URL {
		t.Fatalf("expected URL=%s, got %s", fakeProm.URL, resp.URL)
	}
}

func TestPrometheusHandlerHealth_NoURL(t *testing.T) {
	repo := newMockSettingsRepo()
	repo.settings[domain.SettingPrometheusURL] = ""
	h := NewPrometheusHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prometheus/health", nil)
	rec := httptest.NewRecorder()

	h.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp prometheusHealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Available {
		t.Fatal("expected available=false when no URL configured")
	}
	if resp.Enabled {
		t.Fatal("expected enabled=false when no URL configured")
	}
	if resp.Error != "" {
		t.Fatalf("expected empty error for disabled Prometheus, got %q", resp.Error)
	}
}

func TestPrometheusHandlerHealth_Unreachable(t *testing.T) {
	repo := newMockSettingsRepo()
	// Use a URL that won't be reachable
	repo.settings[domain.SettingPrometheusURL] = "http://127.0.0.1:1"
	h := NewPrometheusHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prometheus/health", nil)
	rec := httptest.NewRecorder()

	h.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp prometheusHealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Available {
		t.Fatal("expected available=false for unreachable Prometheus")
	}
	if !resp.Enabled {
		t.Fatal("expected enabled=true for configured-but-unreachable Prometheus")
	}
	if resp.Error == "" {
		t.Fatal("expected non-empty error message for unreachable Prometheus")
	}
}
