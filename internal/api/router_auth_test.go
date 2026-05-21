package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRouterRequiresOperatorAuthForProtectedSurface(t *testing.T) {
	router := NewRouter(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		WithSecurity(SecurityConfig{OperatorToken: "0123456789abcdef0123456789abcdef"}),
	)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "settings", method: http.MethodPut, path: "/api/v1/settings/bridge.secret"},
		{name: "bridge token", method: http.MethodPost, path: "/api/v1/bridge/token/device-1"},
		{name: "health", method: http.MethodGet, path: "/api/v1/health"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestNewRouterLeavesSessionEndpointPublic(t *testing.T) {
	router := NewRouter(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		WithSecurity(SecurityConfig{OperatorToken: "0123456789abcdef0123456789abcdef"}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
