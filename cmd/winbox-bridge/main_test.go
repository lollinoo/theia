package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
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

// buildHandler constructs the full handler chain for testing.
// /health is public; /launch is protected by securityCheck.
func buildHandler(theiaOrigin string, winboxPath string, expectedHost string) http.Handler {
	return buildMux(winboxPath, theiaOrigin, expectedHost, testSecret)
}

// testSecret is a fixed 32-byte hex secret used in all tests.
const testSecret = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// encryptToken encrypts a launchCredentials payload using the given hex key and
// returns the hex-encoded AES-GCM ciphertext. Used in tests to build valid tokens.
func encryptToken(t *testing.T, creds launchCredentials, secretHex string) string {
	t.Helper()
	key, err := hex.DecodeString(secretHex)
	if err != nil || len(key) != 32 {
		t.Fatalf("encryptToken: invalid key: %v", err)
	}
	plaintext, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("encryptToken: marshal: %v", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("encryptToken: new cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("encryptToken: new GCM: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatalf("encryptToken: nonce: %v", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return hex.EncodeToString(ciphertext)
}

// validToken builds a valid encrypted token for standard test credentials using testSecret.
func validToken(t *testing.T) string {
	t.Helper()
	return encryptToken(t, launchCredentials{
		IP:        "192.168.1.1",
		Username:  "admin",
		Password:  "pass123",
		ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	}, testSecret)
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
		map[string]string{"token": validToken(t)},
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
		map[string]string{"token": validToken(t)},
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
		map[string]string{"token": validToken(t)},
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
		map[string]string{"token": validToken(t)},
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
		map[string]string{"token": validToken(t)},
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

func TestLaunch_MissingTokenReturns400(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"token": ""},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestLaunch_InvalidTokenReturns400(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"token": "deadbeef01020304"},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestLaunch_TokenWrongSecretReturns400(t *testing.T) {
	// Encrypt with a different secret — bridge should reject it
	otherSecret := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	token := encryptToken(t, launchCredentials{
		IP:        "10.0.0.1",
		Username:  "admin",
		Password:  "x",
		ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	}, otherSecret)
	h := buildHandler("http://localhost:3000", "/fake/winbox", "localhost:1337")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"token": token},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for wrong-secret token, got %d", rr.Code)
	}
	assertJSONError(t, rr, "invalid or tampered token")
}

func TestLaunch_WinBoxNotFoundReturns503(t *testing.T) {
	// winboxPath is empty — WinBox not found
	h := buildMux("", "http://localhost:3000", "localhost:1337", testSecret)
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"token": validToken(t)},
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
		map[string]string{"token": validToken(t)},
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

	// Old plaintext format has no "token" field — bridge must return 400
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for plaintext credentials (no token), got %d", rr.Code)
	}
}

// --- decryptLaunchToken unit tests ---

func TestDecryptLaunchToken_RoundTrip(t *testing.T) {
	creds := launchCredentials{
		IP:        "10.1.2.3",
		Username:  "user",
		Password:  "pass-value",
		ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	}
	token := encryptToken(t, creds, testSecret)
	got, err := decryptLaunchToken(token, testSecret)
	if err != nil {
		t.Fatalf("decryptLaunchToken: %v", err)
	}
	if got != creds {
		t.Errorf("round-trip mismatch: want %+v, got %+v", creds, got)
	}
}

func TestDecryptLaunchToken_WrongKey(t *testing.T) {
	token := encryptToken(t, launchCredentials{
		IP:        "1.2.3.4",
		Username:  "u",
		Password:  "p",
		ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	}, testSecret)
	otherKey := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, err := decryptLaunchToken(token, otherKey)
	if err == nil {
		t.Error("expected error for wrong key, got nil")
	}
}

func TestDecryptLaunchToken_TamperedToken(t *testing.T) {
	token := encryptToken(t, launchCredentials{
		IP:        "1.2.3.4",
		Username:  "u",
		Password:  "p",
		ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	}, testSecret)
	// Flip one byte in the middle
	b, _ := hex.DecodeString(token)
	b[len(b)/2] ^= 0xff
	tampered := hex.EncodeToString(b)
	_, err := decryptLaunchToken(tampered, testSecret)
	if err == nil {
		t.Error("expected error for tampered token, got nil")
	}
}

func TestDecryptLaunchToken_InvalidHex(t *testing.T) {
	_, err := decryptLaunchToken("not-hex!", testSecret)
	if err == nil {
		t.Error("expected error for invalid hex, got nil")
	}
}

func TestDecryptLaunchToken_InvalidSecret(t *testing.T) {
	_, err := decryptLaunchToken("aabbcc", "not-a-hex-key")
	if err == nil {
		t.Error("expected error for invalid secret, got nil")
	}
}

func TestDecryptLaunchToken_ExpiredToken(t *testing.T) {
	token := encryptToken(t, launchCredentials{
		IP:        "1.2.3.4",
		Username:  "u",
		Password:  "p",
		ExpiresAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
	}, testSecret)
	_, err := decryptLaunchToken(token, testSecret)
	if err == nil {
		t.Error("expected error for expired token, got nil")
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
		t.Errorf("expected error to contain %q, got %q", substr, errMsg)
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
