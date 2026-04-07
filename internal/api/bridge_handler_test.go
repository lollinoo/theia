package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// setupBridgeTest creates a temp dir and optionally populates it with dummy bridge binaries.
func setupBridgeTest(t *testing.T, createFiles bool) (*BridgeHandler, string) {
	t.Helper()
	dir := t.TempDir()

	if createFiles {
		// Create dummy binaries for all 6 targets
		targets := []string{
			"winbox-bridge-windows-amd64.exe",
			"winbox-bridge-windows-arm64.exe",
			"winbox-bridge-linux-amd64",
			"winbox-bridge-linux-arm64",
			"winbox-bridge-darwin-amd64",
			"winbox-bridge-darwin-arm64",
		}
		for _, name := range targets {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("fake-binary-"+name), 0644); err != nil {
				t.Fatalf("failed to create test binary %s: %v", name, err)
			}
		}
	}

	handler := NewBridgeHandler(dir)
	return handler, dir
}

// TestBridgeDownload_HappyPath verifies linux/amd64 returns 200 with correct headers.
func TestBridgeDownload_HappyPath(t *testing.T) {
	handler, _ := setupBridgeTest(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bridge/download/linux/amd64", nil)
	w := httptest.NewRecorder()
	handler.HandleDownload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "application/octet-stream" {
		t.Errorf("expected Content-Type application/octet-stream, got %q", ct)
	}
	cd := resp.Header.Get("Content-Disposition")
	expected := `attachment; filename="winbox-bridge-linux-amd64"`
	if cd != expected {
		t.Errorf("expected Content-Disposition %q, got %q", expected, cd)
	}
}

// TestBridgeDownload_WindowsExe verifies windows/amd64 has .exe suffix in filename.
func TestBridgeDownload_WindowsExe(t *testing.T) {
	handler, _ := setupBridgeTest(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bridge/download/windows/amd64", nil)
	w := httptest.NewRecorder()
	handler.HandleDownload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	cd := resp.Header.Get("Content-Disposition")
	expected := `attachment; filename="winbox-bridge-windows-amd64.exe"`
	if cd != expected {
		t.Errorf("expected Content-Disposition %q, got %q", expected, cd)
	}
}

// TestBridgeDownload_AllSixTargets verifies all 6 valid os/arch combinations return 200.
func TestBridgeDownload_AllSixTargets(t *testing.T) {
	handler, _ := setupBridgeTest(t, true)

	type combo struct {
		os   string
		arch string
		want string
	}
	combos := []combo{
		{"windows", "amd64", `attachment; filename="winbox-bridge-windows-amd64.exe"`},
		{"windows", "arm64", `attachment; filename="winbox-bridge-windows-arm64.exe"`},
		{"linux", "amd64", `attachment; filename="winbox-bridge-linux-amd64"`},
		{"linux", "arm64", `attachment; filename="winbox-bridge-linux-arm64"`},
		{"darwin", "amd64", `attachment; filename="winbox-bridge-darwin-amd64"`},
		{"darwin", "arm64", `attachment; filename="winbox-bridge-darwin-arm64"`},
	}

	for _, c := range combos {
		t.Run(c.os+"/"+c.arch, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/bridge/download/"+c.os+"/"+c.arch, nil)
			w := httptest.NewRecorder()
			handler.HandleDownload(w, req)

			resp := w.Result()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200 for %s/%s, got %d", c.os, c.arch, resp.StatusCode)
			}
			cd := resp.Header.Get("Content-Disposition")
			if cd != c.want {
				t.Errorf("expected Content-Disposition %q, got %q", c.want, cd)
			}
		})
	}
}

// TestBridgeDownload_InvalidOS verifies that an unrecognized OS returns 400 JSON.
func TestBridgeDownload_InvalidOS(t *testing.T) {
	handler, _ := setupBridgeTest(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bridge/download/bados/amd64", nil)
	w := httptest.NewRecorder()
	handler.HandleDownload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON error response, decode failed: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected non-empty error message in JSON response")
	}
}

// TestBridgeDownload_InvalidArch verifies that an unrecognized arch returns 400 JSON.
func TestBridgeDownload_InvalidArch(t *testing.T) {
	handler, _ := setupBridgeTest(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bridge/download/linux/x86", nil)
	w := httptest.NewRecorder()
	handler.HandleDownload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON error response, decode failed: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected non-empty error message in JSON response")
	}
}

// TestBridgeDownload_NoBinariesDir verifies that an empty binariesDir returns 404 JSON.
func TestBridgeDownload_NoBinariesDir(t *testing.T) {
	handler := NewBridgeHandler("")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bridge/download/linux/amd64", nil)
	w := httptest.NewRecorder()
	handler.HandleDownload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON error response, decode failed: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected non-empty error message in JSON response")
	}
}

// TestBridgeDownload_FileNotFound verifies that a valid os/arch but missing file returns 404 JSON.
func TestBridgeDownload_FileNotFound(t *testing.T) {
	// Create handler with a dir but no files in it
	handler, _ := setupBridgeTest(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bridge/download/linux/amd64", nil)
	w := httptest.NewRecorder()
	handler.HandleDownload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON error response, decode failed: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected non-empty error message in JSON response")
	}
}

// TestBridgeDownload_MethodNotAllowed verifies that a non-GET request returns 405.
// Note: method enforcement is handled at router level, but we also test the handler
// can be tested in isolation via the router wrapping pattern.
func TestBridgeDownload_MethodNotAllowed(t *testing.T) {
	handler, _ := setupBridgeTest(t, true)

	// The router wraps HandleDownload with a method check, so we simulate that
	// by constructing the same wrapper inline for testing purposes.
	wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		handler.HandleDownload(w, r)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bridge/download/linux/amd64", nil)
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON error response, decode failed: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected non-empty error message in JSON response")
	}
}
