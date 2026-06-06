package api

// This file defines area handler HTTP handler behavior and request/response boundaries.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

// AreaHandler provides HTTP handlers for area CRUD operations.
type AreaHandler struct {
	repo domain.AreaRepository
}

// NewAreaHandler creates a new AreaHandler.
func NewAreaHandler(repo domain.AreaRepository) *AreaHandler {
	return &AreaHandler{repo: repo}
}

// --- Request/Response types ---

type areaRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

type areaResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	DeviceCount int    `json:"device_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// HandleList handles GET /api/v1/areas
func (h *AreaHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	areas, err := h.repo.GetAllWithDeviceCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list areas", err)
		return
	}

	resp := make([]areaResponse, 0, len(areas))
	for i := range areas {
		resp = append(resp, areaToResponse(&areas[i].Area, areas[i].DeviceCount))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": resp})
}

// HandleCreate handles POST /api/v1/areas
func (h *AreaHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req areaRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(name) > 100 {
		writeError(w, http.StatusBadRequest, "area name too long (max 100 characters)")
		return
	}

	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "#00E676" // default color
	}
	if !strings.HasPrefix(color, "#") || len(color) != 7 {
		writeError(w, http.StatusBadRequest, "invalid color format (must be #RRGGBB)")
		return
	}

	area := &domain.Area{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Color:       color,
	}

	if err := h.repo.Create(area); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "an area with that name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create area", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": areaToResponse(area, 0)})
}

// HandleGet handles GET /api/v1/areas/{id}
func (h *AreaHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/areas/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid area ID")
		return
	}

	area, err := h.repo.GetByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get area", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": areaToResponse(area, 0)})
}

// HandleUpdate handles PUT /api/v1/areas/{id}
func (h *AreaHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/areas/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid area ID")
		return
	}

	var req areaRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(name) > 100 {
		writeError(w, http.StatusBadRequest, "area name too long (max 100 characters)")
		return
	}

	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "#00E676"
	}
	if !strings.HasPrefix(color, "#") || len(color) != 7 {
		writeError(w, http.StatusBadRequest, "invalid color format (must be #RRGGBB)")
		return
	}

	area, err := h.repo.GetByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get area", err)
		return
	}

	area.Name = name
	area.Description = strings.TrimSpace(req.Description)
	area.Color = color

	if err := h.repo.Update(area); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "an area with that name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update area", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": areaToResponse(area, 0)})
}

// HandleDelete handles DELETE /api/v1/areas/{id}
func (h *AreaHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/areas/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid area ID")
		return
	}

	if err := h.repo.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete area", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func areaToResponse(a *domain.Area, deviceCount int) areaResponse {
	return areaResponse{
		ID:          a.ID.String(),
		Name:        a.Name,
		Description: a.Description,
		Color:       a.Color,
		DeviceCount: deviceCount,
		CreatedAt:   formatAreaTimestamp(a.CreatedAt),
		UpdatedAt:   formatAreaTimestamp(a.UpdatedAt),
	}
}

func formatAreaTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
