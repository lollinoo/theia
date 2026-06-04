package api

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
	if isAuthRoute(path) {
		return nil, true
	}
	if path == "/api/v1/health" || path == "/api/v1/prometheus/health" {
		return []string{domain.PermissionSettingsRead}, true
	}
	return protectedRoutePermissionRegistry.permissionsForRoute(method, path)
}

type routePermissionSpec struct {
	pattern     string
	permissions routePermissionPolicy
}

type routePermissionPolicy func(method string) []string

type routePermissionRegistry struct {
	specs []routePermissionSpec
}

func (s routePermissionSpec) matches(path string) bool {
	return matchRoutePattern(path, s.pattern)
}

func newRoutePermissionRegistry(specs []routePermissionSpec) routePermissionRegistry {
	return routePermissionRegistry{specs: append([]routePermissionSpec(nil), specs...)}
}

func (r routePermissionRegistry) permissionsForRoute(method, path string) ([]string, bool) {
	for _, spec := range r.specs {
		if spec.matches(path) {
			return spec.permissions(method), true
		}
	}
	return nil, false
}

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

var routePermissionSpecs = []routePermissionSpec{
	{pattern: "/api/v1/settings/me", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:   {domain.PermissionAccountManage},
		http.MethodHead:  {domain.PermissionAccountManage},
		http.MethodPatch: {domain.PermissionAccountManage},
	})},
	{pattern: "/api/v1/settings/bridge", permissions: readOnlyRoutePermissions(domain.PermissionAccountManage)},
	{pattern: "/api/v1/settings/bridge/secret", permissions: postOnlyRoutePermissions(domain.PermissionAccountManage)},
	{pattern: "/api/v1/settings/bridge/secret/rotate", permissions: postOnlyRoutePermissions(domain.PermissionAccountManage)},
	{pattern: "/api/v1/settings/bridge/secret/revoke", permissions: postOnlyRoutePermissions(domain.PermissionAccountManage)},
	{pattern: "/api/v1/settings/bridge/connector/config", permissions: readOnlyRoutePermissions(domain.PermissionAccountManage)},
	{pattern: "/api/v1/settings/bridge/connector/download/{os}/{arch}", permissions: readOnlyRoutePermissions(domain.PermissionAccountManage)},
	{pattern: "/api/v1/settings", permissions: readOnlyRoutePermissions(domain.PermissionSettingsRead)},
	{pattern: "/api/v1/settings/{key}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionSettingsRead},
		http.MethodHead: {domain.PermissionSettingsRead},
		http.MethodPut:  {domain.PermissionSettingsUpdate},
	})},

	{pattern: "/api/v1/topology/canvas", permissions: readOnlyRoutePermissions(domain.PermissionTopologyRead)},
	{pattern: "/api/v1/canvas", permissions: readOnlyRoutePermissions(domain.PermissionTopologyRead)},
	{pattern: "/api/v1/canvas/maps", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionTopologyRead},
		http.MethodHead: {domain.PermissionTopologyRead},
		http.MethodPost: {domain.PermissionTopologyUpdate},
	})},
	{pattern: "/api/v1/canvas/maps/{mapID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:    {domain.PermissionTopologyRead},
		http.MethodHead:   {domain.PermissionTopologyRead},
		http.MethodPatch:  {domain.PermissionTopologyUpdate},
		http.MethodDelete: {domain.PermissionTopologyUpdate},
	})},
	{pattern: "/api/v1/canvas/maps/{mapID}/duplicate", permissions: postOnlyRoutePermissions(domain.PermissionTopologyUpdate)},
	{pattern: "/api/v1/canvas/maps/{mapID}/primary", permissions: postOnlyRoutePermissions(domain.PermissionTopologyUpdate)},
	{pattern: "/api/v1/canvas/maps/{mapID}/topology", permissions: readOnlyRoutePermissions(domain.PermissionTopologyRead)},
	{pattern: "/api/v1/canvas/maps/{mapID}/bootstrap", permissions: readOnlyRoutePermissions(domain.PermissionTopologyRead)},
	{pattern: "/api/v1/canvas/maps/{mapID}/positions", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionTopologyRead},
		http.MethodHead: {domain.PermissionTopologyRead},
		http.MethodPut:  {domain.PermissionTopologyUpdate},
	})},
	{pattern: "/api/v1/canvas/maps/{mapID}/device-areas", permissions: putOnlyRoutePermissions(domain.PermissionTopologyUpdate)},
	{pattern: "/api/v1/canvas/maps/{mapID}/areas", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionTopologyRead},
		http.MethodHead: {domain.PermissionTopologyRead},
		http.MethodPost: {domain.PermissionTopologyUpdate},
	})},
	{pattern: "/api/v1/canvas/maps/{mapID}/areas/{areaID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodPut:    {domain.PermissionTopologyUpdate},
		http.MethodDelete: {domain.PermissionTopologyUpdate},
	})},
	{pattern: "/api/v1/canvas/maps/{mapID}/devices/{deviceID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodPost:   {domain.PermissionTopologyUpdate},
		http.MethodPatch:  {domain.PermissionTopologyUpdate},
		http.MethodDelete: {domain.PermissionTopologyUpdate},
	})},

	{pattern: "/api/v1/devices", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionDevicesRead},
		http.MethodHead: {domain.PermissionDevicesRead},
		http.MethodPost: {domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate},
	})},
	{pattern: "/api/v1/devices/batch", permissions: postOnlyRoutePermissions(domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate)},
	{pattern: "/api/v1/devices/orphans", permissions: readOnlyRoutePermissions(domain.PermissionDevicesRead)},
	{pattern: "/api/v1/devices/{deviceID}/winbox-credentials/reveal", permissions: postOnlyRoutePermissions(domain.PermissionCredentialsReveal)},
	{pattern: "/api/v1/devices/{deviceID}/credential-profiles/{profileID}", permissions: deleteOnlyRoutePermissions(domain.PermissionCredentialsUpdate)},
	{pattern: "/api/v1/devices/{deviceID}/credential-profiles", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionCredentialsRead},
		http.MethodHead: {domain.PermissionCredentialsRead},
		http.MethodPost: {domain.PermissionCredentialsUpdate},
	})},
	{pattern: "/api/v1/devices/{deviceID}/winbox-profile", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodPut:    {domain.PermissionCredentialsUpdate},
		http.MethodDelete: {domain.PermissionCredentialsUpdate},
	})},
	{pattern: "/api/v1/devices/{deviceID}/winbox-credentials", permissions: readOnlyRoutePermissions(domain.PermissionCredentialsRead)},
	{pattern: "/api/v1/devices/{deviceID}/backups/latest", permissions: readOnlyRoutePermissions(domain.PermissionBackupsRead)},
	{pattern: "/api/v1/devices/{deviceID}/backups", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionBackupsRead},
		http.MethodHead: {domain.PermissionBackupsRead},
		http.MethodPost: {domain.PermissionBackupsUpdate},
	})},
	{pattern: "/api/v1/devices/{deviceID}/interfaces", permissions: readOnlyRoutePermissions(domain.PermissionTopologyRead)},
	{pattern: "/api/v1/devices/{deviceID}/probe", permissions: postOnlyRoutePermissions(domain.PermissionDevicesUpdate)},
	{pattern: "/api/v1/devices/{deviceID}/snmp-test", permissions: postOnlyRoutePermissions(domain.PermissionDevicesUpdate)},
	{pattern: "/api/v1/devices/{deviceID}/topology-discovery", permissions: postOnlyRoutePermissions(domain.PermissionTopologyUpdate)},
	{pattern: "/api/v1/devices/{deviceID}/ssh-credentials/test", permissions: postOnlyRoutePermissions(domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate)},
	{pattern: "/api/v1/devices/{deviceID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:    {domain.PermissionDevicesRead},
		http.MethodHead:   {domain.PermissionDevicesRead},
		http.MethodPut:    {domain.PermissionDevicesUpdate},
		http.MethodDelete: {domain.PermissionDevicesDelete},
	})},

	{pattern: "/api/v1/links", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionTopologyRead},
		http.MethodHead: {domain.PermissionTopologyRead},
		http.MethodPost: {domain.PermissionTopologyUpdate},
	})},
	{pattern: "/api/v1/links/{linkID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodPut:    {domain.PermissionTopologyUpdate},
		http.MethodDelete: {domain.PermissionTopologyUpdate},
	})},
	{pattern: "/api/v1/positions", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionTopologyRead},
		http.MethodHead: {domain.PermissionTopologyRead},
		http.MethodPut:  {domain.PermissionTopologyUpdate},
	})},

	{pattern: "/api/v1/grafana/dashboard-profiles", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionSettingsRead},
		http.MethodHead: {domain.PermissionSettingsRead},
		http.MethodPost: {domain.PermissionSettingsUpdate},
	})},
	{pattern: "/api/v1/grafana/dashboard-profiles/{profileID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodPut:    {domain.PermissionSettingsUpdate},
		http.MethodDelete: {domain.PermissionSettingsUpdate},
	})},
	{pattern: "/api/v1/grafana/device-overrides/{deviceID}", permissions: putOnlyRoutePermissions(domain.PermissionSettingsUpdate)},

	{pattern: "/api/v1/snmp-profiles/{profileID}/reveal", permissions: postOnlyRoutePermissions(domain.PermissionCredentialsReveal)},
	{pattern: "/api/v1/snmp-profiles", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionCredentialsRead},
		http.MethodHead: {domain.PermissionCredentialsRead},
		http.MethodPost: {domain.PermissionCredentialsUpdate},
	})},
	{pattern: "/api/v1/snmp-profiles/{profileID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:    {domain.PermissionCredentialsRead},
		http.MethodHead:   {domain.PermissionCredentialsRead},
		http.MethodPut:    {domain.PermissionCredentialsUpdate},
		http.MethodDelete: {domain.PermissionCredentialsUpdate},
	})},
	{pattern: "/api/v1/credential-profiles/{profileID}/test", permissions: postOnlyRoutePermissions(domain.PermissionCredentialsUpdate)},
	{pattern: "/api/v1/credential-profiles", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionCredentialsRead},
		http.MethodHead: {domain.PermissionCredentialsRead},
		http.MethodPost: {domain.PermissionCredentialsUpdate},
	})},
	{pattern: "/api/v1/credential-profiles/{profileID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:    {domain.PermissionCredentialsRead},
		http.MethodHead:   {domain.PermissionCredentialsRead},
		http.MethodPut:    {domain.PermissionCredentialsUpdate},
		http.MethodDelete: {domain.PermissionCredentialsUpdate},
	})},

	{pattern: "/api/v1/areas", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionTopologyRead},
		http.MethodHead: {domain.PermissionTopologyRead},
		http.MethodPost: {domain.PermissionTopologyUpdate},
	})},
	{pattern: "/api/v1/areas/{areaID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:    {domain.PermissionTopologyRead},
		http.MethodHead:   {domain.PermissionTopologyRead},
		http.MethodPut:    {domain.PermissionTopologyUpdate},
		http.MethodDelete: {domain.PermissionTopologyUpdate},
	})},

	{pattern: "/api/v1/backups/bulk/status", permissions: readOnlyRoutePermissions(domain.PermissionBackupsRead)},
	{pattern: "/api/v1/backups/bulk-runs/latest", permissions: readOnlyRoutePermissions(domain.PermissionBackupsRead)},
	{pattern: "/api/v1/backups/bulk-runs", permissions: postOnlyRoutePermissions(domain.PermissionBackupsUpdate)},
	{pattern: "/api/v1/backups/bulk-runs/{runID}/pause", permissions: postOnlyRoutePermissions(domain.PermissionBackupsUpdate)},
	{pattern: "/api/v1/backups/bulk-runs/{runID}/resume", permissions: postOnlyRoutePermissions(domain.PermissionBackupsUpdate)},
	{pattern: "/api/v1/backups/bulk-runs/{runID}/cancel", permissions: postOnlyRoutePermissions(domain.PermissionBackupsUpdate)},
	{pattern: "/api/v1/backups/bulk-runs/{runID}", permissions: readOnlyRoutePermissions(domain.PermissionBackupsRead)},
	{pattern: "/api/v1/backups/bulk", permissions: postOnlyRoutePermissions(domain.PermissionBackupsUpdate)},
	{pattern: "/api/v1/backups/bulk-download", permissions: postOnlyRoutePermissions(domain.PermissionBackupsUpdate)},
	{pattern: "/api/v1/backup-jobs/{jobID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:    {domain.PermissionBackupsRead},
		http.MethodHead:   {domain.PermissionBackupsRead},
		http.MethodDelete: {domain.PermissionBackupsUpdate},
	})},
	{pattern: "/api/v1/backup-files/{fileID}/download", permissions: readOnlyRoutePermissions(domain.PermissionBackupsRead)},
	{pattern: "/api/v1/backup-files/{fileID}/content", permissions: readOnlyRoutePermissions(domain.PermissionBackupsRead)},

	{pattern: "/api/v1/vendors", permissions: readOnlyRoutePermissions(domain.PermissionSettingsRead)},
	{pattern: "/api/v1/vendors/{vendorID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionSettingsRead},
		http.MethodHead: {domain.PermissionSettingsRead},
		http.MethodPut:  {domain.PermissionSettingsUpdate},
	})},

	{pattern: "/api/v1/instance-backups", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionBackupsRead},
		http.MethodHead: {domain.PermissionBackupsRead},
		http.MethodPost: {domain.PermissionBackupsUpdate},
	})},
	{pattern: "/api/v1/instance-backups/restore", permissions: postOnlyRoutePermissions(domain.PermissionBackupsUpdate)},
	{pattern: "/api/v1/instance-backups/{backupID}/download", permissions: readOnlyRoutePermissions(domain.PermissionBackupsRead)},
	{pattern: "/api/v1/instance-backups/{backupID}/cancel", permissions: postOnlyRoutePermissions(domain.PermissionBackupsUpdate)},
	{pattern: "/api/v1/instance-backups/{backupID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:    {domain.PermissionBackupsRead},
		http.MethodHead:   {domain.PermissionBackupsRead},
		http.MethodDelete: {domain.PermissionBackupsUpdate},
	})},

	{pattern: "/api/v1/bridge/download/{os}/{arch}", permissions: readOnlyRoutePermissions(domain.PermissionSettingsRead)},
	{pattern: "/api/v1/bridge/launch-requests/{deviceID}", permissions: postOnlyRoutePermissions(domain.PermissionBridgeTokenCreate)},
	{pattern: "/api/v1/bridge/token/{deviceID}", permissions: postOnlyRoutePermissions(domain.PermissionBridgeTokenCreate)},

	{pattern: "/api/v1/admin/dashboard", permissions: readOnlyRoutePermissions(domain.PermissionAdminDashboard)},
	{pattern: "/api/v1/admin/users", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:  {domain.PermissionUsersRead},
		http.MethodHead: {domain.PermissionUsersRead},
		http.MethodPost: {domain.PermissionUsersCreate, domain.PermissionUsersUpdate},
	})},
	{pattern: "/api/v1/admin/users/{userID}/status", permissions: patchOnlyRoutePermissions(domain.PermissionUsersUpdate)},
	{pattern: "/api/v1/admin/users/{userID}/roles/{roleID}", permissions: deleteOnlyRoutePermissions(domain.PermissionRolesAssign)},
	{pattern: "/api/v1/admin/users/{userID}/roles", permissions: postOnlyRoutePermissions(domain.PermissionRolesAssign)},
	{pattern: "/api/v1/admin/users/{userID}/password-reset", permissions: postOnlyRoutePermissions(domain.PermissionUsersUpdate)},
	{pattern: "/api/v1/admin/users/{userID}", permissions: routePermissionsByMethod(map[string][]string{
		http.MethodGet:   {domain.PermissionUsersRead},
		http.MethodHead:  {domain.PermissionUsersRead},
		http.MethodPatch: {domain.PermissionUsersUpdate},
	})},
	{pattern: "/api/v1/admin/roles", permissions: readOnlyRoutePermissions(domain.PermissionRolesRead)},
	{pattern: "/api/v1/admin/permissions", permissions: readOnlyRoutePermissions(domain.PermissionRolesRead)},
	{pattern: "/api/v1/admin/audit-logs", permissions: readOnlyRoutePermissions(domain.PermissionAuditLogsRead)},
}

var protectedRoutePermissionRegistry = newRoutePermissionRegistry(routePermissionSpecs)

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

func splitRouteSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

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
