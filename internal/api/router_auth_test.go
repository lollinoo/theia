package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

const (
	testSessionToken = "test-session-token"
	testCSRFToken    = "test-csrf-token"
)

func TestNewRouterRequiresUserSessionForProtectedSurface(t *testing.T) {
	router := newAuthTestRouter(newFakeAPIAuthProvider())

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "settings", method: http.MethodGet, path: "/api/v1/settings"},
		{name: "bridge token", method: http.MethodPost, path: "/api/v1/bridge/token/00000000-0000-0000-0000-000000000001"},
		{name: "health", method: http.MethodGet, path: "/api/v1/health"},
		{name: "websocket", method: http.MethodGet, path: "/api/v1/ws"},
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

func TestNewRouterRejectsBearerOperatorToken(t *testing.T) {
	router := newAuthTestRouter(newFakeAPIAuthProvider())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.Header.Set("Authorization", "Bearer 0123456789abcdef0123456789abcdef")
	req.Header.Set("X-Theia-Operator", "alice")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestNewRouterRejectsAuthenticatedUserMissingPermission(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(testSessionToken, testCSRFToken, testAPIUser("alice", false))
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouterRejectsNormalRoutesUntilPasswordChanged(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(
		testSessionToken,
		testCSRFToken,
		testAPIUser("bootstrap", true, domain.PermissionSettingsRead),
	)
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "password_change_required" {
		t.Fatalf("code = %q, want password_change_required", body["code"])
	}
}

func TestAuthPasswordChangeAllowedWhilePasswordChangeRequired(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	user := testAPIUser("bootstrap", true, domain.PermissionSettingsRead)
	auth.setSession(testSessionToken, testCSRFToken, user)
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/password/change",
		strings.NewReader(`{"current_password":"theia","new_password":"Correct Horse Battery Staple 2026!"}`),
	)
	addSessionCookie(req, testSessionToken)
	addCSRFCookieAndHeader(req, testCSRFToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Authenticated bool `json:"authenticated"`
		User          struct {
			MustChangePassword bool `json:"must_change_password"`
		} `json:"user"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Authenticated || body.User.MustChangePassword {
		t.Fatalf("response = %+v, want authenticated user with must_change_password=false", body)
	}
	if !auth.changePasswordCalled {
		t.Fatal("ChangePassword was not called")
	}
}

func TestAuthLoginSetsSessionAndCSRFCookies(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.login = &service.LoginResult{
		User:         testAPIUser("alice", false, domain.PermissionSettingsRead).User,
		SessionToken: testSessionToken,
		CSRFToken:    testCSRFToken,
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
	}
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identifier":"alice","password":"password"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	sessionCookie := findCookie(t, rec.Result().Cookies(), authSessionCookieName)
	if !sessionCookie.HttpOnly || sessionCookie.Value != testSessionToken {
		t.Fatalf("session cookie = %+v, want HttpOnly session token", sessionCookie)
	}
	csrfCookie := findCookie(t, rec.Result().Cookies(), authCSRFCookieName)
	if csrfCookie.HttpOnly || csrfCookie.Value != testCSRFToken {
		t.Fatalf("csrf cookie = %+v, want readable csrf token", csrfCookie)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"password_hash", "token_hash", testSessionToken, testCSRFToken} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("login response leaked %q in %s", forbidden, body)
		}
	}
}

func TestAuthMeReturnsSafeCurrentUserPayload(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(
		testSessionToken,
		testCSRFToken,
		testAPIUser("alice", false, domain.PermissionSettingsRead, domain.PermissionTopologyRead),
	)
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "password_hash") || strings.Contains(body, "token_hash") {
		t.Fatalf("me response exposed secret-bearing fields: %s", body)
	}
	var parsed struct {
		Authenticated bool `json:"authenticated"`
		User          struct {
			Username           string   `json:"username"`
			Status             string   `json:"status"`
			MustChangePassword bool     `json:"must_change_password"`
			Permissions        []string `json:"permissions"`
		} `json:"user"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !parsed.Authenticated || parsed.User.Username != "alice" || parsed.User.Status != string(domain.UserStatusActive) {
		t.Fatalf("response = %+v, want authenticated alice", parsed)
	}
	if len(parsed.User.Permissions) != 2 {
		t.Fatalf("permissions = %#v, want 2 entries", parsed.User.Permissions)
	}
}

func TestAuthMeReturnsUnauthenticatedWithoutSession(t *testing.T) {
	router := newAuthTestRouter(newFakeAPIAuthProvider())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Authenticated {
		t.Fatalf("authenticated = true, want false")
	}
}

func TestCSRFRequiredForMutatingProtectedRequests(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(
		testSessionToken,
		testCSRFToken,
		testAPIUser("alice", false, domain.PermissionSettingsUpdate),
	)
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/bridge.secret", strings.NewReader(`{"value":"redacted"}`))
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "csrf_required" {
		t.Fatalf("code = %q, want csrf_required", body["code"])
	}
}

func newAuthTestRouter(auth *fakeAPIAuthProvider) http.Handler {
	return NewRouter(
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
		withAuthProvider(auth),
	)
}

type fakeAPIAuthProvider struct {
	login                *service.LoginResult
	loginErr             error
	changeErr            error
	usersByToken         map[string]*service.AuthenticatedUser
	csrfByToken          map[string]string
	logoutTokens         []string
	changePasswordCalled bool
}

func newFakeAPIAuthProvider() *fakeAPIAuthProvider {
	return &fakeAPIAuthProvider{
		usersByToken: make(map[string]*service.AuthenticatedUser),
		csrfByToken:  make(map[string]string),
	}
}

func (f *fakeAPIAuthProvider) setSession(token, csrf string, user *service.AuthenticatedUser) {
	f.usersByToken[token] = user
	f.csrfByToken[token] = csrf
}

func (f *fakeAPIAuthProvider) Login(context.Context, service.LoginInput) (*service.LoginResult, error) {
	if f.loginErr != nil {
		return nil, f.loginErr
	}
	if f.login == nil {
		return nil, service.ErrInvalidCredentials
	}
	user := &service.AuthenticatedUser{
		User:    f.login.User,
		Session: f.login.Session,
	}
	f.setSession(f.login.SessionToken, f.login.CSRFToken, user)
	return f.login, nil
}

func (f *fakeAPIAuthProvider) CurrentUser(_ context.Context, rawSessionToken string) (*service.AuthenticatedUser, error) {
	user, ok := f.usersByToken[strings.TrimSpace(rawSessionToken)]
	if !ok {
		return nil, service.ErrInvalidSession
	}
	return user, nil
}

func (f *fakeAPIAuthProvider) Logout(_ context.Context, rawSessionToken string) error {
	rawSessionToken = strings.TrimSpace(rawSessionToken)
	if _, ok := f.usersByToken[rawSessionToken]; !ok {
		return service.ErrInvalidSession
	}
	f.logoutTokens = append(f.logoutTokens, rawSessionToken)
	delete(f.usersByToken, rawSessionToken)
	return nil
}

func (f *fakeAPIAuthProvider) ChangePassword(_ context.Context, input service.PasswordChangeInput) error {
	if f.changeErr != nil {
		return f.changeErr
	}
	f.changePasswordCalled = true
	for _, user := range f.usersByToken {
		if user.User.User.ID == input.UserID {
			user.User.User.MustChangePassword = false
			return nil
		}
	}
	return service.ErrInvalidSession
}

func (f *fakeAPIAuthProvider) ValidateCSRF(_ context.Context, rawSessionToken, csrfToken string) error {
	want, ok := f.csrfByToken[strings.TrimSpace(rawSessionToken)]
	if !ok {
		return service.ErrInvalidSession
	}
	if strings.TrimSpace(csrfToken) == "" {
		return errAPICSRFRequired
	}
	if csrfToken != want {
		return service.ErrInvalidSession
	}
	return nil
}

func (f *fakeAPIAuthProvider) RequirePermission(user *service.AuthenticatedUser, permissionKey string) error {
	if user != nil && user.HasPermission(permissionKey) {
		return nil
	}
	return service.ErrPermissionDenied
}

func (f *fakeAPIAuthProvider) RequireRole(user *service.AuthenticatedUser, roleID string) error {
	if user != nil && user.HasRole(roleID) {
		return nil
	}
	return service.ErrPermissionDenied
}

func testAPIUser(username string, mustChange bool, permissions ...string) *service.AuthenticatedUser {
	userID := uuid.New()
	grants := make([]domain.Permission, 0, len(permissions))
	for _, permission := range permissions {
		grants = append(grants, domain.Permission{ID: permission, Key: permission})
	}
	return &service.AuthenticatedUser{
		User: domain.UserWithRolesAndPermissions{
			User: domain.User{
				ID:                 userID,
				Username:           username,
				Email:              username + "@example.test",
				DisplayName:        username,
				Status:             domain.UserStatusActive,
				MustChangePassword: mustChange,
			},
			Roles:       []domain.Role{{ID: domain.RoleUser, Name: domain.RoleUser}},
			Permissions: grants,
		},
		Session: service.AuthenticatedSession{
			ID:     uuid.New(),
			UserID: userID,
		},
	}
}

func addSessionCookie(req *http.Request, token string) {
	req.AddCookie(&http.Cookie{Name: authSessionCookieName, Value: token})
}

func addCSRFCookieAndHeader(req *http.Request, token string) {
	req.AddCookie(&http.Cookie{Name: authCSRFCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
}

func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %s not found in %#v", name, cookies)
	return nil
}

var errAPICSRFRequired = errors.New("csrf token required")
