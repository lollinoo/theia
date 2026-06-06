package api

// This file defines middleware API routing, middleware, and permission policy behavior.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/security"
	"github.com/lollinoo/theia/internal/service"
)

// SecurityConfig controls HTTP authentication and browser origin policy.
type SecurityConfig struct {
	AllowedOrigins []string
}

type authenticatedUserContextKey struct{}

// JSONContentType sets the Content-Type header to application/json on all responses.
func JSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// RequestLogger logs each request with method, path, status code, and duration.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

// CORS applies the default same-origin browser policy.
func CORS(next http.Handler) http.Handler {
	return CORSWithConfig(SecurityConfig{})(next)
}

// CORSWithConfig echoes exact configured origins and same-host origins.
func CORSWithConfig(config SecurityConfig) func(http.Handler) http.Handler {
	allowedOrigins := security.NormalizedAllowedOrigins(config.AllowedOrigins)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin := r.Header.Get("Origin"); origin != "" && security.OriginAllowed(r, allowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OriginGuard rejects browser requests from origins outside the configured allowlist.
func OriginGuard(config SecurityConfig) func(http.Handler) http.Handler {
	allowedOrigins := security.NormalizedAllowedOrigins(config.AllowedOrigins)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Origin") != "" && !security.OriginAllowed(r, allowedOrigins) {
				writeError(w, http.StatusForbidden, "origin not allowed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserAuth requires a valid password-session cookie for protected API routes.
func UserAuth(auth authProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextRequest, user, rawSessionToken, ok := AuthenticateUserRequest(w, r, auth)
			if !ok {
				return
			}
			if user.User.User.MustChangePassword && !passwordChangeAllowedPath(r.URL.Path) {
				writeAuthCodeError(w, http.StatusForbidden, "password_change_required", "password change required")
				return
			}
			if requiresCSRF(r) && !validateRequestCSRF(w, r, auth, rawSessionToken) {
				return
			}
			permissions, known := requiredPermissionsForRoute(r.Method, r.URL.Path)
			if !known {
				writeAuthCodeError(w, http.StatusForbidden, "permission_denied", "permission denied")
				return
			}
			if !requireAnyPermission(w, auth, user, permissions) {
				return
			}
			next.ServeHTTP(w, nextRequest)
		})
	}
}

// AuthenticateUserRequest validates one request and returns it with user context.
func AuthenticateUserRequest(w http.ResponseWriter, r *http.Request, auth authProvider) (*http.Request, *service.AuthenticatedUser, string, bool) {
	if auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication service not configured")
		return r, nil, "", false
	}
	rawSessionToken, ok := sessionTokenFromRequest(r)
	if !ok {
		writeAuthCodeError(w, http.StatusUnauthorized, "authentication_required", "authentication required")
		return r, nil, "", false
	}
	user, err := auth.CurrentUser(r.Context(), rawSessionToken)
	if err != nil {
		if isInvalidCurrentUserError(err) {
			writeAuthCodeError(w, http.StatusUnauthorized, "authentication_required", "authentication required")
			return r, nil, "", false
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return r, nil, "", false
	}
	next := r.WithContext(withAuthenticatedUser(r.Context(), user))
	return next, user, rawSessionToken, true
}

// AuthenticatedUserFromRequest returns the authenticated user stored on a request.
func AuthenticatedUserFromRequest(r *http.Request) (*service.AuthenticatedUser, bool) {
	user, ok := r.Context().Value(authenticatedUserContextKey{}).(*service.AuthenticatedUser)
	return user, ok && user != nil
}

// withAuthenticatedUser stores the authenticated session user on the request context.
func withAuthenticatedUser(ctx context.Context, user *service.AuthenticatedUser) context.Context {
	return context.WithValue(ctx, authenticatedUserContextKey{}, user)
}

// OperatorSubjectFromRequest returns the authenticated user subject for legacy audit call sites.
func OperatorSubjectFromRequest(r *http.Request) security.OperatorSubject {
	user, ok := AuthenticatedUserFromRequest(r)
	if !ok {
		return security.AnonymousSubject
	}
	return security.OperatorSubject{Name: auditSubjectName(user), Authenticated: true}
}

// requireAuthenticatedOperator returns an audit subject or writes a forbidden response.
func requireAuthenticatedOperator(w http.ResponseWriter, r *http.Request, action string) (security.OperatorSubject, bool) {
	subject := OperatorSubjectFromRequest(r)
	if subject.Authenticated {
		return subject, true
	}
	writeError(w, http.StatusForbidden, action+" requires an authenticated user")
	return subject, false
}

// requireAuthenticatedUser returns the current user or writes a forbidden response.
func requireAuthenticatedUser(w http.ResponseWriter, r *http.Request, action string) (*service.AuthenticatedUser, bool) {
	user, ok := AuthenticatedUserFromRequest(r)
	if ok {
		return user, true
	}
	writeError(w, http.StatusForbidden, action+" requires an authenticated user")
	return nil, false
}

// requirePermission enforces one permission and maps RBAC/service failures to HTTP errors.
func requirePermission(w http.ResponseWriter, auth authProvider, user *service.AuthenticatedUser, permission string) bool {
	if auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication service not configured")
		return false
	}
	if err := auth.RequirePermission(user, permission); err != nil {
		if errors.Is(err, service.ErrPermissionDenied) {
			writeAuthCodeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return false
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return false
	}
	return true
}

// requireRole enforces one role and maps RBAC/service failures to HTTP errors.
func requireRole(w http.ResponseWriter, auth authProvider, user *service.AuthenticatedUser, roleID string) bool {
	if auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication service not configured")
		return false
	}
	if err := auth.RequireRole(user, roleID); err != nil {
		if errors.Is(err, service.ErrPermissionDenied) {
			writeAuthCodeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return false
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return false
	}
	return true
}

// requireAnyPermission allows any listed permission and rejects empty permission policies.
func requireAnyPermission(w http.ResponseWriter, auth authProvider, user *service.AuthenticatedUser, permissions []string) bool {
	if len(permissions) == 0 {
		writeAuthCodeError(w, http.StatusForbidden, "permission_denied", "permission denied")
		return false
	}
	for _, permission := range permissions {
		if auth.RequirePermission(user, permission) == nil {
			return true
		}
	}
	writeAuthCodeError(w, http.StatusForbidden, "permission_denied", "permission denied")
	return false
}

// sessionTokenFromRequest extracts the trimmed password-session cookie value.
func sessionTokenFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(authSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return strings.TrimSpace(cookie.Value), true
}

// requiresCSRF marks mutating authenticated routes as CSRF-protected.
func requiresCSRF(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return r.URL.Path != "/api/v1/auth/login"
	default:
		return false
	}
}

// validateRequestCSRF checks the CSRF header against the active session token.
func validateRequestCSRF(w http.ResponseWriter, r *http.Request, auth authProvider, rawSessionToken string) bool {
	csrfToken := strings.TrimSpace(r.Header.Get(csrfHeaderName))
	if csrfToken == "" {
		writeAuthCodeError(w, http.StatusForbidden, "csrf_required", "csrf token required")
		return false
	}
	if err := auth.ValidateCSRF(r.Context(), rawSessionToken, csrfToken); err != nil {
		if errors.Is(err, service.ErrInvalidSession) {
			writeAuthCodeError(w, http.StatusForbidden, "csrf_invalid", "csrf token invalid")
			return false
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return false
	}
	return true
}

// isInvalidCurrentUserError identifies service errors that invalidate a browser session.
func isInvalidCurrentUserError(err error) bool {
	return errors.Is(err, service.ErrInvalidSession) ||
		errors.Is(err, service.ErrUserDisabled) ||
		errors.Is(err, service.ErrUserLocked)
}

// passwordChangeAllowedPath permits only account-recovery routes while a password change is required.
func passwordChangeAllowedPath(path string) bool {
	switch path {
	case "/api/v1/auth/me", "/api/v1/auth/logout", "/api/v1/auth/password/change":
		return true
	default:
		return false
	}
}

// writeAuthCodeError emits the stable JSON shape used by auth and RBAC errors.
func writeAuthCodeError(w http.ResponseWriter, code int, errorCode, message string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
		"code":  errorCode,
	})
}

// auditSubjectName chooses a stable human-readable audit identity for a session user.
func auditSubjectName(user *service.AuthenticatedUser) string {
	if user == nil {
		return "anonymous"
	}
	if username := strings.TrimSpace(user.User.User.Username); username != "" {
		return username
	}
	if email := strings.TrimSpace(user.User.User.Email); email != "" {
		return email
	}
	if user.User.User.ID != uuid.Nil {
		return user.User.User.ID.String()
	}
	return "authenticated-user"
}

// requiredPermissionsForRoute is the auth adapter for protected route metadata.
func requiredPermissionsForRoute(method, path string) ([]string, bool) {
	spec, known := apiRouteMetadata.matchPath(path)
	if !known {
		return nil, false
	}
	if spec.authMode == routeAuthPublic {
		return nil, true
	}
	return protectedRoutePermissionRegistry.permissionsForRoute(method, path)
}

// routePermissionSpec binds one segment-exact route pattern to its method policy.
type routePermissionSpec struct {
	pattern     string
	permissions routePermissionPolicy
}

// routePermissionPolicy resolves permissions for one HTTP method.
type routePermissionPolicy func(method string) []string

// routePermissionRegistry performs ordered, segment-exact route policy lookup.
type routePermissionRegistry struct {
	specs []routePermissionSpec
}

// matches reports whether a concrete path has the same segment shape as the route pattern.
func (s routePermissionSpec) matches(path string) bool {
	return matchRoutePattern(path, s.pattern)
}

// newRoutePermissionRegistry copies permission specs so tests cannot mutate the caller's slice.
func newRoutePermissionRegistry(specs []routePermissionSpec) routePermissionRegistry {
	return routePermissionRegistry{specs: append([]routePermissionSpec(nil), specs...)}
}

// permissionsForRoute returns the permissions for a known route and whether the route matched.
func (r routePermissionRegistry) permissionsForRoute(method, path string) ([]string, bool) {
	for _, spec := range r.specs {
		if spec.matches(path) {
			return spec.permissions(method), true
		}
	}
	return nil, false
}

// validate rejects duplicate, shadowed, or policy-less metadata before it can grant access.
func (r routePermissionRegistry) validate() error {
	seenPatterns := make(map[string]struct{}, len(r.specs))
	for index, spec := range r.specs {
		if _, exists := seenPatterns[spec.pattern]; exists {
			return fmt.Errorf("duplicate route permission pattern %s", spec.pattern)
		}
		seenPatterns[spec.pattern] = struct{}{}
		if spec.permissions == nil {
			return fmt.Errorf("route permission pattern %s has no permission policy", spec.pattern)
		}

		for _, previous := range r.specs[:index] {
			if previous.matches(spec.pattern) {
				return fmt.Errorf(
					"route permission pattern %s is shadowed by earlier pattern %s",
					spec.pattern,
					previous.pattern,
				)
			}
		}
	}
	return nil
}

var protectedRoutePermissionRegistry = newRoutePermissionRegistry(protectedRoutePermissionSpecs(apiRouteSpecs))

// matchRoutePattern compares route patterns by path segment instead of raw string prefixes.
func matchRoutePattern(path, pattern string) bool {
	pathSegments := splitRouteSegments(path)
	patternSegments := splitRouteSegments(pattern)
	if len(pathSegments) != len(patternSegments) {
		return false
	}
	for i, patternSegment := range patternSegments {
		if isRoutePlaceholder(patternSegment) {
			if pathSegments[i] == "" {
				return false
			}
			continue
		}
		if pathSegments[i] != patternSegment {
			return false
		}
	}
	return true
}

// splitRouteSegments normalizes a route path into slash-delimited segments.
func splitRouteSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// isRoutePlaceholder identifies pattern segments like {deviceID}.
func isRoutePlaceholder(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") && len(segment) > 2
}

// routePermissionsByMethod returns a policy that grants permissions only for explicitly listed methods.
func routePermissionsByMethod(methodPermissions map[string][]string) routePermissionPolicy {
	return func(method string) []string {
		return nonEmptyPermissions(methodPermissions[method]...)
	}
}

// routePermissionsForMethods returns one permission set for a fixed set of supported methods.
func routePermissionsForMethods(methods []string, permissions ...string) routePermissionPolicy {
	methodPermissions := make(map[string][]string, len(methods))
	for _, method := range methods {
		methodPermissions[method] = append([]string(nil), permissions...)
	}
	return routePermissionsByMethod(methodPermissions)
}

// readOnlyRoutePermissions grants read permissions for GET and HEAD only.
func readOnlyRoutePermissions(permissions ...string) routePermissionPolicy {
	return routePermissionsForMethods([]string{http.MethodGet, http.MethodHead}, permissions...)
}

// postOnlyRoutePermissions grants permissions for POST only.
func postOnlyRoutePermissions(permissions ...string) routePermissionPolicy {
	return routePermissionsForMethods([]string{http.MethodPost}, permissions...)
}

// putOnlyRoutePermissions grants permissions for PUT only.
func putOnlyRoutePermissions(permissions ...string) routePermissionPolicy {
	return routePermissionsForMethods([]string{http.MethodPut}, permissions...)
}

// patchOnlyRoutePermissions grants permissions for PATCH only.
func patchOnlyRoutePermissions(permissions ...string) routePermissionPolicy {
	return routePermissionsForMethods([]string{http.MethodPatch}, permissions...)
}

// deleteOnlyRoutePermissions grants permissions for DELETE only.
func deleteOnlyRoutePermissions(permissions ...string) routePermissionPolicy {
	return routePermissionsForMethods([]string{http.MethodDelete}, permissions...)
}

// fixedRoutePermissions preserves registry validation tests that need a method-agnostic policy.
func fixedRoutePermissions(permissions ...string) routePermissionPolicy {
	return func(string) []string {
		return nonEmptyPermissions(permissions...)
	}
}

// nonEmptyPermissions trims, de-duplicates, and drops blank permission keys.
func nonEmptyPermissions(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// applyMiddleware layers protected API middleware in the order expected by auth tests.
func applyMiddleware(next http.Handler, config SecurityConfig, auth authProvider, includeJSON bool, bodyLimit int64) http.Handler {
	handler := next
	if includeJSON {
		handler = JSONContentType(handler)
	}
	if bodyLimit > 0 {
		handler = MaxBodySize(bodyLimit)(handler)
	}
	handler = UserAuth(auth)(handler)
	handler = OriginGuard(config)(handler)
	handler = RequestLogger(handler)
	handler = CORSWithConfig(config)(handler)
	return handler
}

// applyPublicMiddleware layers origin, logging, and optional body limits for public routes.
func applyPublicMiddleware(next http.Handler, config SecurityConfig, includeJSON bool, bodyLimit int64) http.Handler {
	handler := next
	if includeJSON {
		handler = JSONContentType(handler)
	}
	if bodyLimit > 0 {
		handler = MaxBodySize(bodyLimit)(handler)
	}
	handler = OriginGuard(config)(handler)
	handler = RequestLogger(handler)
	handler = CORSWithConfig(config)(handler)
	return handler
}

// MaxBodySize limits the size of request bodies to prevent memory exhaustion.
// When the limit is exceeded, subsequent reads return an error that triggers HTTP 413.
func MaxBodySize(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader records the response status before forwarding it to the wrapped writer.
func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
