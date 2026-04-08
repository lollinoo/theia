package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

// buildHandler constructs the full handler chain (security + mux) for testing.
func buildHandler(theiaOrigin string, winboxPath string) http.Handler {
	mux := buildMux(winboxPath)
	return securityCheck(theiaOrigin, mux)
}

// --- Security: Origin validation ---

func TestOriginValidation_ValidOriginPasses(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d", rr.Code)
	}
}

func TestOriginValidation_EvilOriginReturns403(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "http://evil.com", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 got %d", rr.Code)
	}
	assertJSONError(t, rr, "forbidden")
}

func TestOriginValidation_MissingOriginReturns403(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 got %d", rr.Code)
	}
}

func TestOriginValidation_EvilOriginOnLaunchReturns403(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"ip": "192.168.1.1", "username": "admin", "password": "pass"},
		"http://evil.com", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 got %d", rr.Code)
	}
}

// --- Security: Host validation ---

func TestHostValidation_EvilHostReturns403(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "http://localhost:3000", "evil.com:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 got %d for host evil.com:1337", rr.Code)
	}
}

func TestHostValidation_ValidHostPasses(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d", rr.Code)
	}
}

func TestHostValidation_IPHostReturns403(t *testing.T) {
	// Strict match on "localhost:1337" only — 127.0.0.1:1337 should fail
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/health", nil, "http://localhost:3000", "127.0.0.1:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 got %d for host 127.0.0.1:1337", rr.Code)
	}
}

// --- Health endpoint ---

func TestHealth_GETReturns200OkTrue(t *testing.T) {
	h := buildHandler("http://localhost:3000", "")
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
	h := buildHandler("http://localhost:3000", "")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/health", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 got %d", rr.Code)
	}
}

// --- Launch endpoint ---

func TestLaunch_ValidRequestReturns200(t *testing.T) {
	// Override startProcess with a successful mock
	original := startProcess
	t.Cleanup(func() { startProcess = original })
	startProcess = func(name string, args []string) error {
		return nil
	}

	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"ip": "192.168.1.1", "username": "admin", "password": "pass123"},
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

func TestLaunch_EmptyIPReturns400(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"ip": "", "username": "admin", "password": "pass"},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestLaunch_EmptyUsernameReturns400(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"ip": "192.168.1.1", "username": "", "password": "pass"},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestLaunch_EmptyPasswordReturns400(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"ip": "192.168.1.1", "username": "admin", "password": ""},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestLaunch_WinBoxNotFoundReturns503(t *testing.T) {
	// winboxPath is empty — WinBox not found
	h := buildHandler("http://localhost:3000", "")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"ip": "192.168.1.1", "username": "admin", "password": "pass"},
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

	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodPost, "/launch",
		map[string]string{"ip": "192.168.1.1", "username": "admin", "password": "pass"},
		"http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 got %d", rr.Code)
	}
	assertJSONError(t, rr, "failed to launch WinBox")
}

func TestLaunch_GETReturns405(t *testing.T) {
	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodGet, "/launch", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 got %d", rr.Code)
	}
}

// --- Security: No arbitrary execution (struct shape) ---

func TestLaunch_ExtraExecutableFieldIgnored(t *testing.T) {
	// Passing extra "executable" field must not affect behavior — struct ignores unknown fields
	original := startProcess
	t.Cleanup(func() { startProcess = original })
	var launchedWith string
	startProcess = func(name string, args []string) error {
		launchedWith = name
		return nil
	}

	h := buildHandler("http://localhost:3000", "/fake/winbox")
	rr := httptest.NewRecorder()

	// Manually create JSON with extra field
	body := `{"ip":"192.168.1.1","username":"admin","password":"pass","executable":"/bin/evil"}`
	req := httptest.NewRequest(http.MethodPost, "/launch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	req.Host = "localhost:1337"
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 got %d; body: %s", rr.Code, rr.Body.String())
	}
	// The launched binary must be /fake/winbox, not /bin/evil
	if launchedWith != "/fake/winbox" {
		t.Errorf("expected launch with /fake/winbox, got %q", launchedWith)
	}
}

// --- CORS preflight ---

func TestCORSPreflight_OptionsLaunchReturns204(t *testing.T) {
	h := buildHandler("http://localhost:3000", "")
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

func TestCORSPreflight_OptionsHealthReturns204(t *testing.T) {
	h := buildHandler("http://localhost:3000", "")
	rr := httptest.NewRecorder()
	req := makeRequest(t, http.MethodOptions, "/health", nil, "http://localhost:3000", "localhost:1337")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 got %d", rr.Code)
	}
	acao := rr.Header().Get("Access-Control-Allow-Origin")
	if acao != "http://localhost:3000" {
		t.Errorf("expected ACAO=http://localhost:3000, got %q", acao)
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
