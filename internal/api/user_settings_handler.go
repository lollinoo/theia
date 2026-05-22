package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/service"
)

type userSettingsService interface {
	GetSettings(context.Context, *service.AuthenticatedUser) (*service.UserSettingsResult, error)
	UpdateSettings(context.Context, *service.AuthenticatedUser, service.UpdateUserSettingsInput) (*service.UserSettingsResult, error)
	GenerateSecret(context.Context, *service.AuthenticatedUser) (*service.BridgeSecretResult, error)
	RotateSecret(context.Context, *service.AuthenticatedUser, string) (*service.BridgeSecretResult, error)
	RevokeSecret(context.Context, *service.AuthenticatedUser, string) (*service.BridgeCredentialMetadata, error)
	RecordConnectorDownload(context.Context, *service.AuthenticatedUser, string, string, string) error
}

type UserSettingsHandler struct {
	service     userSettingsService
	binariesDir string
}

func NewUserSettingsHandler(service userSettingsService, binariesDir string) *UserSettingsHandler {
	return &UserSettingsHandler{service: service, binariesDir: binariesDir}
}

func (h *UserSettingsHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthenticatedUser(w, r, "settings")
	if !ok {
		return
	}
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "user settings service not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.service.GetSettings(r.Context(), user)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		json.NewEncoder(w).Encode(result)
	case http.MethodPatch:
		input, ok := decodeUserSettingsPatch(w, r)
		if !ok {
			return
		}
		result, err := h.service.UpdateSettings(r.Context(), user, input)
		if err != nil {
			writeUserSettingsServiceError(w, err)
			return
		}
		json.NewEncoder(w).Encode(result)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *UserSettingsHandler) HandleBridge(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthenticatedUser(w, r, "settings")
	if !ok {
		return
	}
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "user settings service not configured")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := h.service.GetSettings(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	json.NewEncoder(w).Encode(result.Bridge)
}

func (h *UserSettingsHandler) HandleBridgeSecret(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthenticatedUser(w, r, "settings")
	if !ok {
		return
	}
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "user settings service not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var result *service.BridgeSecretResult
	var err error
	switch r.URL.Path {
	case "/api/v1/settings/bridge/secret":
		result, err = h.service.GenerateSecret(r.Context(), user)
	case "/api/v1/settings/bridge/secret/rotate":
		var req struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		result, err = h.service.RotateSecret(r.Context(), user, req.Reason)
	default:
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeUserSettingsServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

func (h *UserSettingsHandler) HandleBridgeSecretRevoke(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthenticatedUser(w, r, "settings")
	if !ok {
		return
	}
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "user settings service not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	result, err := h.service.RevokeSecret(r.Context(), user, req.Reason)
	if err != nil {
		writeUserSettingsServiceError(w, err)
		return
	}
	json.NewEncoder(w).Encode(result)
}

func (h *UserSettingsHandler) HandleConnectorConfig(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthenticatedUser(w, r, "settings")
	if !ok {
		return
	}
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "user settings service not configured")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := h.service.GetSettings(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	baseURL := originBaseURL(r)
	const downloadPrefix = "/api/v1/settings/bridge/connector/download/"
	config := map[string]any{
		"winbox_path":    "",
		"listen_port":    result.Preferences.BridgePort,
		"theia_origin":   baseURL,
		"theia_base_url": baseURL,
		"bridge_secret":  "<paste-secret-shown-once>",
		"log_level":      "info",
	}
	json.NewEncoder(w).Encode(map[string]any{
		"config":    config,
		"downloads": bridgeConnectorDownloadTargets(h.binariesDir, downloadPrefix),
	})
}

func (h *UserSettingsHandler) HandleConnectorDownload(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthenticatedUser(w, r, "settings")
	if !ok {
		return
	}
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "user settings service not configured")
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/settings/bridge/connector/download/")
	parts := strings.Split(strings.TrimSuffix(suffix, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path: expected /api/v1/settings/bridge/connector/download/{os}/{arch}")
		return
	}
	settings, err := h.service.GetSettings(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if !bridgeCredentialDownloadReady(settings.Bridge.Credential) {
		writeError(w, http.StatusUnprocessableEntity, "bridge secret not configured")
		return
	}
	_ = h.service.RecordConnectorDownload(r.Context(), user, parts[0]+"/"+parts[1], clientIPAddress(r), r.UserAgent())
	bridge := NewBridgeHandler(h.binariesDir)
	bridge.HandleDownloadWithPrefix(w, r, "/api/v1/settings/bridge/connector/download/")
}

func bridgeCredentialDownloadReady(credential *service.BridgeCredentialMetadata) bool {
	if credential == nil || credential.Status != "active" || credential.RevokedAt != nil {
		return false
	}
	return credential.ExpiresAt == nil || time.Now().UTC().Before(*credential.ExpiresAt)
}

func decodeUserSettingsPatch(w http.ResponseWriter, r *http.Request) (service.UpdateUserSettingsInput, bool) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return service.UpdateUserSettingsInput{}, false
	}
	for key := range raw {
		if !allowedUserSettingsPatchField(key) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("field %q cannot be updated from user settings", key))
			return service.UpdateUserSettingsInput{}, false
		}
	}
	var input service.UpdateUserSettingsInput
	if value, ok := raw["display_name"]; ok {
		var parsed string
		if err := json.Unmarshal(value, &parsed); err != nil {
			writeError(w, http.StatusBadRequest, "display_name must be a string")
			return input, false
		}
		input.DisplayName = &parsed
	}
	if value, ok := raw["username"]; ok {
		var parsed string
		if err := json.Unmarshal(value, &parsed); err != nil {
			writeError(w, http.StatusBadRequest, "username must be a string")
			return input, false
		}
		input.Username = &parsed
	}
	if value, ok := raw["email"]; ok {
		var parsed string
		if err := json.Unmarshal(value, &parsed); err != nil {
			writeError(w, http.StatusBadRequest, "email must be a string")
			return input, false
		}
		input.Email = &parsed
	}
	if value, ok := raw["timezone"]; ok {
		var parsed string
		if err := json.Unmarshal(value, &parsed); err != nil {
			writeError(w, http.StatusBadRequest, "timezone must be a string")
			return input, false
		}
		input.Timezone = &parsed
	}
	if value, ok := raw["locale"]; ok {
		var parsed string
		if err := json.Unmarshal(value, &parsed); err != nil {
			writeError(w, http.StatusBadRequest, "locale must be a string")
			return input, false
		}
		input.Locale = &parsed
	}
	if value, ok := raw["bridge_port"]; ok {
		var parsed int
		if err := json.Unmarshal(value, &parsed); err != nil {
			writeError(w, http.StatusBadRequest, "bridge_port must be an integer")
			return input, false
		}
		input.BridgePort = &parsed
	}
	return input, true
}

func allowedUserSettingsPatchField(key string) bool {
	switch key {
	case "display_name", "username", "email", "timezone", "locale", "bridge_port":
		return true
	default:
		return false
	}
}

func writeUserSettingsServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidUserSettings):
		writeError(w, http.StatusBadRequest, "invalid user settings")
	case errors.Is(err, service.ErrDuplicateUserIdentifier):
		writeError(w, http.StatusConflict, "username or email already exists")
	case errors.Is(err, service.ErrBridgeSecretAlreadyConfigured):
		writeError(w, http.StatusConflict, "bridge secret already configured")
	case errors.Is(err, service.ErrBridgeCredentialNotConfigured):
		writeError(w, http.StatusUnprocessableEntity, "bridge secret not configured")
	default:
		writeError(w, http.StatusInternalServerError, "internal error", err)
	}
}

func originBaseURL(r *http.Request) string {
	if origin := sanitizeRequestBaseURL(r.Header.Get("Origin")); origin != "" {
		return origin
	}
	if forwarded := baseURLFromForwardedHeaders(r); forwarded != "" {
		return forwarded
	}
	if referer := sanitizeRequestBaseURL(r.Header.Get("Referer")); referer != "" {
		return referer
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return baseURLFromSchemeHost(scheme, r.Host)
}

func baseURLFromForwardedHeaders(r *http.Request) string {
	if baseURL := baseURLFromSchemeHost(
		firstHeaderValue(r.Header.Get("X-Forwarded-Proto")),
		firstHeaderValue(r.Header.Get("X-Forwarded-Host")),
	); baseURL != "" {
		return baseURL
	}
	return baseURLFromForwardedHeader(r.Header.Get("Forwarded"))
}

func baseURLFromForwardedHeader(value string) string {
	first := firstHeaderValue(value)
	if first == "" {
		return ""
	}
	var proto, host string
	for _, part := range strings.Split(first, ";") {
		key, rawValue, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		cleanValue := strings.Trim(strings.TrimSpace(rawValue), `"`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "proto":
			proto = cleanValue
		case "host":
			host = cleanValue
		}
	}
	return baseURLFromSchemeHost(proto, host)
}

func baseURLFromSchemeHost(scheme, host string) string {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	host = strings.TrimSpace(host)
	if scheme == "" {
		scheme = "http"
	}
	if host == "" {
		return ""
	}
	return sanitizeRequestBaseURL(scheme + "://" + host)
}

func firstHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	first, _, _ := strings.Cut(value, ",")
	return strings.TrimSpace(first)
}

func sanitizeRequestBaseURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
