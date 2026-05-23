package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/security"
	"github.com/lollinoo/theia/internal/service"
)

const (
	authSessionCookieName = "theia_session"
	authCSRFCookieName    = "theia_csrf"
	csrfHeaderName        = "X-CSRF-Token"
)

type authProvider interface {
	Login(ctx context.Context, input service.LoginInput) (*service.LoginResult, error)
	CurrentUser(ctx context.Context, rawSessionToken string) (*service.AuthenticatedUser, error)
	Logout(ctx context.Context, rawSessionToken string) error
	ChangePassword(ctx context.Context, input service.PasswordChangeInput) error
	CompletePasswordReset(ctx context.Context, input service.PasswordResetCompleteInput) error
	ValidateCSRF(ctx context.Context, rawSessionToken, rawCSRFToken string) error
	RequirePermission(user *service.AuthenticatedUser, permissionKey string) error
	RequireRole(user *service.AuthenticatedUser, roleID string) error
}

// AuthHandler manages first-party password sessions.
type AuthHandler struct {
	auth authProvider
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(auth authProvider) *AuthHandler {
	return &AuthHandler{auth: auth}
}

type authResponse struct {
	Authenticated bool              `json:"authenticated"`
	User          *safeUserResponse `json:"user,omitempty"`
}

type safeUserResponse struct {
	ID                 string   `json:"id"`
	Username           string   `json:"username"`
	Email              string   `json:"email"`
	DisplayName        string   `json:"display_name"`
	Status             string   `json:"status"`
	MustChangePassword bool     `json:"must_change_password"`
	Roles              []string `json:"roles"`
	Permissions        []string `json:"permissions"`
}

// ServeHTTP handles /api/v1/auth/* and read-only legacy session aliases.
func (h *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/auth/login":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleLogin(w, r)
	case "/api/v1/auth/logout":
		if r.Method != http.MethodPost && r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleLogout(w, r)
	case "/api/v1/auth/me":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleMe(w, r)
	case "/api/v1/me":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleMe(w, r)
	case "/api/v1/auth/password/change":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handlePasswordChange(w, r)
	case "/api/v1/auth/password/reset":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handlePasswordReset(w, r)
	case "/api/v1/session":
		h.handleLegacySession(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication service not configured")
		return
	}
	var req struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := h.auth.Login(r.Context(), service.LoginInput{
		Identifier: req.Identifier,
		Password:   req.Password,
		IPAddress:  clientIPAddress(r),
		UserAgent:  r.UserAgent(),
	})
	if err != nil {
		writeAuthServiceError(w, err)
		return
	}
	secure := security.SecureCookieForRequest(r)
	http.SetCookie(w, authSessionCookie(result.SessionToken, result.ExpiresAt, secure))
	http.SetCookie(w, authCSRFCookie(result.CSRFToken, result.ExpiresAt, secure))
	json.NewEncoder(w).Encode(authResponse{
		Authenticated: true,
		User:          safeUserFromAggregate(result.User),
	})
}

func (h *AuthHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	secure := security.SecureCookieForRequest(r)
	rawSessionToken, hasSession := sessionTokenFromRequest(r)
	if hasSession && h.auth != nil {
		if !validateRequestCSRF(w, r, h.auth, rawSessionToken) {
			return
		}
		if err := h.auth.Logout(r.Context(), rawSessionToken); err != nil && !errors.Is(err, service.ErrInvalidSession) {
			writeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
	}
	http.SetCookie(w, clearAuthCookie(authSessionCookieName, true, secure))
	http.SetCookie(w, clearAuthCookie(authCSRFCookieName, false, secure))
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUserOrAnonymous(w, r)
	if !ok || user == nil {
		json.NewEncoder(w).Encode(authResponse{Authenticated: false})
		return
	}
	json.NewEncoder(w).Encode(authResponse{
		Authenticated: true,
		User:          safeUserFromAggregate(user.User),
	})
}

func (h *AuthHandler) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	user, rawSessionToken, ok := h.requireCurrentUser(w, r)
	if !ok {
		return
	}
	if !validateRequestCSRF(w, r, h.auth, rawSessionToken) {
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.CurrentPassword) == "" || strings.TrimSpace(req.NewPassword) == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}
	sessionID := user.Session.ID
	if err := h.auth.ChangePassword(r.Context(), service.PasswordChangeInput{
		UserID:           user.User.User.ID,
		CurrentSessionID: &sessionID,
		CurrentPassword:  req.CurrentPassword,
		NewPassword:      req.NewPassword,
	}); err != nil {
		writePasswordChangeError(w, err)
		return
	}
	updated, err := h.auth.CurrentUser(r.Context(), rawSessionToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	json.NewEncoder(w).Encode(authResponse{
		Authenticated: true,
		User:          safeUserFromAggregate(updated.User),
	})
}

func (h *AuthHandler) handlePasswordReset(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication service not configured")
		return
	}
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Token) == "" || strings.TrimSpace(req.NewPassword) == "" {
		writeError(w, http.StatusBadRequest, "token and new_password are required")
		return
	}
	if err := h.auth.CompletePasswordReset(r.Context(), service.PasswordResetCompleteInput{
		Token:       req.Token,
		NewPassword: req.NewPassword,
	}); err != nil {
		writePasswordResetError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) handleLegacySession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleMe(w, r)
	case http.MethodDelete:
		h.handleLogout(w, r)
	case http.MethodPost:
		writeError(w, http.StatusGone, "legacy session login is no longer supported; use /api/v1/auth/login")
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *AuthHandler) currentUserOrAnonymous(w http.ResponseWriter, r *http.Request) (*service.AuthenticatedUser, bool) {
	if h.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication service not configured")
		return nil, false
	}
	rawSessionToken, ok := sessionTokenFromRequest(r)
	if !ok {
		return nil, true
	}
	user, err := h.auth.CurrentUser(r.Context(), rawSessionToken)
	if err != nil {
		if isInvalidCurrentUserError(err) {
			return nil, true
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return nil, false
	}
	return user, true
}

func (h *AuthHandler) requireCurrentUser(w http.ResponseWriter, r *http.Request) (*service.AuthenticatedUser, string, bool) {
	if h.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication service not configured")
		return nil, "", false
	}
	rawSessionToken, ok := sessionTokenFromRequest(r)
	if !ok {
		writeAuthCodeError(w, http.StatusUnauthorized, "authentication_required", "authentication required")
		return nil, "", false
	}
	user, err := h.auth.CurrentUser(r.Context(), rawSessionToken)
	if err != nil {
		if isInvalidCurrentUserError(err) {
			writeAuthCodeError(w, http.StatusUnauthorized, "authentication_required", "authentication required")
			return nil, "", false
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return nil, "", false
	}
	return user, rawSessionToken, true
}

func authSessionCookie(value string, expiresAt time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     authSessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   cookieMaxAge(expiresAt),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	}
}

func authCSRFCookie(value string, expiresAt time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     authCSRFCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   cookieMaxAge(expiresAt),
		HttpOnly: false,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	}
}

func clearAuthCookie(name string, httpOnly bool, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: httpOnly,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	}
}

func cookieMaxAge(expiresAt time.Time) int {
	seconds := int(time.Until(expiresAt).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}

func safeUserFromAggregate(aggregate domain.UserWithRolesAndPermissions) *safeUserResponse {
	roles := make([]string, 0, len(aggregate.Roles))
	for _, role := range aggregate.Roles {
		if role.ID != "" {
			roles = append(roles, role.ID)
		}
	}
	permissions := make([]string, 0, len(aggregate.Permissions))
	for _, permission := range aggregate.Permissions {
		if permission.Key != "" {
			permissions = append(permissions, permission.Key)
		}
	}
	sort.Strings(roles)
	sort.Strings(permissions)
	user := aggregate.User
	return &safeUserResponse{
		ID:                 user.ID.String(),
		Username:           user.Username,
		Email:              user.Email,
		DisplayName:        user.DisplayName,
		Status:             string(user.Status),
		MustChangePassword: user.MustChangePassword,
		Roles:              roles,
		Permissions:        permissions,
	}
}

func writeAuthServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		writeAuthCodeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
	case errors.Is(err, service.ErrUserDisabled), errors.Is(err, service.ErrUserLocked):
		writeAuthCodeError(w, http.StatusForbidden, "account_unavailable", "user cannot authenticate")
	default:
		writeError(w, http.StatusInternalServerError, "internal error", err)
	}
}

func writePasswordChangeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		writeAuthCodeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid current password")
	case errors.Is(err, service.ErrPasswordPolicyViolation):
		writeAuthCodeError(w, http.StatusBadRequest, "password_policy_violation", "password does not meet policy")
	default:
		writeError(w, http.StatusInternalServerError, "internal error", err)
	}
}

func writePasswordResetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		writeAuthCodeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid reset token")
	case errors.Is(err, service.ErrPasswordResetExpired):
		writeAuthCodeError(w, http.StatusGone, "password_reset_expired", "password reset token expired")
	case errors.Is(err, service.ErrPasswordPolicyViolation):
		writeAuthCodeError(w, http.StatusBadRequest, "password_policy_violation", "password does not meet policy")
	default:
		writeError(w, http.StatusInternalServerError, "internal error", err)
	}
}

func clientIPAddress(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}
