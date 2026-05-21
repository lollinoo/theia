package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
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

func requireAuthenticatedOperator(w http.ResponseWriter, r *http.Request, action string) (security.OperatorSubject, bool) {
	subject := OperatorSubjectFromRequest(r)
	if subject.Authenticated {
		return subject, true
	}
	writeError(w, http.StatusForbidden, action+" requires an authenticated user")
	return subject, false
}

func requireAuthenticatedUser(w http.ResponseWriter, r *http.Request, action string) (*service.AuthenticatedUser, bool) {
	user, ok := AuthenticatedUserFromRequest(r)
	if ok {
		return user, true
	}
	writeError(w, http.StatusForbidden, action+" requires an authenticated user")
	return nil, false
}

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

func sessionTokenFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(authSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return strings.TrimSpace(cookie.Value), true
}

func requiresCSRF(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return r.URL.Path != "/api/v1/auth/login"
	default:
		return false
	}
}

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

func isInvalidCurrentUserError(err error) bool {
	return errors.Is(err, service.ErrInvalidSession) ||
		errors.Is(err, service.ErrUserDisabled) ||
		errors.Is(err, service.ErrUserLocked)
}

func passwordChangeAllowedPath(path string) bool {
	switch path {
	case "/api/v1/auth/me", "/api/v1/auth/logout", "/api/v1/auth/password/change":
		return true
	default:
		return false
	}
}

func writeAuthCodeError(w http.ResponseWriter, code int, errorCode, message string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
		"code":  errorCode,
	})
}

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

func requiredPermissionsForRoute(method, path string) ([]string, bool) {
	if path == "/api/v1/ws" {
		return []string{domain.PermissionTopologyRead}, true
	}
	if strings.HasPrefix(path, "/api/v1/auth/") || path == "/api/v1/session" {
		return nil, true
	}
	if path == "/api/v1/health" || path == "/api/v1/prometheus/health" {
		return []string{domain.PermissionSettingsRead}, true
	}
	if strings.HasPrefix(path, "/api/v1/settings") {
		return permissionsForMethod(method, domain.PermissionSettingsRead, "", domain.PermissionSettingsUpdate, ""), true
	}
	if path == "/api/v1/topology/canvas" || path == "/api/v1/canvas" {
		return []string{domain.PermissionTopologyRead}, true
	}
	if strings.HasPrefix(path, "/api/v1/canvas/") || path == "/api/v1/canvas/maps" {
		return permissionsForMethod(method, domain.PermissionTopologyRead, domain.PermissionTopologyUpdate, domain.PermissionTopologyUpdate, domain.PermissionTopologyUpdate), true
	}
	if strings.HasPrefix(path, "/api/v1/devices/") {
		return deviceRoutePermissions(method, path)
	}
	if path == "/api/v1/devices" || path == "/api/v1/devices/batch" || path == "/api/v1/devices/orphans" {
		return permissionsForMethod(method, domain.PermissionDevicesRead, domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate, domain.PermissionDevicesDelete), true
	}
	if path == "/api/v1/links" || strings.HasPrefix(path, "/api/v1/links/") || path == "/api/v1/positions" {
		return permissionsForMethod(method, domain.PermissionTopologyRead, domain.PermissionTopologyUpdate, domain.PermissionTopologyUpdate, domain.PermissionTopologyUpdate), true
	}
	if path == "/api/v1/snmp-profiles" || strings.HasPrefix(path, "/api/v1/snmp-profiles/") ||
		path == "/api/v1/credential-profiles" || strings.HasPrefix(path, "/api/v1/credential-profiles/") {
		return credentialRoutePermissions(method, path), true
	}
	if path == "/api/v1/areas" || strings.HasPrefix(path, "/api/v1/areas/") {
		return permissionsForMethod(method, domain.PermissionTopologyRead, domain.PermissionTopologyUpdate, domain.PermissionTopologyUpdate, domain.PermissionTopologyUpdate), true
	}
	if strings.HasPrefix(path, "/api/v1/backups/") || strings.HasPrefix(path, "/api/v1/backup-jobs/") || strings.HasPrefix(path, "/api/v1/backup-files/") {
		return backupRoutePermissions(method), true
	}
	if path == "/api/v1/vendors" || strings.HasPrefix(path, "/api/v1/vendors/") {
		return permissionsForMethod(method, domain.PermissionSettingsRead, "", domain.PermissionSettingsUpdate, ""), true
	}
	if path == "/api/v1/instance-backups" || strings.HasPrefix(path, "/api/v1/instance-backups/") {
		return backupRoutePermissions(method), true
	}
	if strings.HasPrefix(path, "/api/v1/bridge/token/") {
		return []string{domain.PermissionBridgeTokenCreate}, true
	}
	if strings.HasPrefix(path, "/api/v1/bridge/download/") {
		return []string{domain.PermissionSettingsRead}, true
	}
	if strings.HasPrefix(path, "/api/v1/users") {
		return permissionsForMethod(method, domain.PermissionUsersRead, domain.PermissionUsersCreate, domain.PermissionUsersUpdate, domain.PermissionUsersDelete), true
	}
	if strings.HasPrefix(path, "/api/v1/roles") {
		return permissionsForMethod(method, domain.PermissionRolesRead, domain.PermissionRolesAssign, domain.PermissionRolesUpdate, domain.PermissionRolesUpdate), true
	}
	if strings.HasPrefix(path, "/api/v1/admin") {
		return []string{domain.PermissionAdminDashboard}, true
	}
	return nil, false
}

func deviceRoutePermissions(method, path string) ([]string, bool) {
	switch {
	case isWinboxCredentialsRevealPath(path):
		return []string{domain.PermissionCredentialsReveal}, true
	case strings.Contains(path, "/credential-profiles") || strings.HasSuffix(path, "/winbox-profile"):
		return permissionsForMethod(method, domain.PermissionCredentialsRead, domain.PermissionCredentialsUpdate, domain.PermissionCredentialsUpdate, domain.PermissionCredentialsUpdate), true
	case strings.HasSuffix(path, "/winbox-credentials"):
		return []string{domain.PermissionCredentialsRead}, true
	case strings.Contains(path, "/backups"):
		return backupRoutePermissions(method), true
	case strings.HasSuffix(path, "/interfaces"):
		return []string{domain.PermissionTopologyRead}, true
	case strings.HasSuffix(path, "/probe") || strings.HasSuffix(path, "/snmp-test"):
		return []string{domain.PermissionDevicesUpdate}, true
	case strings.HasSuffix(path, "/topology-discovery"):
		return []string{domain.PermissionTopologyUpdate}, true
	default:
		return permissionsForMethod(method, domain.PermissionDevicesRead, domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate, domain.PermissionDevicesDelete), true
	}
}

func credentialRoutePermissions(method, path string) []string {
	if strings.HasSuffix(path, "/reveal") {
		return []string{domain.PermissionCredentialsReveal}
	}
	return permissionsForMethod(method, domain.PermissionCredentialsRead, domain.PermissionCredentialsUpdate, domain.PermissionCredentialsUpdate, domain.PermissionCredentialsUpdate)
}

func backupRoutePermissions(method string) []string {
	return permissionsForMethod(method, domain.PermissionBackupsRead, domain.PermissionBackupsUpdate, domain.PermissionBackupsUpdate, domain.PermissionBackupsUpdate)
}

func permissionsForMethod(method, read, create, update, deletePermission string) []string {
	switch method {
	case http.MethodGet, http.MethodHead:
		return nonEmptyPermissions(read)
	case http.MethodPost:
		return nonEmptyPermissions(create, update)
	case http.MethodPut, http.MethodPatch:
		return nonEmptyPermissions(update)
	case http.MethodDelete:
		return nonEmptyPermissions(deletePermission, update)
	default:
		return nil
	}
}

func nonEmptyPermissions(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

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

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
