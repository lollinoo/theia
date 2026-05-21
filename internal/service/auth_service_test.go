package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/security"
)

const testAuthPassword = "Correct Horse Battery Staple 2026!"

type authServiceHarness struct {
	service       *AuthService
	store         *fakeAuthStore
	now           time.Time
	sessionSecret []byte
}

func newAuthServiceHarness(t *testing.T) *authServiceHarness {
	t.Helper()

	h := &authServiceHarness{
		store:         newFakeAuthStore(),
		now:           time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC),
		sessionSecret: []byte("test-session-secret-with-enough-entropy"),
	}
	service, err := NewAuthService(AuthServiceConfig{
		Users:              h.store,
		Roles:              h.store,
		Sessions:           h.store,
		PasswordResets:     h.store,
		AuditLogs:          h.store,
		SessionSecret:      h.sessionSecret,
		Now:                func() time.Time { return h.now },
		SessionTTL:         2 * time.Hour,
		PasswordResetTTL:   30 * time.Minute,
		FailedLoginSleeper: func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	h.service = service
	return h
}

func (h *authServiceHarness) addUser(t *testing.T, username, email, password string, status domain.UserStatus) domain.User {
	t.Helper()

	hash, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := domain.User{
		ID:                 uuid.New(),
		Username:           username,
		UsernameNormalized: testNormalize(username),
		Email:              email,
		EmailNormalized:    testNormalize(email),
		PasswordHash:       hash,
		DisplayName:        username + " display",
		Status:             status,
		CreatedAt:          h.now,
		UpdatedAt:          h.now,
		PasswordChangedAt:  &h.now,
	}
	if err := h.store.CreateUser(context.Background(), &user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return user
}

func (h *authServiceHarness) assignRole(t *testing.T, userID uuid.UUID, roleID string) {
	t.Helper()

	if err := h.store.AssignRole(context.Background(), userID, roleID, nil); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
}

func TestAuthServiceLoginWithEmailAndUsername(t *testing.T) {
	h := newAuthServiceHarness(t)
	ctx := context.Background()
	user := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)
	h.assignRole(t, user.ID, domain.RoleViewer)

	emailLogin, err := h.service.Login(ctx, LoginInput{
		Identifier: " Alice@Example.Test ",
		Password:   testAuthPassword,
		IPAddress:  "203.0.113.10",
		UserAgent:  "test-agent",
	})
	if err != nil {
		t.Fatalf("Login by email: %v", err)
	}
	if emailLogin.User.User.ID != user.ID {
		t.Fatalf("Login by email user ID = %s, want %s", emailLogin.User.User.ID, user.ID)
	}
	if emailLogin.SessionToken == "" || emailLogin.CSRFToken == "" {
		t.Fatal("Login returned empty session or CSRF token")
	}
	if _, ok := reflect.TypeOf(emailLogin.Session).FieldByName("TokenHash"); ok {
		t.Fatal("LoginResult session exposes a token hash field")
	}
	storedSession := h.store.session(t, emailLogin.Session.ID)
	if storedSession.TokenHash == emailLogin.SessionToken || strings.Contains(storedSession.TokenHash, emailLogin.SessionToken) {
		t.Fatal("stored session hash exposes the raw session token")
	}

	usernameLogin, err := h.service.Login(ctx, LoginInput{
		Identifier: " ALICE ",
		Password:   testAuthPassword,
	})
	if err != nil {
		t.Fatalf("Login by username: %v", err)
	}
	if usernameLogin.User.User.ID != user.ID {
		t.Fatalf("Login by username user ID = %s, want %s", usernameLogin.User.User.ID, user.ID)
	}

	stored := h.store.user(t, user.ID)
	if stored.LastLoginAt == nil || !stored.LastLoginAt.Equal(h.now) {
		t.Fatalf("LastLoginAt = %#v, want %s", stored.LastLoginAt, h.now)
	}
	if stored.FailedLoginAttempts != 0 {
		t.Fatalf("FailedLoginAttempts = %d, want 0", stored.FailedLoginAttempts)
	}
}

func TestAuthServiceFailedLoginWrongPasswordIsGeneric(t *testing.T) {
	h := newAuthServiceHarness(t)
	ctx := context.Background()
	user := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)

	_, err := h.service.Login(ctx, LoginInput{
		Identifier: user.Username,
		Password:   "wrong password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login error = %v, want ErrInvalidCredentials", err)
	}
	if err == nil || err.Error() != "invalid username or password" {
		t.Fatalf("Login wrong password error text = %q, want generic invalid credentials", err)
	}

	stored := h.store.user(t, user.ID)
	if stored.FailedLoginAttempts != 1 {
		t.Fatalf("FailedLoginAttempts = %d, want 1", stored.FailedLoginAttempts)
	}
}

func TestAuthServiceDisabledUserCannotLogin(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "disabled", "disabled@example.test", testAuthPassword, domain.UserStatusDisabled)

	_, err := h.service.Login(context.Background(), LoginInput{
		Identifier: user.Email,
		Password:   testAuthPassword,
	})
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("Login disabled user error = %v, want ErrUserDisabled", err)
	}
}

func TestAuthServiceLockedUserCannotLogin(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "locked", "locked@example.test", testAuthPassword, domain.UserStatusActive)
	lockedUntil := h.now.Add(15 * time.Minute)
	user.LockedUntil = &lockedUntil
	if err := h.store.UpdateUser(context.Background(), &user); err != nil {
		t.Fatalf("UpdateUser locked user: %v", err)
	}

	_, err := h.service.Login(context.Background(), LoginInput{
		Identifier: user.Username,
		Password:   testAuthPassword,
	})
	if !errors.Is(err, ErrUserLocked) {
		t.Fatalf("Login locked user error = %v, want ErrUserLocked", err)
	}
}

func TestAuthServiceFailedLoginLocksAfterRepeatedFailures(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "lockout", "lockout@example.test", testAuthPassword, domain.UserStatusActive)

	for i := 0; i < 5; i++ {
		_, err := h.service.Login(context.Background(), LoginInput{
			Identifier: user.Username,
			Password:   "wrong password",
		})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Login attempt %d error = %v, want ErrInvalidCredentials", i+1, err)
		}
	}

	stored := h.store.user(t, user.ID)
	if stored.FailedLoginAttempts != 5 {
		t.Fatalf("FailedLoginAttempts = %d, want 5", stored.FailedLoginAttempts)
	}
	if stored.LockedUntil == nil || !stored.LockedUntil.After(h.now) {
		t.Fatalf("LockedUntil = %#v, want future lock", stored.LockedUntil)
	}

	_, err := h.service.Login(context.Background(), LoginInput{
		Identifier: user.Username,
		Password:   testAuthPassword,
	})
	if !errors.Is(err, ErrUserLocked) {
		t.Fatalf("Login after lock error = %v, want ErrUserLocked", err)
	}
}

func TestAuthServiceCurrentUserSessionLookup(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "viewer", "viewer@example.test", testAuthPassword, domain.UserStatusActive)
	h.assignRole(t, user.ID, domain.RoleViewer)

	login, err := h.service.Login(context.Background(), LoginInput{
		Identifier: user.Username,
		Password:   testAuthPassword,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	current, err := h.service.CurrentUser(context.Background(), login.SessionToken)
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if current.User.User.ID != user.ID {
		t.Fatalf("CurrentUser user ID = %s, want %s", current.User.User.ID, user.ID)
	}
	if current.Session.ID != login.Session.ID {
		t.Fatalf("CurrentUser session ID = %s, want %s", current.Session.ID, login.Session.ID)
	}
	if _, ok := reflect.TypeOf(current.Session).FieldByName("TokenHash"); ok {
		t.Fatal("CurrentUser session exposes a token hash field")
	}
	if !current.HasRole(domain.RoleViewer) || !current.HasPermission(domain.PermissionTopologyRead) {
		t.Fatalf("CurrentUser grants = roles:%#v permissions:%#v", current.User.Roles, current.User.Permissions)
	}
}

func TestAuthServicePermissionsFromRoleAggregate(t *testing.T) {
	aggregate := domain.UserWithRolesAndPermissions{
		Roles: []domain.Role{
			{ID: domain.RoleViewer, Name: domain.RoleViewer},
		},
		Permissions: []domain.Permission{
			{ID: domain.PermissionTopologyRead, Key: domain.PermissionTopologyRead},
		},
	}
	user := AuthenticatedUser{User: aggregate}

	if !user.HasRole(domain.RoleViewer) {
		t.Fatalf("HasRole(%q) = false, want true", domain.RoleViewer)
	}
	if !user.HasPermission(domain.PermissionTopologyRead) {
		t.Fatalf("HasPermission(%q) = false, want true", domain.PermissionTopologyRead)
	}
	if user.HasPermission(domain.PermissionCredentialsReveal) {
		t.Fatalf("HasPermission(%q) = true, want false", domain.PermissionCredentialsReveal)
	}
}

func TestAuthServiceUserWithoutPermissionDenied(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := &AuthenticatedUser{
		User: domain.UserWithRolesAndPermissions{
			Permissions: []domain.Permission{
				{ID: domain.PermissionTopologyRead, Key: domain.PermissionTopologyRead},
			},
		},
	}

	err := h.service.RequirePermission(user, domain.PermissionUsersDelete)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("RequirePermission error = %v, want ErrPermissionDenied", err)
	}
}

func TestAuthServicePasswordResetTokensExpire(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "reset", "reset@example.test", testAuthPassword, domain.UserStatusActive)

	reset, err := h.service.CreatePasswordResetToken(context.Background(), PasswordResetCreateInput{
		UserID: user.ID,
	})
	if err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}
	if reset.Token == "" {
		t.Fatal("CreatePasswordResetToken returned an empty token")
	}
	storedToken := h.store.passwordResetByHash(t, security.HashToken(reset.Token, h.sessionSecret))
	if strings.Contains(storedToken.TokenHash, reset.Token) {
		t.Fatal("stored password reset hash exposes raw token")
	}

	h.now = h.now.Add(31 * time.Minute)
	err = h.service.CompletePasswordReset(context.Background(), PasswordResetCompleteInput{
		Token:       reset.Token,
		NewPassword: "Another Correct Horse Battery Staple 2026!",
	})
	if !errors.Is(err, ErrPasswordResetExpired) {
		t.Fatalf("CompletePasswordReset expired error = %v, want ErrPasswordResetExpired", err)
	}

	unchanged := h.store.user(t, user.ID)
	ok, err := security.VerifyPassword(testAuthPassword, unchanged.PasswordHash)
	if err != nil {
		t.Fatalf("VerifyPassword unchanged password: %v", err)
	}
	if !ok {
		t.Fatal("expired reset token changed the password")
	}
}

func TestAuthServicePasswordChangeRevokesOtherSessionsAndClearsMustChange(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "changer", "changer@example.test", testAuthPassword, domain.UserStatusActive)
	user.MustChangePassword = true
	if err := h.store.UpdateUser(context.Background(), &user); err != nil {
		t.Fatalf("UpdateUser must-change: %v", err)
	}
	login, err := h.service.Login(context.Background(), LoginInput{
		Identifier: user.Username,
		Password:   testAuthPassword,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	otherSession := domain.AuthSession{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: "other-session-token-hash",
		CreatedAt: h.now,
		ExpiresAt: h.now.Add(time.Hour),
	}
	if err := h.store.CreateSession(context.Background(), &otherSession); err != nil {
		t.Fatalf("CreateSession other: %v", err)
	}

	newPassword := "New Correct Horse Battery Staple 2026!"
	if err := h.service.ChangePassword(context.Background(), PasswordChangeInput{
		UserID:           user.ID,
		CurrentSessionID: &login.Session.ID,
		CurrentPassword:  testAuthPassword,
		NewPassword:      newPassword,
	}); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	stored := h.store.user(t, user.ID)
	if stored.MustChangePassword {
		t.Fatal("ChangePassword left MustChangePassword true")
	}
	if stored.PasswordChangedAt == nil || !stored.PasswordChangedAt.Equal(h.now) {
		t.Fatalf("PasswordChangedAt = %#v, want %s", stored.PasswordChangedAt, h.now)
	}
	ok, err := security.VerifyPassword(newPassword, stored.PasswordHash)
	if err != nil {
		t.Fatalf("VerifyPassword new password: %v", err)
	}
	if !ok {
		t.Fatal("new password does not verify")
	}

	currentSession := h.store.session(t, login.Session.ID)
	if currentSession.RevokedAt != nil {
		t.Fatalf("current session was revoked at %#v", currentSession.RevokedAt)
	}
	revokedOther := h.store.session(t, otherSession.ID)
	if revokedOther.RevokedAt == nil || !revokedOther.RevokedAt.Equal(h.now) {
		t.Fatalf("other session RevokedAt = %#v, want %s", revokedOther.RevokedAt, h.now)
	}
}

func TestAuthServiceBootstrapCreatesForcedChangeSuperAdminOnlyForEmptyStore(t *testing.T) {
	h := newAuthServiceHarness(t)

	created, didCreate, err := h.service.EnsureBootstrapSuperAdmin(context.Background())
	if err != nil {
		t.Fatalf("EnsureBootstrapSuperAdmin: %v", err)
	}
	if !didCreate {
		t.Fatal("EnsureBootstrapSuperAdmin didCreate = false, want true")
	}
	if created.Username != "administrator" || created.Email != "administrator@theia.local" || created.DisplayName != "Administrator" {
		t.Fatalf("bootstrap user = %#v", created)
	}
	if created.Status != domain.UserStatusActive || !created.MustChangePassword {
		t.Fatalf("bootstrap status/must-change = %s/%t", created.Status, created.MustChangePassword)
	}
	ok, err := security.VerifyPassword("theia", created.PasswordHash)
	if err != nil {
		t.Fatalf("VerifyPassword bootstrap password: %v", err)
	}
	if !ok {
		t.Fatal("bootstrap password does not verify")
	}
	aggregate, err := h.store.GetUserRolesAndPermissions(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetUserRolesAndPermissions: %v", err)
	}
	if !aggregate.HasRole(domain.RoleSuperAdmin) {
		t.Fatalf("bootstrap roles = %#v, want super_admin", aggregate.Roles)
	}
	if len(h.store.auditLogs()) != 1 {
		t.Fatalf("audit log count = %d, want 1", len(h.store.auditLogs()))
	}

	again, didCreate, err := h.service.EnsureBootstrapSuperAdmin(context.Background())
	if err != nil {
		t.Fatalf("EnsureBootstrapSuperAdmin second call: %v", err)
	}
	if didCreate || again != nil {
		t.Fatalf("second bootstrap = (%#v, %t), want nil false", again, didCreate)
	}
	count, err := h.store.CountUsers(context.Background())
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 1 {
		t.Fatalf("user count = %d, want 1", count)
	}
}

type fakeAuthStore struct {
	mu             sync.Mutex
	users          map[uuid.UUID]domain.User
	usersByLogin   map[string]uuid.UUID
	roles          map[string]domain.Role
	permissions    map[string]domain.Permission
	userRoles      map[uuid.UUID]map[string]struct{}
	sessions       map[uuid.UUID]domain.AuthSession
	sessionsByHash map[string]uuid.UUID
	resets         map[uuid.UUID]domain.PasswordResetToken
	resetsByHash   map[string]uuid.UUID
	audit          []domain.AuditLog
}

func newFakeAuthStore() *fakeAuthStore {
	store := &fakeAuthStore{
		users:          make(map[uuid.UUID]domain.User),
		usersByLogin:   make(map[string]uuid.UUID),
		roles:          make(map[string]domain.Role),
		permissions:    make(map[string]domain.Permission),
		userRoles:      make(map[uuid.UUID]map[string]struct{}),
		sessions:       make(map[uuid.UUID]domain.AuthSession),
		sessionsByHash: make(map[string]uuid.UUID),
		resets:         make(map[uuid.UUID]domain.PasswordResetToken),
		resetsByHash:   make(map[string]uuid.UUID),
	}
	for _, name := range domain.SystemRoleNames() {
		store.roles[name] = domain.Role{ID: name, Name: name, IsSystemRole: true}
	}
	for _, permission := range domain.SystemPermissions() {
		store.permissions[permission.Key] = domain.Permission{
			ID:          permission.Key,
			Key:         permission.Key,
			Description: permission.Description,
			Resource:    permission.Resource,
			Action:      permission.Action,
		}
	}
	return store
}

func (s *fakeAuthStore) CreateUser(_ context.Context, user *domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	stored := *user
	s.users[stored.ID] = stored
	s.usersByLogin[stored.UsernameNormalized] = stored.ID
	s.usersByLogin[stored.EmailNormalized] = stored.ID
	return nil
}

func (s *fakeAuthStore) GetUserByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[id]
	if !ok {
		return nil, domain.ErrAuthUserNotFound
	}
	return copyAuthUser(user), nil
}

func (s *fakeAuthStore) GetUserByLoginIdentifier(_ context.Context, normalized string) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.usersByLogin[normalized]
	if !ok {
		return nil, domain.ErrAuthUserNotFound
	}
	user := s.users[id]
	return copyAuthUser(user), nil
}

func (s *fakeAuthStore) ListUsers(_ context.Context, _ domain.UserListFilter) ([]domain.UserWithRolesAndPermissions, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	users := make([]domain.UserWithRolesAndPermissions, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, s.aggregateLocked(user))
	}
	return users, nil
}

func (s *fakeAuthStore) UpdateUser(_ context.Context, user *domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[user.ID]; !ok {
		return domain.ErrAuthUserNotFound
	}
	stored := *user
	s.users[stored.ID] = stored
	s.usersByLogin[stored.UsernameNormalized] = stored.ID
	s.usersByLogin[stored.EmailNormalized] = stored.ID
	return nil
}

func (s *fakeAuthStore) CountUsers(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.users), nil
}

func (s *fakeAuthStore) CountActiveSuperAdmins(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for id, user := range s.users {
		if user.Status != domain.UserStatusActive {
			continue
		}
		if _, ok := s.userRoles[id][domain.RoleSuperAdmin]; ok {
			count++
		}
	}
	return count, nil
}

func (s *fakeAuthStore) GetUserRolesAndPermissions(_ context.Context, userID uuid.UUID) (*domain.UserWithRolesAndPermissions, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[userID]
	if !ok {
		return nil, domain.ErrAuthUserNotFound
	}
	aggregate := s.aggregateLocked(user)
	return &aggregate, nil
}

func (s *fakeAuthStore) ListRoles(_ context.Context) ([]domain.Role, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	roles := make([]domain.Role, 0, len(s.roles))
	for _, role := range s.roles {
		roles = append(roles, role)
	}
	return roles, nil
}

func (s *fakeAuthStore) ListPermissions(_ context.Context) ([]domain.Permission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	permissions := make([]domain.Permission, 0, len(s.permissions))
	for _, permission := range s.permissions {
		permissions = append(permissions, permission)
	}
	return permissions, nil
}

func (s *fakeAuthStore) GetRoleByName(_ context.Context, name string) (*domain.Role, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	role, ok := s.roles[name]
	if !ok {
		return nil, domain.ErrAuthRoleNotFound
	}
	return &role, nil
}

func (s *fakeAuthStore) AssignRole(_ context.Context, userID uuid.UUID, roleID string, _ *uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[userID]; !ok {
		return domain.ErrAuthUserNotFound
	}
	if _, ok := s.roles[roleID]; !ok {
		return domain.ErrAuthRoleNotFound
	}
	if s.userRoles[userID] == nil {
		s.userRoles[userID] = make(map[string]struct{})
	}
	s.userRoles[userID][roleID] = struct{}{}
	return nil
}

func (s *fakeAuthStore) RemoveRole(_ context.Context, userID uuid.UUID, roleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.userRoles[userID], roleID)
	return nil
}

func (s *fakeAuthStore) CreateSession(_ context.Context, session *domain.AuthSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	stored := *session
	s.sessions[stored.ID] = stored
	s.sessionsByHash[stored.TokenHash] = stored.ID
	return nil
}

func (s *fakeAuthStore) GetSessionByTokenHash(_ context.Context, tokenHash string) (*domain.AuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.sessionsByHash[tokenHash]
	if !ok {
		return nil, domain.ErrAuthSessionNotFound
	}
	session := s.sessions[id]
	return &session, nil
}

func (s *fakeAuthStore) RevokeSession(_ context.Context, sessionID uuid.UUID, when time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return domain.ErrAuthSessionNotFound
	}
	session.RevokedAt = &when
	s.sessions[sessionID] = session
	return nil
}

func (s *fakeAuthStore) RevokeUserSessions(_ context.Context, userID uuid.UUID, exceptSessionID *uuid.UUID, when time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, session := range s.sessions {
		if session.UserID != userID {
			continue
		}
		if exceptSessionID != nil && id == *exceptSessionID {
			continue
		}
		session.RevokedAt = &when
		s.sessions[id] = session
	}
	return nil
}

func (s *fakeAuthStore) TouchSession(_ context.Context, sessionID uuid.UUID, when time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return domain.ErrAuthSessionNotFound
	}
	session.LastSeenAt = &when
	s.sessions[sessionID] = session
	return nil
}

func (s *fakeAuthStore) CreatePasswordResetToken(_ context.Context, token *domain.PasswordResetToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	stored := *token
	s.resets[stored.ID] = stored
	s.resetsByHash[stored.TokenHash] = stored.ID
	return nil
}

func (s *fakeAuthStore) GetPasswordResetTokenByHash(_ context.Context, tokenHash string) (*domain.PasswordResetToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.resetsByHash[tokenHash]
	if !ok {
		return nil, domain.ErrPasswordResetTokenNotFound
	}
	token := s.resets[id]
	return &token, nil
}

func (s *fakeAuthStore) MarkPasswordResetTokenUsed(_ context.Context, tokenID uuid.UUID, when time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, ok := s.resets[tokenID]
	if !ok {
		return domain.ErrPasswordResetTokenNotFound
	}
	token.UsedAt = &when
	s.resets[tokenID] = token
	return nil
}

func (s *fakeAuthStore) AppendAuditLog(_ context.Context, log *domain.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	stored := *log
	s.audit = append(s.audit, stored)
	return nil
}

func (s *fakeAuthStore) ListAuditLogs(_ context.Context, _ domain.AuditLogFilter) ([]domain.AuditLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	logs := make([]domain.AuditLog, len(s.audit))
	copy(logs, s.audit)
	return logs, nil
}

func (s *fakeAuthStore) DashboardStats(_ context.Context) (*domain.AdminDashboardStats, error) {
	return &domain.AdminDashboardStats{}, nil
}

func (s *fakeAuthStore) user(t *testing.T, id uuid.UUID) domain.User {
	t.Helper()

	user, err := s.GetUserByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	return *user
}

func (s *fakeAuthStore) session(t *testing.T, id uuid.UUID) domain.AuthSession {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		t.Fatalf("session %s not found", id)
	}
	return session
}

func (s *fakeAuthStore) passwordResetByHash(t *testing.T, tokenHash string) domain.PasswordResetToken {
	t.Helper()

	token, err := s.GetPasswordResetTokenByHash(context.Background(), tokenHash)
	if err != nil {
		t.Fatalf("GetPasswordResetTokenByHash: %v", err)
	}
	return *token
}

func (s *fakeAuthStore) auditLogs() []domain.AuditLog {
	s.mu.Lock()
	defer s.mu.Unlock()

	logs := make([]domain.AuditLog, len(s.audit))
	copy(logs, s.audit)
	return logs
}

func (s *fakeAuthStore) aggregateLocked(user domain.User) domain.UserWithRolesAndPermissions {
	roles := make([]domain.Role, 0, len(s.userRoles[user.ID]))
	permissionsByKey := make(map[string]domain.Permission)
	for roleID := range s.userRoles[user.ID] {
		role := s.roles[roleID]
		roles = append(roles, role)
		for _, key := range domain.SystemRolePermissionKeys(roleID) {
			permissionsByKey[key] = s.permissions[key]
		}
	}
	permissions := make([]domain.Permission, 0, len(permissionsByKey))
	for _, permission := range permissionsByKey {
		permissions = append(permissions, permission)
	}
	return domain.UserWithRolesAndPermissions{
		User:        user,
		Roles:       roles,
		Permissions: permissions,
	}
}

func copyAuthUser(user domain.User) *domain.User {
	copied := user
	return &copied
}

func testNormalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
