package api

// This file exposes actor-scoped controls for durable one-time topology imports.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

const deviceImportTopologyRunPrefix = "/api/v1/admin/device-imports/topology-runs/"

type deviceImportTopologyProvider interface {
	GetRun(context.Context, uuid.UUID, uuid.UUID) (domain.DeviceImportTopologyRunSnapshot, error)
	GetActiveRun(context.Context, uuid.UUID, uuid.UUID) (domain.DeviceImportTopologyRunSnapshot, error)
	Retry(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) error
	Continue(context.Context, uuid.UUID, uuid.UUID) error
	MarkManualEdit(context.Context, uuid.UUID, uuid.UUID) error
	ApplyLayout(context.Context, uuid.UUID, uuid.UUID, domain.DeviceImportTopologyLayoutApply) error
}

// DeviceImportTopologyHandler serves resumable progress and safe operator controls.
type DeviceImportTopologyHandler struct {
	topology deviceImportTopologyProvider
	auth     authProvider
}

// NewDeviceImportTopologyHandler creates the topology import control boundary.
func NewDeviceImportTopologyHandler(
	topology deviceImportTopologyProvider,
	auth authProvider,
) *DeviceImportTopologyHandler {
	return &DeviceImportTopologyHandler{topology: topology, auth: auth}
}

// ServeHTTP dispatches exact topology-run routes under the Admin Area import surface.
func (h *DeviceImportTopologyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	actor, ok := authorizeDeviceImportBaseline(w, r, h.auth)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionCredentialsRead) {
		return
	}
	if h == nil || h.topology == nil {
		writeError(w, http.StatusServiceUnavailable, "device import topology service unavailable")
		return
	}

	if r.URL.Path == deviceImportTopologyRunPrefix+"active" {
		h.handleActive(w, r, actor.User.User.ID)
		return
	}
	suffix, found := strings.CutPrefix(r.URL.Path, deviceImportTopologyRunPrefix)
	if !found || suffix == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	parts := strings.Split(suffix, "/")
	if len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	runID, err := uuid.Parse(parts[0])
	if err != nil || runID == uuid.Nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if len(parts) == 1 {
		h.handleGet(w, r, runID, actor.User.User.ID)
		return
	}
	h.handleAction(w, r, runID, actor.User.User.ID, parts[1])
}

func (h *DeviceImportTopologyHandler) handleActive(
	w http.ResponseWriter,
	r *http.Request,
	actorUserID uuid.UUID,
) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	mapID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("map_id")))
	if err != nil || mapID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "invalid map_id")
		return
	}
	snapshot, err := h.topology.GetActiveRun(r.Context(), mapID, actorUserID)
	if err != nil {
		writeDeviceImportTopologyError(w, err)
		return
	}
	json.NewEncoder(w).Encode(snapshot)
}

func (h *DeviceImportTopologyHandler) handleGet(
	w http.ResponseWriter,
	r *http.Request,
	runID, actorUserID uuid.UUID,
) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snapshot, err := h.topology.GetRun(r.Context(), runID, actorUserID)
	if err != nil {
		writeDeviceImportTopologyError(w, err)
		return
	}
	json.NewEncoder(w).Encode(snapshot)
}

func (h *DeviceImportTopologyHandler) handleAction(
	w http.ResponseWriter,
	r *http.Request,
	runID, actorUserID uuid.UUID,
	action string,
) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var err error
	switch action {
	case "retry":
		var body struct {
			DeviceIDs []uuid.UUID `json:"device_ids"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		err = h.topology.Retry(r.Context(), runID, actorUserID, body.DeviceIDs)
	case "continue":
		err = h.topology.Continue(r.Context(), runID, actorUserID)
	case "manual-edit":
		err = h.topology.MarkManualEdit(r.Context(), runID, actorUserID)
	case "layout":
		var body domain.DeviceImportTopologyLayoutApply
		if !decodeJSON(w, r, &body) {
			return
		}
		err = h.topology.ApplyLayout(r.Context(), runID, actorUserID, body)
	default:
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeDeviceImportTopologyError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeDeviceImportTopologyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrDeviceImportTopologyRunNotFound):
		writeError(w, http.StatusNotFound, "device import topology run not found")
	case errors.Is(err, domain.ErrDeviceImportTopologyRunConflict),
		errors.Is(err, domain.ErrDeviceImportTopologyLayoutStale):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrDeviceImportStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, domain.ErrDeviceImportStoreUnavailable.Error())
	default:
		writeDeviceImportInternalError(w, nil)
	}
}
