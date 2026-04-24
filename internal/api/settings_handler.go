package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
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

// validSettingKeys is the allowlist of permitted setting keys. Unknown keys are rejected.
var validSettingKeys = map[string]bool{
	domain.SettingPrometheusURL:                 true,
	domain.SettingGrafanaURL:                    true,
	domain.SettingPollingInterval:               true,
	domain.SettingSNMPWorkerPoolSize:            true,
	domain.SettingSNMPWorkerPoolPerformance:     true,
	domain.SettingSNMPWorkerPoolOperational:     true,
	domain.SettingSNMPWorkerPoolStatic:          true,
	domain.SettingSNMPTimeout:                   true,
	domain.SettingSNMPRetries:                   true,
	domain.SettingPollingEssentialWorkers:       true,
	domain.SettingPollingMaxWorkersPerSite:      true,
	domain.SettingPollingMaxWorkersPerSubnet:    true,
	domain.SettingPollingMaxWorkersPerDevice:    true,
	domain.SettingPollingMaxInflightPerProfile:  true,
	domain.SettingPollingEssentialTimeoutMillis: true,
	domain.SettingPollingEssentialRetries:       true,
	domain.SettingPollingWebSocketCoalesceMS:    true,
	domain.SettingPollingPersistenceBatchMS:     true,
	domain.SettingPollingCapacitySafetyMargin:   true,
	domain.SettingPollingForceOverCapacity:      true,
	domain.SettingTimezone:                      true,
	domain.SettingTopologyDiscoveryDefaultMode:  true,
	domain.SettingInstanceBackupIntervalHours:   true,
	domain.SettingInstanceBackupRetentionCount:  true,
	domain.SettingDeviceBackupIntervalHours:     true,
	domain.SettingDeviceBackupRetentionCount:    true,
	domain.SettingBridgeSecret:                  true,
	domain.SettingBridgePort:                    true,
}

// numericSettings lists keys that must parse as valid integers.
var numericSettings = map[string]bool{
	domain.SettingPollingInterval:               true,
	domain.SettingSNMPWorkerPoolSize:            true,
	domain.SettingSNMPWorkerPoolPerformance:     true,
	domain.SettingSNMPWorkerPoolOperational:     true,
	domain.SettingSNMPWorkerPoolStatic:          true,
	domain.SettingSNMPTimeout:                   true,
	domain.SettingSNMPRetries:                   true,
	domain.SettingPollingEssentialWorkers:       true,
	domain.SettingPollingMaxWorkersPerSite:      true,
	domain.SettingPollingMaxWorkersPerSubnet:    true,
	domain.SettingPollingMaxWorkersPerDevice:    true,
	domain.SettingPollingMaxInflightPerProfile:  true,
	domain.SettingPollingEssentialTimeoutMillis: true,
	domain.SettingPollingEssentialRetries:       true,
	domain.SettingPollingWebSocketCoalesceMS:    true,
	domain.SettingPollingPersistenceBatchMS:     true,
	domain.SettingInstanceBackupRetentionCount:  true,
	domain.SettingDeviceBackupRetentionCount:    true,
	domain.SettingBridgePort:                    true,
}

// floatSettings lists keys that must parse as finite floats.
var floatSettings = map[string]bool{
	domain.SettingPollingCapacitySafetyMargin: true,
}

// boolSettings lists keys that must parse as valid booleans.
var boolSettings = map[string]bool{
	domain.SettingPollingForceOverCapacity: true,
}

// urlSettings lists keys that must be valid http/https URLs (or empty to clear).
var urlSettings = map[string]bool{
	domain.SettingPrometheusURL: true,
	domain.SettingGrafanaURL:    true,
}

// intervalSettings lists keys whose value must be one of the allowed interval hours.
var intervalSettings = map[string]bool{
	domain.SettingInstanceBackupIntervalHours: true,
	domain.SettingDeviceBackupIntervalHours:   true,
}

// validIntervalHours is the allowed set of backup interval hours.
var validIntervalHours = map[int]bool{0: true, 6: true, 12: true, 24: true, 48: true, 168: true}

// validateSetting validates that key is in the allowlist and value matches
// the expected type for that key. Returns nil if valid, error with specific message if not.
func validateSetting(key, value string) error {
	if !validSettingKeys[key] {
		return fmt.Errorf("unknown setting key: %s", key)
	}
	if numericSettings[key] {
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("%s must be a valid integer", key)
		}
	}
	if floatSettings[key] {
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf("%s must be a valid float", key)
		}
	}
	if boolSettings[key] {
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("%s must be a valid boolean", key)
		}
	}
	if urlSettings[key] && value != "" {
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%s must be a valid http/https URL", key)
		}
	}
	if key == domain.SettingTimezone && value != "" {
		if _, err := time.LoadLocation(value); err != nil {
			return fmt.Errorf("invalid timezone: %s", value)
		}
	}
	if key == domain.SettingTopologyDiscoveryDefaultMode {
		switch domain.TopologyDiscoveryMode(value) {
		case domain.TopologyDiscoveryModeOff,
			domain.TopologyDiscoveryModeLLDP,
			domain.TopologyDiscoveryModeLLDPCDP,
			domain.TopologyDiscoveryModeBootstrapOnce:
			// valid
		default:
			return fmt.Errorf("%s must be one of: off, lldp, lldp_cdp, bootstrap_once", key)
		}
	}
	if intervalSettings[key] {
		n, err := strconv.Atoi(value)
		if err != nil || !validIntervalHours[n] {
			return fmt.Errorf("%s must be one of: 0, 6, 12, 24, 48, 168", key)
		}
	}
	return nil
}

// HandleGetAll handles GET /api/v1/settings
func (h *SettingsHandler) HandleGetAll(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get settings", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": settings})
}

// HandleGet handles GET /api/v1/settings/{key}
func (h *SettingsHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/settings/")
	if key == "" {
		writeError(w, http.StatusBadRequest, "setting key is required")
		return
	}

	if !validSettingKeys[key] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown setting key: %s", key))
		return
	}

	value, err := h.repo.Get(key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get setting", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]string{key: value},
	})
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

	if err := validateSetting(key, req.Value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.repo.Set(key, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update setting", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]string{key: req.Value},
	})
}
