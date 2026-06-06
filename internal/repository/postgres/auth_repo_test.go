package postgres

// This file exercises auth repo behavior so refactors preserve the documented contract.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func newAuthRepoForTest(t *testing.T) (*AuthRepo, context.Context) {
	t.Helper()

	db := setupTestDB(t)
	if err := seedAuthSystemRolesAndPermissions(db); err != nil {
		t.Fatalf("seeding auth roles and permissions: %v", err)
	}

	return NewAuthRepo(db), context.Background()
}

func testAuthUser(username, email string) domain.User {
	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	return domain.User{
		ID:                 uuid.New(),
		Username:           username,
		UsernameNormalized: username,
		Email:              email,
		EmailNormalized:    email,
		PasswordHash:       "hashed-password",
		DisplayName:        username + " display",
		Status:             domain.UserStatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
		PasswordChangedAt:  &now,
	}
}

func TestAuthRepoCreateUserAndLookupByNormalizedIdentifiers(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	user := testAuthUser("alice", "alice@example.test")
	if err := repo.CreateUser(ctx, &user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	byUsername, err := repo.GetUserByLoginIdentifier(ctx, user.UsernameNormalized)
	if err != nil {
		t.Fatalf("GetUserByLoginIdentifier username: %v", err)
	}
	if byUsername.ID != user.ID || byUsername.PasswordHash == "" {
		t.Fatalf("lookup by username returned %#v", byUsername)
	}

	byEmail, err := repo.GetUserByLoginIdentifier(ctx, user.EmailNormalized)
	if err != nil {
		t.Fatalf("GetUserByLoginIdentifier email: %v", err)
	}
	if byEmail.ID != user.ID {
		t.Fatalf("lookup by email ID = %s, want %s", byEmail.ID, user.ID)
	}

	byID, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if byID.Username != user.Username || byID.Email != user.Email {
		t.Fatalf("GetUserByID returned %#v", byID)
	}
}

func TestAuthRepoCreateUserRejectsDuplicateNormalizedUsernameAndEmail(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	first := testAuthUser("duplicate", "duplicate@example.test")
	if err := repo.CreateUser(ctx, &first); err != nil {
		t.Fatalf("CreateUser first: %v", err)
	}

	duplicateUsername := testAuthUser("duplicate", "unique@example.test")
	if err := repo.CreateUser(ctx, &duplicateUsername); err == nil {
		t.Fatal("CreateUser duplicate username returned nil error")
	}

	duplicateEmail := testAuthUser("unique", "duplicate@example.test")
	if err := repo.CreateUser(ctx, &duplicateEmail); err == nil {
		t.Fatal("CreateUser duplicate email returned nil error")
	}
}

func TestAuthRepoListsSeededRolesAndPermissions(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	roles, err := repo.ListRoles(ctx)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(roles) != len(domain.SystemRoleNames()) {
		t.Fatalf("ListRoles count = %d, want %d", len(roles), len(domain.SystemRoleNames()))
	}

	permissions, err := repo.ListPermissions(ctx)
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if len(permissions) != len(domain.SystemPermissions()) {
		t.Fatalf("ListPermissions count = %d, want %d", len(permissions), len(domain.SystemPermissions()))
	}

	role, err := repo.GetRoleByName(ctx, domain.RoleSuperAdmin)
	if err != nil {
		t.Fatalf("GetRoleByName: %v", err)
	}
	if role.ID != domain.RoleSuperAdmin || !role.IsSystemRole {
		t.Fatalf("GetRoleByName returned %#v", role)
	}
}

func TestAuthRepoNotFoundErrorsMatchDomainSentinels(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	if _, err := repo.GetUserByID(ctx, uuid.New()); !errors.Is(err, domain.ErrAuthUserNotFound) {
		t.Fatalf("GetUserByID error = %v, want ErrAuthUserNotFound", err)
	}
	if _, err := repo.GetUserByLoginIdentifier(ctx, "missing@example.test"); !errors.Is(err, domain.ErrAuthUserNotFound) {
		t.Fatalf("GetUserByLoginIdentifier error = %v, want ErrAuthUserNotFound", err)
	}
	if err := repo.UpdateUser(ctx, &domain.User{ID: uuid.New()}); !errors.Is(err, domain.ErrAuthUserNotFound) {
		t.Fatalf("UpdateUser error = %v, want ErrAuthUserNotFound", err)
	}
	if _, err := repo.GetRoleByName(ctx, "missing-role"); !errors.Is(err, domain.ErrAuthRoleNotFound) {
		t.Fatalf("GetRoleByName error = %v, want ErrAuthRoleNotFound", err)
	}
	if _, err := repo.GetSessionByTokenHash(ctx, "missing-session"); !errors.Is(err, domain.ErrAuthSessionNotFound) {
		t.Fatalf("GetSessionByTokenHash error = %v, want ErrAuthSessionNotFound", err)
	}
	if err := repo.RevokeSession(ctx, uuid.New(), time.Now().UTC()); !errors.Is(err, domain.ErrAuthSessionNotFound) {
		t.Fatalf("RevokeSession error = %v, want ErrAuthSessionNotFound", err)
	}
	if err := repo.TouchSession(ctx, uuid.New(), time.Now().UTC()); !errors.Is(err, domain.ErrAuthSessionNotFound) {
		t.Fatalf("TouchSession error = %v, want ErrAuthSessionNotFound", err)
	}
	if _, err := repo.GetPasswordResetTokenByHash(ctx, "missing-reset-token"); !errors.Is(err, domain.ErrPasswordResetTokenNotFound) {
		t.Fatalf("GetPasswordResetTokenByHash error = %v, want ErrPasswordResetTokenNotFound", err)
	}
	if err := repo.MarkPasswordResetTokenUsed(ctx, uuid.New(), time.Now().UTC()); !errors.Is(err, domain.ErrPasswordResetTokenNotFound) {
		t.Fatalf("MarkPasswordResetTokenUsed error = %v, want ErrPasswordResetTokenNotFound", err)
	}
}

func TestAuthRepoAssignRemoveRoleIsIdempotentAndLoadsAggregate(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	user := testAuthUser("role-user", "role-user@example.test")
	if err := repo.CreateUser(ctx, &user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := repo.AssignRole(ctx, user.ID, domain.RoleViewer, nil); err != nil {
		t.Fatalf("AssignRole first: %v", err)
	}
	if err := repo.AssignRole(ctx, user.ID, domain.RoleViewer, nil); err != nil {
		t.Fatalf("AssignRole second: %v", err)
	}

	aggregate, err := repo.GetUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserRolesAndPermissions: %v", err)
	}
	if len(aggregate.Roles) != 1 || aggregate.Roles[0].ID != domain.RoleViewer {
		t.Fatalf("aggregate roles = %#v", aggregate.Roles)
	}
	if len(aggregate.Permissions) == 0 {
		t.Fatal("aggregate permissions is empty")
	}

	listed, err := repo.ListUsers(ctx, domain.UserListFilter{Status: domain.UserStatusActive})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(listed) != 1 || listed[0].User.ID != user.ID || len(listed[0].Roles) != 1 {
		t.Fatalf("ListUsers returned %#v", listed)
	}

	if err := repo.RemoveRole(ctx, user.ID, domain.RoleViewer); err != nil {
		t.Fatalf("RemoveRole first: %v", err)
	}
	if err := repo.RemoveRole(ctx, user.ID, domain.RoleViewer); err != nil {
		t.Fatalf("RemoveRole second: %v", err)
	}
	aggregate, err = repo.GetUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserRolesAndPermissions after remove: %v", err)
	}
	if len(aggregate.Roles) != 0 || len(aggregate.Permissions) != 0 {
		t.Fatalf("aggregate after remove = %#v", aggregate)
	}
}

func TestAuthRepoGuardedSuperAdminMutationsPreserveLastActive(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	root := testAuthUser("guarded-root", "guarded-root@example.test")
	if err := repo.CreateUser(ctx, &root); err != nil {
		t.Fatalf("CreateUser root: %v", err)
	}
	if err := repo.AssignRole(ctx, root.ID, domain.RoleSuperAdmin, nil); err != nil {
		t.Fatalf("AssignRole root: %v", err)
	}

	if err := repo.RemoveRolePreservingLastActiveSuperAdmin(ctx, root.ID, domain.RoleSuperAdmin); !errors.Is(err, domain.ErrAuthLastActiveSuperAdmin) {
		t.Fatalf("RemoveRolePreservingLastActiveSuperAdmin error = %v, want ErrAuthLastActiveSuperAdmin", err)
	}
	aggregate, err := repo.GetUserRolesAndPermissions(ctx, root.ID)
	if err != nil {
		t.Fatalf("GetUserRolesAndPermissions after blocked remove: %v", err)
	}
	if !aggregate.HasRole(domain.RoleSuperAdmin) {
		t.Fatal("blocked guarded role removal removed the super_admin role")
	}

	disabledRoot := root
	disabledRoot.Status = domain.UserStatusDisabled
	if err := repo.UpdateUserPreservingLastActiveSuperAdmin(ctx, &disabledRoot); !errors.Is(err, domain.ErrAuthLastActiveSuperAdmin) {
		t.Fatalf("UpdateUserPreservingLastActiveSuperAdmin error = %v, want ErrAuthLastActiveSuperAdmin", err)
	}
	storedRoot, err := repo.GetUserByID(ctx, root.ID)
	if err != nil {
		t.Fatalf("GetUserByID after blocked status update: %v", err)
	}
	if storedRoot.Status != domain.UserStatusActive {
		t.Fatalf("status after blocked guarded update = %s, want active", storedRoot.Status)
	}

	other := testAuthUser("second-root", "second-root@example.test")
	if err := repo.CreateUser(ctx, &other); err != nil {
		t.Fatalf("CreateUser second root: %v", err)
	}
	if err := repo.AssignRole(ctx, other.ID, domain.RoleSuperAdmin, nil); err != nil {
		t.Fatalf("AssignRole second root: %v", err)
	}
	if err := repo.UpdateUserPreservingLastActiveSuperAdmin(ctx, &disabledRoot); err != nil {
		t.Fatalf("UpdateUserPreservingLastActiveSuperAdmin with second root: %v", err)
	}
	storedRoot, err = repo.GetUserByID(ctx, root.ID)
	if err != nil {
		t.Fatalf("GetUserByID after allowed status update: %v", err)
	}
	if storedRoot.Status != domain.UserStatusDisabled {
		t.Fatalf("status after allowed guarded update = %s, want disabled", storedRoot.Status)
	}
}

func TestAuthRepoListUsersFiltersByRole(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	viewer := testAuthUser("viewer-user", "viewer-user@example.test")
	if err := repo.CreateUser(ctx, &viewer); err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	admin := testAuthUser("admin-user", "admin-user@example.test")
	if err := repo.CreateUser(ctx, &admin); err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := repo.AssignRole(ctx, viewer.ID, domain.RoleViewer, nil); err != nil {
		t.Fatalf("AssignRole viewer: %v", err)
	}
	if err := repo.AssignRole(ctx, admin.ID, domain.RoleAdmin, nil); err != nil {
		t.Fatalf("AssignRole admin: %v", err)
	}

	listed, err := repo.ListUsers(ctx, domain.UserListFilter{RoleID: domain.RoleViewer})
	if err != nil {
		t.Fatalf("ListUsers role filter: %v", err)
	}
	if len(listed) != 1 || listed[0].User.ID != viewer.ID {
		t.Fatalf("ListUsers role filter returned %#v", listed)
	}
	if len(listed[0].Roles) != 1 || listed[0].Roles[0].ID != domain.RoleViewer {
		t.Fatalf("ListUsers role filter roles = %#v", listed[0].Roles)
	}
}

func TestAuthRepoSessionCreateGetRevokeAndTouch(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	user := testAuthUser("session-user", "session-user@example.test")
	if err := repo.CreateUser(ctx, &user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	session := domain.AuthSession{
		ID:            uuid.New(),
		UserID:        user.ID,
		TokenHash:     "session-token-hash",
		CSRFTokenHash: "csrf-token-hash",
		CreatedAt:     now,
		ExpiresAt:     now.Add(time.Hour),
		IPAddress:     "192.0.2.10",
		UserAgent:     "test-agent",
	}
	if err := repo.CreateSession(ctx, &session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	seen := now.Add(5 * time.Minute)
	if err := repo.TouchSession(ctx, session.ID, seen); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	got, err := repo.GetSessionByTokenHash(ctx, session.TokenHash)
	if err != nil {
		t.Fatalf("GetSessionByTokenHash: %v", err)
	}
	if got.ID != session.ID || got.LastSeenAt == nil || !got.LastSeenAt.Equal(seen) {
		t.Fatalf("session after touch = %#v", got)
	}

	revoked := now.Add(10 * time.Minute)
	if err := repo.RevokeSession(ctx, session.ID, revoked); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	got, err = repo.GetSessionByTokenHash(ctx, session.TokenHash)
	if err != nil {
		t.Fatalf("GetSessionByTokenHash after revoke: %v", err)
	}
	if got.RevokedAt == nil || !got.RevokedAt.Equal(revoked) {
		t.Fatalf("session revoked_at = %#v", got.RevokedAt)
	}

	other := session
	other.ID = uuid.New()
	other.TokenHash = "other-session-token-hash"
	if err := repo.CreateSession(ctx, &other); err != nil {
		t.Fatalf("CreateSession other: %v", err)
	}
	if err := repo.RevokeUserSessions(ctx, user.ID, &session.ID, revoked); err != nil {
		t.Fatalf("RevokeUserSessions: %v", err)
	}
	otherGot, err := repo.GetSessionByTokenHash(ctx, other.TokenHash)
	if err != nil {
		t.Fatalf("GetSessionByTokenHash other: %v", err)
	}
	if otherGot.RevokedAt == nil {
		t.Fatal("RevokeUserSessions did not revoke non-excepted session")
	}
}

func TestAuthRepoPasswordResetTokenPersistsExpiryAndUsedAt(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	user := testAuthUser("reset-user", "reset-user@example.test")
	if err := repo.CreateUser(ctx, &user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	now := time.Date(2026, 5, 21, 11, 0, 0, 0, time.UTC)
	token := domain.PasswordResetToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: "reset-token-hash",
		CreatedAt: now,
		ExpiresAt: now.Add(30 * time.Minute),
	}
	if err := repo.CreatePasswordResetToken(ctx, &token); err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}

	got, err := repo.GetPasswordResetTokenByHash(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("GetPasswordResetTokenByHash: %v", err)
	}
	if got.ID != token.ID || !got.ExpiresAt.Equal(token.ExpiresAt) || got.UsedAt != nil {
		t.Fatalf("password reset token = %#v", got)
	}

	usedAt := now.Add(5 * time.Minute)
	if err := repo.MarkPasswordResetTokenUsed(ctx, token.ID, usedAt); err != nil {
		t.Fatalf("MarkPasswordResetTokenUsed: %v", err)
	}
	got, err = repo.GetPasswordResetTokenByHash(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("GetPasswordResetTokenByHash after used: %v", err)
	}
	if got.UsedAt == nil || !got.UsedAt.Equal(usedAt) {
		t.Fatalf("UsedAt = %#v, want %s", got.UsedAt, usedAt)
	}
}

func TestAuthRepoCompletePasswordResetUpdatesPasswordRevokesSessionsAndConsumesToken(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	user := testAuthUser("reset-complete-user", "reset-complete-user@example.test")
	user.MustChangePassword = true
	user.FailedLoginAttempts = 4
	lockedUntil := user.CreatedAt.Add(time.Hour)
	user.LockedUntil = &lockedUntil
	if err := repo.CreateUser(ctx, &user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	now := time.Date(2026, 5, 21, 11, 30, 0, 0, time.UTC)
	session := domain.AuthSession{
		ID:            uuid.New(),
		UserID:        user.ID,
		TokenHash:     "reset-complete-session-hash",
		CSRFTokenHash: "reset-complete-csrf-hash",
		CreatedAt:     now,
		ExpiresAt:     now.Add(time.Hour),
	}
	if err := repo.CreateSession(ctx, &session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	token := domain.PasswordResetToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: "reset-complete-token-hash",
		CreatedAt: now,
		ExpiresAt: now.Add(30 * time.Minute),
	}
	if err := repo.CreatePasswordResetToken(ctx, &token); err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}

	completed, err := repo.CompletePasswordReset(ctx, token.TokenHash, "new-reset-password-hash", now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("CompletePasswordReset: %v", err)
	}
	if completed.ID != token.ID || completed.UserID != user.ID {
		t.Fatalf("completed token = %#v, want token ID %s user ID %s", completed, token.ID, user.ID)
	}

	gotUser, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if gotUser.PasswordHash != "new-reset-password-hash" {
		t.Fatalf("PasswordHash = %q, want new reset hash", gotUser.PasswordHash)
	}
	if gotUser.MustChangePassword {
		t.Fatal("MustChangePassword = true, want false")
	}
	if gotUser.PasswordChangedAt == nil || !gotUser.PasswordChangedAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("PasswordChangedAt = %#v, want reset completion time", gotUser.PasswordChangedAt)
	}
	if gotUser.FailedLoginAttempts != 0 || gotUser.LockedUntil != nil {
		t.Fatalf("login failure state = attempts:%d locked:%#v, want cleared", gotUser.FailedLoginAttempts, gotUser.LockedUntil)
	}

	gotToken, err := repo.GetPasswordResetTokenByHash(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("GetPasswordResetTokenByHash: %v", err)
	}
	if gotToken.UsedAt == nil || !gotToken.UsedAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("UsedAt = %#v, want reset completion time", gotToken.UsedAt)
	}
	gotSession, err := repo.GetSessionByTokenHash(ctx, session.TokenHash)
	if err != nil {
		t.Fatalf("GetSessionByTokenHash: %v", err)
	}
	if gotSession.RevokedAt == nil || !gotSession.RevokedAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("RevokedAt = %#v, want reset completion time", gotSession.RevokedAt)
	}
}

func TestAuthRepoCompletePasswordResetRejectsUsedAndExpiredTokens(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	user := testAuthUser("reset-reject-user", "reset-reject-user@example.test")
	if err := repo.CreateUser(ctx, &user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	now := time.Date(2026, 5, 21, 11, 45, 0, 0, time.UTC)
	session := domain.AuthSession{
		ID:            uuid.New(),
		UserID:        user.ID,
		TokenHash:     "reset-reject-session-hash",
		CSRFTokenHash: "reset-reject-csrf-hash",
		CreatedAt:     now,
		ExpiresAt:     now.Add(time.Hour),
	}
	if err := repo.CreateSession(ctx, &session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	usedAt := now.Add(-time.Minute)
	usedToken := domain.PasswordResetToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: "used-reset-token-hash",
		CreatedAt: now.Add(-10 * time.Minute),
		ExpiresAt: now.Add(20 * time.Minute),
		UsedAt:    &usedAt,
	}
	if err := repo.CreatePasswordResetToken(ctx, &usedToken); err != nil {
		t.Fatalf("CreatePasswordResetToken used: %v", err)
	}
	expiredToken := domain.PasswordResetToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: "expired-reset-token-hash",
		CreatedAt: now.Add(-time.Hour),
		ExpiresAt: now.Add(-time.Minute),
	}
	if err := repo.CreatePasswordResetToken(ctx, &expiredToken); err != nil {
		t.Fatalf("CreatePasswordResetToken expired: %v", err)
	}

	for _, tokenHash := range []string{usedToken.TokenHash, expiredToken.TokenHash} {
		if _, err := repo.CompletePasswordReset(ctx, tokenHash, "new-password-hash", now); !errors.Is(err, domain.ErrPasswordResetTokenExpired) {
			t.Fatalf("CompletePasswordReset(%q) error = %v, want ErrPasswordResetTokenExpired", tokenHash, err)
		}
	}

	gotUser, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if gotUser.PasswordHash != user.PasswordHash {
		t.Fatalf("PasswordHash = %q, want unchanged %q", gotUser.PasswordHash, user.PasswordHash)
	}
	gotSession, err := repo.GetSessionByTokenHash(ctx, session.TokenHash)
	if err != nil {
		t.Fatalf("GetSessionByTokenHash: %v", err)
	}
	if gotSession.RevokedAt != nil {
		t.Fatalf("RevokedAt = %#v, want nil for rejected reset", gotSession.RevokedAt)
	}
}

func TestAuthRepoAuditAppendListOrderingAndDashboardStats(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)

	active := testAuthUser("active-user", "active-user@example.test")
	active.LastLoginAt = ptrTime(time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC))
	if err := repo.CreateUser(ctx, &active); err != nil {
		t.Fatalf("CreateUser active: %v", err)
	}
	disabled := testAuthUser("disabled-user", "disabled-user@example.test")
	disabled.Status = domain.UserStatusDisabled
	if err := repo.CreateUser(ctx, &disabled); err != nil {
		t.Fatalf("CreateUser disabled: %v", err)
	}
	locked := testAuthUser("locked-user", "locked-user@example.test")
	locked.Status = domain.UserStatusLocked
	if err := repo.CreateUser(ctx, &locked); err != nil {
		t.Fatalf("CreateUser locked: %v", err)
	}

	first := domain.AuditLog{
		ID:           uuid.New(),
		ActorUserID:  &active.ID,
		TargetUserID: &disabled.ID,
		Action:       "auth.login_failed",
		Resource:     "auth",
		MetadataJSON: `{"reason":"bad_password"}`,
		CreatedAt:    time.Date(2026, 5, 21, 12, 1, 0, 0, time.UTC),
	}
	second := first
	second.ID = uuid.New()
	second.Action = "auth.login"
	second.CreatedAt = first.CreatedAt.Add(time.Minute)

	if err := repo.AppendAuditLog(ctx, &first); err != nil {
		t.Fatalf("AppendAuditLog first: %v", err)
	}
	if err := repo.AppendAuditLog(ctx, &second); err != nil {
		t.Fatalf("AppendAuditLog second: %v", err)
	}

	logs, err := repo.ListAuditLogs(ctx, domain.AuditLogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLogs: %v", err)
	}
	if len(logs) != 2 || logs[0].ID != second.ID || logs[1].ID != first.ID {
		t.Fatalf("audit logs order = %#v", logs)
	}

	stats, err := repo.DashboardStats(ctx)
	if err != nil {
		t.Fatalf("DashboardStats: %v", err)
	}
	if stats.TotalUsers != 3 || stats.ActiveUsers != 1 || stats.DisabledUsers != 1 || stats.LockedUsers != 1 {
		t.Fatalf("DashboardStats users = %#v", stats)
	}
	if stats.RecentLogins != 1 || stats.RecentFailedLoginAttempts != 1 {
		t.Fatalf("DashboardStats recent activity = %#v", stats)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
