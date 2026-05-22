package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// AuthRepo implements auth-related repositories using PostgreSQL.
type AuthRepo struct {
	db *DB
}

const authSuperAdminMutationLockID int64 = 8413371001

// NewAuthRepo creates a PostgreSQL-backed auth repository.
func NewAuthRepo(db *sql.DB) *AuthRepo {
	return &AuthRepo{db: wrapDB(db)}
}

// CreateUser inserts a user.
func (r *AuthRepo) CreateUser(ctx context.Context, user *domain.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = user.CreatedAt
	}
	if user.Status == "" {
		user.Status = domain.UserStatusPending
	}

	_, err := r.execContext(ctx,
		`INSERT INTO users (
			id, username, username_normalized, email, email_normalized, password_hash,
			display_name, status, must_change_password, created_at, updated_at,
			last_login_at, password_changed_at, failed_login_attempts, locked_until,
			created_by, updated_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID.String(),
		user.Username,
		user.UsernameNormalized,
		user.Email,
		user.EmailNormalized,
		user.PasswordHash,
		user.DisplayName,
		string(user.Status),
		user.MustChangePassword,
		user.CreatedAt,
		user.UpdatedAt,
		user.LastLoginAt,
		user.PasswordChangedAt,
		user.FailedLoginAttempts,
		user.LockedUntil,
		uuidPtrString(user.CreatedBy),
		uuidPtrString(user.UpdatedBy),
	)
	if err != nil {
		return fmt.Errorf("creating auth user: %w", err)
	}
	return nil
}

// GetUserByID returns a user by ID.
func (r *AuthRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	user, err := r.scanUser(r.queryRowContext(ctx, userSelectSQL()+` WHERE id = ?`, id.String()))
	if err != nil {
		return nil, fmt.Errorf("getting auth user by id: %w", err)
	}
	return user, nil
}

// GetUserByLoginIdentifier returns a user by normalized username or email.
func (r *AuthRepo) GetUserByLoginIdentifier(ctx context.Context, normalized string) (*domain.User, error) {
	user, err := r.scanUser(r.queryRowContext(ctx,
		userSelectSQL()+` WHERE username_normalized = ? OR email_normalized = ?`,
		normalized,
		normalized,
	))
	if err != nil {
		return nil, fmt.Errorf("getting auth user by login identifier: %w", err)
	}
	return user, nil
}

// ListUsers returns users with their roles and effective permissions.
func (r *AuthRepo) ListUsers(ctx context.Context, filter domain.UserListFilter) ([]domain.UserWithRolesAndPermissions, error) {
	query := userSelectSQL()
	var conditions []string
	var args []interface{}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(filter.Status))
	}
	if strings.TrimSpace(filter.Query) != "" {
		conditions = append(conditions, "(username_normalized LIKE ? OR email_normalized LIKE ? OR display_name ILIKE ?)")
		like := "%" + strings.ToLower(strings.TrimSpace(filter.Query)) + "%"
		args = append(args, like, like, "%"+strings.TrimSpace(filter.Query)+"%")
	}
	if filter.RoleID != "" {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = users.id AND ur.role_id = ?)")
		args = append(args, filter.RoleID)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY username_normalized ASC, id ASC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := r.queryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing auth users: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		user, err := r.scanUserRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning listed auth user: %w", err)
		}
		users = append(users, *user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating auth users: %w", err)
	}

	aggregates := make([]domain.UserWithRolesAndPermissions, 0, len(users))
	for _, user := range users {
		aggregate, err := r.userRolesAndPermissions(ctx, user)
		if err != nil {
			return nil, fmt.Errorf("loading auth user aggregate: %w", err)
		}
		aggregates = append(aggregates, *aggregate)
	}
	return aggregates, nil
}

// UpdateUser updates profile, status, password, and login counter fields.
func (r *AuthRepo) UpdateUser(ctx context.Context, user *domain.User) error {
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = time.Now().UTC()
	}
	res, err := r.execContext(ctx,
		`UPDATE users SET
			username = ?,
			username_normalized = ?,
			email = ?,
			email_normalized = ?,
			password_hash = ?,
			display_name = ?,
			status = ?,
			must_change_password = ?,
			updated_at = ?,
			last_login_at = ?,
			password_changed_at = ?,
			failed_login_attempts = ?,
			locked_until = ?,
			updated_by = ?
		WHERE id = ?`,
		user.Username,
		user.UsernameNormalized,
		user.Email,
		user.EmailNormalized,
		user.PasswordHash,
		user.DisplayName,
		string(user.Status),
		user.MustChangePassword,
		user.UpdatedAt,
		user.LastLoginAt,
		user.PasswordChangedAt,
		user.FailedLoginAttempts,
		user.LockedUntil,
		uuidPtrString(user.UpdatedBy),
		user.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("updating auth user: %w", err)
	}
	if err := requireRowsAffected(res, domain.ErrAuthUserNotFound); err != nil {
		return fmt.Errorf("updating auth user: %w", err)
	}
	return nil
}

// UpdateUserPreservingLastActiveSuperAdmin updates a user while rejecting demotion of the final active super-admin.
func (r *AuthRepo) UpdateUserPreservingLastActiveSuperAdmin(ctx context.Context, user *domain.User) error {
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = time.Now().UTC()
	}
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning guarded auth user update transaction: %w", err)
	}
	defer tx.Rollback()

	if err := lockAuthSuperAdminMutation(ctx, tx); err != nil {
		return fmt.Errorf("locking guarded auth user update: %w", err)
	}
	if user.Status != domain.UserStatusActive {
		activeSuperAdmin, err := txUserIsActiveSuperAdmin(ctx, tx, user.ID)
		if err != nil {
			return fmt.Errorf("checking guarded auth user status: %w", err)
		}
		if activeSuperAdmin {
			count, err := txCountActiveSuperAdmins(ctx, tx)
			if err != nil {
				return fmt.Errorf("counting guarded active super admins: %w", err)
			}
			if count <= 1 {
				return domain.ErrAuthLastActiveSuperAdmin
			}
		}
	}

	res, err := tx.ExecContext(ctx, rebindQuery(
		`UPDATE users SET
			username = ?,
			username_normalized = ?,
			email = ?,
			email_normalized = ?,
			password_hash = ?,
			display_name = ?,
			status = ?,
			must_change_password = ?,
			updated_at = ?,
			last_login_at = ?,
			password_changed_at = ?,
			failed_login_attempts = ?,
			locked_until = ?,
			updated_by = ?
		WHERE id = ?`),
		user.Username,
		user.UsernameNormalized,
		user.Email,
		user.EmailNormalized,
		user.PasswordHash,
		user.DisplayName,
		string(user.Status),
		user.MustChangePassword,
		user.UpdatedAt,
		user.LastLoginAt,
		user.PasswordChangedAt,
		user.FailedLoginAttempts,
		user.LockedUntil,
		uuidPtrString(user.UpdatedBy),
		user.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("updating guarded auth user: %w", err)
	}
	if err := requireRowsAffected(res, domain.ErrAuthUserNotFound); err != nil {
		return fmt.Errorf("updating guarded auth user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing guarded auth user update transaction: %w", err)
	}
	return nil
}

// CountUsers returns the total number of users.
func (r *AuthRepo) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := r.queryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting auth users: %w", err)
	}
	return count, nil
}

// CountActiveSuperAdmins returns active users assigned the super-admin role.
func (r *AuthRepo) CountActiveSuperAdmins(ctx context.Context) (int, error) {
	var count int
	err := r.queryRowContext(ctx,
		`SELECT COUNT(DISTINCT u.id)
		 FROM users u
		 JOIN user_roles ur ON ur.user_id = u.id
		 WHERE u.status = ? AND ur.role_id = ?`,
		string(domain.UserStatusActive),
		domain.RoleSuperAdmin,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting active super admins: %w", err)
	}
	return count, nil
}

// ListRoles returns all roles.
func (r *AuthRepo) ListRoles(ctx context.Context) ([]domain.Role, error) {
	rows, err := r.queryContext(ctx,
		`SELECT id, name, description, is_system_role, created_at, updated_at
		 FROM roles
		 ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing auth roles: %w", err)
	}
	defer rows.Close()

	var roles []domain.Role
	for rows.Next() {
		role, err := scanRoleRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning auth role: %w", err)
		}
		roles = append(roles, *role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating auth roles: %w", err)
	}
	return roles, nil
}

// ListPermissions returns all permissions.
func (r *AuthRepo) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	rows, err := r.queryContext(ctx,
		`SELECT id, key, description, resource, action
		 FROM permissions
		 ORDER BY key ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing auth permissions: %w", err)
	}
	defer rows.Close()

	var permissions []domain.Permission
	for rows.Next() {
		permission, err := scanPermissionRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning auth permission: %w", err)
		}
		permissions = append(permissions, *permission)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating auth permissions: %w", err)
	}
	return permissions, nil
}

// GetRoleByName returns a role by name.
func (r *AuthRepo) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	role, err := scanRole(r.queryRowContext(ctx,
		`SELECT id, name, description, is_system_role, created_at, updated_at
		 FROM roles
		 WHERE name = ?`,
		name,
	))
	if err != nil {
		return nil, fmt.Errorf("getting auth role by name: %w", err)
	}
	return role, nil
}

// AssignRole assigns a role to a user.
func (r *AuthRepo) AssignRole(ctx context.Context, userID uuid.UUID, roleID string, createdBy *uuid.UUID) error {
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning assign auth role transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, rebindQuery(
		`INSERT INTO user_roles (user_id, role_id, created_by)
		 VALUES (?, ?, ?)
		 ON CONFLICT(user_id, role_id) DO NOTHING`),
		userID.String(),
		roleID,
		uuidPtrString(createdBy),
	); err != nil {
		return fmt.Errorf("assigning auth role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing assign auth role transaction: %w", err)
	}
	return nil
}

// RemoveRole removes a role assignment from a user.
func (r *AuthRepo) RemoveRole(ctx context.Context, userID uuid.UUID, roleID string) error {
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning remove auth role transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, rebindQuery(
		`DELETE FROM user_roles
		 WHERE user_id = ? AND role_id = ?`),
		userID.String(),
		roleID,
	); err != nil {
		return fmt.Errorf("removing auth role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing remove auth role transaction: %w", err)
	}
	return nil
}

// RemoveRolePreservingLastActiveSuperAdmin removes a role while preserving the final active super-admin.
func (r *AuthRepo) RemoveRolePreservingLastActiveSuperAdmin(ctx context.Context, userID uuid.UUID, roleID string) error {
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning guarded remove auth role transaction: %w", err)
	}
	defer tx.Rollback()

	if err := lockAuthSuperAdminMutation(ctx, tx); err != nil {
		return fmt.Errorf("locking guarded remove auth role: %w", err)
	}
	if roleID == domain.RoleSuperAdmin {
		activeSuperAdmin, err := txUserIsActiveSuperAdmin(ctx, tx, userID)
		if err != nil {
			return fmt.Errorf("checking guarded auth role removal target: %w", err)
		}
		if activeSuperAdmin {
			count, err := txCountActiveSuperAdmins(ctx, tx)
			if err != nil {
				return fmt.Errorf("counting guarded active super admins: %w", err)
			}
			if count <= 1 {
				return domain.ErrAuthLastActiveSuperAdmin
			}
		}
	}

	if _, err := tx.ExecContext(ctx, rebindQuery(
		`DELETE FROM user_roles
		 WHERE user_id = ? AND role_id = ?`),
		userID.String(),
		roleID,
	); err != nil {
		return fmt.Errorf("removing guarded auth role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing guarded remove auth role transaction: %w", err)
	}
	return nil
}

// GetUserRolesAndPermissions returns a user with roles and permissions.
func (r *AuthRepo) GetUserRolesAndPermissions(ctx context.Context, userID uuid.UUID) (*domain.UserWithRolesAndPermissions, error) {
	user, err := r.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("getting auth user aggregate user: %w", err)
	}
	aggregate, err := r.userRolesAndPermissions(ctx, *user)
	if err != nil {
		return nil, fmt.Errorf("getting auth user aggregate grants: %w", err)
	}
	return aggregate, nil
}

// CreateSession inserts an authentication session.
func (r *AuthRepo) CreateSession(ctx context.Context, session *domain.AuthSession) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}

	_, err := r.execContext(ctx,
		`INSERT INTO auth_sessions (
			id, user_id, token_hash, csrf_token_hash, created_at, expires_at,
			revoked_at, last_seen_at, ip_address, user_agent
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID.String(),
		session.UserID.String(),
		session.TokenHash,
		session.CSRFTokenHash,
		session.CreatedAt,
		session.ExpiresAt,
		session.RevokedAt,
		session.LastSeenAt,
		session.IPAddress,
		session.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("creating auth session: %w", err)
	}
	return nil
}

// GetSessionByTokenHash returns a session by token hash.
func (r *AuthRepo) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*domain.AuthSession, error) {
	session, err := r.scanSession(r.queryRowContext(ctx,
		`SELECT id, user_id, token_hash, csrf_token_hash, created_at, expires_at,
		        revoked_at, last_seen_at, ip_address, user_agent
		 FROM auth_sessions
		 WHERE token_hash = ?`,
		tokenHash,
	))
	if err != nil {
		return nil, fmt.Errorf("getting auth session by token hash: %w", err)
	}
	return session, nil
}

// RevokeSession marks one session revoked.
func (r *AuthRepo) RevokeSession(ctx context.Context, sessionID uuid.UUID, when time.Time) error {
	res, err := r.execContext(ctx,
		`UPDATE auth_sessions SET revoked_at = ? WHERE id = ?`,
		when,
		sessionID.String(),
	)
	if err != nil {
		return fmt.Errorf("revoking auth session: %w", err)
	}
	if err := requireRowsAffected(res, domain.ErrAuthSessionNotFound); err != nil {
		return fmt.Errorf("revoking auth session: %w", err)
	}
	return nil
}

// RevokeUserSessions marks all of a user's sessions revoked except one optional session.
func (r *AuthRepo) RevokeUserSessions(ctx context.Context, userID uuid.UUID, exceptSessionID *uuid.UUID, when time.Time) error {
	query := `UPDATE auth_sessions SET revoked_at = ? WHERE user_id = ?`
	args := []interface{}{when, userID.String()}
	if exceptSessionID != nil {
		query += ` AND id <> ?`
		args = append(args, exceptSessionID.String())
	}
	if _, err := r.execContext(ctx, query, args...); err != nil {
		return fmt.Errorf("revoking auth user sessions: %w", err)
	}
	return nil
}

// TouchSession updates a session's last seen timestamp.
func (r *AuthRepo) TouchSession(ctx context.Context, sessionID uuid.UUID, when time.Time) error {
	res, err := r.execContext(ctx,
		`UPDATE auth_sessions SET last_seen_at = ? WHERE id = ?`,
		when,
		sessionID.String(),
	)
	if err != nil {
		return fmt.Errorf("touching auth session: %w", err)
	}
	if err := requireRowsAffected(res, domain.ErrAuthSessionNotFound); err != nil {
		return fmt.Errorf("touching auth session: %w", err)
	}
	return nil
}

// CreatePasswordResetToken inserts a password reset token.
func (r *AuthRepo) CreatePasswordResetToken(ctx context.Context, token *domain.PasswordResetToken) error {
	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}

	_, err := r.execContext(ctx,
		`INSERT INTO password_reset_tokens (
			id, user_id, token_hash, created_at, expires_at, used_at, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		token.ID.String(),
		token.UserID.String(),
		token.TokenHash,
		token.CreatedAt,
		token.ExpiresAt,
		token.UsedAt,
		uuidPtrString(token.CreatedBy),
	)
	if err != nil {
		return fmt.Errorf("creating password reset token: %w", err)
	}
	return nil
}

// GetPasswordResetTokenByHash returns a password reset token by token hash.
func (r *AuthRepo) GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error) {
	token, err := r.scanPasswordResetToken(r.queryRowContext(ctx,
		`SELECT id, user_id, token_hash, created_at, expires_at, used_at, created_by
		 FROM password_reset_tokens
		 WHERE token_hash = ?`,
		tokenHash,
	))
	if err != nil {
		return nil, fmt.Errorf("getting password reset token by hash: %w", err)
	}
	return token, nil
}

// MarkPasswordResetTokenUsed records that a password reset token was used.
func (r *AuthRepo) MarkPasswordResetTokenUsed(ctx context.Context, tokenID uuid.UUID, when time.Time) error {
	res, err := r.execContext(ctx,
		`UPDATE password_reset_tokens SET used_at = ? WHERE id = ?`,
		when,
		tokenID.String(),
	)
	if err != nil {
		return fmt.Errorf("marking password reset token used: %w", err)
	}
	if err := requireRowsAffected(res, domain.ErrPasswordResetTokenNotFound); err != nil {
		return fmt.Errorf("marking password reset token used: %w", err)
	}
	return nil
}

// CompletePasswordReset atomically consumes a valid reset token, updates the user's password, and revokes sessions.
func (r *AuthRepo) CompletePasswordReset(ctx context.Context, tokenHash string, passwordHash string, when time.Time) (*domain.PasswordResetToken, error) {
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning complete password reset transaction: %w", err)
	}
	defer tx.Rollback()

	token, err := r.scanPasswordResetToken(tx.QueryRowContext(ctx, rebindQuery(
		`SELECT id, user_id, token_hash, created_at, expires_at, used_at, created_by
		 FROM password_reset_tokens
		 WHERE token_hash = ?
		 FOR UPDATE`),
		tokenHash,
	))
	if err != nil {
		return nil, fmt.Errorf("completing password reset token lookup: %w", err)
	}
	if token.UsedAt != nil || !token.ExpiresAt.After(when) {
		return nil, domain.ErrPasswordResetTokenExpired
	}

	res, err := tx.ExecContext(ctx, rebindQuery(
		`UPDATE users SET
			password_hash = ?,
			must_change_password = ?,
			updated_at = ?,
			password_changed_at = ?,
			failed_login_attempts = ?,
			locked_until = NULL
		WHERE id = ?`),
		passwordHash,
		false,
		when,
		when,
		0,
		token.UserID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("updating password reset user: %w", err)
	}
	if err := requireRowsAffected(res, domain.ErrAuthUserNotFound); err != nil {
		return nil, fmt.Errorf("updating password reset user: %w", err)
	}

	res, err = tx.ExecContext(ctx, rebindQuery(
		`UPDATE password_reset_tokens
		 SET used_at = ?
		 WHERE id = ? AND used_at IS NULL`),
		when,
		token.ID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("marking password reset token used: %w", err)
	}
	if err := requireRowsAffected(res, domain.ErrPasswordResetTokenExpired); err != nil {
		return nil, fmt.Errorf("marking password reset token used: %w", err)
	}

	if _, err := tx.ExecContext(ctx, rebindQuery(
		`UPDATE auth_sessions SET revoked_at = ? WHERE user_id = ?`),
		when,
		token.UserID.String(),
	); err != nil {
		return nil, fmt.Errorf("revoking auth sessions after password reset: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing complete password reset transaction: %w", err)
	}
	token.UsedAt = &when
	return token, nil
}

// AppendAuditLog inserts an audit log.
func (r *AuthRepo) AppendAuditLog(ctx context.Context, log *domain.AuditLog) error {
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	if log.MetadataJSON == "" {
		log.MetadataJSON = "{}"
	}

	_, err := r.execContext(ctx,
		`INSERT INTO audit_logs (
			id, actor_user_id, target_user_id, action, resource, resource_id,
			metadata_json, ip_address, user_agent, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ID.String(),
		uuidPtrString(log.ActorUserID),
		uuidPtrString(log.TargetUserID),
		log.Action,
		log.Resource,
		log.ResourceID,
		log.MetadataJSON,
		log.IPAddress,
		log.UserAgent,
		log.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("appending auth audit log: %w", err)
	}
	return nil
}

// ListAuditLogs returns audit logs in reverse chronological order.
func (r *AuthRepo) ListAuditLogs(ctx context.Context, filter domain.AuditLogFilter) ([]domain.AuditLog, error) {
	query := `SELECT id, actor_user_id, target_user_id, action, resource, resource_id,
	                 metadata_json, ip_address, user_agent, created_at
	          FROM audit_logs`
	var conditions []string
	var args []interface{}
	if filter.ActorUserID != nil {
		conditions = append(conditions, "actor_user_id = ?")
		args = append(args, filter.ActorUserID.String())
	}
	if filter.TargetUserID != nil {
		conditions = append(conditions, "target_user_id = ?")
		args = append(args, filter.TargetUserID.String())
	}
	if filter.Action != "" {
		conditions = append(conditions, "action = ?")
		args = append(args, filter.Action)
	}
	if filter.Resource != "" {
		conditions = append(conditions, "resource = ?")
		args = append(args, filter.Resource)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC, id DESC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := r.queryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing auth audit logs: %w", err)
	}
	defer rows.Close()

	var logs []domain.AuditLog
	for rows.Next() {
		log, err := scanAuditLogRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning auth audit log: %w", err)
		}
		logs = append(logs, *log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating auth audit logs: %w", err)
	}
	return logs, nil
}

// DashboardStats returns high-level auth dashboard counters.
func (r *AuthRepo) DashboardStats(ctx context.Context) (*domain.AdminDashboardStats, error) {
	var stats domain.AdminDashboardStats
	err := r.queryRowContext(ctx,
		`SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = ?),
			COUNT(*) FILTER (WHERE status = ?),
			COUNT(*) FILTER (WHERE status = ?),
			COUNT(*) FILTER (WHERE last_login_at >= NOW() - INTERVAL '24 hours')
		 FROM users`,
		string(domain.UserStatusActive),
		string(domain.UserStatusDisabled),
		string(domain.UserStatusLocked),
	).Scan(
		&stats.TotalUsers,
		&stats.ActiveUsers,
		&stats.DisabledUsers,
		&stats.LockedUsers,
		&stats.RecentLogins,
	)
	if err != nil {
		return nil, fmt.Errorf("loading auth dashboard user stats: %w", err)
	}
	if err := r.queryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM audit_logs
		 WHERE action = ? AND created_at >= NOW() - INTERVAL '24 hours'`,
		"auth.login_failed",
	).Scan(&stats.RecentFailedLoginAttempts); err != nil {
		return nil, fmt.Errorf("loading auth dashboard failed login stats: %w", err)
	}
	return &stats, nil
}

type authRowScanner interface {
	Scan(dest ...interface{}) error
}

func userSelectSQL() string {
	return `SELECT id, username, username_normalized, email, email_normalized, password_hash,
	               display_name, status, must_change_password, created_at, updated_at,
	               last_login_at, password_changed_at, failed_login_attempts, locked_until,
	               created_by, updated_by
	        FROM users`
}

func (r *AuthRepo) userRolesAndPermissions(ctx context.Context, user domain.User) (*domain.UserWithRolesAndPermissions, error) {
	roles, err := r.rolesForUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	permissions, err := r.permissionsForUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return &domain.UserWithRolesAndPermissions{
		User:        user,
		Roles:       roles,
		Permissions: permissions,
	}, nil
}

func (r *AuthRepo) rolesForUser(ctx context.Context, userID uuid.UUID) ([]domain.Role, error) {
	rows, err := r.queryContext(ctx,
		`SELECT r.id, r.name, r.description, r.is_system_role, r.created_at, r.updated_at
		 FROM roles r
		 JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = ?
		 ORDER BY r.name ASC`,
		userID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("listing auth user roles: %w", err)
	}
	defer rows.Close()

	var roles []domain.Role
	for rows.Next() {
		role, err := scanRoleRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning auth user role: %w", err)
		}
		roles = append(roles, *role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating auth user roles: %w", err)
	}
	return roles, nil
}

func (r *AuthRepo) permissionsForUser(ctx context.Context, userID uuid.UUID) ([]domain.Permission, error) {
	rows, err := r.queryContext(ctx,
		`SELECT DISTINCT p.id, p.key, p.description, p.resource, p.action
		 FROM permissions p
		 JOIN role_permissions rp ON rp.permission_id = p.id
		 JOIN user_roles ur ON ur.role_id = rp.role_id
		 WHERE ur.user_id = ?
		 ORDER BY p.key ASC`,
		userID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("listing auth user permissions: %w", err)
	}
	defer rows.Close()

	var permissions []domain.Permission
	for rows.Next() {
		permission, err := scanPermissionRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning auth user permission: %w", err)
		}
		permissions = append(permissions, *permission)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating auth user permissions: %w", err)
	}
	return permissions, nil
}

func (r *AuthRepo) scanUser(row authRowScanner) (*domain.User, error) {
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrAuthUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func (r *AuthRepo) scanUserRows(rows *sql.Rows) (*domain.User, error) {
	return scanUser(rows)
}

func scanUser(row authRowScanner) (*domain.User, error) {
	var user domain.User
	var id string
	var status string
	var createdBy sql.NullString
	var updatedBy sql.NullString
	err := row.Scan(
		&id,
		&user.Username,
		&user.UsernameNormalized,
		&user.Email,
		&user.EmailNormalized,
		&user.PasswordHash,
		&user.DisplayName,
		&status,
		&user.MustChangePassword,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
		&user.PasswordChangedAt,
		&user.FailedLoginAttempts,
		&user.LockedUntil,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parsing auth user id: %w", err)
	}
	user.ID = parsedID
	user.Status = domain.UserStatus(status)
	user.CreatedBy, err = parseNullUUID(createdBy)
	if err != nil {
		return nil, fmt.Errorf("parsing auth user created_by: %w", err)
	}
	user.UpdatedBy, err = parseNullUUID(updatedBy)
	if err != nil {
		return nil, fmt.Errorf("parsing auth user updated_by: %w", err)
	}
	return &user, nil
}

func scanRole(row authRowScanner) (*domain.Role, error) {
	role, err := scanRoleValue(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrAuthRoleNotFound
		}
		return nil, err
	}
	return role, nil
}

func scanRoleRows(rows *sql.Rows) (*domain.Role, error) {
	return scanRoleValue(rows)
}

func scanRoleValue(row authRowScanner) (*domain.Role, error) {
	var role domain.Role
	if err := row.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystemRole, &role.CreatedAt, &role.UpdatedAt); err != nil {
		return nil, err
	}
	return &role, nil
}

func scanPermissionRows(rows *sql.Rows) (*domain.Permission, error) {
	var permission domain.Permission
	if err := rows.Scan(&permission.ID, &permission.Key, &permission.Description, &permission.Resource, &permission.Action); err != nil {
		return nil, err
	}
	return &permission, nil
}

func (r *AuthRepo) scanSession(row authRowScanner) (*domain.AuthSession, error) {
	var session domain.AuthSession
	var id string
	var userID string
	err := row.Scan(
		&id,
		&userID,
		&session.TokenHash,
		&session.CSRFTokenHash,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.LastSeenAt,
		&session.IPAddress,
		&session.UserAgent,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrAuthSessionNotFound
		}
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parsing auth session id: %w", err)
	}
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parsing auth session user id: %w", err)
	}
	session.ID = parsedID
	session.UserID = parsedUserID
	return &session, nil
}

func (r *AuthRepo) scanPasswordResetToken(row authRowScanner) (*domain.PasswordResetToken, error) {
	var token domain.PasswordResetToken
	var id string
	var userID string
	var createdBy sql.NullString
	err := row.Scan(&id, &userID, &token.TokenHash, &token.CreatedAt, &token.ExpiresAt, &token.UsedAt, &createdBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrPasswordResetTokenNotFound
		}
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parsing password reset token id: %w", err)
	}
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parsing password reset token user id: %w", err)
	}
	token.ID = parsedID
	token.UserID = parsedUserID
	token.CreatedBy, err = parseNullUUID(createdBy)
	if err != nil {
		return nil, fmt.Errorf("parsing password reset token created_by: %w", err)
	}
	return &token, nil
}

func scanAuditLogRows(rows *sql.Rows) (*domain.AuditLog, error) {
	var log domain.AuditLog
	var id string
	var actorUserID sql.NullString
	var targetUserID sql.NullString
	err := rows.Scan(
		&id,
		&actorUserID,
		&targetUserID,
		&log.Action,
		&log.Resource,
		&log.ResourceID,
		&log.MetadataJSON,
		&log.IPAddress,
		&log.UserAgent,
		&log.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parsing auth audit log id: %w", err)
	}
	log.ID = parsedID
	log.ActorUserID, err = parseNullUUID(actorUserID)
	if err != nil {
		return nil, fmt.Errorf("parsing auth audit log actor_user_id: %w", err)
	}
	log.TargetUserID, err = parseNullUUID(targetUserID)
	if err != nil {
		return nil, fmt.Errorf("parsing auth audit log target_user_id: %w", err)
	}
	return &log, nil
}

func parseNullUUID(value sql.NullString) (*uuid.UUID, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func uuidPtrString(value *uuid.UUID) interface{} {
	if value == nil {
		return nil
	}
	return value.String()
}

func requireRowsAffected(result sql.Result, notFoundErr error) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return notFoundErr
	}
	return nil
}

func lockAuthSuperAdminMutation(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, rebindQuery(`SELECT pg_advisory_xact_lock(?)`), authSuperAdminMutationLockID)
	return err
}

func txUserIsActiveSuperAdmin(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, rebindQuery(
		`SELECT EXISTS (
			SELECT 1
			FROM users u
			JOIN user_roles ur ON ur.user_id = u.id
			WHERE u.id = ? AND u.status = ? AND ur.role_id = ?
		)`),
		userID.String(),
		string(domain.UserStatusActive),
		domain.RoleSuperAdmin,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func txCountActiveSuperAdmins(ctx context.Context, tx *sql.Tx) (int, error) {
	var count int
	err := tx.QueryRowContext(ctx, rebindQuery(
		`SELECT COUNT(DISTINCT u.id)
		 FROM users u
		 JOIN user_roles ur ON ur.user_id = u.id
		 WHERE u.status = ? AND ur.role_id = ?`),
		string(domain.UserStatusActive),
		domain.RoleSuperAdmin,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *AuthRepo) execContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return r.db.raw.ExecContext(ctx, rebindQuery(query), args...)
}

func (r *AuthRepo) queryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return r.db.raw.QueryContext(ctx, rebindQuery(query), args...)
}

func (r *AuthRepo) queryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return r.db.raw.QueryRowContext(ctx, rebindQuery(query), args...)
}
