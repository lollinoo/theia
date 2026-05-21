package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testOperatorToken = "0123456789abcdef0123456789abcdef"

func TestAuthenticateRequestAcceptsBearerTokenAndSubject(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer "+testOperatorToken)
	req.Header.Set("X-Theia-Operator", "alice")

	subject, ok := AuthenticateRequest(req, testOperatorToken, nil)
	if !ok {
		t.Fatal("AuthenticateRequest rejected valid bearer token")
	}
	if !subject.Authenticated || subject.Name != "alice" {
		t.Fatalf("subject = %+v, want authenticated alice", subject)
	}
}

func TestAuthenticateRequestRejectsMissingTokenWhenConfigured(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)

	if _, ok := AuthenticateRequest(req, testOperatorToken, nil); ok {
		t.Fatal("AuthenticateRequest accepted missing token")
	}
}

func TestSessionManagerCreatesAndVerifiesHttpOnlyCookie(t *testing.T) {
	manager := NewSessionManager(testOperatorToken)
	now := time.Unix(1_700_000_000, 0)
	manager.now = func() time.Time { return now }

	cookie, _, ok := manager.CreateCookie("alice", true)
	if !ok {
		t.Fatal("CreateCookie failed")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || !cookie.Secure {
		t.Fatalf("cookie security flags = HttpOnly:%t SameSite:%v Secure:%t", cookie.HttpOnly, cookie.SameSite, cookie.Secure)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	req.AddCookie(cookie)
	subject, ok := manager.SubjectFromRequest(req)
	if !ok {
		t.Fatal("SubjectFromRequest rejected valid cookie")
	}
	if subject.Name != "alice" || !subject.Authenticated {
		t.Fatalf("subject = %+v, want authenticated alice", subject)
	}
}

func TestSessionManagerRejectsExpiredCookie(t *testing.T) {
	manager := NewSessionManager(testOperatorToken)
	now := time.Unix(1_700_000_000, 0)
	manager.now = func() time.Time { return now }
	cookie, _, ok := manager.CreateCookie("alice", false)
	if !ok {
		t.Fatal("CreateCookie failed")
	}

	manager.now = func() time.Time { return now.Add(defaultSessionTTL + time.Second) }
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	req.AddCookie(cookie)
	if _, ok := manager.SubjectFromRequest(req); ok {
		t.Fatal("SubjectFromRequest accepted expired cookie")
	}
}

func TestSessionManagerRejectsTamperedCookie(t *testing.T) {
	manager := NewSessionManager(testOperatorToken)
	cookie, _, ok := manager.CreateCookie("alice", false)
	if !ok {
		t.Fatal("CreateCookie failed")
	}
	cookie.Value += "tampered"

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	req.AddCookie(cookie)
	if _, ok := manager.SubjectFromRequest(req); ok {
		t.Fatal("SubjectFromRequest accepted a tampered cookie")
	}
}

func TestOriginAllowedRejectsUnlistedCrossOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://theia.local/api/v1/devices", nil)
	req.Host = "theia.local"
	req.Header.Set("Origin", "https://evil.example")

	if OriginAllowed(req, []string{"https://ops.example"}) {
		t.Fatal("OriginAllowed accepted unlisted origin")
	}
}

func TestOriginAllowedAcceptsSameHostAndConfiguredOrigin(t *testing.T) {
	sameHostReq := httptest.NewRequest(http.MethodGet, "http://theia.local/api/v1/devices", nil)
	sameHostReq.Host = "theia.local"
	sameHostReq.Header.Set("Origin", "http://theia.local")
	if !OriginAllowed(sameHostReq, nil) {
		t.Fatal("OriginAllowed rejected same-host origin")
	}

	configuredReq := httptest.NewRequest(http.MethodGet, "http://backend.local/api/v1/devices", nil)
	configuredReq.Host = "backend.local"
	configuredReq.Header.Set("Origin", "https://ops.example")
	if !OriginAllowed(configuredReq, []string{"https://ops.example"}) {
		t.Fatal("OriginAllowed rejected configured origin")
	}
}
