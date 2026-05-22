package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/service"
)

type bridgeLaunchService interface {
	CreateLaunchRequest(context.Context, *service.AuthenticatedUser, uuid.UUID) (*service.BridgeLaunchRequestResult, error)
	ResolveConnectorLaunch(context.Context, string, string, string, string) (*service.BridgeLaunchCredentials, error)
}

// BridgeHandler provides bridge binary download and connector launch endpoints.
type BridgeHandler struct {
	binariesDir string
	service     bridgeLaunchService
	limiter     *bridgeRateLimiter
}

// NewBridgeHandler creates a new BridgeHandler.
// binariesDir is the directory containing pre-built bridge binaries.
// If empty, all download requests return 404.
func NewBridgeHandler(binariesDir string) *BridgeHandler {
	return &BridgeHandler{binariesDir: binariesDir, limiter: newBridgeRateLimiter(20, 0, nil)}
}

func NewBridgeHandlerWithService(binariesDir string, bridgeService bridgeLaunchService) *BridgeHandler {
	return &BridgeHandler{binariesDir: binariesDir, service: bridgeService, limiter: newBridgeRateLimiter(20, 0, nil)}
}

// HandleDownload handles GET /api/v1/bridge/download/{os}/{arch}.
// Streams the bridge binary with Content-Disposition and Content-Type headers.
// Valid os: windows, linux, darwin. Valid arch: amd64, arm64.
func (h *BridgeHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	h.HandleDownloadWithPrefix(w, r, "/api/v1/bridge/download/")
}

func (h *BridgeHandler) HandleDownloadWithPrefix(w http.ResponseWriter, r *http.Request, prefix string) {
	suffix := strings.TrimPrefix(r.URL.Path, prefix)
	suffix = strings.TrimSuffix(suffix, "/")
	parts := strings.Split(suffix, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path: expected "+prefix+"{os}/{arch}")
		return
	}
	osName, arch := parts[0], parts[1]

	validOS := map[string]bool{"windows": true, "linux": true, "darwin": true}
	validArch := map[string]bool{"amd64": true, "arm64": true}
	if !validOS[osName] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unrecognized os: %s (valid: windows, linux, darwin)", osName))
		return
	}
	if !validArch[arch] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unrecognized arch: %s (valid: amd64, arm64)", arch))
		return
	}

	if h.binariesDir == "" {
		writeError(w, http.StatusNotFound, "bridge binary not available for this platform")
		return
	}

	filename := fmt.Sprintf("winbox-bridge-%s-%s", osName, arch)
	if osName == "windows" {
		filename += ".exe"
	}
	filePath := filepath.Join(h.binariesDir, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "bridge binary not available for this platform")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, filePath)
}

// HandleBridgeToken handles the deprecated global-secret token endpoint.
func (h *BridgeHandler) HandleBridgeToken(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusGone, "bridge token endpoint is deprecated; use /api/v1/bridge/launch-requests/{deviceId}")
}

func (h *BridgeHandler) HandleCreateLaunchRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, ok := requireAuthenticatedUser(w, r, "bridge launch request")
	if !ok {
		return
	}
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "bridge service not configured")
		return
	}
	deviceID, err := extractBridgeLaunchDeviceID(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}
	result, err := h.service.CreateLaunchRequest(r.Context(), user, deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	json.NewEncoder(w).Encode(result)
	log.Printf("bridge launch request issued user_id=%s device_id=%s outcome=success", user.User.User.ID, deviceID)
}

func (h *BridgeHandler) HandleConnectorLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "bridge service not configured")
		return
	}
	rawSecret, ok := bridgeAuthorizationSecret(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "bridge secret required")
		return
	}
	if h.limiter != nil && !h.limiter.allow(bridgeRateLimitKey(r, rawSecret)) {
		writeError(w, http.StatusTooManyRequests, "too many bridge authentication attempts")
		return
	}
	var req struct {
		LaunchToken string `json:"launch_token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.LaunchToken) == "" {
		writeError(w, http.StatusBadRequest, "launch_token is required")
		return
	}
	result, err := h.service.ResolveConnectorLaunch(r.Context(), rawSecret, req.LaunchToken, clientIPAddress(r), r.UserAgent())
	if err != nil {
		writeBridgeConnectorError(w, err)
		return
	}
	json.NewEncoder(w).Encode(result)
}

func bridgeRateLimitKey(r *http.Request, rawSecret string) string {
	prefix, _, ok := strings.Cut(strings.TrimSpace(rawSecret), ".")
	if !ok || prefix == "" {
		prefix = "unknown"
	}
	return clientIPAddress(r) + "|" + prefix
}

func extractBridgeLaunchDeviceID(path string) (uuid.UUID, error) {
	const prefix = "/api/v1/bridge/launch-requests/"
	if !strings.HasPrefix(path, prefix) {
		return uuid.Nil, fmt.Errorf("invalid bridge launch path")
	}
	idPart := strings.TrimPrefix(path, prefix)
	if idPart == "" || strings.Contains(idPart, "/") {
		return uuid.Nil, fmt.Errorf("invalid bridge launch path")
	}
	return uuid.Parse(idPart)
}

func bridgeAuthorizationSecret(r *http.Request) (string, bool) {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bridge "
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	secret := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	return secret, secret != ""
}

func writeBridgeConnectorError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrBridgeSecretInvalid),
		errors.Is(err, service.ErrBridgeCredentialRevoked),
		errors.Is(err, service.ErrBridgeCredentialExpired):
		writeError(w, http.StatusUnauthorized, "bridge secret invalid")
	case errors.Is(err, service.ErrBridgeLaunchUserMismatch):
		writeError(w, http.StatusForbidden, "bridge launch token belongs to a different user")
	case errors.Is(err, service.ErrBridgeLaunchTokenUsed):
		writeError(w, http.StatusConflict, "bridge launch token already used")
	case errors.Is(err, service.ErrBridgeLaunchTokenExpired):
		writeError(w, http.StatusGone, "bridge launch token expired")
	case errors.Is(err, service.ErrBridgeLaunchTokenInvalid):
		writeError(w, http.StatusUnauthorized, "bridge launch token invalid")
	default:
		if strings.Contains(err.Error(), "no WinBox profile") {
			writeError(w, http.StatusNotFound, "no WinBox profile designated")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
	}
}
