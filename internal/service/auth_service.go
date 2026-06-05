package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/security"
)

const (
	defaultAuthSessionTTL        = 12 * time.Hour
	defaultPasswordResetTTL      = 30 * time.Minute
	defaultFailedLoginThreshold  = 5
	defaultFailedLoginDelayAfter = 3
	defaultFailedLoginDelay      = 750 * time.Millisecond
	defaultFailedLoginLock       = 15 * time.Minute
)

var (
	// ErrInvalidCredentials is returned for unknown users and password mismatches.
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrUserDisabled is returned when an account is not allowed to authenticate.
	ErrUserDisabled = errors.New("user cannot authenticate")
	// ErrUserLocked is returned when an account is temporarily locked.
	ErrUserLocked = errors.New("user cannot authenticate")
	// ErrInvalidSession is returned when a session token is missing, unknown, revoked, or expired.
	ErrInvalidSession = errors.New("invalid session")
	// ErrPermissionDenied is returned when an authenticated user lacks a required grant.
	ErrPermissionDenied = errors.New("permission denied")
	// ErrPasswordPolicyViolation is returned when a proposed password does not meet policy.
	ErrPasswordPolicyViolation = errors.New("password does not meet policy")
	// ErrPasswordReuse is returned when the proposed password matches the current password.
	ErrPasswordReuse = errors.New("new password must be different from current password")
	// ErrPasswordResetExpired is returned when a password reset token is expired or already used.
	ErrPasswordResetExpired = errors.New("password reset token expired")
)

// AuthServiceConfig contains dependencies and security settings for AuthService.
type AuthServiceConfig struct {
	Users              domain.UserRepository
	Roles              domain.RoleRepository
	Sessions           domain.SessionRepository
	PasswordResets     domain.PasswordResetRepository
	AuditLogs          domain.AuditLogRepository
	SessionSecret      []byte
	Now                func() time.Time
	SessionTTL         time.Duration
	PasswordResetTTL   time.Duration
	FailedLoginSleeper func(time.Duration)
}

// AuthService coordinates first-party authentication and RBAC workflows.
type AuthService struct {
	users              domain.UserRepository
	roles              domain.RoleRepository
	sessions           domain.SessionRepository
	passwordResets     domain.PasswordResetRepository
	auditLogs          domain.AuditLogRepository
	sessionSecret      []byte
	dummyPasswordHash  string
	verifyPassword     func(password, hash string) (bool, error)
	now                func() time.Time
	sessionTTL         time.Duration
	passwordResetTTL   time.Duration
	failedThreshold    int
	failedDelayAfter   int
	failedDelay        time.Duration
	failedLockDuration time.Duration
	failedSleeper      func(time.Duration)
}

// LoginInput contains credentials and request metadata for a login attempt.
type LoginInput struct {
	Identifier string
	Password   string
	IPAddress  string
	UserAgent  string
}

// LoginResult contains the authenticated user and raw tokens returned exactly once.
type LoginResult struct {
	User         domain.UserWithRolesAndPermissions
	Session      AuthenticatedSession
	SessionToken string
	CSRFToken    string
	ExpiresAt    time.Time
}

// AuthenticatedSession is the response-safe session view used by auth callers.
type AuthenticatedSession struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt *time.Time
	IPAddress  string
	UserAgent  string
}

// AuthenticatedUser contains the current user aggregate and backing session.
type AuthenticatedUser struct {
	User    domain.UserWithRolesAndPermissions
	Session AuthenticatedSession
}

// PasswordChangeInput contains a password change request for an authenticated user.
type PasswordChangeInput struct {
	UserID           uuid.UUID
	CurrentSessionID *uuid.UUID
	CurrentPassword  string
	NewPassword      string
}

// PasswordResetCreateInput identifies a user that should receive a reset token.
type PasswordResetCreateInput struct {
	UserID    uuid.UUID
	CreatedBy *uuid.UUID
}

// PasswordResetTokenResult returns the reset token and its expiration.
type PasswordResetTokenResult struct {
	Token     string
	ExpiresAt time.Time
}

// PasswordResetCompleteInput completes a password reset with a raw token.
type PasswordResetCompleteInput struct {
	Token       string
	NewPassword string
}

// NewAuthService creates an AuthService from repository dependencies.
func NewAuthService(config AuthServiceConfig) (*AuthService, error) {
	if config.Users == nil {
		return nil, errors.New("auth service users repository is required")
	}
	if config.Roles == nil {
		return nil, errors.New("auth service roles repository is required")
	}
	if config.Sessions == nil {
		return nil, errors.New("auth service sessions repository is required")
	}
	if config.PasswordResets == nil {
		return nil, errors.New("auth service password resets repository is required")
	}
	if config.AuditLogs == nil {
		return nil, errors.New("auth service audit logs repository is required")
	}
	if len(config.SessionSecret) == 0 {
		return nil, errors.New("auth service session secret is required")
	}
	dummyPasswordHash, err := security.HashPassword("dummy auth password verifier")
	if err != nil {
		return nil, fmt.Errorf("hashing dummy auth password: %w", err)
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	sessionTTL := config.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = defaultAuthSessionTTL
	}
	passwordResetTTL := config.PasswordResetTTL
	if passwordResetTTL <= 0 {
		passwordResetTTL = defaultPasswordResetTTL
	}
	sleeper := config.FailedLoginSleeper
	if sleeper == nil {
		sleeper = time.Sleep
	}
	return &AuthService{
		users:              config.Users,
		roles:              config.Roles,
		sessions:           config.Sessions,
		passwordResets:     config.PasswordResets,
		auditLogs:          config.AuditLogs,
		sessionSecret:      append([]byte(nil), config.SessionSecret...),
		dummyPasswordHash:  dummyPasswordHash,
		verifyPassword:     security.VerifyPassword,
		now:                func() time.Time { return now().UTC() },
		sessionTTL:         sessionTTL,
		passwordResetTTL:   passwordResetTTL,
		failedThreshold:    defaultFailedLoginThreshold,
		failedDelayAfter:   defaultFailedLoginDelayAfter,
		failedDelay:        defaultFailedLoginDelay,
		failedLockDuration: defaultFailedLoginLock,
		failedSleeper:      sleeper,
	}, nil
}

// EnsureBootstrapSuperAdmin creates or repairs the fixed bootstrap super-admin when none are active.
func (s *AuthService) EnsureBootstrapSuperAdmin(ctx context.Context) (*domain.User, bool, error) {
	count, err := s.users.CountActiveSuperAdmins(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("counting active bootstrap super admins: %w", err)
	}
	if count > 0 {
		return nil, false, nil
	}

	now := s.now()
	user, created, err := s.ensureBootstrapUser(ctx, now)
	if err != nil {
		return nil, false, err
	}
	repaired := created
	if user.Status != domain.UserStatusActive || user.LockedUntil != nil {
		user.Status = domain.UserStatusActive
		user.MustChangePassword = true
		user.FailedLoginAttempts = 0
		user.LockedUntil = nil
		user.UpdatedAt = now
		if err := s.users.UpdateUser(ctx, user); err != nil {
			return nil, false, fmt.Errorf("reactivating bootstrap super admin: %w", err)
		}
		repaired = true
	}
	aggregate, err := s.users.GetUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return nil, false, fmt.Errorf("loading bootstrap super admin roles: %w", err)
	}
	if !aggregate.HasRole(domain.RoleSuperAdmin) {
		if err := s.roles.AssignRole(ctx, user.ID, domain.RoleSuperAdmin, nil); err != nil {
			return nil, false, fmt.Errorf("assigning bootstrap super admin role: %w", err)
		}
		repaired = true
	}
	if repaired {
		action := "auth.bootstrap_super_admin_repaired"
		if created {
			action = "auth.bootstrap_super_admin_created"
		}
		if err := s.appendAuditLog(ctx, nil, &user.ID, action, "auth", user.ID.String(), `{}`); err != nil {
			return nil, false, err
		}
	}
	return user, created, nil
}

func (s *AuthService) ensureBootstrapUser(ctx context.Context, now time.Time) (*domain.User, bool, error) {
	user, err := s.users.GetUserByLoginIdentifier(ctx, normalizeLoginIdentifier("administrator"))
	if err == nil {
		return user, false, nil
	}
	if !errors.Is(err, domain.ErrAuthUserNotFound) {
		return nil, false, fmt.Errorf("getting bootstrap super admin: %w", err)
	}

	passwordHash, err := security.HashPassword("theia")
	if err != nil {
		return nil, false, fmt.Errorf("hashing bootstrap password: %w", err)
	}
	user = &domain.User{
		ID:                 uuid.New(),
		Username:           "administrator",
		UsernameNormalized: normalizeLoginIdentifier("administrator"),
		Email:              "administrator@theia.local",
		EmailNormalized:    normalizeLoginIdentifier("administrator@theia.local"),
		PasswordHash:       passwordHash,
		DisplayName:        "Administrator",
		Status:             domain.UserStatusActive,
		MustChangePassword: true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.users.CreateUser(ctx, user); err != nil {
		return nil, false, fmt.Errorf("creating bootstrap super admin: %w", err)
	}
	return user, true, nil
}

// Login authenticates a username or email and creates a server-side session.
func (s *AuthService) Login(ctx context.Context, input LoginInput) (*LoginResult, error) {
	normalized := normalizeLoginIdentifier(input.Identifier)
	if normalized == "" {
		s.runDummyPasswordVerification(input.Password)
		_ = s.appendAuditLog(ctx, nil, nil, "auth.login_failed", "auth", "", `{"reason":"invalid_credentials"}`)
		return nil, ErrInvalidCredentials
	}

	user, err := s.users.GetUserByLoginIdentifier(ctx, normalized)
	if err != nil {
		if errors.Is(err, domain.ErrAuthUserNotFound) {
			s.runDummyPasswordVerification(input.Password)
			_ = s.appendAuditLog(ctx, nil, nil, "auth.login_failed", "auth", "", `{"reason":"invalid_credentials"}`)
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("getting auth user by login identifier: %w", err)
	}
	s.resetExpiredLockState(user)
	if err := s.rejectInactiveOrLocked(ctx, user, input.IPAddress, input.UserAgent); err != nil {
		s.runDummyPasswordVerification(input.Password)
		return nil, err
	}

	ok, err := s.verifyPassword(input.Password, user.PasswordHash)
	if err != nil || !ok {
		if updateErr := s.recordFailedLogin(ctx, user, input.IPAddress, input.UserAgent); updateErr != nil {
			return nil, updateErr
		}
		return nil, ErrInvalidCredentials
	}

	now := s.now()
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := s.users.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("updating successful auth login: %w", err)
	}

	aggregate, err := s.users.GetUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("loading auth user grants: %w", err)
	}
	sessionToken, err := security.GenerateToken()
	if err != nil {
		return nil, err
	}
	csrfToken, err := security.GenerateToken()
	if err != nil {
		return nil, err
	}
	session := domain.AuthSession{
		ID:            uuid.New(),
		UserID:        user.ID,
		TokenHash:     security.HashToken(sessionToken, s.sessionSecret),
		CSRFTokenHash: security.HashToken(csrfToken, s.sessionSecret),
		CreatedAt:     now,
		ExpiresAt:     now.Add(s.sessionTTL),
		IPAddress:     input.IPAddress,
		UserAgent:     input.UserAgent,
	}
	if err := s.sessions.CreateSession(ctx, &session); err != nil {
		return nil, fmt.Errorf("creating auth session: %w", err)
	}
	if err := s.appendAuditLog(ctx, &user.ID, &user.ID, "auth.login", "auth", user.ID.String(), `{}`); err != nil {
		return nil, err
	}
	return &LoginResult{
		User:         *aggregate,
		Session:      authenticatedSessionFromDomain(session),
		SessionToken: sessionToken,
		CSRFToken:    csrfToken,
		ExpiresAt:    session.ExpiresAt,
	}, nil
}

// CurrentUser loads the current authenticated user from a raw session token.
func (s *AuthService) CurrentUser(ctx context.Context, rawSessionToken string) (*AuthenticatedUser, error) {
	rawSessionToken = strings.TrimSpace(rawSessionToken)
	if rawSessionToken == "" {
		return nil, ErrInvalidSession
	}
	session, err := s.sessions.GetSessionByTokenHash(ctx, security.HashToken(rawSessionToken, s.sessionSecret))
	if err != nil {
		if errors.Is(err, domain.ErrAuthSessionNotFound) {
			return nil, ErrInvalidSession
		}
		return nil, fmt.Errorf("getting auth session: %w", err)
	}
	now := s.now()
	if session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		return nil, ErrInvalidSession
	}

	aggregate, err := s.users.GetUserRolesAndPermissions(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("loading auth session user: %w", err)
	}
	if err := s.ensureAggregateCanAuthenticate(&aggregate.User); err != nil {
		return nil, err
	}
	if err := s.sessions.TouchSession(ctx, session.ID, now); err != nil {
		return nil, fmt.Errorf("touching auth session: %w", err)
	}
	session.LastSeenAt = &now
	return &AuthenticatedUser{User: *aggregate, Session: authenticatedSessionFromDomain(*session)}, nil
}

// ValidateCSRF verifies that a raw CSRF token belongs to the current session.
func (s *AuthService) ValidateCSRF(ctx context.Context, rawSessionToken, rawCSRFToken string) error {
	rawSessionToken = strings.TrimSpace(rawSessionToken)
	rawCSRFToken = strings.TrimSpace(rawCSRFToken)
	if rawSessionToken == "" || rawCSRFToken == "" {
		return ErrInvalidSession
	}
	session, err := s.sessions.GetSessionByTokenHash(ctx, security.HashToken(rawSessionToken, s.sessionSecret))
	if err != nil {
		if errors.Is(err, domain.ErrAuthSessionNotFound) {
			return ErrInvalidSession
		}
		return fmt.Errorf("getting auth session for csrf validation: %w", err)
	}
	now := s.now()
	if session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		return ErrInvalidSession
	}
	if security.HashToken(rawCSRFToken, s.sessionSecret) != session.CSRFTokenHash {
		return ErrInvalidSession
	}
	return nil
}

// Logout revokes the session identified by rawSessionToken.
func (s *AuthService) Logout(ctx context.Context, rawSessionToken string) error {
	session, err := s.sessions.GetSessionByTokenHash(ctx, security.HashToken(strings.TrimSpace(rawSessionToken), s.sessionSecret))
	if err != nil {
		if errors.Is(err, domain.ErrAuthSessionNotFound) {
			return ErrInvalidSession
		}
		return fmt.Errorf("getting logout session: %w", err)
	}
	if err := s.sessions.RevokeSession(ctx, session.ID, s.now()); err != nil {
		return fmt.Errorf("revoking auth session: %w", err)
	}
	return nil
}

// ChangePassword changes a user's password and revokes other sessions.
func (s *AuthService) ChangePassword(ctx context.Context, input PasswordChangeInput) error {
	user, err := s.users.GetUserByID(ctx, input.UserID)
	if err != nil {
		return fmt.Errorf("getting password change user: %w", err)
	}
	ok, err := s.verifyPassword(input.CurrentPassword, user.PasswordHash)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	if err := security.ValidatePasswordPolicy(input.NewPassword); err != nil {
		return fmt.Errorf("%w: %v", ErrPasswordPolicyViolation, err)
	}
	samePassword, err := s.verifyPassword(input.NewPassword, user.PasswordHash)
	if err != nil {
		return fmt.Errorf("verifying changed password reuse: %w", err)
	}
	if samePassword {
		return ErrPasswordReuse
	}
	now := s.now()
	passwordHash, err := security.HashPassword(input.NewPassword)
	if err != nil {
		return fmt.Errorf("hashing changed password: %w", err)
	}
	user.PasswordHash = passwordHash
	user.MustChangePassword = false
	user.PasswordChangedAt = &now
	user.UpdatedAt = now
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	if err := s.users.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("updating changed password: %w", err)
	}
	if err := s.sessions.RevokeUserSessions(ctx, user.ID, input.CurrentSessionID, now); err != nil {
		return fmt.Errorf("revoking auth sessions after password change: %w", err)
	}
	if err := s.appendAuditLog(ctx, &user.ID, &user.ID, "auth.password_changed", "auth", user.ID.String(), `{}`); err != nil {
		return err
	}
	return nil
}

// CreatePasswordResetToken creates and stores a hashed password reset token.
func (s *AuthService) CreatePasswordResetToken(ctx context.Context, input PasswordResetCreateInput) (*PasswordResetTokenResult, error) {
	user, err := s.users.GetUserByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("getting password reset user: %w", err)
	}
	if err := s.ensureAggregateCanAuthenticate(user); err != nil {
		return nil, err
	}
	rawToken, err := security.GenerateToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	reset := domain.PasswordResetToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: security.HashToken(rawToken, s.sessionSecret),
		CreatedAt: now,
		ExpiresAt: now.Add(s.passwordResetTTL),
		CreatedBy: input.CreatedBy,
	}
	if err := s.passwordResets.CreatePasswordResetToken(ctx, &reset); err != nil {
		return nil, fmt.Errorf("creating password reset token: %w", err)
	}
	if err := s.appendAuditLog(ctx, input.CreatedBy, &user.ID, "auth.password_reset_token_created", "auth", user.ID.String(), `{}`); err != nil {
		return nil, err
	}
	return &PasswordResetTokenResult{Token: rawToken, ExpiresAt: reset.ExpiresAt}, nil
}

// CompletePasswordReset consumes a valid reset token and changes the user's password.
func (s *AuthService) CompletePasswordReset(ctx context.Context, input PasswordResetCompleteInput) error {
	if err := security.ValidatePasswordPolicy(input.NewPassword); err != nil {
		return fmt.Errorf("%w: %v", ErrPasswordPolicyViolation, err)
	}
	now := s.now()
	tokenHash := security.HashToken(strings.TrimSpace(input.Token), s.sessionSecret)
	reset, err := s.passwordResets.GetPasswordResetTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, domain.ErrPasswordResetTokenNotFound) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("getting password reset token: %w", err)
	}
	if reset.UsedAt != nil || !reset.ExpiresAt.After(now) {
		return ErrPasswordResetExpired
	}
	user, err := s.users.GetUserByID(ctx, reset.UserID)
	if err != nil {
		return fmt.Errorf("getting password reset user: %w", err)
	}
	samePassword, err := s.verifyPassword(input.NewPassword, user.PasswordHash)
	if err != nil {
		return fmt.Errorf("verifying reset password reuse: %w", err)
	}
	if samePassword {
		return ErrPasswordReuse
	}
	passwordHash, err := security.HashPassword(input.NewPassword)
	if err != nil {
		return fmt.Errorf("hashing reset password: %w", err)
	}
	reset, err = s.passwordResets.CompletePasswordReset(ctx, tokenHash, passwordHash, now)
	if err != nil {
		if errors.Is(err, domain.ErrPasswordResetTokenNotFound) {
			return ErrInvalidCredentials
		}
		if errors.Is(err, domain.ErrPasswordResetTokenExpired) {
			return ErrPasswordResetExpired
		}
		return fmt.Errorf("completing password reset: %w", err)
	}
	if err := s.appendAuditLog(ctx, reset.CreatedBy, &reset.UserID, "auth.password_reset_completed", "auth", reset.UserID.String(), `{}`); err != nil {
		return err
	}
	return nil
}

// HasRole returns true when the authenticated user has roleID.
func (u *AuthenticatedUser) HasRole(roleID string) bool {
	if u == nil {
		return false
	}
	return u.User.HasRole(roleID)
}

// HasPermission returns true when the authenticated user has permissionKey.
func (u *AuthenticatedUser) HasPermission(permissionKey string) bool {
	if u == nil {
		return false
	}
	return u.User.HasPermission(permissionKey)
}

// RequirePermission returns ErrPermissionDenied unless user has permissionKey.
func (s *AuthService) RequirePermission(user *AuthenticatedUser, permissionKey string) error {
	if user == nil || !user.HasPermission(permissionKey) {
		return ErrPermissionDenied
	}
	return nil
}

// RequireRole returns ErrPermissionDenied unless user has roleID.
func (s *AuthService) RequireRole(user *AuthenticatedUser, roleID string) error {
	if user == nil || !user.HasRole(roleID) {
		return ErrPermissionDenied
	}
	return nil
}

func (s *AuthService) rejectInactiveOrLocked(ctx context.Context, user *domain.User, ipAddress, userAgent string) error {
	if err := s.ensureAggregateCanAuthenticate(user); err != nil {
		_ = s.appendAuditLog(ctx, nil, &user.ID, "auth.login_failed", "auth", user.ID.String(), `{"reason":"account_unavailable"}`)
		return err
	}
	return nil
}

func (s *AuthService) ensureAggregateCanAuthenticate(user *domain.User) error {
	if user.Status == domain.UserStatusDisabled || user.Status == domain.UserStatusPending {
		return ErrUserDisabled
	}
	if user.Status == domain.UserStatusLocked {
		return ErrUserLocked
	}
	if user.LockedUntil != nil && user.LockedUntil.After(s.now()) {
		return ErrUserLocked
	}
	return nil
}

func (s *AuthService) resetExpiredLockState(user *domain.User) {
	if user.LockedUntil != nil && !user.LockedUntil.After(s.now()) {
		user.LockedUntil = nil
		user.FailedLoginAttempts = 0
	}
}

func (s *AuthService) recordFailedLogin(ctx context.Context, user *domain.User, ipAddress, userAgent string) error {
	now := s.now()
	if user.LockedUntil != nil && !user.LockedUntil.After(now) {
		user.LockedUntil = nil
		user.FailedLoginAttempts = 0
	}
	user.FailedLoginAttempts++
	user.UpdatedAt = now
	if user.FailedLoginAttempts >= s.failedThreshold {
		lockedUntil := now.Add(s.failedLockDuration)
		user.LockedUntil = &lockedUntil
	}
	if err := s.users.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("updating failed auth login: %w", err)
	}
	if user.FailedLoginAttempts >= s.failedDelayAfter && s.failedDelay > 0 {
		s.failedSleeper(s.failedDelay)
	}
	if err := s.appendAuditLog(ctx, nil, &user.ID, "auth.login_failed", "auth", user.ID.String(), `{"reason":"invalid_credentials"}`); err != nil {
		return err
	}
	return nil
}

func (s *AuthService) runDummyPasswordVerification(password string) {
	_, _ = s.verifyPassword(password, s.dummyPasswordHash)
}

func (s *AuthService) appendAuditLog(ctx context.Context, actorUserID, targetUserID *uuid.UUID, action, resource, resourceID, metadataJSON string) error {
	if metadataJSON == "" {
		metadataJSON = `{}`
	}
	log := domain.AuditLog{
		ID:           uuid.New(),
		ActorUserID:  actorUserID,
		TargetUserID: targetUserID,
		Action:       action,
		Resource:     resource,
		ResourceID:   resourceID,
		MetadataJSON: metadataJSON,
		CreatedAt:    s.now(),
	}
	if err := s.auditLogs.AppendAuditLog(ctx, &log); err != nil {
		return fmt.Errorf("appending auth audit log: %w", err)
	}
	return nil
}

func normalizeLoginIdentifier(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func authenticatedSessionFromDomain(session domain.AuthSession) AuthenticatedSession {
	return AuthenticatedSession{
		ID:         session.ID,
		UserID:     session.UserID,
		CreatedAt:  session.CreatedAt,
		ExpiresAt:  session.ExpiresAt,
		LastSeenAt: session.LastSeenAt,
		IPAddress:  session.IPAddress,
		UserAgent:  session.UserAgent,
	}
}
