package api

// This file exercises routes metadata behavior so refactors preserve the documented contract.

import (
	"net/http"
	"testing"
)

func TestProtectedPermissionRegistryIsDerivedFromAPIRouteSpecs(t *testing.T) {
	derived := protectedRoutePermissionSpecs(apiRouteSpecs)

	if len(protectedRoutePermissionRegistry.specs) != len(derived) {
		t.Fatalf("protected registry has %d specs, want %d derived route specs", len(protectedRoutePermissionRegistry.specs), len(derived))
	}
	for i, want := range derived {
		got := protectedRoutePermissionRegistry.specs[i]
		if got.pattern != want.pattern {
			t.Fatalf("registry spec %d pattern = %q, want derived pattern %q", i, got.pattern, want.pattern)
		}
	}
}

func TestProtectedRouteSpecsDeclareNonEmptyMethodPermissions(t *testing.T) {
	for _, spec := range apiRouteSpecs {
		if spec.authMode != routeAuthProtected && spec.authMode != routeAuthWebSocket {
			continue
		}
		if len(spec.methodPolicies) == 0 {
			t.Fatalf("protected route %s has no method policies", spec.pattern)
		}
		for method, permissions := range spec.methodPolicies {
			if method == "" {
				t.Fatalf("protected route %s has an empty method", spec.pattern)
			}
			if len(permissions) == 0 {
				t.Fatalf("protected route %s %s has no permissions", method, spec.pattern)
			}
		}
	}
}

func TestPublicAndSpecialMiddlewareRoutesAreDeclaredInRouteMetadata(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		auth    routeAuthMode
		profile routeMiddlewareProfile
	}{
		{name: "auth login", method: http.MethodPost, path: "/api/v1/auth/login", auth: routeAuthPublic, profile: routeMiddlewarePublicJSONSmallBody},
		{name: "auth logout", method: http.MethodPost, path: "/api/v1/auth/logout", auth: routeAuthPublic, profile: routeMiddlewarePublicJSONSmallBody},
		{name: "auth me", method: http.MethodGet, path: "/api/v1/auth/me", auth: routeAuthPublic, profile: routeMiddlewarePublicJSONSmallBody},
		{name: "legacy me", method: http.MethodGet, path: "/api/v1/me", auth: routeAuthPublic, profile: routeMiddlewarePublicJSONSmallBody},
		{name: "password change", method: http.MethodPost, path: "/api/v1/auth/password/change", auth: routeAuthPublic, profile: routeMiddlewarePublicJSONSmallBody},
		{name: "password reset", method: http.MethodPost, path: "/api/v1/auth/password/reset", auth: routeAuthPublic, profile: routeMiddlewarePublicJSONSmallBody},
		{name: "legacy session", method: http.MethodGet, path: "/api/v1/session", auth: routeAuthPublic, profile: routeMiddlewarePublicJSONSmallBody},
		{name: "websocket", method: http.MethodGet, path: "/api/v1/ws", auth: routeAuthWebSocket, profile: routeMiddlewareWebSocketUpgrade},
		{name: "backup file download", method: http.MethodGet, path: "/api/v1/backup-files/file-1/download", auth: routeAuthProtected, profile: routeMiddlewareBinaryDownload},
		{name: "instance backup download", method: http.MethodGet, path: "/api/v1/instance-backups/backup-1/download", auth: routeAuthProtected, profile: routeMiddlewareBinaryDownload},
		{name: "bridge download", method: http.MethodGet, path: "/api/v1/bridge/download/linux/amd64", auth: routeAuthProtected, profile: routeMiddlewareBinaryDownload},
		{name: "restore upload", method: http.MethodPost, path: "/api/v1/instance-backups/restore", auth: routeAuthProtected, profile: routeMiddlewareRestoreUpload},
		{name: "bridge connector launch", method: http.MethodPost, path: "/api/v1/bridge/connector/launch", auth: routeAuthPublic, profile: routeMiddlewarePublicJSONSmallBody},
		{name: "health", method: http.MethodGet, path: "/api/v1/health", auth: routeAuthProtected, profile: routeMiddlewareNormalJSON},
		{name: "prometheus health", method: http.MethodGet, path: "/api/v1/prometheus/health", auth: routeAuthProtected, profile: routeMiddlewareNormalJSON},
		{name: "runtime overview", method: http.MethodGet, path: "/api/v1/runtime/overview", auth: routeAuthProtected, profile: routeMiddlewareNormalJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := apiRouteMetadata.match(tt.method, tt.path)
			if !ok {
				t.Fatalf("route metadata did not match %s %s", tt.method, tt.path)
			}
			if spec.authMode != tt.auth {
				t.Fatalf("authMode = %v, want %v", spec.authMode, tt.auth)
			}
			if spec.middlewareProfile != tt.profile {
				t.Fatalf("middlewareProfile = %v, want %v", spec.middlewareProfile, tt.profile)
			}
		})
	}
}

func TestIsAuthRouteUsesRouteMetadata(t *testing.T) {
	for _, spec := range apiRouteSpecs {
		if spec.authMode != routeAuthPublic || spec.handlerKey != routeHandlerAuth {
			continue
		}
		if !isAuthRoute(spec.pattern) {
			t.Fatalf("isAuthRoute(%q) = false for public auth route spec", spec.pattern)
		}
	}
}

func TestAuthRoutesDeclareAuthEndpointDispatch(t *testing.T) {
	for _, spec := range apiRouteSpecs {
		if spec.handlerKey != routeHandlerAuth {
			continue
		}
		if spec.authEndpoint == routeAuthEndpointNone {
			t.Fatalf("auth route %s does not declare auth endpoint dispatch", spec.pattern)
		}
	}
}

func TestRouteMetadataBuildsServeMuxRegistrations(t *testing.T) {
	handlers := make(map[routeHandlerKey]http.Handler)
	for _, spec := range apiRouteSpecs {
		handlers[spec.handlerKey] = http.NotFoundHandler()
	}

	registrations, err := routeMuxRegistrations(apiRouteSpecs, handlers)
	if err != nil {
		t.Fatalf("routeMuxRegistrations() error = %v", err)
	}
	if len(registrations) == 0 {
		t.Fatal("routeMuxRegistrations() returned no registrations")
	}

	seenPatterns := make(map[string]struct{}, len(registrations))
	for _, registration := range registrations {
		if registration.pattern == "" {
			t.Fatal("registration has empty pattern")
		}
		if registration.handler == nil {
			t.Fatalf("registration %s has nil handler", registration.pattern)
		}
		if _, exists := seenPatterns[registration.pattern]; exists {
			t.Fatalf("duplicate mux registration for %s", registration.pattern)
		}
		seenPatterns[registration.pattern] = struct{}{}
	}

	for _, spec := range apiRouteSpecs {
		if _, ok := seenPatterns[spec.serveMuxPattern]; !ok {
			t.Fatalf("route %s serveMuxPattern %s was not registered", spec.name, spec.serveMuxPattern)
		}
	}
}
