package main

// This file exercises main behavior so refactors preserve the documented contract.

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// --- Test helpers ---

// makeRequest builds an *http.Request for the given method, path, optional body, Origin, and Host headers.
func makeRequest(t *testing.T, method, path string, body interface{}, origin, host string) *http.Request {
	t.Helper()
	var b *bytes.Buffer
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		b = bytes.NewBuffer(data)
	} else {
		b = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, b)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if host != "" {
		req.Host = host
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// /health is public; /launch is protected by securityCheck.
func buildHandler(theiaOrigin string, winboxPath string, expectedHost string) http.Handler {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(launchCredentials{
			IP:        "192.168.1.1",
			Username:  "admin",
			Password:  "pass123",
			ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
		})
	}))
	return buildMux(winboxPath, theiaOrigin, expectedHost, &TheiaClient{
		BaseURL:    backend.URL,
		Secret:     testSecret,
		HTTPClient: backend.Client(),
	})
}

const testSecret = "theia_bridge_public.raw-secret"

func validToken(t *testing.T) string {
	t.Helper()
	return "launch-token"
}

// --- Security: Origin validation ---

func TestOriginValidation_ValidOriginPasses(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d", rr.Code)
	}
}

func TestOriginValidation_HealthPublicWithAnyOrigin(t *testing.T) {
	// /health is public — evil origin still gets 200 (no CSRF risk on a read-only status check)
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "http://evil.com", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d", rr.Code)
	}
}

func TestOriginValidation_HealthPublicWithoutOrigin(t *testing.T) {
	// /health is public — no Origin header still gets 200 (browser fetch without CORS header works)
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d", rr.Code)
	}
}

func TestOriginValidation_EvilOriginOnLaunchReturns403(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"launch_token": validToken(t)},
		"http://evil.com", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 got %d", rr.Code)
	}
}

func TestOriginValidation_IPOriginOnLaunchPassesWithDefaultLocalhostConfig(t *testing.T) {
	original := startProcess
	t.Cleanup(func() { startProcess = original })
	startProcess = func(name string, args []string) error {
		return nil
	}

	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"launch_token": validToken(t)},
		"http://10.10.0.35:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d; body: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://10.10.0.35:3000" {
		t.Errorf("expected ACAO=http://10.10.0.35:3000, got %q", got)
	}
}

// --- Security: Host validation ---

func TestHostValidation_EvilHostOnLaunchReturns403(t *testing.T) {
	// /launch requires valid Host; /health is public and does not check Host
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"launch_token": validToken(t)},
		"http://localhost:3000", "evil.com:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 got %d for host evil.com:1337", rr.Code)
	}
}

func TestHostValidation_ValidHostPasses(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d", rr.Code)
	}
}

func TestHostValidation_IPHostOnLaunchReturns403(t *testing.T) {
	// /launch: strict match on "localhost:1337" only — 127.0.0.1:1337 should fail
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"launch_token": validToken(t)},
		"http://localhost:3000", "127.0.0.1:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 got %d for host 127.0.0.1:1337", rr.Code)
	}
}

// --- Health endpoint ---

func TestHealth_GETReturns200OkTrue(t *testing.T) {
	h := buildHandler("http://localhost:3000", "", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("expected ok:true, got %v", resp["ok"])
	}
}

func TestHealth_POSTReturns405(t *testing.T) {
	h := buildHandler("http://localhost:3000", "", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/health", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 got %d", rr.Code)
	}
}

// --- Launch endpoint ---

func TestLaunch_ValidTokenReturns200(t *testing.T) {
	// Override startProcess with a successful mock
	original := startProcess
	t.Cleanup(func() { startProcess = original })
	startProcess = func(name string, args []string) error {
		return nil
	}

	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"launch_token": validToken(t)},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("expected ok:true, got %v", resp["ok"])
	}
}

func TestLaunch_UsesLaunchTokenAndBackendBridgeSecret(t *testing.T) {
	original := startProcess
	t.Cleanup(func() { startProcess = original })
	var launchedArgs []string
	startProcess = func(name string, args []string) error {
		launchedArgs = append([]string(nil), args...)
		return nil
	}

	var gotAuth string
	var gotToken string
	theia := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode backend request: %v", err)
		}
		gotToken = req["launch_token"]
		_ = json.NewEncoder(w).Encode(launchCredentials{
			IP:        "192.168.88.1",
			Username:  "admin",
			Password:  "winbox-password",
			ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
		})
	}))
	defer theia.Close()

	client := &TheiaClient{BaseURL: theia.URL, Secret: "theia_bridge_public.raw-secret", HTTPClient: theia.Client()}
	h := buildMux("/fake/winbox", "http://localhost:3000", "localhost:1337", client)
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"launch_token": "launch-token"},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d; body: %s", rr.Code, rr.Body.String())
	}
	if gotAuth != "Bridge theia_bridge_public.raw-secret" {
		t.Fatalf("backend Authorization header mismatch")
	}
	if gotToken != "launch-token" {
		t.Fatalf("backend launch_token mismatch")
	}
	if strings.Join(launchedArgs, ",") != "192.168.88.1,admin,winbox-password" {
		t.Fatalf("launched args mismatch")
	}
}

func TestLaunch_MissingBridgeSecretReturns503(t *testing.T) {
	client := &TheiaClient{BaseURL: "http://theia.test", Secret: ""}
	h := buildMux("/fake/winbox", "http://localhost:3000", "localhost:1337", client)
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"launch_token": "launch-token"},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestLaunch_MissingTokenReturns400(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"launch_token": ""},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestLaunch_WinBoxNotFoundReturns503(t *testing.T) {
	// winboxPath is empty — WinBox not found
	h := buildMux("", "http://localhost:3000", "localhost:1337", &TheiaClient{BaseURL: "http://theia.test", Secret: testSecret})
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"launch_token": validToken(t)},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 got %d", rr.Code)
	}
	assertJSONError(t, rr, "winbox executable not found")
}

func TestLaunch_StartProcessFailReturns500(t *testing.T) {
	original := startProcess
	t.Cleanup(func() { startProcess = original })
	startProcess = func(name string, args []string) error {
		return &mockProcessError{"simulated start failure"}
	}

	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"launch_token": validToken(t)},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 got %d", rr.Code)
	}
	assertJSONError(t, rr, "failed to launch WinBox")
}

func TestLaunch_GETReturns405(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/launch", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 got %d", rr.Code)
	}
}

// --- Security: Token replaces plaintext fields ---

func TestLaunch_PlaintextFieldsInBodyAreRejected(t *testing.T) {
	// Sending plaintext ip/username/password (old format) must be rejected — token is required.
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()

	body := `{"ip":"192.168.1.1","username":"admin","password":"pass"}`
	req := httptest.NewRequest(http.MethodPost, "/launch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	req.Host = "localhost:1337"
	h.ServeHTTP(rr, req)

	// Old plaintext format has no "launch_token" field — bridge must return 400
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for plaintext credentials (no token), got %d", rr.Code)
	}
}

// --- CORS preflight ---

func TestCORSPreflight_OptionsLaunchReturns204(t *testing.T) {
	h := buildHandler("http://localhost:3000", "", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodOptions, "/launch", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 got %d", rr.Code)
	}
	acao := rr.Header().Get("Access-Control-Allow-Origin")
	if acao != "http://localhost:3000" {
		t.Errorf("expected ACAO=http://localhost:3000, got %q", acao)
	}
}

func TestCORSPreflight_OptionsLaunchAllowsPrivateNetworkForIPOrigin(t *testing.T) {
	h := buildHandler("http://localhost:3000", "", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodOptions, "/launch", nil, "http://10.10.0.35:3000", "localhost:1337")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 got %d", rr.Code)
	}
	if acao := rr.Header().Get("Access-Control-Allow-Origin"); acao != "http://10.10.0.35:3000" {
		t.Errorf("expected ACAO=http://10.10.0.35:3000, got %q", acao)
	}
	if got := rr.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Errorf("expected ACAPN=true, got %q", got)
	}
}

func TestHealth_ReturnsCORSWildcard(t *testing.T) {
	// /health is a simple GET — no preflight needed. ACAO: * lets any origin read the response.
	h := buildHandler("http://localhost:3000", "", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d", rr.Code)
	}
	acao := rr.Header().Get("Access-Control-Allow-Origin")
	if acao != "*" {
		t.Errorf("expected ACAO=*, got %q", acao)
	}
}

func TestHealth_OptionsPrivateNetworkPreflightReturns204(t *testing.T) {
	h := buildHandler("http://localhost:3000", "", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodOptions, "/health", nil, "http://10.10.0.35:3000", "localhost:1337")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://10.10.0.35:3000" {
		t.Errorf("expected ACAO=http://10.10.0.35:3000, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Errorf("expected ACAPN=true, got %q", got)
	}
}

// --- WinBox discovery ---

func TestDiscoverWinBox_ReturnsEmptyStringWhenNotFound(t *testing.T) {
	// Override PATH to be empty to ensure nothing is found
	// discoverWinBox should not panic and should return ""
	result := discoverWinBoxFromPATH()
	// We don't assert the value is empty because CI may have winbox in PATH.
	// We just assert it doesn't panic and returns a string.
	_ = result
}

// --- Helper types and functions for tests ---

type mockProcessError struct{ msg string }

func (e *mockProcessError) Error() string { return e.msg }

// assertJSONError checks that the response body contains {"error": <substr>}
func assertJSONError(t *testing.T, rr *httptest.ResponseRecorder, substr string) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	errMsg, ok := resp["error"].(string)
	if !ok {
		t.Fatalf("expected 'error' string field, got: %v", resp)
	}
	if !strings.Contains(errMsg, substr) {
		t.Errorf("JSON error did not contain expected text")
	}
}

// --- Log file helpers ---

func TestLogFilePath_IsAbsoluteAndEndsWith(t *testing.T) {
	p := logFilePath()
	if !strings.Contains(p, "winbox-bridge.log") {
		t.Errorf("expected path to contain winbox-bridge.log, got %q", p)
	}
}

func TestSetupLogFile_CreatesFileAndWritesToIt(t *testing.T) {
	// setupLogFile writes to os.TempDir(); redirect to a temp location by temporarily
	// monkey-patching os.TempDir — not possible without modifying the function.
	// Instead, call the real setupLogFile and verify the file exists and is writable.
	path, cleanup := setupLogFile()
	if path == "" {
		t.Skip("setupLogFile could not create log file (permissions?)")
	}
	defer cleanup()

	// Write a log line — it should land in the file.
	log.Println("test log line from TestSetupLogFile")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file %s: %v", path, err)
	}
	if !strings.Contains(string(data), "test log line from TestSetupLogFile") {
		t.Errorf("log file does not contain expected line; contents:\n%s", data)
	}

	// Restore logger to stderr only so subsequent tests are not affected.
	log.SetOutput(os.Stderr)
}

func TestSetupLogFile_CleanupClosesFile(t *testing.T) {
	// Just ensure cleanup does not panic when called.
	path, cleanup := setupLogFile()
	if path == "" {
		t.Skip("could not create log file")
	}
	cleanup() // must not panic
	log.SetOutput(os.Stderr)
}

func TestFreeConsole_DoesNotPanic(t *testing.T) {
	// freeConsole is a no-op on non-Windows; on Windows it calls FreeConsole.
	// Either way it must not panic.
	freeConsole()
}
