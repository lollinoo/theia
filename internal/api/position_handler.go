package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// PositionHandler provides HTTP handlers for device position persistence.
type PositionHandler struct {
	repo            domain.PositionRepository
	mapRepo         domain.CanvasMapRepository
	mapPositionRepo domain.CanvasMapPositionRepository
}

// NewPositionHandler creates a new PositionHandler.
func NewPositionHandler(repo domain.PositionRepository, optionalRepos ...any) *PositionHandler {
	handler := &PositionHandler{repo: repo}
	for _, optional := range optionalRepos {
		switch typed := optional.(type) {
		case domain.CanvasMapRepository:
			handler.mapRepo = typed
		case domain.CanvasMapPositionRepository:
			handler.mapPositionRepo = typed
		}
	}
	return handler
}

type bulkPositionsRequest struct {
	Positions []positionPayload `json:"positions"`
}

type positionPayload struct {
	DeviceID string  `json:"device_id"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Pinned   bool    `json:"pinned"`
}

// HandleList handles GET /api/v1/positions.
func (h *PositionHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if h.mapRepo != nil && h.mapPositionRepo != nil {
		defaultMap, err := h.mapRepo.GetDefault()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		positions, err := h.mapPositionRepo.GetAllForMap(defaultMap.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"data": positions})
		return
	}

	positions, err := h.repo.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": positions})
}

// HandleSaveAll handles PUT /api/v1/positions.
func (h *PositionHandler) HandleSaveAll(w http.ResponseWriter, r *http.Request) {
	var req bulkPositionsRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	positions := make([]domain.DevicePosition, 0, len(req.Positions))
	for _, position := range req.Positions {
		deviceID, err := uuid.Parse(position.DeviceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid device_id")
			return
		}

		positions = append(positions, domain.DevicePosition{
			DeviceID: deviceID,
			X:        position.X,
			Y:        position.Y,
			Pinned:   position.Pinned,
		})
	}

	for _, pos := range positions {
		if math.IsNaN(pos.X) || math.IsInf(pos.X, 0) ||
			math.IsNaN(pos.Y) || math.IsInf(pos.Y, 0) {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("invalid coordinate for device %s: NaN and Infinity are not allowed", pos.DeviceID))
			return
		}
	}

	if h.mapRepo != nil && h.mapPositionRepo != nil {
		defaultMap, err := h.mapRepo.GetDefault()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		if err := h.mapPositionRepo.SaveAllForMap(defaultMap.ID, positions); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
	}

	if err := h.repo.SaveAll(positions); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"count":  len(positions),
	})
}
