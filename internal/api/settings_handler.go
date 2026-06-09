package api

// This file defines settings handler HTTP handler behavior and request/response boundaries.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
)

// SettingsHandler provides HTTP handlers for runtime settings.
type SettingsHandler struct {
	repo domain.SettingsRepository
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(repo domain.SettingsRepository) *SettingsHandler {
	return &SettingsHandler{repo: repo}
}

type settingSecretState struct {
	Present  bool `json:"present"`
	Redacted bool `json:"redacted"`
}

type settingsResponseMeta struct {
	Secrets map[string]settingSecretState `json:"secrets,omitempty"`
}

type settingsResponse struct {
	Data map[string]string     `json:"data"`
	Meta *settingsResponseMeta `json:"meta,omitempty"`
}

// validSettingKeys is the allowlist of permitted setting keys. Unknown keys are rejected.
var validSettingKeys = map[string]bool{
	domain.SettingPrometheusURL:                 true,
	domain.SettingGrafanaURL:                    true,
	domain.SettingGrafanaDashboardConfig:        true,
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
	domain.SettingBridgePort:                    true,
	domain.SettingNetworkProbePorts:             true,
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
// the expected type for that key. It returns the normalized value to persist.
func validateSetting(key, value string) (string, error) {
	if !isValidSettingKey(key) {
		return "", fmt.Errorf("unknown setting key: %s", key)
	}
	normalized, err := domain.NormalizeConstrainedSetting(key, value)
	if err != nil {
		return "", err
	}
	if key == domain.SettingNetworkProbePorts {
		ports, err := domain.ParseProbePortsCSV(value)
		if err != nil {
			return "", fmt.Errorf("%s invalid: %w", key, err)
		}
		if len(ports) == 0 {
			return "", fmt.Errorf("%s must contain at least one probe port", key)
		}
		normalized = domain.FormatProbePortsCSV(ports)
	}
	if boolSettings[key] {
		trimmed := strings.TrimSpace(value)
		if _, err := strconv.ParseBool(trimmed); err != nil {
			return "", fmt.Errorf("%s must be a valid boolean", key)
		}
		normalized = trimmed
	}
	if urlSettings[key] && value != "" {
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", fmt.Errorf("%s must be a valid http/https URL", key)
		}
	}
	if isLegacyGrafanaDeviceURLSetting(key) && value != "" {
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", fmt.Errorf("%s must be a valid http/https URL", key)
		}
	}
	if key == domain.SettingTimezone && value != "" {
		if _, err := time.LoadLocation(value); err != nil {
			return "", fmt.Errorf("invalid timezone: %s", value)
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
			return "", fmt.Errorf("%s must be one of: off, lldp, lldp_cdp, bootstrap_once", key)
		}
	}
	if intervalSettings[key] {
		trimmed := strings.TrimSpace(value)
		n, err := strconv.Atoi(trimmed)
		if err != nil || !validIntervalHours[n] {
			return "", fmt.Errorf("%s must be one of: 0, 6, 12, 24, 48, 168", key)
		}
		normalized = trimmed
	}
	return normalized, nil
}

func isValidSettingKey(key string) bool {
	return validSettingKeys[key] || isLegacyGrafanaDeviceURLSetting(key)
}

func isLegacyGrafanaDeviceURLSetting(key string) bool {
	if !strings.HasPrefix(key, domain.SettingGrafanaLegacyDeviceURLPrefix) {
		return false
	}
	deviceID := strings.TrimPrefix(key, domain.SettingGrafanaLegacyDeviceURLPrefix)
	_, err := uuid.Parse(deviceID)
	return err == nil
}

// HandleGetAll handles GET /api/v1/settings
func (h *SettingsHandler) HandleGetAll(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get settings", err)
		return
	}

	json.NewEncoder(w).Encode(buildSettingsResponse(settings))
}

// HandleGet handles GET /api/v1/settings/{key}
func (h *SettingsHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/settings/")
	if key == "" {
		writeError(w, http.StatusBadRequest, "setting key is required")
		return
	}

	if !isValidSettingKey(key) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown setting key: %s", key))
		return
	}

	value, err := h.repo.Get(key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get setting", err)
		return
	}

	if settingResponseSensitive(key) {
		json.NewEncoder(w).Encode(buildSettingsResponse(map[string]string{key: value}))
		return
	}

	json.NewEncoder(w).Encode(settingsResponse{Data: map[string]string{key: value}})
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

	value, err := validateSetting(key, req.Value)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	previous, previousErr := h.repo.Get(key)
	if err := h.repo.Set(key, value); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update setting", err)
		return
	}
	logging.Debugf(
		"settings changed key=%s previous=%s new=%s affects=%s",
		key,
		debugSettingValue(key, previous, previousErr),
		debugSettingValue(key, value, nil),
		debugSettingAffects(key),
	)

	if settingResponseSensitive(key) {
		json.NewEncoder(w).Encode(buildSettingsResponse(map[string]string{key: value}))
		return
	}

	json.NewEncoder(w).Encode(settingsResponse{Data: map[string]string{key: value}})
}

func buildSettingsResponse(settings map[string]string) settingsResponse {
	data := make(map[string]string, len(settings))
	secrets := make(map[string]settingSecretState)
	for key, value := range settings {
		if !isValidSettingKey(key) {
			continue
		}
		if settingResponseSensitive(key) {
			secrets[key] = settingSecretState{
				Present:  strings.TrimSpace(value) != "",
				Redacted: true,
			}
			continue
		}
		data[key] = value
	}

	if len(secrets) == 0 {
		return settingsResponse{Data: data}
	}
	return settingsResponse{
		Data: data,
		Meta: &settingsResponseMeta{Secrets: secrets},
	}
}

func settingResponseSensitive(key string) bool {
	return false
}

func debugSettingValue(key string, value string, err error) string {
	if err != nil {
		return "<unavailable>"
	}
	value = strings.TrimSpace(value)
	if debugSettingSensitive(key) {
		if value == "" {
			return "<empty>"
		}
		return "<set>"
	}
	if value == "" {
		return "<empty>"
	}
	return value
}

func debugSettingSensitive(key string) bool {
	switch key {
	case domain.SettingPrometheusURL, domain.SettingGrafanaURL:
		return true
	default:
		return false
	}
}

func debugSettingAffects(key string) string {
	switch key {
	case domain.SettingPrometheusURL:
		return "prometheus"
	case domain.SettingGrafanaURL:
		return "grafana"
	case domain.SettingTopologyDiscoveryDefaultMode:
		return "topology"
	case domain.SettingInstanceBackupIntervalHours,
		domain.SettingInstanceBackupRetentionCount,
		domain.SettingDeviceBackupIntervalHours,
		domain.SettingDeviceBackupRetentionCount:
		return "backup"
	case domain.SettingBridgePort:
		return "bridge"
	case domain.SettingPollingInterval,
		domain.SettingSNMPWorkerPoolSize,
		domain.SettingSNMPWorkerPoolPerformance,
		domain.SettingSNMPWorkerPoolOperational,
		domain.SettingSNMPWorkerPoolStatic,
		domain.SettingSNMPTimeout,
		domain.SettingSNMPRetries,
		domain.SettingPollingEssentialWorkers,
		domain.SettingPollingMaxWorkersPerSite,
		domain.SettingPollingMaxWorkersPerSubnet,
		domain.SettingPollingMaxWorkersPerDevice,
		domain.SettingPollingMaxInflightPerProfile,
		domain.SettingPollingEssentialTimeoutMillis,
		domain.SettingPollingEssentialRetries,
		domain.SettingPollingWebSocketCoalesceMS,
		domain.SettingPollingPersistenceBatchMS,
		domain.SettingPollingCapacitySafetyMargin,
		domain.SettingPollingForceOverCapacity:
		return "polling"
	default:
		return "runtime"
	}
}
