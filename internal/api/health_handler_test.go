package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lollinoo/theia/internal/polling"
)

type fakeHealthPoller struct{}

func (fakeHealthPoller) Status() string {
	return "running"
}

func (fakeHealthPoller) PollingHealth() polling.HealthSnapshot {
	return polling.HealthSnapshot{ConfiguredWorkers: 2, ActiveWorkers: 1}
}

func TestHealthHandlerOmitsBuildVersionMetadataAndReportsEnvironment(t *testing.T) {
	handler := NewHealthHandler(nil, fakeHealthPoller{}, "staging")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	handler.HandleHealth(rec, req)

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal health payload: %v", err)
	}
	if _, ok := payload["version"]; ok {
		t.Fatalf("health payload exposed version metadata: %#v", payload["version"])
	}
	if _, ok := payload["git_commit"]; ok {
		t.Fatalf("health payload exposed git commit: %#v", payload["git_commit"])
	}
	if _, ok := payload["build_date"]; ok {
		t.Fatalf("health payload exposed build date: %#v", payload["build_date"])
	}
	if got := payload["environment"]; got != "staging" {
		t.Fatalf("environment = %#v, want staging", got)
	}
}
