package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// BridgeHandler provides the bridge binary download endpoint.
type BridgeHandler struct {
	binariesDir string
}

// NewBridgeHandler creates a new BridgeHandler.
// binariesDir is the directory containing pre-built bridge binaries.
// If empty, all download requests return 404.
func NewBridgeHandler(binariesDir string) *BridgeHandler {
	return &BridgeHandler{binariesDir: binariesDir}
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
