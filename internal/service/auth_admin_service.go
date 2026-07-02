package service

// This file defines auth admin service service behavior and domain orchestration rules.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/security"
)

var (
	// ErrAdminInvalidInput is returned for malformed admin workflow input.
	ErrAdminInvalidInput = errors.New("invalid admin input")
	// ErrAdminLastActiveSuperAdmin is returned when an operation would remove the final active super-admin.
	ErrAdminLastActiveSuperAdmin = errors.New("cannot remove the last active super admin")
)

// AdminDashboardResult contains dashboard counters and recent audit activity.
type AdminDashboardResult struct {
	Stats           *domain.AdminDashboardStats
	RecentAuditLogs []domain.AuditLog
}

// AdminCreateUserInput contains admin-supplied user creation fields.
type AdminCreateUserInput struct {
	Username           string
	Email              string
	DisplayName        string
	Password           string
	MustChangePassword bool
	Roles              []string
}

// AdminUpdateUserInput contains safe mutable user profile fields.
type AdminUpdateUserInput struct {
	UserID      uuid.UUID
	Username    *string
	Email       *string
	DisplayName *string
	Status      *domain.UserStatus
}

// AdminUserStatusInput contains a status transition request.
type AdminUserStatusInput struct {
	UserID uuid.UUID
	Status domain.UserStatus
}

// AdminUserRoleInput identifies a user role mutation.
type AdminUserRoleInput struct {
	UserID uuid.UUID
	RoleID string
}

// AdminRolePermissionsInput contains a full replacement permission set for one role.
type AdminRolePermissionsInput struct {
	RoleID      string
	Permissions []string
}

// AdminRole contains a role with the permissions currently known for that role.
type AdminRole struct {
	Role           domain.Role
	PermissionKeys []string
}

// AdminDashboard returns overview stats and recent audit activity.
func (s *AuthService) AdminDashboard(ctx context.Context, actor *AuthenticatedUser) (*AdminDashboardResult, error) {
	if err := s.RequirePermission(actor, domain.PermissionAdminDashboard); err != nil {
		return nil, err
	}
	stats, err := s.auditLogs.DashboardStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading admin dashboard stats: %w", err)
	}
	recent, err := s.auditLogs.ListAuditLogs(ctx, domain.AuditLogFilter{Limit: 10})
	if err != nil {
		return nil, fmt.Errorf("loading admin dashboard audit logs: %w", err)
	}
	return &AdminDashboardResult{Stats: stats, RecentAuditLogs: recent}, nil
}

// ListAdminUsers returns users visible to user administrators.
func (s *AuthService) ListAdminUsers(ctx context.Context, actor *AuthenticatedUser, filter domain.UserListFilter) ([]domain.UserWithRolesAndPermissions, error) {
	if err := s.RequirePermission(actor, domain.PermissionUsersRead); err != nil {
		return nil, err
	}
	return s.users.ListUsers(ctx, normalizeUserListFilter(filter))
}

// CreateAdminUser creates an active local user and assigns requested roles.
func (s *AuthService) CreateAdminUser(ctx context.Context, actor *AuthenticatedUser, input AdminCreateUserInput) (*domain.UserWithRolesAndPermissions, error) {
	if err := s.RequirePermission(actor, domain.PermissionUsersCreate); err != nil {
		return nil, err
	}
	if len(input.Roles) > 0 {
		if err := s.RequirePermission(actor, domain.PermissionRolesAssign); err != nil {
			return nil, err
		}
	}
	username := strings.TrimSpace(input.Username)
	email := strings.TrimSpace(input.Email)
	displayName := strings.TrimSpace(input.DisplayName)
	if username == "" || email == "" || strings.TrimSpace(input.Password) == "" {
		return nil, ErrAdminInvalidInput
	}
	if !input.MustChangePassword {
		if err := security.ValidatePasswordPolicy(input.Password); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPasswordPolicyViolation, err)
		}
	}
	roleIDs, err := s.validateAdminRoleIDs(ctx, actor, input.Roles)
	if err != nil {
		return nil, err
	}
	passwordHash, err := security.HashPassword(input.Password)
	if err != nil {
		return nil, fmt.Errorf("hashing admin-created user password: %w", err)
	}
	now := s.now()
	actorID := actorUserID(actor)
	user := domain.User{
		ID:                 uuid.New(),
		Username:           username,
		UsernameNormalized: normalizeLoginIdentifier(username),
		Email:              email,
		EmailNormalized:    normalizeLoginIdentifier(email),
		PasswordHash:       passwordHash,
		DisplayName:        displayName,
		Status:             domain.UserStatusActive,
		MustChangePassword: input.MustChangePassword,
		CreatedAt:          now,
		UpdatedAt:          now,
		CreatedBy:          actorID,
		UpdatedBy:          actorID,
	}
	if err := s.users.CreateUser(ctx, &user); err != nil {
		return nil, fmt.Errorf("creating admin user: %w", err)
	}
	for _, roleID := range roleIDs {
		if err := s.roles.AssignRole(ctx, user.ID, roleID, actorID); err != nil {
			return nil, fmt.Errorf("assigning admin-created user role: %w", err)
		}
	}
	if err := s.appendAuditLog(ctx, actorID, &user.ID, "admin.user_created", "auth_user", user.ID.String(), auditMetadata(map[string]interface{}{
		"roles": roleIDs,
	})); err != nil {
		return nil, err
	}
	aggregate, err := s.users.GetUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("loading created admin user: %w", err)
	}
	return aggregate, nil
}

// GetAdminUser returns a single user aggregate.
func (s *AuthService) GetAdminUser(ctx context.Context, actor *AuthenticatedUser, userID uuid.UUID) (*domain.UserWithRolesAndPermissions, error) {
	if err := s.RequirePermission(actor, domain.PermissionUsersRead); err != nil {
		return nil, err
	}
	aggregate, err := s.users.GetUserRolesAndPermissions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("loading admin user: %w", err)
	}
	return aggregate, nil
}

// UpdateAdminUser applies safe profile fields and optional status.
func (s *AuthService) UpdateAdminUser(ctx context.Context, actor *AuthenticatedUser, input AdminUpdateUserInput) (*domain.UserWithRolesAndPermissions, error) {
	if err := s.RequirePermission(actor, domain.PermissionUsersUpdate); err != nil {
		return nil, err
	}
	target, err := s.users.GetUserRolesAndPermissions(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("loading admin update user: %w", err)
	}
	if target.HasRole(domain.RoleSuperAdmin) && !actorIsSuperAdmin(actor) && adminUpdateTouchesProfile(input) {
		return nil, ErrPermissionDenied
	}
	user := target.User
	changed := make([]string, 0, 4)
	if input.Username != nil {
		username := strings.TrimSpace(*input.Username)
		if username == "" {
			return nil, ErrAdminInvalidInput
		}
		user.Username = username
		user.UsernameNormalized = normalizeLoginIdentifier(username)
		changed = append(changed, "username")
	}
	if input.Email != nil {
		email := strings.TrimSpace(*input.Email)
		if email == "" {
			return nil, ErrAdminInvalidInput
		}
		user.Email = email
		user.EmailNormalized = normalizeLoginIdentifier(email)
		changed = append(changed, "email")
	}
	if input.DisplayName != nil {
		user.DisplayName = strings.TrimSpace(*input.DisplayName)
		changed = append(changed, "display_name")
	}
	if input.Status != nil {
		if err := s.canChangeAdminUserStatus(ctx, actor, target, *input.Status); err != nil {
			return nil, err
		}
		user.Status = *input.Status
		changed = append(changed, "status")
	}
	if len(changed) == 0 {
		return target, nil
	}
	user.UpdatedAt = s.now()
	user.UpdatedBy = actorUserID(actor)
	if err := s.updateAdminUser(ctx, target, &user); err != nil {
		return nil, err
	}
	if err := s.appendAuditLog(ctx, actorUserID(actor), &user.ID, "admin.user_updated", "auth_user", user.ID.String(), auditMetadata(map[string]interface{}{
		"fields": changed,
	})); err != nil {
		return nil, err
	}
	updated, err := s.users.GetUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("loading updated admin user: %w", err)
	}
	return updated, nil
}

// SetAdminUserStatus changes a user's authentication status.
func (s *AuthService) SetAdminUserStatus(ctx context.Context, actor *AuthenticatedUser, input AdminUserStatusInput) (*domain.UserWithRolesAndPermissions, error) {
	if err := s.RequirePermission(actor, domain.PermissionUsersUpdate); err != nil {
		return nil, err
	}
	if input.Status != domain.UserStatusActive {
		if err := s.RequirePermission(actor, domain.PermissionUsersDisable); err != nil {
			return nil, err
		}
	}
	target, err := s.users.GetUserRolesAndPermissions(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("loading admin status user: %w", err)
	}
	if err := s.canChangeAdminUserStatus(ctx, actor, target, input.Status); err != nil {
		return nil, err
	}
	user := target.User
	user.Status = input.Status
	user.UpdatedAt = s.now()
	user.UpdatedBy = actorUserID(actor)
	if err := s.updateAdminUser(ctx, target, &user); err != nil {
		return nil, err
	}
	if err := s.appendAuditLog(ctx, actorUserID(actor), &user.ID, "admin.user_status_changed", "auth_user", user.ID.String(), auditMetadata(map[string]interface{}{
		"status": string(input.Status),
	})); err != nil {
		return nil, err
	}
	updated, err := s.users.GetUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("loading status-updated admin user: %w", err)
	}
	return updated, nil
}

// AssignAdminUserRole assigns one role to a user with escalation guards.
func (s *AuthService) AssignAdminUserRole(ctx context.Context, actor *AuthenticatedUser, input AdminUserRoleInput) (*domain.UserWithRolesAndPermissions, error) {
	if err := s.RequirePermission(actor, domain.PermissionRolesAssign); err != nil {
		return nil, err
	}
	roleID, err := s.validateAdminRoleID(ctx, actor, input.RoleID)
	if err != nil {
		return nil, err
	}
	if actorIsTarget(actor, input.UserID) {
		return nil, ErrPermissionDenied
	}
	target, err := s.users.GetUserRolesAndPermissions(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("loading admin role target: %w", err)
	}
	if target.HasRole(domain.RoleSuperAdmin) && !actorIsSuperAdmin(actor) {
		return nil, ErrPermissionDenied
	}
	if err := s.roles.AssignRole(ctx, input.UserID, roleID, actorUserID(actor)); err != nil {
		return nil, fmt.Errorf("assigning admin user role: %w", err)
	}
	if err := s.appendAuditLog(ctx, actorUserID(actor), &input.UserID, "admin.user_role_assigned", "auth_user", input.UserID.String(), auditMetadata(map[string]interface{}{
		"role_id": roleID,
	})); err != nil {
		return nil, err
	}
	updated, err := s.users.GetUserRolesAndPermissions(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("loading role-assigned admin user: %w", err)
	}
	return updated, nil
}

// RemoveAdminUserRole removes one role from a user with escalation guards.
func (s *AuthService) RemoveAdminUserRole(ctx context.Context, actor *AuthenticatedUser, input AdminUserRoleInput) (*domain.UserWithRolesAndPermissions, error) {
	if err := s.RequirePermission(actor, domain.PermissionRolesAssign); err != nil {
		return nil, err
	}
	roleID, err := s.validateAdminRoleID(ctx, actor, input.RoleID)
	if err != nil {
		return nil, err
	}
	target, err := s.users.GetUserRolesAndPermissions(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("loading admin role target: %w", err)
	}
	if actorIsTarget(actor, input.UserID) {
		if roleID == domain.RoleSuperAdmin && target.User.Status == domain.UserStatusActive && target.HasRole(domain.RoleSuperAdmin) {
			if err := s.ensureNotLastActiveSuperAdmin(ctx); err != nil {
				return nil, err
			}
		}
		return nil, ErrPermissionDenied
	}
	if (roleID == domain.RoleSuperAdmin || target.HasRole(domain.RoleSuperAdmin)) && !actorIsSuperAdmin(actor) {
		return nil, ErrPermissionDenied
	}
	if err := s.removeAdminUserRole(ctx, target, roleID); err != nil {
		return nil, err
	}
	if err := s.appendAuditLog(ctx, actorUserID(actor), &input.UserID, "admin.user_role_removed", "auth_user", input.UserID.String(), auditMetadata(map[string]interface{}{
		"role_id": roleID,
	})); err != nil {
		return nil, err
	}
	updated, err := s.users.GetUserRolesAndPermissions(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("loading role-removed admin user: %w", err)
	}
	return updated, nil
}

// CreateAdminPasswordResetToken creates a reset token for a user administrator.
func (s *AuthService) CreateAdminPasswordResetToken(ctx context.Context, actor *AuthenticatedUser, userID uuid.UUID) (*PasswordResetTokenResult, error) {
	if err := s.RequirePermission(actor, domain.PermissionUsersUpdate); err != nil {
		return nil, err
	}
	if actorIsTarget(actor, userID) {
		return nil, ErrPermissionDenied
	}
	target, err := s.users.GetUserRolesAndPermissions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("loading admin password reset target: %w", err)
	}
	if target.HasRole(domain.RoleSuperAdmin) && !actorIsSuperAdmin(actor) {
		return nil, ErrPermissionDenied
	}
	return s.CreatePasswordResetToken(ctx, PasswordResetCreateInput{
		UserID:    userID,
		CreatedBy: actorUserID(actor),
	})
}

// ListAdminRoles returns roles with persisted permission-key mappings.
func (s *AuthService) ListAdminRoles(ctx context.Context, actor *AuthenticatedUser) ([]AdminRole, error) {
	if err := s.RequirePermission(actor, domain.PermissionRolesRead); err != nil {
		return nil, err
	}
	roles, err := s.roles.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing admin roles: %w", err)
	}
	out := make([]AdminRole, 0, len(roles))
	for _, role := range roles {
		permissions, err := s.roles.ListRolePermissions(ctx, role.ID)
		if err != nil {
			return nil, fmt.Errorf("listing admin role permissions: %w", err)
		}
		out = append(out, AdminRole{
			Role:           role,
			PermissionKeys: permissionKeysFromPermissions(permissions),
		})
	}
	return out, nil
}

// UpdateAdminRolePermissions replaces permissions for an editable role.
func (s *AuthService) UpdateAdminRolePermissions(ctx context.Context, actor *AuthenticatedUser, input AdminRolePermissionsInput) (*AdminRole, error) {
	if err := s.RequirePermission(actor, domain.PermissionRolesUpdate); err != nil {
		return nil, err
	}
	roleID := strings.TrimSpace(input.RoleID)
	if roleID == "" {
		return nil, ErrAdminInvalidInput
	}
	if roleID == domain.RoleSuperAdmin {
		return nil, ErrPermissionDenied
	}
	role, err := s.roleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	permissionKeys, err := s.normalizeAdminRolePermissionKeys(ctx, input.Permissions)
	if err != nil {
		return nil, err
	}
	replacement, err := s.roles.ReplaceRolePermissions(ctx, role.ID, permissionKeys)
	if err != nil {
		return nil, fmt.Errorf("updating admin role permissions: %w", err)
	}
	oldKeys := permissionKeysFromPermissions(replacement.OldPermissions)
	newKeys := permissionKeysFromPermissions(replacement.NewPermissions)
	metadata, err := json.Marshal(map[string]interface{}{
		"added_permissions":   stringSetDifference(newKeys, oldKeys),
		"removed_permissions": stringSetDifference(oldKeys, newKeys),
		"old_permissions":     oldKeys,
		"new_permissions":     newKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding role permission audit metadata: %w", err)
	}
	if err := s.appendAuditLog(ctx, actorUserID(actor), nil, "role.permissions_updated", "role", role.ID, string(metadata)); err != nil {
		return nil, err
	}
	return &AdminRole{Role: *role, PermissionKeys: newKeys}, nil
}

// ListAdminPermissions returns known RBAC permissions.
func (s *AuthService) ListAdminPermissions(ctx context.Context, actor *AuthenticatedUser) ([]domain.Permission, error) {
	if err := s.RequirePermission(actor, domain.PermissionRolesRead); err != nil {
		return nil, err
	}
	permissions, err := s.roles.ListPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing admin permissions: %w", err)
	}
	return permissions, nil
}

// ListAdminAuditLogs returns filtered audit logs.
func (s *AuthService) ListAdminAuditLogs(ctx context.Context, actor *AuthenticatedUser, filter domain.AuditLogFilter) ([]domain.AuditLog, error) {
	if err := s.RequirePermission(actor, domain.PermissionAuditLogsRead); err != nil {
		return nil, err
	}
	return s.auditLogs.ListAuditLogs(ctx, normalizeAuditLogFilter(filter))
}

func (s *AuthService) validateAdminRoleIDs(ctx context.Context, actor *AuthenticatedUser, roleIDs []string) ([]string, error) {
	out := make([]string, 0, len(roleIDs))
	seen := make(map[string]struct{}, len(roleIDs))
	for _, roleID := range roleIDs {
		validated, err := s.validateAdminRoleID(ctx, actor, roleID)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[validated]; ok {
			continue
		}
		seen[validated] = struct{}{}
		out = append(out, validated)
	}
	return out, nil
}

func (s *AuthService) validateAdminRoleID(ctx context.Context, actor *AuthenticatedUser, roleID string) (string, error) {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return "", ErrAdminInvalidInput
	}
	if roleID == domain.RoleSuperAdmin && !actorIsSuperAdmin(actor) {
		return "", ErrPermissionDenied
	}
	roles, err := s.roles.ListRoles(ctx)
	if err != nil {
		return "", fmt.Errorf("listing roles for validation: %w", err)
	}
	for _, role := range roles {
		if role.ID == roleID || role.Name == roleID {
			return role.ID, nil
		}
	}
	return "", domain.ErrAuthRoleNotFound
}

func (s *AuthService) roleByID(ctx context.Context, roleID string) (*domain.Role, error) {
	roles, err := s.roles.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}
	for _, role := range roles {
		if role.ID == roleID {
			return &role, nil
		}
	}
	return nil, domain.ErrAuthRoleNotFound
}

func (s *AuthService) normalizeAdminRolePermissionKeys(ctx context.Context, raw []string) ([]string, error) {
	known, err := s.roles.ListPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing permissions for role update: %w", err)
	}
	knownByKey := make(map[string]struct{}, len(known))
	for _, permission := range known {
		knownByKey[permission.Key] = struct{}{}
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		key := strings.TrimSpace(value)
		if key == "" {
			return nil, ErrAdminInvalidInput
		}
		if _, ok := knownByKey[key]; !ok {
			return nil, ErrAdminInvalidInput
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out, nil
}

func permissionKeysFromPermissions(permissions []domain.Permission) []string {
	keys := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		keys = append(keys, permission.Key)
	}
	return keys
}

func stringSetDifference(left []string, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	out := make([]string, 0)
	for _, value := range left {
		if _, ok := rightSet[value]; !ok {
			out = append(out, value)
		}
	}
	return out
}

func (s *AuthService) canChangeAdminUserStatus(ctx context.Context, actor *AuthenticatedUser, target *domain.UserWithRolesAndPermissions, status domain.UserStatus) error {
	if !validUserStatus(status) {
		return ErrAdminInvalidInput
	}
	if target.HasRole(domain.RoleSuperAdmin) && !actorIsSuperAdmin(actor) {
		return ErrPermissionDenied
	}
	if actorIsTarget(actor, target.User.ID) && status != domain.UserStatusActive {
		if target.User.Status == domain.UserStatusActive && target.HasRole(domain.RoleSuperAdmin) {
			if err := s.ensureNotLastActiveSuperAdmin(ctx); err != nil {
				return err
			}
		}
		return ErrPermissionDenied
	}
	return nil
}

func (s *AuthService) updateAdminUser(ctx context.Context, current *domain.UserWithRolesAndPermissions, next *domain.User) error {
	if current.User.Status == domain.UserStatusActive && next.Status != domain.UserStatusActive && current.HasRole(domain.RoleSuperAdmin) {
		if err := s.users.UpdateUserPreservingLastActiveSuperAdmin(ctx, next); err != nil {
			return mapAdminLastActiveSuperAdminError("updating admin user", err)
		}
		return nil
	}
	if err := s.users.UpdateUser(ctx, next); err != nil {
		return fmt.Errorf("updating admin user: %w", err)
	}
	return nil
}

func (s *AuthService) removeAdminUserRole(ctx context.Context, target *domain.UserWithRolesAndPermissions, roleID string) error {
	if roleID == domain.RoleSuperAdmin && target.User.Status == domain.UserStatusActive && target.HasRole(domain.RoleSuperAdmin) {
		if err := s.roles.RemoveRolePreservingLastActiveSuperAdmin(ctx, target.User.ID, roleID); err != nil {
			return mapAdminLastActiveSuperAdminError("removing admin user role", err)
		}
		return nil
	}
	if err := s.roles.RemoveRole(ctx, target.User.ID, roleID); err != nil {
		return fmt.Errorf("removing admin user role: %w", err)
	}
	return nil
}

func mapAdminLastActiveSuperAdminError(operation string, err error) error {
	if errors.Is(err, domain.ErrAuthLastActiveSuperAdmin) || errors.Is(err, ErrAdminLastActiveSuperAdmin) {
		return ErrAdminLastActiveSuperAdmin
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (s *AuthService) ensureNotLastActiveSuperAdmin(ctx context.Context) error {
	count, err := s.users.CountActiveSuperAdmins(ctx)
	if err != nil {
		return fmt.Errorf("counting active super admins: %w", err)
	}
	if count <= 1 {
		return ErrAdminLastActiveSuperAdmin
	}
	return nil
}

func adminUpdateTouchesProfile(input AdminUpdateUserInput) bool {
	return input.Username != nil || input.Email != nil || input.DisplayName != nil
}

func actorIsSuperAdmin(actor *AuthenticatedUser) bool {
	return actor != nil && actor.HasRole(domain.RoleSuperAdmin)
}

func actorIsTarget(actor *AuthenticatedUser, targetUserID uuid.UUID) bool {
	return actor != nil && actor.User.User.ID == targetUserID
}

func actorUserID(actor *AuthenticatedUser) *uuid.UUID {
	if actor == nil || actor.User.User.ID == uuid.Nil {
		return nil
	}
	id := actor.User.User.ID
	return &id
}

func validUserStatus(status domain.UserStatus) bool {
	switch status {
	case domain.UserStatusActive, domain.UserStatusDisabled, domain.UserStatusPending, domain.UserStatusLocked:
		return true
	default:
		return false
	}
}

func normalizeUserListFilter(filter domain.UserListFilter) domain.UserListFilter {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.RoleID = strings.TrimSpace(filter.RoleID)
	filter.Limit = normalizeLimit(filter.Limit)
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter
}

func normalizeAuditLogFilter(filter domain.AuditLogFilter) domain.AuditLogFilter {
	filter.Action = strings.TrimSpace(filter.Action)
	filter.Resource = strings.TrimSpace(filter.Resource)
	filter.Limit = normalizeLimit(filter.Limit)
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func auditMetadata(values map[string]interface{}) string {
	if len(values) == 0 {
		return `{}`
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return `{}`
	}
	return string(encoded)
}
