package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

// SettingsHandler provides HTTP handlers for runtime settings.
type SettingsHandler struct {
	repo domain.SettingsRepository
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(repo domain.SettingsRepository) *SettingsHandler {
	return &SettingsHandler{repo: repo}
}

// HandleGetAll handles GET /api/v1/settings
func (h *SettingsHandler) HandleGetAll(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": settings})
}

// HandleUpdate handles PUT /api/v1/settings/{key}
func (h *SettingsHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/settings/")
	if key == "" {
		writeError(w, http.StatusBadRequest, "setting key is required")
		return
	}

	var req struct {
		Value string `json:"value"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	// Validate timezone values
	if key == domain.SettingTimezone && req.Value != "" {
		if _, err := time.LoadLocation(req.Value); err != nil {
			writeError(w, http.StatusBadRequest, "invalid timezone: "+req.Value)
			return
		}
	}

	if err := h.repo.Set(key, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]string{key: req.Value},
	})
}
