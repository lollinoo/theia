package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/service"
)

// BridgeHandler provides the bridge binary download and credential token endpoints.
type BridgeHandler struct {
	binariesDir           string
	backupSvc             *service.BackupService
	credentialProfileRepo *sqlite.CredentialProfileRepo
}

// NewBridgeHandler creates a new BridgeHandler.
// binariesDir is the directory containing pre-built bridge binaries.
// If empty, all download requests return 404.
// backupSvc and credentialProfileRepo are used by HandleBridgeToken to resolve
// WinBox credentials; pass nil to disable the token endpoint (returns 503).
func NewBridgeHandler(binariesDir string) *BridgeHandler {
	return &BridgeHandler{binariesDir: binariesDir}
}

// NewBridgeHandlerWithCredentials creates a BridgeHandler that also supports
// the POST /api/v1/bridge/token/{deviceId} credential encryption endpoint.
func NewBridgeHandlerWithCredentials(
	binariesDir string,
	backupSvc *service.BackupService,
	credentialProfileRepo *sqlite.CredentialProfileRepo,
) *BridgeHandler {
	return &BridgeHandler{
		binariesDir:           binariesDir,
		backupSvc:             backupSvc,
		credentialProfileRepo: credentialProfileRepo,
	}
}

// HandleDownload handles GET /api/v1/bridge/download/{os}/{arch}.
// Streams the bridge binary with Content-Disposition and Content-Type headers.
// Valid os: windows, linux, darwin. Valid arch: amd64, arm64.
func (h *BridgeHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/bridge/download/")
	suffix = strings.TrimSuffix(suffix, "/")
	parts := strings.Split(suffix, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path: expected /api/v1/bridge/download/{os}/{arch}")
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

// HandleBridgeToken handles POST /api/v1/bridge/token/{deviceId}.
// Resolves the WinBox credentials for the device, encrypts them with the
// bridge secret supplied in the request body, and returns the hex-encoded
// AES-GCM ciphertext.  The bridge binary decrypts the token on POST /launch.
//
// Request body: {"bridge_secret": "<64-hex-char key>"}
// Response:     {"token": "<hex-encoded AES-GCM ciphertext>"}
//
// The bridge secret is a 32-byte key (64 hex chars) stored in the bridge's
// config.json (~/.config/winbox-bridge/config.json).  It is never stored on
// the Theia backend — it is sent per-request and used only for one-time
// encryption of a short-lived credential bundle.
func (h *BridgeHandler) HandleBridgeToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.backupSvc == nil || h.credentialProfileRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "bridge token endpoint not configured")
		return
	}

	// Extract device ID from path: /api/v1/bridge/token/{deviceId}
	deviceID, err := extractIDFromPath(r.URL.Path, "/api/v1/bridge/token/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	var body struct {
		BridgeSecret string `json:"bridge_secret"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.BridgeSecret == "" {
		writeError(w, http.StatusBadRequest, "bridge_secret is required")
		return
	}
	keyBytes, err := hex.DecodeString(body.BridgeSecret)
	if err != nil || len(keyBytes) != 32 {
		writeError(w, http.StatusBadRequest, "bridge_secret must be a 64-character hex string (32 bytes)")
		return
	}

	// Resolve WinBox credentials for the device
	assignment, err := h.credentialProfileRepo.GetWinboxAssignment(deviceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no WinBox profile designated")
		return
	}

	ip, password, err := h.backupSvc.GetWinboxCredentials(deviceID, assignment.EncryptedSecret, assignment.Username)
	if err != nil {
		if strings.Contains(err.Error(), "no password") {
			writeError(w, http.StatusUnprocessableEntity, "WinBox profile has no password configured")
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	// Build plaintext credential bundle
	payload := map[string]string{
		"ip":       ip,
		"username": assignment.Username,
		"password": password,
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	// Encrypt with AES-256-GCM using the bridge secret
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	json.NewEncoder(w).Encode(map[string]string{
		"token": hex.EncodeToString(ciphertext),
	})
}
