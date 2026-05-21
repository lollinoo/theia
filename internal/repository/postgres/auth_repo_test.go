package postgres

import (
	"context"
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
