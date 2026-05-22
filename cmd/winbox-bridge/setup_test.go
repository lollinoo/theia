package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeAutostartProvider struct {
	mu      sync.Mutex
	enabled bool
	set     []bool
}

func (p *fakeAutostartProvider) Enabled() (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.enabled, nil
}

func (p *fakeAutostartProvider) SetEnabled(enabled bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = enabled
	p.set = append(p.set, enabled)
	return nil
}

func (p *fakeAutostartProvider) setCalls() []bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]bool(nil), p.set...)
}

func setupTestHandler(t *testing.T, cfg Config, opts setupOptions) (http.Handler, *Config) {
	t.Helper()
	saved := cfg
	if opts.LoadConfig == nil {
		opts.LoadConfig = func() (Config, error) { return saved, nil }
	}
	if opts.SaveConfig == nil {
		opts.SaveConfig = func(next Config) error {
			saved = next
			return nil
		}
	}
	if opts.ConfigPath == nil {
		opts.ConfigPath = func() (string, error) {
			return filepath.Join(t.TempDir(), "config.json"), nil
		}
	}
	if opts.Autostart == nil {
		opts.Autostart = &fakeAutostartProvider{}
	}
	if opts.PickWinBoxPath == nil {
		opts.PickWinBoxPath = func() (string, error) { return `C:\WinBox\winbox64.exe`, nil }
	}
	h := buildMuxWithSetup("/fake/winbox", cfg.TheiaOrigin, "localhost:1337", &TheiaClient{
		BaseURL: cfg.TheiaBaseURL,
		Secret:  cfg.BridgeSecret,
	}, opts)
	return h, &saved
}

func setupRequest(t *testing.T, h http.Handler, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := makeRequest(t, method, path, body, "", "localhost:1337")
	req.RemoteAddr = "127.0.0.1:49152"
	if token != "" {
		req.Header.Set("X-Setup-Token", token)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func setupToken(t *testing.T, h http.Handler) string {
	t.Helper()
	rr := setupRequest(t, h, http.MethodGet, "/setup", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /setup status = %d; body: %s", rr.Code, rr.Body.String())
	}
	match := regexp.MustCompile(`setupToken = "([^"]+)"`).FindStringSubmatch(rr.Body.String())
	if len(match) != 2 {
		t.Fatalf("setup token not found in HTML")
	}
	return match[1]
}

func TestSetupStatusRedactsBridgeSecret(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WinBoxPath = `C:\Tools\winbox64.exe`
	cfg.BridgeSecret = "theia_bridge_public.saved-secret"
	cfg.LogLevel = "debug"
	cfg.TheiaOrigin = "http://theia.local:3000"
	cfg.TheiaBaseURL = "http://theia.local:8080"
	autostart := &fakeAutostartProvider{enabled: true}
	h, _ := setupTestHandler(t, cfg, setupOptions{
		LogPath:   func() string { return filepath.Join(t.TempDir(), "winbox-bridge.log") },
		Autostart: autostart,
	})

	rr := setupRequest(t, h, http.MethodGet, "/setup/status", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rr.Code, rr.Body.String())
	}

	var got map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if _, ok := got["bridge_secret"]; ok {
		t.Fatalf("status leaked bridge_secret")
	}
	if got["bridge_secret_configured"] != true {
		t.Fatalf("bridge_secret_configured = %v", got["bridge_secret_configured"])
	}
	if got["winbox_path"] != cfg.WinBoxPath || got["theia_origin"] != cfg.TheiaOrigin ||
		got["theia_base_url"] != cfg.TheiaBaseURL || got["log_level"] != cfg.LogLevel ||
		got["autostart_enabled"] != true {
		t.Fatalf("unexpected status body")
	}
}

func TestSetupMutationsRequireToken(t *testing.T) {
	cfg := DefaultConfig()
	h, _ := setupTestHandler(t, cfg, setupOptions{})

	rr := setupRequest(t, h, http.MethodPost, "/setup/config", map[string]interface{}{
		"winbox_path":    "/tmp/winbox",
		"listen_port":    1777,
		"theia_origin":   "http://localhost:3000",
		"theia_base_url": "http://localhost:3000",
		"bridge_secret":  "secret",
		"log_level":      "debug",
	}, "")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("POST /setup/config without token status = %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestSetupConfigSavesTrimmedValuesAndReportsRestartRequired(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenPort = 1337
	cfg.TheiaOrigin = "http://localhost:3000"
	cfg.TheiaBaseURL = "http://localhost:3000"
	h, saved := setupTestHandler(t, cfg, setupOptions{})
	token := setupToken(t, h)

	rr := setupRequest(t, h, http.MethodPost, "/setup/config", map[string]interface{}{
		"winbox_path":    "  /opt/winbox  ",
		"listen_port":    1444,
		"theia_origin":   "  http://localhost:5173  ",
		"theia_base_url": "  http://localhost:8080  ",
		"bridge_secret":  "  new-secret  ",
		"log_level":      "  debug  ",
	}, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /setup/config status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got["ok"] || !got["restart_required"] {
		t.Fatalf("response = %#v", got)
	}
	if saved.WinBoxPath != "/opt/winbox" || saved.ListenPort != 1444 ||
		saved.TheiaOrigin != "http://localhost:5173" ||
		saved.TheiaBaseURL != "http://localhost:8080" ||
		saved.BridgeSecret != "new-secret" || saved.LogLevel != "debug" {
		t.Fatalf("saved config not trimmed/applied")
	}
}

func TestSetupConfigPreservesExistingSecretWhenSubmittedSecretEmpty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BridgeSecret = "theia_bridge_public.existing-secret"
	h, saved := setupTestHandler(t, cfg, setupOptions{})
	token := setupToken(t, h)

	rr := setupRequest(t, h, http.MethodPost, "/setup/config", map[string]interface{}{
		"winbox_path":    cfg.WinBoxPath,
		"listen_port":    cfg.ListenPort,
		"theia_origin":   cfg.TheiaOrigin,
		"theia_base_url": cfg.TheiaBaseURL,
		"bridge_secret":  "   ",
		"log_level":      cfg.LogLevel,
	}, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /setup/config status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["restart_required"] {
		t.Fatalf("empty bridge secret unexpectedly required restart")
	}
	if saved.BridgeSecret != cfg.BridgeSecret {
		t.Fatalf("bridge secret was not preserved")
	}
}

func TestSetupConfigWinBoxPathOnlyChangeRequiresRestart(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WinBoxPath = "/old/winbox"
	cfg.BridgeSecret = "existing-secret"
	h, _ := setupTestHandler(t, cfg, setupOptions{})
	token := setupToken(t, h)

	rr := setupRequest(t, h, http.MethodPost, "/setup/config", map[string]interface{}{
		"winbox_path":    "/new/winbox",
		"listen_port":    cfg.ListenPort,
		"theia_origin":   cfg.TheiaOrigin,
		"theia_base_url": cfg.TheiaBaseURL,
		"bridge_secret":  "",
		"log_level":      cfg.LogLevel,
	}, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /setup/config status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got["restart_required"] {
		t.Fatalf("winbox_path change did not require restart")
	}
}

func TestSetupConfigBridgeSecretOnlyChangeRequiresRestart(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BridgeSecret = "existing-secret"
	h, _ := setupTestHandler(t, cfg, setupOptions{})
	token := setupToken(t, h)

	rr := setupRequest(t, h, http.MethodPost, "/setup/config", map[string]interface{}{
		"winbox_path":    cfg.WinBoxPath,
		"listen_port":    cfg.ListenPort,
		"theia_origin":   cfg.TheiaOrigin,
		"theia_base_url": cfg.TheiaBaseURL,
		"bridge_secret":  "new-secret",
		"log_level":      cfg.LogLevel,
	}, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /setup/config status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got["restart_required"] {
		t.Fatalf("bridge_secret change did not require restart")
	}
}

func TestSetupConfigLogLevelOnlyChangeRequiresRestart(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LogLevel = "info"
	h, _ := setupTestHandler(t, cfg, setupOptions{})
	token := setupToken(t, h)

	rr := setupRequest(t, h, http.MethodPost, "/setup/config", map[string]interface{}{
		"winbox_path":    cfg.WinBoxPath,
		"listen_port":    cfg.ListenPort,
		"theia_origin":   cfg.TheiaOrigin,
		"theia_base_url": cfg.TheiaBaseURL,
		"bridge_secret":  "",
		"log_level":      "debug",
	}, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /setup/config status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got["restart_required"] {
		t.Fatalf("log_level change did not require restart")
	}
}

func TestSetupWinBoxSelectUsesInjectedPicker(t *testing.T) {
	cfg := DefaultConfig()
	h, _ := setupTestHandler(t, cfg, setupOptions{
		PickWinBoxPath: func() (string, error) { return "/selected/winbox", nil },
	})
	token := setupToken(t, h)

	rr := setupRequest(t, h, http.MethodPost, "/setup/winbox/select", nil, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /setup/winbox/select status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["winbox_path"] != "/selected/winbox" {
		t.Fatalf("winbox_path = %q", got["winbox_path"])
	}
}

func TestSetupAutostartEndpointsUseInjectedProvider(t *testing.T) {
	cfg := DefaultConfig()
	autostart := &fakeAutostartProvider{enabled: true}
	h, _ := setupTestHandler(t, cfg, setupOptions{Autostart: autostart})

	rr := setupRequest(t, h, http.MethodGet, "/setup/autostart", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /setup/autostart status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var getResp map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if !getResp["enabled"] {
		t.Fatalf("enabled = false, want true")
	}

	token := setupToken(t, h)
	rr = setupRequest(t, h, http.MethodPost, "/setup/autostart", map[string]bool{"enabled": false}, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /setup/autostart status = %d; body: %s", rr.Code, rr.Body.String())
	}
	if calls := autostart.setCalls(); len(calls) != 1 || calls[0] {
		t.Fatalf("SetEnabled calls = %#v", calls)
	}
}

func TestSetupHTMLContainsFieldsAndDoesNotLeakSavedSecret(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BridgeSecret = "theia_bridge_public.saved-secret"
	h, _ := setupTestHandler(t, cfg, setupOptions{})

	rr := setupRequest(t, h, http.MethodGet, "/setup", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /setup status = %d; body: %s", rr.Code, rr.Body.String())
	}
	html := rr.Body.String()
	for _, field := range []string{
		"winbox_path",
		"bridge_secret",
		"theia_base_url",
		"theia_origin",
		"listen_port",
		"log_level",
		"autostart_enabled",
	} {
		if !strings.Contains(html, field) {
			t.Fatalf("HTML missing field/control name %q", field)
		}
	}
	if strings.Contains(html, cfg.BridgeSecret) {
		t.Fatalf("HTML leaked saved bridge secret")
	}
}

func TestSetupHTMLSetsAntiFramingHeaders(t *testing.T) {
	cfg := DefaultConfig()
	h, _ := setupTestHandler(t, cfg, setupOptions{})

	rr := setupRequest(t, h, http.MethodGet, "/setup", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /setup status = %d; body: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q", got)
	}
	if got := rr.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
		t.Fatalf("Content-Security-Policy = %q", got)
	}
}

func TestSetupTokenGenerationFailureClosesSetupEndpoints(t *testing.T) {
	cfg := DefaultConfig()
	h, _ := setupTestHandler(t, cfg, setupOptions{
		NewSetupToken: func() (string, error) {
			return "", errors.New("entropy unavailable")
		},
	})

	for _, path := range []string{"/setup", "/setup/status"} {
		rr := setupRequest(t, h, http.MethodGet, path, nil, "")
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("GET %s status = %d; body: %s", path, rr.Code, rr.Body.String())
		}
	}
}

func TestSetupRejectsNonLoopbackHost(t *testing.T) {
	cfg := DefaultConfig()
	h, _ := setupTestHandler(t, cfg, setupOptions{})

	req := makeRequest(t, http.MethodGet, "/setup/status", nil, "", "evil.example:1337")
	req.RemoteAddr = "127.0.0.1:49152"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("GET /setup/status with non-loopback Host status = %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestSetupRejectsNonLoopbackRemoteAddrWithLocalhostHost(t *testing.T) {
	cfg := DefaultConfig()
	h, _ := setupTestHandler(t, cfg, setupOptions{})

	req := makeRequest(t, http.MethodGet, "/setup/status", nil, "", "localhost:1337")
	req.RemoteAddr = "203.0.113.10:49152"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("GET /setup/status with non-loopback RemoteAddr status = %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestSetupAllowsLoopbackRemoteAddrWithLocalhostHost(t *testing.T) {
	cfg := DefaultConfig()
	h, _ := setupTestHandler(t, cfg, setupOptions{})

	req := makeRequest(t, http.MethodGet, "/setup/status", nil, "", "localhost:1337")
	req.RemoteAddr = "[::1]:49152"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /setup/status with loopback RemoteAddr status = %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestSetupStatusUsesReturnedConfigWhenLoadConfigWarns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WinBoxPath = "/repair/winbox"
	h, _ := setupTestHandler(t, cfg, setupOptions{
		LoadConfig: func() (Config, error) {
			return cfg, errors.New("parse config failed")
		},
	})

	rr := setupRequest(t, h, http.MethodGet, "/setup/status", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /setup/status status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if got["winbox_path"] != cfg.WinBoxPath {
		t.Fatalf("status did not use returned config")
	}
}

func TestSetupConfigCanSaveWhenLoadConfigWarns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BridgeSecret = "existing-secret"
	h, saved := setupTestHandler(t, cfg, setupOptions{
		LoadConfig: func() (Config, error) {
			return cfg, errors.New("parse config failed")
		},
	})
	token := setupToken(t, h)

	rr := setupRequest(t, h, http.MethodPost, "/setup/config", map[string]interface{}{
		"winbox_path":    "/repaired/winbox",
		"listen_port":    cfg.ListenPort,
		"theia_origin":   cfg.TheiaOrigin,
		"theia_base_url": cfg.TheiaBaseURL,
		"bridge_secret":  "",
		"log_level":      cfg.LogLevel,
	}, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /setup/config status = %d; body: %s", rr.Code, rr.Body.String())
	}
	if saved.WinBoxPath != "/repaired/winbox" {
		t.Fatalf("config was not saved after load warning")
	}
}

func TestAutostartWindowsDisableIgnoresMissingRunValue(t *testing.T) {
	var calls []string
	err := setWindowsAutostart(false, `C:\Program Files\Theia\winbox-bridge.exe`, func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return []byte("The system was unable to find the specified registry key or value."), errors.New("exit status 1")
	})
	if err != nil {
		t.Fatalf("disable returned error for missing Run value")
	}
	if len(calls) != 1 || !strings.Contains(calls[0], "delete") {
		t.Fatalf("unexpected command call for disable")
	}
}

func TestAutostartWindowsEnableQuotesRunCommand(t *testing.T) {
	var gotArgs []string
	err := setWindowsAutostart(true, `C:\Program Files\Theia\winbox-bridge.exe`, func(name string, args ...string) ([]byte, error) {
		gotArgs = append([]string(nil), args...)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("enable returned error: %v", err)
	}
	for i, arg := range gotArgs {
		if arg == "/d" && i+1 < len(gotArgs) {
			if gotArgs[i+1] != `"C:\Program Files\Theia\winbox-bridge.exe"` {
				t.Fatalf("Run command value was not quoted")
			}
			return
		}
	}
	t.Fatalf("Run command value argument not found")
}

func TestLinuxAutostartExecEscapesSpacesAndPercents(t *testing.T) {
	got := linuxDesktopExecValue(`/opt/Theia Bridge/winbox%bridge`)
	want := `"/opt/Theia Bridge/winbox%%bridge"`
	if got != want {
		t.Fatalf("escaped Exec value mismatch")
	}
}

func TestSetupRestartSchedulesInjectedRestartAfterResponse(t *testing.T) {
	cfg := DefaultConfig()
	restarted := make(chan struct{}, 1)
	h, _ := setupTestHandler(t, cfg, setupOptions{
		Restart: func() { restarted <- struct{}{} },
	})
	token := setupToken(t, h)

	rr := setupRequest(t, h, http.MethodPost, "/setup/restart", nil, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /setup/restart status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got["ok"] {
		t.Fatalf("response = %#v", got)
	}
	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("restart was not scheduled")
	}
}
