package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"fyne.io/systray"
)

// --- Request / response types ---

// launchRequest is the POST /launch request body.
// Accepts a one-time launch token produced by the Theia backend.
type launchRequest struct {
	LaunchToken string `json:"launch_token"`
}

// launchCredentials is the plaintext payload embedded inside the encrypted token.
type launchCredentials struct {
	IP        string `json:"ip"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	ExpiresAt string `json:"expires_at"`
}

// --- Process injection (for testability) ---

// startProcess is the function used to start the WinBox process.
// Tests override this variable to mock process launch behaviour.
// The default implementation is platform-specific (see launch_windows.go /
// launch_other.go) so that WinBox opens in the foreground on Windows.
var startProcess = defaultStartProcess

// --- WinBox discovery ---

// discoverWinBox returns the path to the WinBox executable.
// If winboxPathFlag is non-empty, it is used as-is (validated at startup).
// Otherwise discoverWinBoxFromPATH() is called, then platform defaults are checked.
// Returns "" if nothing is found — bridge starts but /launch returns 503 (D-03).
func discoverWinBox(winboxPathFlag string) string {
	if winboxPathFlag != "" {
		if _, err := os.Stat(winboxPathFlag); err == nil {
			log.Printf("winbox-bridge: using --winbox-path=%s", winboxPathFlag)
			return winboxPathFlag
		}
		log.Printf("WARNING: --winbox-path=%s does not exist or is not accessible; treating as not found", winboxPathFlag)
		return ""
	}

	// Search PATH
	if p := discoverWinBoxFromPATH(); p != "" {
		log.Printf("winbox-bridge: found WinBox in PATH: %s", p)
		return p
	}

	// Fall back to platform-specific defaults
	defaults := platformDefaults()
	for _, candidate := range defaults {
		if _, err := os.Stat(candidate); err == nil {
			log.Printf("winbox-bridge: found WinBox at platform default: %s", candidate)
			return candidate
		}
	}

	cfgPath, _ := configFilePath()
	log.Printf("winbox-bridge: WinBox not found — set \"winbox_path\" in %s", cfgPath)
	return ""
}

// discoverWinBoxFromPATH searches PATH for WinBox executables.
// Exported as a testable helper (called in tests).
func discoverWinBoxFromPATH() string {
	if runtime.GOOS == "windows" {
		for _, name := range []string{"winbox64.exe", "winbox.exe"} {
			if p, err := exec.LookPath(name); err == nil {
				return p
			}
		}
		return ""
	}
	// Linux / macOS
	if p, err := exec.LookPath("winbox"); err == nil {
		return p
	}
	return ""
}

// platformDefaults returns the ordered list of candidate paths to check
// when WinBox is not found in PATH.
func platformDefaults() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{`C:\Program Files\WinBox\winbox64.exe`}
	case "darwin":
		return []string{"/Applications/WinBox.app/Contents/MacOS/WinBox"}
	default: // linux
		return []string{"/usr/bin/winbox"}
	}
}

// --- HTTP helpers ---

// writeJSON sets Content-Type, writes the status code, and encodes v as JSON.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("winbox-bridge: failed to encode JSON response: %v", err)
	}
}

// writeError writes a JSON error response: {"error": msg}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func allowPrivateNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
	}
}

func originAllowed(origin string, allowedOrigin string) bool {
	if origin == "" {
		return false
	}
	if origin == allowedOrigin {
		return true
	}
	if allowedOrigin != DefaultConfig().TheiaOrigin {
		return false
	}

	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	allowedURL, err := url.Parse(allowedOrigin)
	if err != nil {
		return false
	}

	if originURL.Scheme != allowedURL.Scheme {
		return false
	}

	originHost := originURL.Hostname()
	if originHost != "localhost" && net.ParseIP(originHost) == nil {
		return false
	}

	allowedPort := allowedURL.Port()
	if allowedPort == "" {
		if allowedURL.Scheme == "https" {
			allowedPort = "443"
		} else {
			allowedPort = "80"
		}
	}
	originPort := originURL.Port()
	if originPort == "" {
		if originURL.Scheme == "https" {
			originPort = "443"
		} else {
			originPort = "80"
		}
	}

	return allowedPort == originPort
}

// --- Security middleware ---

// securityCheck validates Origin and Host headers on every request (D-04, D-05, D-06, D-07).
// expectedHost is the required value for the Host header (e.g. "localhost:1337").
// Using a parameter instead of a hardcoded value allows dynamic port configuration (T-29-04).
// CORS preflight (OPTIONS) that passes validation is handled here with proper CORS headers.
// Non-OPTIONS requests that pass validation also get the ACAO header set.
func securityCheck(allowedOrigin string, expectedHost string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if !originAllowed(origin, allowedOrigin) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		host := r.Host
		if host != expectedHost {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		// Echo the requesting origin so LAN/IP-hosted Theia instances can use the bridge
		// when the bridge still has the default localhost dev origin configured.
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		allowPrivateNetwork(w, r)

		// Handle CORS preflight
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// --- Route handlers ---

// handleHealth handles GET /health (D-12).
// Public endpoint — no Origin/Host check. Returns Access-Control-Allow-Origin: *
// so any Theia instance can poll bridge status regardless of its own origin.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		allowPrivateNetwork(w, r)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleLaunch returns an http.HandlerFunc for POST /launch.
// winboxPath is the resolved WinBox executable path (may be "").
func handleLaunch(winboxPath string, client *TheiaClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req launchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.LaunchToken == "" {
			writeError(w, http.StatusBadRequest, "launch_token is required")
			return
		}

		if client == nil || strings.TrimSpace(client.Secret) == "" {
			writeError(w, http.StatusServiceUnavailable,
				"bridge secret not configured — generate one in Theia Settings and paste it into config.json")
			return
		}

		if winboxPath == "" {
			log.Printf("winbox-bridge: /launch: WinBox not found — set \"winbox_path\" in config.json")
			writeError(w, http.StatusServiceUnavailable,
				"winbox executable not found — set winbox_path in config.json")
			return
		}

		creds, err := client.ResolveLaunch(r.Context(), req.LaunchToken)
		if err != nil {
			log.Printf("winbox-bridge: launch resolve failed: %v", err)
			writeError(w, http.StatusBadGateway, "failed to resolve launch token with Theia")
			return
		}

		if creds.IP == "" || creds.Username == "" || creds.Password == "" {
			writeError(w, http.StatusBadRequest, "launch response missing required fields")
			return
		}

		// Construct args: winbox <ip> <username> <password> (D-08)
		// exec.Command does NOT invoke a shell — each arg is a separate argv element (T-26-08).
		args := []string{creds.IP, creds.Username, creds.Password}
		if err := startProcess(winboxPath, args); err != nil {
			log.Printf("winbox-bridge: failed to start WinBox: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to launch WinBox")
			return
		}

		log.Printf("winbox-bridge: launched WinBox for %s", creds.IP)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// --- Mux builder (extracted for testability) ---

// buildMux creates the http.Handler with per-route security:
// /health is public (no auth — any origin may poll bridge status).
// /launch is protected by securityCheck (CSRF guard for sensitive launch action).
func buildMux(winboxPath, allowedOrigin, expectedHost string, client *TheiaClient) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.Handle("/launch", securityCheck(allowedOrigin, expectedHost, handleLaunch(winboxPath, client)))
	return mux
}

// --- Helpers ---

// parsePort extracts a port number from an address string.
// Accepts ":1337", "0.0.0.0:1337", or plain "1337".
// Returns 1337 if parsing fails.
func parsePort(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// Try as a plain port number
		if p, err := strconv.Atoi(addr); err == nil {
			return p
		}
		return 1337
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return 1337
	}
	return p
}

// --- Log file helpers ---

// logFilePath returns the path used for the debug log file.
// Uses os.TempDir() so no special permissions are needed on any platform.
// Windows: %TEMP%\winbox-bridge.log   Linux/macOS: /tmp/winbox-bridge.log
func logFilePath() string {
	return filepath.Join(os.TempDir(), "winbox-bridge.log")
}

// setupLogFile opens (or creates/truncates) the log file and configures the
// standard logger to write to both stderr and the file simultaneously.
// Returns the log file path and a cleanup function that closes the file.
// If the file cannot be opened the logger is left writing to stderr only and
// an empty path is returned so callers can suppress the "Open Log File" menu item.
func setupLogFile() (path string, cleanup func()) {
	path = logFilePath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		log.Printf("winbox-bridge: could not open log file %s: %v (logging to stderr only)", path, err)
		return "", func() {}
	}
	// Tee: write to both the file and the original stderr so terminal use works too.
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	return path, func() { f.Close() }
}

// --- main ---

func main() {
	// CLI flags — still supported for headless/scripted use and backward compatibility.
	theiaOriginFlag := flag.String("theia-origin", "",
		"Accepted Theia frontend origin (overrides config file)")
	theiaBaseURLFlag := flag.String("theia-base-url", "",
		"Theia backend base URL used by the connector (overrides config file)")
	winboxPathFlag := flag.String("winbox-path", "",
		"Path to WinBox executable (overrides config file and auto-discovery)")
	listenFlag := flag.String("listen", "",
		"Address to listen on, e.g. :1337 (overrides config file)")
	noTray := flag.Bool("no-tray", false,
		"Run in headless mode without system tray (for servers without a display)")
	logLevel := flag.String("log-level", "info",
		"Log verbosity level: info (default) or debug (verbose request/response logging)")
	flag.Parse()

	// Load persistent config first — log level may be set there.
	// Falls back to defaults if missing (first run).
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("winbox-bridge: config load warning: %v (using defaults)", err)
	}

	// Determine effective log level: CLI flag overrides config file.
	// flag.Visit only visits flags that were explicitly set by the user.
	logLevelExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "log-level" {
			logLevelExplicit = true
		}
	})
	if !logLevelExplicit && cfg.LogLevel == "debug" {
		*logLevel = "debug"
	}

	// Apply log verbosity — debug adds timestamps and file/line info.
	if *logLevel == "debug" {
		log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	} else {
		log.SetFlags(log.LstdFlags)
	}

	// In tray mode, always write to a log file — the tray has no visible console,
	// so the file is the only way to inspect logs without restarting with --no-tray.
	// In headless mode (--no-tray), only write to a file when --log-level debug is set.
	var activeLogFile string // non-empty when log output is tee'd to a file
	if !*noTray || *logLevel == "debug" {
		path, cleanup := setupLogFile()
		defer cleanup()
		activeLogFile = path
		log.Printf("winbox-bridge: logging to file: %s", activeLogFile)
	}

	// CLI flag overrides — flags set to non-empty values win over config file.
	if *theiaOriginFlag != "" {
		cfg.TheiaOrigin = *theiaOriginFlag
	}
	if *theiaBaseURLFlag != "" {
		cfg.TheiaBaseURL = *theiaBaseURLFlag
	}
	if *winboxPathFlag != "" {
		cfg.WinBoxPath = *winboxPathFlag
	}
	if *listenFlag != "" {
		cfg.ListenPort = parsePort(*listenFlag)
	}

	mgr := &ServerManager{}

	// headless path: start server, block on signal, stop
	runHeadless := func() {
		if err := mgr.Start(cfg); err != nil {
			log.Fatalf("winbox-bridge: failed to start server: %v", err)
		}
		log.Printf("winbox-bridge: started on :%d (origin=%s)", cfg.ListenPort, cfg.TheiaOrigin)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("winbox-bridge: received signal %s, shutting down...", sig)
		if err := mgr.Stop(); err != nil {
			log.Printf("winbox-bridge: shutdown error: %v", err)
		}
		log.Println("winbox-bridge: stopped")
	}

	if *noTray {
		runHeadless()
		return
	}

	// When --no-tray is false, run with system tray (default desktop mode).
	// Auto-start server on launch so the bridge is immediately usable.
	if err := mgr.Start(cfg); err != nil {
		log.Printf("winbox-bridge: auto-start failed: %v", err)
	}

	// On Windows, detach from the console window so no black terminal flashes
	// when the user double-clicks the .exe.  This is done programmatically
	// (not via -H=windowsgui) so that terminal usage (--no-tray) still works.
	freeConsole()

	// systray.Run MUST block the main goroutine (macOS Cocoa requirement).
	// All other goroutines are spawned inside setupTray (onReady callback).
	systray.Run(
		func() { setupTray(mgr, cfg, activeLogFile) },
		func() { mgr.Stop() }, //nolint:errcheck — best-effort shutdown on exit
	)
}

// Ensure fmt is referenced (used in server.go via securityCheck caller).
var _ = fmt.Sprintf
