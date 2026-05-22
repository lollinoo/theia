package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type autostartProvider interface {
	Status(expectedTarget string) (autostartStatus, error)
	SetEnabled(enabled bool, targetPath string) error
}

type commandRunner func(name string, args ...string) ([]byte, error)

type setupOptions struct {
	ConfigPath     func() (string, error)
	LoadConfig     func() (Config, error)
	SaveConfig     func(Config) error
	LogPath        func() string
	PickWinBoxPath func() (string, error)
	Autostart      autostartProvider
	Installer      connectorInstaller
	Restart        func()
	NewSetupToken  func() (string, error)
}

type autostartStatus struct {
	Enabled      bool   `json:"enabled"`
	TargetPath   string `json:"target_path,omitempty"`
	TargetExists bool   `json:"target_exists"`
	Healthy      bool   `json:"healthy"`
}

type setupConfigRequest struct {
	WinBoxPath   string `json:"winbox_path"`
	ListenPort   int    `json:"listen_port"`
	TheiaOrigin  string `json:"theia_origin"`
	TheiaBaseURL string `json:"theia_base_url"`
	BridgeSecret string `json:"bridge_secret"`
	LogLevel     string `json:"log_level"`
}

type setupStatusResponse struct {
	ConfigPath             string `json:"config_path"`
	LogPath                string `json:"log_path,omitempty"`
	WinBoxPath             string `json:"winbox_path"`
	ListenPort             int    `json:"listen_port"`
	TheiaOrigin            string `json:"theia_origin"`
	TheiaBaseURL           string `json:"theia_base_url"`
	LogLevel               string `json:"log_level"`
	BridgeSecretConfigured bool   `json:"bridge_secret_configured"`
	AutostartEnabled       bool   `json:"autostart_enabled"`
	AutostartTargetPath    string `json:"autostart_target_path,omitempty"`
	AutostartTargetExists  bool   `json:"autostart_target_exists"`
	AutostartHealthy       bool   `json:"autostart_healthy"`
	InstalledPath          string `json:"installed_path"`
	Installed              bool   `json:"installed"`
	RunningFromInstalled   bool   `json:"running_from_installed_path"`
}

var currentLogFilePath string

func defaultSetupOptions() setupOptions {
	return setupOptions{
		ConfigPath:     configFilePath,
		LoadConfig:     loadConfig,
		SaveConfig:     saveConfig,
		LogPath:        func() string { return currentLogFilePath },
		PickWinBoxPath: pickWinBoxPath,
		Autostart:      systemAutostartProvider{},
		Installer:      systemConnectorInstaller{},
		NewSetupToken:  generateSetupToken,
	}
}

func (opts setupOptions) withDefaults() setupOptions {
	defaults := defaultSetupOptions()
	if opts.ConfigPath == nil {
		opts.ConfigPath = defaults.ConfigPath
	}
	if opts.LoadConfig == nil {
		opts.LoadConfig = defaults.LoadConfig
	}
	if opts.SaveConfig == nil {
		opts.SaveConfig = defaults.SaveConfig
	}
	if opts.LogPath == nil {
		opts.LogPath = defaults.LogPath
	}
	if opts.PickWinBoxPath == nil {
		opts.PickWinBoxPath = defaults.PickWinBoxPath
	}
	if opts.Autostart == nil {
		opts.Autostart = defaults.Autostart
	}
	if opts.Installer == nil {
		opts.Installer = defaults.Installer
	}
	if opts.NewSetupToken == nil {
		opts.NewSetupToken = defaults.NewSetupToken
	}
	return opts
}

func buildMuxWithSetup(winboxPath, allowedOrigin, expectedHost string, client *TheiaClient, opts setupOptions) http.Handler {
	opts = opts.withDefaults()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.Handle("/launch", securityCheck(allowedOrigin, expectedHost, handleLaunch(winboxPath, client)))
	setupToken, err := opts.NewSetupToken()
	if err != nil {
		mux.Handle("/setup", setupLocalhostOnly(handleSetupUnavailable()))
		mux.Handle("/setup/", setupLocalhostOnly(handleSetupUnavailable()))
		return mux
	}
	mux.Handle("/setup", setupLocalhostOnly(handleSetupPage(setupToken)))
	mux.Handle("/setup/", setupLocalhostOnly(handleSetupAPI(setupToken, opts)))
	return mux
}

func setupLocalhostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) || !isLoopbackRemoteAddr(r.RemoteAddr) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(hostport string) bool {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return false
	}
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isLoopbackRemoteAddr(remoteAddr string) bool {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func requireSetupToken(w http.ResponseWriter, r *http.Request, token string) bool {
	if r.Header.Get("X-Setup-Token") != token {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func generateSetupToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("setup token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func handleSetupUnavailable() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusServiceUnavailable, "setup unavailable")
	})
}

func handleSetupPage(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/setup" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		_, _ = w.Write([]byte(setupHTML(token)))
	})
}

func handleSetupAPI(token string, opts setupOptions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/setup/status":
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			handleSetupStatus(w, opts)
		case "/setup/config":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			if !requireSetupToken(w, r, token) {
				return
			}
			handleSetupConfig(w, r, opts)
		case "/setup/winbox/select":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			if !requireSetupToken(w, r, token) {
				return
			}
			handleSetupWinBoxSelect(w, opts)
		case "/setup/autostart":
			handleSetupAutostart(w, r, token, opts)
		case "/setup/install":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			if !requireSetupToken(w, r, token) {
				return
			}
			handleSetupInstall(w, opts)
		case "/setup/restart":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			if !requireSetupToken(w, r, token) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			if opts.Restart != nil {
				go func() {
					time.Sleep(100 * time.Millisecond)
					opts.Restart()
				}()
			}
		default:
			http.NotFound(w, r)
		}
	})
}

func handleSetupStatus(w http.ResponseWriter, opts setupOptions) {
	cfg, _ := opts.LoadConfig()
	configPath, err := opts.ConfigPath()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to locate config")
		return
	}
	installStatus, err := opts.Installer.Status()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read install status")
		return
	}
	autostartStatus, err := opts.Autostart.Status(installStatus.InstalledPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read autostart")
		return
	}
	writeJSON(w, http.StatusOK, setupStatusResponse{
		ConfigPath:             configPath,
		LogPath:                opts.LogPath(),
		WinBoxPath:             cfg.WinBoxPath,
		ListenPort:             cfg.ListenPort,
		TheiaOrigin:            cfg.TheiaOrigin,
		TheiaBaseURL:           cfg.TheiaBaseURL,
		LogLevel:               cfg.LogLevel,
		BridgeSecretConfigured: strings.TrimSpace(cfg.BridgeSecret) != "",
		AutostartEnabled:       autostartStatus.Enabled,
		AutostartTargetPath:    autostartStatus.TargetPath,
		AutostartTargetExists:  autostartStatus.TargetExists,
		AutostartHealthy:       autostartStatus.Healthy,
		InstalledPath:          installStatus.InstalledPath,
		Installed:              installStatus.Installed,
		RunningFromInstalled:   installStatus.RunningFromInstalledPath,
	})
}

func handleSetupConfig(w http.ResponseWriter, r *http.Request, opts setupOptions) {
	var req setupConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ListenPort < 1 || req.ListenPort > 65535 {
		writeError(w, http.StatusBadRequest, "listen_port must be 1-65535")
		return
	}
	cfg, _ := opts.LoadConfig()
	previous := cfg
	cfg.WinBoxPath = strings.TrimSpace(req.WinBoxPath)
	cfg.ListenPort = req.ListenPort
	cfg.TheiaOrigin = strings.TrimSpace(req.TheiaOrigin)
	cfg.TheiaBaseURL = strings.TrimSpace(req.TheiaBaseURL)
	secretChanged := false
	if secret := strings.TrimSpace(req.BridgeSecret); secret != "" {
		secretChanged = previous.BridgeSecret != secret
		cfg.BridgeSecret = secret
	}
	cfg.LogLevel = strings.TrimSpace(req.LogLevel)
	if cfg.LogLevel == "" {
		cfg.LogLevel = DefaultConfig().LogLevel
	}
	if err := opts.SaveConfig(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	restartRequired := previous.ListenPort != cfg.ListenPort ||
		previous.TheiaOrigin != cfg.TheiaOrigin ||
		previous.TheiaBaseURL != cfg.TheiaBaseURL ||
		previous.WinBoxPath != cfg.WinBoxPath ||
		previous.LogLevel != cfg.LogLevel ||
		secretChanged
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "restart_required": restartRequired})
}

func handleSetupWinBoxSelect(w http.ResponseWriter, opts setupOptions) {
	path, err := opts.PickWinBoxPath()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to select WinBox")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"winbox_path": path})
}

func handleSetupInstall(w http.ResponseWriter, opts setupOptions) {
	status, err := opts.Installer.EnsureInstalled()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to install connector")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func handleSetupAutostart(w http.ResponseWriter, r *http.Request, token string, opts setupOptions) {
	switch r.Method {
	case http.MethodGet:
		installStatus, err := opts.Installer.Status()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read install status")
			return
		}
		status, err := opts.Autostart.Status(installStatus.InstalledPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read autostart")
			return
		}
		writeJSON(w, http.StatusOK, status)
	case http.MethodPost:
		if !requireSetupToken(w, r, token) {
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		installStatus, err := opts.Installer.Status()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read install status")
			return
		}
		if req.Enabled {
			installStatus, err = opts.Installer.EnsureInstalled()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to install connector")
				return
			}
		}
		if err := opts.Autostart.SetEnabled(req.Enabled, installStatus.InstalledPath); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update autostart")
			return
		}
		status, err := opts.Autostart.Status(installStatus.InstalledPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read autostart")
			return
		}
		writeJSON(w, http.StatusOK, status)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func setupHTML(token string) string {
	escapedToken, _ := json.Marshal(token)
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Theia WinBox Bridge Setup</title>
  <style>
    body { font-family: system-ui, -apple-system, Segoe UI, sans-serif; margin: 2rem; max-width: 760px; color: #17202a; }
    label { display: block; margin-top: 1rem; font-weight: 600; }
    input, select { width: 100%%; box-sizing: border-box; margin-top: .35rem; padding: .55rem; }
    button { margin-top: 1rem; padding: .6rem .8rem; }
    .row { display: flex; gap: .75rem; align-items: end; }
    .row > label { flex: 1; }
    .install { margin-top: 1rem; padding: .75rem; border: 1px solid #ccd6e0; border-radius: .35rem; }
    .meta { margin-top: .35rem; color: #4c5f73; font-size: .9rem; overflow-wrap: anywhere; }
    .status { margin-top: 1rem; min-height: 1.5rem; }
  </style>
</head>
<body>
  <h1>Theia WinBox Bridge Setup</h1>
  <form id="setup_form">
    <div class="row">
      <label>WinBox path
        <input name="winbox_path" id="winbox_path" autocomplete="off">
      </label>
      <button type="button" id="select_winbox">Browse</button>
    </div>
    <label>Bridge secret
      <input name="bridge_secret" id="bridge_secret" type="password" autocomplete="new-password" placeholder="Paste a new secret or leave blank to keep the saved secret">
    </label>
    <label>Theia base URL
      <input name="theia_base_url" id="theia_base_url" autocomplete="off">
    </label>
    <label>Theia origin
      <input name="theia_origin" id="theia_origin" autocomplete="off">
    </label>
    <label>Listen port
      <input name="listen_port" id="listen_port" type="number" min="1" max="65535">
    </label>
    <label>Log level
      <select name="log_level" id="log_level">
        <option value="info">info</option>
        <option value="debug">debug</option>
      </select>
    </label>
    <div class="install">
      <strong>Installed connector</strong>
      <div id="installed_path" class="meta">Not installed</div>
      <div id="autostart_health" class="meta">Autostart is disabled.</div>
      <button type="button" id="install_connector">Install / Repair Connector</button>
    </div>
    <label>
      <input name="autostart_enabled" id="autostart_enabled" type="checkbox" style="width:auto">
      Start automatically when I sign in
    </label>
    <button type="submit">Save</button>
    <button type="button" id="restart_server">Restart Server</button>
  </form>
  <div id="status" class="status" role="status"></div>
  <script>
    const setupToken = %s;
    const statusEl = document.getElementById("status");
    const installedPathEl = document.getElementById("installed_path");
    const autostartHealthEl = document.getElementById("autostart_health");
    const setStatus = (text) => { statusEl.textContent = text; };
    async function api(path, options = {}) {
      const headers = options.headers || {};
      if (options.method && options.method !== "GET") headers["X-Setup-Token"] = setupToken;
      const response = await fetch(path, { ...options, headers });
      if (!response.ok) throw new Error(await response.text());
      return response.json();
    }
    function renderInstallStatus(status) {
      installedPathEl.textContent = status.installed
        ? status.installed_path
        : "Not installed. Use Install / Repair before enabling autostart.";
      if (!status.autostart_enabled) {
        autostartHealthEl.textContent = "Autostart is disabled.";
      } else if (status.autostart_healthy) {
        autostartHealthEl.textContent = "Autostart points to the installed connector.";
      } else if (!status.autostart_target_exists) {
        autostartHealthEl.textContent = "Autostart needs repair because its target is missing.";
      } else {
        autostartHealthEl.textContent = "Autostart needs repair because it points to another path.";
      }
    }
    async function loadStatus(updateMessage = true) {
      const status = await api("/setup/status");
      for (const name of ["winbox_path", "theia_base_url", "theia_origin", "listen_port", "log_level"]) {
        const el = document.querySelector("[name=" + name + "]");
        if (el && status[name] !== undefined) el.value = status[name];
      }
      document.querySelector("[name=autostart_enabled]").checked = !!status.autostart_enabled;
      renderInstallStatus(status);
      if (updateMessage) {
        setStatus(status.bridge_secret_configured ? "Bridge secret is configured." : "Bridge secret is not configured.");
      }
    }
    document.getElementById("select_winbox").addEventListener("click", async () => {
      try {
        const selected = await api("/setup/winbox/select", { method: "POST" });
        document.querySelector("[name=winbox_path]").value = selected.winbox_path || "";
      } catch (err) { setStatus("WinBox selection failed."); }
    });
    document.getElementById("install_connector").addEventListener("click", async () => {
      try {
        await api("/setup/install", { method: "POST" });
        await loadStatus(false);
        setStatus("Connector installed.");
      } catch (err) { setStatus("Connector install failed."); }
    });
    document.getElementById("setup_form").addEventListener("submit", async (event) => {
      event.preventDefault();
      const data = Object.fromEntries(new FormData(event.currentTarget).entries());
      data.listen_port = Number(data.listen_port);
      try {
        const result = await api("/setup/config", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(data),
        });
        await api("/setup/autostart", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ enabled: document.querySelector("[name=autostart_enabled]").checked }),
        });
        await loadStatus(false);
        setStatus(result.restart_required ? "Saved. Restart required." : "Saved.");
      } catch (err) { setStatus("Save failed."); }
    });
    document.getElementById("restart_server").addEventListener("click", async () => {
      try { await api("/setup/restart", { method: "POST" }); setStatus("Restart requested."); }
      catch (err) { setStatus("Restart failed."); }
    });
    loadStatus().catch(() => setStatus("Could not load setup status."));
  </script>
</body>
</html>`, string(escapedToken))
}

func pickWinBoxPath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		script := `Add-Type -AssemblyName System.Windows.Forms; $dialog = New-Object System.Windows.Forms.OpenFileDialog; $dialog.Filter = 'WinBox executable|winbox*.exe|Executable files|*.exe|All files|*.*'; if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $dialog.FileName }`
		out, err := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script).Output() //nolint:gosec
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "darwin":
		out, err := exec.Command("osascript", "-e", `POSIX path of (choose file with prompt "Select WinBox")`).Output() //nolint:gosec
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	default:
		out, err := exec.Command("zenity", "--file-selection", "--title=Select WinBox").Output() //nolint:gosec
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}
		out, err = exec.Command("kdialog", "--getopenfilename", ".", "*").Output() //nolint:gosec
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
}

type systemAutostartProvider struct{}

func runCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput() //nolint:gosec
}

func (systemAutostartProvider) Status(expectedTarget string) (autostartStatus, error) {
	switch runtime.GOOS {
	case "windows":
		output, err := runCommand("reg.exe", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "Theia WinBox Bridge")
		if err != nil {
			return buildAutostartStatus(false, "", expectedTarget), nil
		}
		target := windowsAutostartTarget(output)
		return buildAutostartStatus(target != "", target, expectedTarget), nil
	case "darwin":
		path, err := launchAgentPath()
		if err != nil {
			return autostartStatus{}, err
		}
		target, enabled, err := launchAgentTarget(path)
		if err != nil {
			return autostartStatus{}, err
		}
		return buildAutostartStatus(enabled, target, expectedTarget), nil
	default:
		path, err := linuxAutostartPath()
		if err != nil {
			return autostartStatus{}, err
		}
		target, enabled, err := linuxAutostartTarget(path)
		if err != nil {
			return autostartStatus{}, err
		}
		return buildAutostartStatus(enabled, target, expectedTarget), nil
	}
}

func (systemAutostartProvider) SetEnabled(enabled bool, targetPath string) error {
	if enabled && strings.TrimSpace(targetPath) == "" {
		return fmt.Errorf("autostart target path is required")
	}
	switch runtime.GOOS {
	case "windows":
		return setWindowsAutostart(enabled, targetPath, runCommand)
	case "darwin":
		path, err := launchAgentPath()
		if err != nil {
			return err
		}
		if !enabled {
			return removeIfExists(path)
		}
		return writeLaunchAgent(path, targetPath)
	default:
		path, err := linuxAutostartPath()
		if err != nil {
			return err
		}
		if !enabled {
			return removeIfExists(path)
		}
		return writeLinuxAutostart(path, targetPath)
	}
}

func buildAutostartStatus(enabled bool, targetPath string, expectedTarget string) autostartStatus {
	targetExists := fileExists(targetPath)
	return autostartStatus{
		Enabled:      enabled,
		TargetPath:   targetPath,
		TargetExists: targetExists,
		Healthy:      enabled && targetExists && sameFilePath(targetPath, expectedTarget),
	}
}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.theia.winbox-bridge.plist"), nil
}

func linuxAutostartPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "theia-winbox-bridge.desktop"), nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeLaunchAgent(path string, exe string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	buf.WriteString("<plist version=\"1.0\">\n<dict>\n")
	buf.WriteString("  <key>Label</key><string>com.theia.winbox-bridge</string>\n")
	buf.WriteString("  <key>ProgramArguments</key><array><string>")
	buf.WriteString(html.EscapeString(exe))
	buf.WriteString("</string></array>\n")
	buf.WriteString("  <key>RunAtLoad</key><true/>\n")
	buf.WriteString("</dict>\n</plist>\n")
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func writeLinuxAutostart(path string, exe string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	contents := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=Theia WinBox Bridge\nExec=%s\nX-GNOME-Autostart-enabled=true\n", linuxDesktopExecValue(exe))
	return os.WriteFile(path, []byte(contents), 0o600)
}

func launchAgentTarget(path string) (string, bool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	text := string(contents)
	marker := "<key>ProgramArguments</key>"
	markerIndex := strings.Index(text, marker)
	if markerIndex < 0 {
		return "", true, nil
	}
	rest := text[markerIndex+len(marker):]
	start := strings.Index(rest, "<string>")
	if start < 0 {
		return "", true, nil
	}
	rest = rest[start+len("<string>"):]
	end := strings.Index(rest, "</string>")
	if end < 0 {
		return "", true, nil
	}
	return html.UnescapeString(rest[:end]), true, nil
}

func linuxAutostartTarget(path string) (string, bool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Exec=") {
			return linuxDesktopExecTarget(strings.TrimPrefix(line, "Exec=")), true, nil
		}
	}
	return "", true, nil
}

func setWindowsAutostart(enabled bool, exe string, run commandRunner) error {
	if !enabled {
		output, err := run("reg.exe", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "Theia WinBox Bridge", "/f")
		if err != nil && !windowsRegistryValueMissing(output) {
			return err
		}
		return nil
	}
	_, err := run("reg.exe", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "Theia WinBox Bridge", "/t", "REG_SZ", "/d", windowsRunCommandValue(exe), "/f")
	return err
}

func windowsRegistryValueMissing(output []byte) bool {
	text := strings.ToLower(string(output))
	return strings.Contains(text, "unable to find") ||
		strings.Contains(text, "cannot find") ||
		strings.Contains(text, "not found")
}

func windowsAutostartTarget(output []byte) string {
	const valueName = "Theia WinBox Bridge"
	for _, line := range strings.Split(string(output), "\n") {
		index := strings.Index(line, valueName)
		if index < 0 {
			continue
		}
		value := strings.TrimSpace(line[index+len(valueName):])
		fields := strings.Fields(value)
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "REG_") {
			return ""
		}
		return windowsRunCommandTarget(strings.TrimSpace(strings.TrimPrefix(value, fields[0])))
	}
	return ""
}

func windowsRunCommandValue(exe string) string {
	return `"` + strings.Trim(exe, `"`) + `"`
}

func windowsRunCommandTarget(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, `"`) {
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return ""
		}
		return fields[0]
	}
	rest := strings.TrimPrefix(value, `"`)
	end := strings.Index(rest, `"`)
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func linuxDesktopExecValue(exe string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"`", "\\`",
		"$", `\$`,
		"%", "%%",
	)
	return `"` + replacer.Replace(exe) + `"`
}

func linuxDesktopExecTarget(value string) string {
	return strings.ReplaceAll(autostartCommandPath(value), "%%", "%")
}

func autostartCommandPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, `"`) {
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return ""
		}
		return fields[0]
	}
	var builder strings.Builder
	escaped := false
	for _, char := range strings.TrimPrefix(value, `"`) {
		if escaped {
			builder.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			break
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func setupRestartFunc(mgr *ServerManager) func() {
	return func() {
		cfg, err := loadConfig()
		if err != nil {
			log.Printf("winbox-bridge: setup restart config reload error: %v", err)
			return
		}
		if err := mgr.Restart(cfg); err != nil {
			log.Printf("winbox-bridge: setup restart error: %v", err)
		}
	}
}
