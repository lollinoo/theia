package security

// This file exercises http behavior so refactors preserve the documented contract.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestNormalizedAllowedOriginsFiltersInvalidAndDuplicateValues(t *testing.T) {
	got := NormalizedAllowedOrigins([]string{
		"https://ops.example",
		"not an origin",
		"https://ops.example",
		"http://theia.local/path",
		"wss://ws.example",
	})

	want := []string{"https://ops.example", "wss://ws.example"}
	if len(got) != len(want) {
		t.Fatalf("normalized origins = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalized origins = %#v, want %#v", got, want)
		}
	}
}

func TestSecureCookieForRequestUsesTLSOrForwardedProto(t *testing.T) {
	plainReq := httptest.NewRequest(http.MethodGet, "http://theia.local/api/v1/auth/me", nil)
	if SecureCookieForRequest(plainReq) {
		t.Fatal("SecureCookieForRequest returned true for plain HTTP request")
	}

	forwardedReq := httptest.NewRequest(http.MethodGet, "http://theia.local/api/v1/auth/me", nil)
	forwardedReq.Header.Set("X-Forwarded-Proto", "https")
	if !SecureCookieForRequest(forwardedReq) {
		t.Fatal("SecureCookieForRequest ignored X-Forwarded-Proto=https")
	}

	tlsReq := httptest.NewRequest(http.MethodGet, "https://theia.local/api/v1/auth/me", nil)
	if !SecureCookieForRequest(tlsReq) {
		t.Fatal("SecureCookieForRequest returned false for TLS request")
	}
}
