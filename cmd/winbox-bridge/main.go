package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"

	"fyne.io/systray"
)

// --- Request / response types ---

// launchRequest is the POST /launch request body.
// Exactly 3 fields: IP, Username, Password. No executable/command field (D-09).
type launchRequest struct {
	IP       string `json:"ip"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// --- Process injection (for testability) ---

// startProcess is the function used to start the WinBox process.
// Tests override this variable to mock process launch behaviour.
var startProcess = defaultStartProcess

// defaultStartProcess launches the winbox binary with the given arguments.
// It detaches stdout/stderr and calls cmd.Process.Release() to orphan the process
// so the bridge returns immediately (fire-and-forget, per D-10).
func defaultStartProcess(name string, args []string) error {
	cmd := exec.Command(name, args...) //nolint:gosec — name comes from trusted discoverWinBox(), not request
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	// Release ownership so the child outlives the bridge (D-10)
	return cmd.Process.Release()
}

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

// --- Security middleware ---

// securityCheck validates Origin and Host headers on every request (D-04, D-05, D-06, D-07).
// expectedHost is the required value for the Host header (e.g. "localhost:1337").
// Using a parameter instead of a hardcoded value allows dynamic port configuration (T-29-04).
// CORS preflight (OPTIONS) that passes validation is handled here with proper CORS headers.
// Non-OPTIONS requests that pass validation also get the ACAO header set.
func securityCheck(allowedOrigin string, expectedHost string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" || origin != allowedOrigin {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		host := r.Host
		if host != expectedHost {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		// Set CORS header on all passing requests so the browser can read the response
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)

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
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleLaunch returns an http.HandlerFunc for POST /launch (D-08, D-09, D-10, D-11).
// winboxPath is the resolved WinBox executable path (may be "").
func handleLaunch(winboxPath string) http.HandlerFunc {
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

		if req.IP == "" || req.Username == "" || req.Password == "" {
			writeError(w, http.StatusBadRequest, "ip, username, and password are required")
			return
		}

		if winboxPath == "" {
			writeError(w, http.StatusServiceUnavailable,
				"winbox executable not found — use --winbox-path to specify location")
			return
		}

		// Construct args: winbox <ip> <username> <password> (D-08)
		// exec.Command does NOT invoke a shell — each arg is a separate argv element (T-26-08).
		args := []string{req.IP, req.Username, req.Password}
		if err := startProcess(winboxPath, args); err != nil {
			log.Printf("winbox-bridge: failed to start WinBox: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to launch WinBox")
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// --- Mux builder (extracted for testability) ---

// buildMux creates the http.Handler with per-route security:
// /health is public (no auth — any origin may poll bridge status).
// /launch is protected by securityCheck (CSRF guard for sensitive launch action).
func buildMux(winboxPath, allowedOrigin, expectedHost string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.Handle("/launch", securityCheck(allowedOrigin, expectedHost, handleLaunch(winboxPath)))
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

// --- main ---

func main() {
	// CLI flags — still supported for headless/scripted use and backward compatibility.
	theiaOriginFlag := flag.String("theia-origin", "",
		"Accepted Theia frontend origin (overrides config file)")
	winboxPathFlag := flag.String("winbox-path", "",
		"Path to WinBox executable (overrides config file and auto-discovery)")
	listenFlag := flag.String("listen", "",
		"Address to listen on, e.g. :1337 (overrides config file)")
	noTray := flag.Bool("no-tray", false,
		"Run in headless mode without system tray (for servers without a display)")
	flag.Parse()

	// Load persistent config — falls back to defaults if missing (first run).
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("winbox-bridge: config load warning: %v (using defaults)", err)
	}

	// CLI flag overrides — flags set to non-empty values win over config file.
	if *theiaOriginFlag != "" {
		cfg.TheiaOrigin = *theiaOriginFlag
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
	// systray.Run MUST block the main goroutine (macOS Cocoa requirement).
	// All other goroutines are spawned inside setupTray (onReady callback).
	systray.Run(
		func() { setupTray(mgr, cfg) },
		func() { mgr.Stop() }, //nolint:errcheck — best-effort shutdown on exit
	)
}

// Ensure fmt is referenced (used in server.go via securityCheck caller).
var _ = fmt.Sprintf

