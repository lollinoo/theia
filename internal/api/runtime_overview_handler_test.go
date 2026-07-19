package api

// This file exercises the runtime-only recovery response boundary.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lollinoo/theia/internal/ws"
)

func TestRuntimeOverviewResponseFromStateReturnsExactClonedState(t *testing.T) {
	lastCollectedAt := "2026-07-14T12:00:00Z"
	cpuPercent := 17.5
	stale := false
	linkUtilization := 0.25
	snapshot := ws.EmptySnapshot()
	snapshot.Devices["device-1"] = ws.DeviceRuntimeDTO{
		DeviceID:        "device-1",
		RuntimeFlags:    []string{"metrics_stale"},
		FieldStates:     map[string]string{"cpu_percent": "stale"},
		LastCollectedAt: &lastCollectedAt,
		CPUPercent:      &cpuPercent,
		Stale:           &stale,
	}
	snapshot.Links["link-1"] = ws.LinkRuntimeDTO{
		LinkID:      "link-1",
		Utilization: &linkUtilization,
	}
	state := ws.RuntimeOverviewState{
		Snapshot: snapshot,
		Version:  42,
		StreamID: "runtime-stream-1",
	}

	response := runtimeOverviewResponseFromState(state)

	if response.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", response.SchemaVersion)
	}
	if response.RuntimeStreamID != state.StreamID {
		t.Fatalf("runtime_stream_id = %q, want %q", response.RuntimeStreamID, state.StreamID)
	}
	if response.RuntimeVersion != state.Version {
		t.Fatalf("runtime_version = %d, want %d", response.RuntimeVersion, state.Version)
	}
	if response.RuntimeSnapshot == snapshot {
		t.Fatal("runtime_snapshot aliases state snapshot")
	}

	lastCollectedAt = "source-mutated"
	cpuPercent = 99
	stale = true
	linkUtilization = 0.99
	responseDevice := response.RuntimeSnapshot.Devices["device-1"]
	responseLink := response.RuntimeSnapshot.Links["link-1"]
	if got := *responseDevice.LastCollectedAt; got != "2026-07-14T12:00:00Z" {
		t.Fatalf("response LastCollectedAt changed with source: %q", got)
	}
	if got := *responseDevice.CPUPercent; got != 17.5 {
		t.Fatalf("response CPUPercent changed with source: %v", got)
	}
	if got := *responseDevice.Stale; got {
		t.Fatal("response Stale changed with source")
	}
	if got := *responseLink.Utilization; got != 0.25 {
		t.Fatalf("response Utilization changed with source: %v", got)
	}
	if response.RuntimeIdentity != ws.RuntimeIdentityForSnapshot(response.RuntimeSnapshot) {
		t.Fatalf("runtime_identity = %q, want exact emitted snapshot identity", response.RuntimeIdentity)
	}

	clonedDevice := response.RuntimeSnapshot.Devices["device-1"]
	clonedDevice.RuntimeFlags[0] = "mutated"
	clonedDevice.FieldStates["cpu_percent"] = "mutated"
	if got := snapshot.Devices["device-1"].RuntimeFlags[0]; got != "metrics_stale" {
		t.Fatalf("state runtime flag mutated through response clone: %q", got)
	}
	if got := snapshot.Devices["device-1"].FieldStates["cpu_percent"]; got != "stale" {
		t.Fatalf("state field status mutated through response clone: %q", got)
	}
}

func TestRuntimeOverviewHandlerServesGETAndHEADWithoutCaching(t *testing.T) {
	calls := 0
	handler := NewRuntimeOverviewHandler(func() ws.RuntimeOverviewState {
		calls++
		return ws.RuntimeOverviewState{
			Snapshot: ws.EmptySnapshot(),
			Version:  7,
			StreamID: "runtime-stream-7",
		}
	})

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/overview", nil)
	getResponse := httptest.NewRecorder()
	handler.Handle(getResponse, getRequest)

	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body=%s", getResponse.Code, getResponse.Body.String())
	}
	if got := getResponse.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("GET Cache-Control = %q, want no-store", got)
	}
	var body runtimeOverviewResponse
	if err := json.NewDecoder(getResponse.Body).Decode(&body); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if body.RuntimeStreamID != "runtime-stream-7" || body.RuntimeVersion != 7 {
		t.Fatalf("GET runtime cursor = (%q, %d), want (runtime-stream-7, 7)", body.RuntimeStreamID, body.RuntimeVersion)
	}
	if calls != 1 {
		t.Fatalf("state callback calls after GET = %d, want 1", calls)
	}

	headRequest := httptest.NewRequest(http.MethodHead, "/api/v1/runtime/overview", nil)
	headResponse := httptest.NewRecorder()
	handler.Handle(headResponse, headRequest)

	if headResponse.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", headResponse.Code)
	}
	if headResponse.Body.Len() != 0 {
		t.Fatalf("HEAD body = %q, want empty", headResponse.Body.String())
	}
	if got := headResponse.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("HEAD Cache-Control = %q, want no-store", got)
	}
	if calls != 2 {
		t.Fatalf("state callback calls after HEAD = %d, want 2", calls)
	}
}

func TestRuntimeOverviewHandlerRejectsUnsupportedMethods(t *testing.T) {
	handler := NewRuntimeOverviewHandler(func() ws.RuntimeOverviewState {
		return ws.RuntimeOverviewState{Snapshot: ws.EmptySnapshot()}
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/overview", nil)
	response := httptest.NewRecorder()

	handler.Handle(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", response.Code)
	}
}
