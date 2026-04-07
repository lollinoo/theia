package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/vendor"
)

// VendorHandler provides HTTP handlers for vendor configuration management.
type VendorHandler struct {
	registry        *vendor.Registry
	vendorConfigRepo domain.VendorConfigRepository
}

// NewVendorHandler creates a new VendorHandler.
func NewVendorHandler(registry *vendor.Registry, vendorConfigRepo domain.VendorConfigRepository) *VendorHandler {
	return &VendorHandler{
		registry:        registry,
		vendorConfigRepo: vendorConfigRepo,
	}
}

// HandleListVendors handles GET /api/v1/vendors
func (h *VendorHandler) HandleListVendors(w http.ResponseWriter, r *http.Request) {
	configs, err := h.registry.ExportAllConfigs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list vendors", err)
		return
	}

	type vendorEntry struct {
		Name        string          `json:"name"`
		DisplayName string          `json:"display_name"`
		Config      json.RawMessage `json:"config"`
	}

	var data []vendorEntry
	for _, name := range h.registry.GetAllVendorNames() {
		cfg := configs[name]
		data = append(data, vendorEntry{
			Name:        name,
			DisplayName: h.registry.GetDisplayName(name),
			Config:      cfg,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

// HandleGetVendor handles GET /api/v1/vendors/{name}
func (h *VendorHandler) HandleGetVendor(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/vendors/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "vendor name required")
		return
	}

	cfg, err := h.registry.ExportConfig(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"name":         name,
			"display_name": h.registry.GetDisplayName(name),
			"config":       json.RawMessage(cfg),
		},
	})
}

// HandleUpdateVendor handles PUT /api/v1/vendors/{name}
func (h *VendorHandler) HandleUpdateVendor(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/vendors/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "vendor name required")
		return
	}

	var body json.RawMessage
	if !decodeJSON(w, r, &body) {
		return
	}

	// Update the in-memory registry
	if err := h.registry.UpdateConfig(name, []byte(body)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Persist to DB
	now := time.Now().UTC()
	displayName := h.registry.GetDisplayName(name)
	record := &domain.VendorConfigRecord{
		Name:        name,
		DisplayName: displayName,
		ConfigJSON:  string(body),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.vendorConfigRepo.Upsert(record); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist vendor config", err)
		return
	}

	// Return the updated config
	cfg, _ := h.registry.ExportConfig(name)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"name":         name,
			"display_name": displayName,
			"config":       json.RawMessage(cfg),
		},
	})
}
