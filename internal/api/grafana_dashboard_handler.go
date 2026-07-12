package api

// This file defines grafana dashboard handler HTTP handler behavior and request/response boundaries.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// GrafanaDashboardHandler represents grafana dashboard handler data used by the HTTP boundary and route policy.
type GrafanaDashboardHandler struct {
	repo domain.AtomicSettingsRepository
}

// NewGrafanaDashboardHandler constructs grafana dashboard handler state for the HTTP boundary and route policy.
func NewGrafanaDashboardHandler(repo domain.AtomicSettingsRepository) *GrafanaDashboardHandler {
	return &GrafanaDashboardHandler{repo: repo}
}

type grafanaDashboardProfile struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	URLTemplate    string `json:"url_template"`
	VariableSource string `json:"variable_source"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

type grafanaDeviceDashboardOverride struct {
	ProfileID *string `json:"profile_id"`
	CustomURL string  `json:"custom_url"`
	UpdatedAt string  `json:"updated_at,omitempty"`
}

type grafanaDashboardConfig struct {
	Profiles         []grafanaDashboardProfile                 `json:"profiles"`
	DefaultProfileID string                                    `json:"default_profile_id"`
	DeviceOverrides  map[string]grafanaDeviceDashboardOverride `json:"device_overrides"`
}

type grafanaDashboardConfigResponse struct {
	Data grafanaDashboardConfig `json:"data"`
}

type grafanaDashboardProfileRequest struct {
	Name           string `json:"name"`
	URLTemplate    string `json:"url_template"`
	VariableSource string `json:"variable_source"`
	IsDefault      bool   `json:"is_default"`
}

type grafanaDeviceOverrideRequest struct {
	ProfileID *string `json:"profile_id"`
	CustomURL string  `json:"custom_url"`
}

var grafanaTemplatePlaceholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

var errGrafanaConfigUpdateRejected = errors.New("Grafana dashboard config update rejected")

var grafanaVariableSources = map[string]bool{
	"hostname": true,
	"ip":       true,
	"map_name": true,
	"map_id":   true,
}

func (h *GrafanaDashboardHandler) HandleProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		config, ok := h.loadConfig(w)
		if !ok {
			return
		}
		json.NewEncoder(w).Encode(grafanaDashboardConfigResponse{Data: config})
	case http.MethodPost:
		h.handleCreateProfile(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *GrafanaDashboardHandler) HandleProfile(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/grafana/dashboard-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	switch r.Method {
	case http.MethodPut:
		h.handleUpdateProfile(w, r, id.String())
	case http.MethodDelete:
		h.handleDeleteProfile(w, id.String())
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *GrafanaDashboardHandler) HandleDeviceOverride(w http.ResponseWriter, r *http.Request) {
	deviceID, err := extractIDFromPath(r.URL.Path, "/api/v1/grafana/device-overrides/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req grafanaDeviceOverrideRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	customURL := strings.TrimSpace(req.CustomURL)
	if customURL != "" {
		if err := validateGrafanaURL(customURL, "custom_url"); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	var profileID *string
	if req.ProfileID != nil && strings.TrimSpace(*req.ProfileID) != "" {
		rawProfileID := strings.TrimSpace(*req.ProfileID)
		if _, err := uuid.Parse(rawProfileID); err != nil {
			writeError(w, http.StatusBadRequest, "profile_id must be a valid UUID")
			return
		}
		profileID = &rawProfileID
	}

	config, ok := h.updateConfig(w, func(config *grafanaDashboardConfig) bool {
		if profileID != nil && !grafanaProfileExists(config.Profiles, *profileID) {
			writeError(w, http.StatusBadRequest, "profile_id does not match an existing Grafana dashboard profile")
			return false
		}
		if profileID == nil && customURL == "" {
			delete(config.DeviceOverrides, deviceID.String())
		} else {
			config.DeviceOverrides[deviceID.String()] = grafanaDeviceDashboardOverride{
				ProfileID: profileID,
				CustomURL: customURL,
				UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			}
		}
		return true
	})
	if !ok {
		return
	}
	writeGrafanaConfigResponse(w, http.StatusOK, config)
}

func (h *GrafanaDashboardHandler) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var req grafanaDashboardProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	profile, ok := buildGrafanaDashboardProfile(w, req, uuid.New().String(), "", "")
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	profile.CreatedAt = now
	profile.UpdatedAt = now
	config, ok := h.updateConfig(w, func(config *grafanaDashboardConfig) bool {
		if grafanaProfileNameExists(config.Profiles, profile.Name, "") {
			writeError(w, http.StatusConflict, "a Grafana dashboard profile with that name already exists")
			return false
		}
		config.Profiles = append(config.Profiles, profile)
		if req.IsDefault || config.DefaultProfileID == "" {
			config.DefaultProfileID = profile.ID
		}
		return true
	})
	if !ok {
		return
	}
	writeGrafanaConfigResponse(w, http.StatusCreated, config)
}

func (h *GrafanaDashboardHandler) handleUpdateProfile(w http.ResponseWriter, r *http.Request, profileID string) {
	var req grafanaDashboardProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	config, ok := h.updateConfig(w, func(config *grafanaDashboardConfig) bool {
		index := grafanaProfileIndex(config.Profiles, profileID)
		if index < 0 {
			writeError(w, http.StatusNotFound, "Grafana dashboard profile not found")
			return false
		}
		current := config.Profiles[index]
		profile, valid := buildGrafanaDashboardProfile(w, req, profileID, current.CreatedAt, time.Now().UTC().Format(time.RFC3339Nano))
		if !valid {
			return false
		}
		if grafanaProfileNameExists(config.Profiles, profile.Name, profileID) {
			writeError(w, http.StatusConflict, "a Grafana dashboard profile with that name already exists")
			return false
		}
		config.Profiles[index] = profile
		if req.IsDefault {
			config.DefaultProfileID = profileID
		} else if config.DefaultProfileID == profileID {
			config.DefaultProfileID = ""
		}
		return true
	})
	if !ok {
		return
	}
	writeGrafanaConfigResponse(w, http.StatusOK, config)
}

func (h *GrafanaDashboardHandler) handleDeleteProfile(w http.ResponseWriter, profileID string) {
	_, ok := h.updateConfig(w, func(config *grafanaDashboardConfig) bool {
		index := grafanaProfileIndex(config.Profiles, profileID)
		if index < 0 {
			writeError(w, http.StatusNotFound, "Grafana dashboard profile not found")
			return false
		}
		config.Profiles = append(config.Profiles[:index], config.Profiles[index+1:]...)
		if config.DefaultProfileID == profileID {
			config.DefaultProfileID = ""
		}
		for deviceID, override := range config.DeviceOverrides {
			if override.ProfileID != nil && *override.ProfileID == profileID {
				override.ProfileID = nil
				if override.CustomURL == "" {
					delete(config.DeviceOverrides, deviceID)
				} else {
					config.DeviceOverrides[deviceID] = override
				}
			}
		}
		return true
	})
	if !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func buildGrafanaDashboardProfile(
	w http.ResponseWriter,
	req grafanaDashboardProfileRequest,
	id string,
	createdAt string,
	updatedAt string,
) (grafanaDashboardProfile, bool) {
	name := strings.TrimSpace(req.Name)
	urlTemplate := strings.TrimSpace(req.URLTemplate)
	variableSource := strings.TrimSpace(req.VariableSource)
	if variableSource == "" {
		variableSource = "hostname"
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return grafanaDashboardProfile{}, false
	}
	if len(name) > 120 {
		writeError(w, http.StatusBadRequest, "name too long (max 120 characters)")
		return grafanaDashboardProfile{}, false
	}
	if !grafanaVariableSources[variableSource] {
		writeError(w, http.StatusBadRequest, "variable_source must be one of: hostname, ip, map_name, map_id")
		return grafanaDashboardProfile{}, false
	}
	if err := validateGrafanaTemplateURL(urlTemplate); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return grafanaDashboardProfile{}, false
	}
	return grafanaDashboardProfile{
		ID:             id,
		Name:           name,
		URLTemplate:    urlTemplate,
		VariableSource: variableSource,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}, true
}

func (h *GrafanaDashboardHandler) loadConfig(w http.ResponseWriter) (grafanaDashboardConfig, bool) {
	config, err := h.readConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load Grafana dashboard config", err)
		return grafanaDashboardConfig{}, false
	}
	return config, true
}

func (h *GrafanaDashboardHandler) readConfig() (grafanaDashboardConfig, error) {
	raw, err := h.repo.Get(domain.SettingGrafanaDashboardConfig)
	if err != nil {
		raw = "{}"
	}
	return h.decodeConfig(raw)
}

func (h *GrafanaDashboardHandler) decodeConfig(raw string) (grafanaDashboardConfig, error) {
	var config grafanaDashboardConfig
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			return grafanaDashboardConfig{}, fmt.Errorf("decoding Grafana dashboard config: %w", err)
		}
	}
	if config.Profiles == nil {
		config.Profiles = []grafanaDashboardProfile{}
	}
	if config.DeviceOverrides == nil {
		config.DeviceOverrides = map[string]grafanaDeviceDashboardOverride{}
	}
	h.mergeLegacyDeviceURLs(&config)
	if config.DefaultProfileID != "" && !grafanaProfileExists(config.Profiles, config.DefaultProfileID) {
		config.DefaultProfileID = ""
	}
	return config, nil
}

func (h *GrafanaDashboardHandler) mergeLegacyDeviceURLs(config *grafanaDashboardConfig) {
	settings, err := h.repo.GetAll()
	if err != nil {
		return
	}
	for key, value := range settings {
		if !strings.HasPrefix(key, domain.SettingGrafanaLegacyDeviceURLPrefix) || strings.TrimSpace(value) == "" {
			continue
		}
		deviceID := strings.TrimPrefix(key, domain.SettingGrafanaLegacyDeviceURLPrefix)
		if _, err := uuid.Parse(deviceID); err != nil {
			continue
		}
		if _, ok := config.DeviceOverrides[deviceID]; ok {
			continue
		}
		config.DeviceOverrides[deviceID] = grafanaDeviceDashboardOverride{CustomURL: strings.TrimSpace(value)}
	}
}

func writeGrafanaConfigResponse(w http.ResponseWriter, status int, config grafanaDashboardConfig) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(grafanaDashboardConfigResponse{Data: config})
}

func (h *GrafanaDashboardHandler) updateConfig(
	w http.ResponseWriter,
	update func(config *grafanaDashboardConfig) bool,
) (grafanaDashboardConfig, bool) {
	var updated grafanaDashboardConfig
	_, err := h.repo.Update(domain.SettingGrafanaDashboardConfig, func(raw string) (string, error) {
		config, err := h.decodeConfig(raw)
		if err != nil {
			return "", err
		}
		if !update(&config) {
			return "", errGrafanaConfigUpdateRejected
		}
		payload, err := json.Marshal(config)
		if err != nil {
			return "", fmt.Errorf("encoding Grafana dashboard config: %w", err)
		}
		updated = config
		return string(payload), nil
	})
	if err != nil {
		if errors.Is(err, errGrafanaConfigUpdateRejected) {
			return grafanaDashboardConfig{}, false
		}
		writeError(w, http.StatusInternalServerError, "failed to save Grafana dashboard config", err)
		return grafanaDashboardConfig{}, false
	}
	return updated, true
}

func grafanaProfileIndex(profiles []grafanaDashboardProfile, profileID string) int {
	for i, profile := range profiles {
		if profile.ID == profileID {
			return i
		}
	}
	return -1
}

func grafanaProfileExists(profiles []grafanaDashboardProfile, profileID string) bool {
	return grafanaProfileIndex(profiles, profileID) >= 0
}

func grafanaProfileNameExists(profiles []grafanaDashboardProfile, name string, exceptID string) bool {
	for _, profile := range profiles {
		if profile.ID != exceptID && strings.EqualFold(profile.Name, name) {
			return true
		}
	}
	return false
}

func validateGrafanaTemplateURL(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("url_template is required")
	}
	replaced := grafanaTemplatePlaceholderPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := grafanaTemplatePlaceholderPattern.FindStringSubmatch(match)
		if len(parts) != 2 || !grafanaVariableSources[parts[1]] {
			return match
		}
		return "theia-placeholder"
	})
	if strings.Contains(replaced, "{{") || strings.Contains(replaced, "}}") {
		return fmt.Errorf("url_template contains an unsupported placeholder")
	}
	return validateGrafanaURL(replaced, "url_template")
}

func validateGrafanaURL(value string, field string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%s must be a valid http/https URL", field)
	}
	return nil
}
