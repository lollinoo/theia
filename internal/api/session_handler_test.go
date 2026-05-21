package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/security"
)

func TestSessionHandlerCreatesHttpOnlySessionCookie(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef"
	handler := NewSessionHandler(SecurityConfig{
		OperatorToken: token,
		Sessions:      security.NewSessionManager(token),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(`{"token":"`+token+`","operator":"alice"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != security.OperatorSessionCookieName || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v, want secure operator session cookie", cookie)
	}

	var body sessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Authenticated || body.Subject != "alice" {
		t.Fatalf("response = %+v, want authenticated alice", body)
	}
}

func TestSessionHandlerRejectsInvalidToken(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef"
	handler := NewSessionHandler(SecurityConfig{
		OperatorToken: token,
		Sessions:      security.NewSessionManager(token),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(`{"token":"wrong"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("invalid login set a session cookie")
	}
}

func TestSessionHandlerGetReportsUnauthenticatedWithoutCookie(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef"
	handler := NewSessionHandler(SecurityConfig{
		OperatorToken: token,
		Sessions:      security.NewSessionManager(token),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body sessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Authenticated {
		t.Fatalf("response = %+v, want unauthenticated", body)
	}
}

func TestSessionHandlerDevelopmentWithoutTokenIsAnonymousAuthenticated(t *testing.T) {
	handler := NewSessionHandler(SecurityConfig{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body sessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Authenticated || body.Subject != "anonymous" {
		t.Fatalf("response = %+v, want anonymous authenticated", body)
	}
}
