package api

// This file defines the runtime-only HTTP recovery boundary.

import (
	"encoding/json"
	"net/http"

	"github.com/lollinoo/theia/internal/ws"
)

// RuntimeOverviewHandler serves an atomic runtime snapshot without reloading structural topology.
type RuntimeOverviewHandler struct {
	runtimeStateFunc ws.RuntimeOverviewStateFunc
}

// NewRuntimeOverviewHandler constructs the compact runtime recovery endpoint.
func NewRuntimeOverviewHandler(runtimeStateFunc ws.RuntimeOverviewStateFunc) *RuntimeOverviewHandler {
	return &RuntimeOverviewHandler{runtimeStateFunc: runtimeStateFunc}
}

type runtimeOverviewResponse struct {
	SchemaVersion   int                 `json:"schema_version"`
	RuntimeStreamID string              `json:"runtime_stream_id"`
	RuntimeVersion  uint64              `json:"runtime_version"`
	RuntimeIdentity string              `json:"runtime_identity"`
	RuntimeSnapshot *ws.SnapshotPayload `json:"runtime_snapshot"`
}

// runtimeOverviewResponseFromState copies one atomic state tuple across the HTTP boundary.
func runtimeOverviewResponseFromState(state ws.RuntimeOverviewState) runtimeOverviewResponse {
	snapshot := ws.CloneSnapshot(state.Snapshot)
	return runtimeOverviewResponse{
		SchemaVersion:   1,
		RuntimeStreamID: state.StreamID,
		RuntimeVersion:  state.Version,
		RuntimeIdentity: ws.RuntimeIdentityForSnapshot(snapshot),
		RuntimeSnapshot: snapshot,
	}
}

// Handle serves GET and HEAD /api/v1/runtime/overview.
func (h *RuntimeOverviewHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.runtimeStateFunc == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime overview unavailable")
		return
	}

	response := runtimeOverviewResponseFromState(h.runtimeStateFunc())
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	json.NewEncoder(w).Encode(response)
}
