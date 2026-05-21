package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// UserStatus describes whether a user can authenticate and use the system.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusPending  UserStatus = "pending"
	UserStatusLocked   UserStatus = "locked"
)

const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleManager    = "manager"
	RoleUser       = "user"
	RoleViewer     = "viewer"
)

const (
	PermissionUsersRead         = "users:read"
	PermissionUsersCreate       = "users:create"
	PermissionUsersUpdate       = "users:update"
	PermissionUsersDisable      = "users:disable"
	PermissionUsersDelete       = "users:delete"
	PermissionRolesRead         = "roles:read"
	PermissionRolesAssign       = "roles:assign"
	PermissionRolesUpdate       = "roles:update"
	PermissionAdminDashboard    = "admin:dashboard:read"
	PermissionAuditLogsRead     = "audit_logs:read"
	PermissionSettingsRead      = "settings:read"
	PermissionSettingsUpdate    = "settings:update"
	PermissionTopologyRead      = "topology:read"
	PermissionTopologyUpdate    = "topology:update"
	PermissionDevicesRead       = "devices:read"
	PermissionDevicesCreate     = "devices:create"
	PermissionDevicesUpdate     = "devices:update"
	PermissionDevicesDelete     = "devices:delete"
	PermissionCredentialsRead   = "credentials:read"
	PermissionCredentialsUpdate = "credentials:update"
	PermissionCredentialsReveal = "credentials:reveal"
	PermissionBackupsRead       = "backups:read"
	PermissionBackupsUpdate     = "backups:update"
	PermissionBridgeTokenCreate = "bridge:token:create"
)

var (
	// ErrAuthUserNotFound indicates that an auth user lookup or update target does not exist.
	ErrAuthUserNotFound = errors.New("auth user not found")
	// ErrAuthRoleNotFound indicates that an auth role lookup target does not exist.
	ErrAuthRoleNotFound = errors.New("auth role not found")
	// ErrAuthSessionNotFound indicates that an auth session lookup or update target does not exist.
	ErrAuthSessionNotFound = errors.New("auth session not found")
	// ErrPasswordResetTokenNotFound indicates that a password reset token lookup or update target does not exist.
	ErrPasswordResetTokenNotFound = errors.New("password reset token not found")
)

// SystemPermission describes a built-in RBAC permission.
type SystemPermission struct {
	Key         string
	Description string
	Resource    string
	Action      string
}

// User is the repository model for an authenticated system user.
//
// PasswordHash is needed at the repository/service boundary for authentication
// checks. It must not be exposed by API response DTOs.
type User struct {
	ID                  uuid.UUID
	Username            string
	UsernameNormalized  string
	Email               string
	EmailNormalized     string
	PasswordHash        string
	DisplayName         string
	Status              UserStatus
	MustChangePassword  bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
	LastLoginAt         *time.Time
	PasswordChangedAt   *time.Time
	FailedLoginAttempts int
	LockedUntil         *time.Time
	CreatedBy           *uuid.UUID
	UpdatedBy           *uuid.UUID
}

// Role describes an RBAC role.
type Role struct {
	ID           string
	Name         string
	Description  string
	IsSystemRole bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Permission describes an RBAC permission.
type Permission struct {
	ID          string
	Key         string
	Description string
	Resource    string
	Action      string
}

// UserRole records a role assignment for a user.
type UserRole struct {
	UserID    uuid.UUID
	RoleID    string
	CreatedAt time.Time
	CreatedBy *uuid.UUID
}

// AuthSession stores a server-side authentication session.
type AuthSession struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	TokenHash     string
	CSRFTokenHash string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	LastSeenAt    *time.Time
	IPAddress     string
	UserAgent     string
}

// PasswordResetToken stores a single password reset token hash and lifecycle.
type PasswordResetToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedBy *uuid.UUID
}

// AuditLog records an authentication or authorization administration event.
type AuditLog struct {
	ID           uuid.UUID
	ActorUserID  *uuid.UUID
	TargetUserID *uuid.UUID
	Action       string
	Resource     string
	ResourceID   string
	MetadataJSON string
	IPAddress    string
	UserAgent    string
	CreatedAt    time.Time
}

// UserWithRolesAndPermissions aggregates a user with its effective RBAC grants.
type UserWithRolesAndPermissions struct {
	User        User
	Roles       []Role
	Permissions []Permission
}

// HasRole returns true when the aggregate includes roleID.
func (u UserWithRolesAndPermissions) HasRole(roleID string) bool {
	for _, role := range u.Roles {
		if role.ID == roleID || role.Name == roleID {
			return true
		}
	}
	return false
}

// HasPermission returns true when the aggregate includes permissionKey.
func (u UserWithRolesAndPermissions) HasPermission(permissionKey string) bool {
	for _, permission := range u.Permissions {
		if permission.Key == permissionKey || permission.ID == permissionKey {
			return true
		}
	}
	return false
}

// AdminDashboardStats contains high-level auth administration counters.
type AdminDashboardStats struct {
	TotalUsers                int
	ActiveUsers               int
	DisabledUsers             int
	LockedUsers               int
	RecentLogins              int
	RecentFailedLoginAttempts int
}

// UserListFilter filters user administration lists.
type UserListFilter struct {
	Status UserStatus
	Query  string
	RoleID string
	Limit  int
	Offset int
}

// AuditLogFilter filters audit log administration lists.
type AuditLogFilter struct {
	ActorUserID  *uuid.UUID
	TargetUserID *uuid.UUID
	Action       string
	Resource     string
	Limit        int
	Offset       int
}

// UserRepository persists users and user aggregates.
type UserRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetUserByLoginIdentifier(ctx context.Context, normalized string) (*User, error)
	ListUsers(ctx context.Context, filter UserListFilter) ([]UserWithRolesAndPermissions, error)
	UpdateUser(ctx context.Context, user *User) error
	CountUsers(ctx context.Context) (int, error)
	CountActiveSuperAdmins(ctx context.Context) (int, error)
	GetUserRolesAndPermissions(ctx context.Context, userID uuid.UUID) (*UserWithRolesAndPermissions, error)
}

// RoleRepository persists roles and role assignments.
type RoleRepository interface {
	ListRoles(ctx context.Context) ([]Role, error)
	ListPermissions(ctx context.Context) ([]Permission, error)
	GetRoleByName(ctx context.Context, name string) (*Role, error)
	AssignRole(ctx context.Context, userID uuid.UUID, roleID string, createdBy *uuid.UUID) error
	RemoveRole(ctx context.Context, userID uuid.UUID, roleID string) error
}

// SessionRepository persists authentication sessions.
type SessionRepository interface {
	CreateSession(ctx context.Context, session *AuthSession) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*AuthSession, error)
	RevokeSession(ctx context.Context, sessionID uuid.UUID, when time.Time) error
	RevokeUserSessions(ctx context.Context, userID uuid.UUID, exceptSessionID *uuid.UUID, when time.Time) error
	TouchSession(ctx context.Context, sessionID uuid.UUID, when time.Time) error
}

// PasswordResetRepository persists password reset tokens.
type PasswordResetRepository interface {
	CreatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error
	GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	MarkPasswordResetTokenUsed(ctx context.Context, tokenID uuid.UUID, when time.Time) error
}

// AuditLogRepository persists auth audit logs and derived stats.
type AuditLogRepository interface {
	AppendAuditLog(ctx context.Context, log *AuditLog) error
	ListAuditLogs(ctx context.Context, filter AuditLogFilter) ([]AuditLog, error)
	DashboardStats(ctx context.Context) (*AdminDashboardStats, error)
}

// SystemRoleNames returns built-in role names in deterministic order.
func SystemRoleNames() []string {
	return []string{
		RoleSuperAdmin,
		RoleAdmin,
		RoleManager,
		RoleUser,
		RoleViewer,
	}
}

// SystemPermissions returns built-in permissions in deterministic order.
func SystemPermissions() []SystemPermission {
	return []SystemPermission{
		{Key: PermissionUsersRead, Description: "Read users", Resource: "users", Action: "read"},
		{Key: PermissionUsersCreate, Description: "Create users", Resource: "users", Action: "create"},
		{Key: PermissionUsersUpdate, Description: "Update users", Resource: "users", Action: "update"},
		{Key: PermissionUsersDisable, Description: "Disable users", Resource: "users", Action: "disable"},
		{Key: PermissionUsersDelete, Description: "Delete users", Resource: "users", Action: "delete"},
		{Key: PermissionRolesRead, Description: "Read roles", Resource: "roles", Action: "read"},
		{Key: PermissionRolesAssign, Description: "Assign roles", Resource: "roles", Action: "assign"},
		{Key: PermissionRolesUpdate, Description: "Update roles", Resource: "roles", Action: "update"},
		{Key: PermissionAdminDashboard, Description: "Read admin dashboard", Resource: "admin:dashboard", Action: "read"},
		{Key: PermissionAuditLogsRead, Description: "Read audit logs", Resource: "audit_logs", Action: "read"},
		{Key: PermissionSettingsRead, Description: "Read settings", Resource: "settings", Action: "read"},
		{Key: PermissionSettingsUpdate, Description: "Update settings", Resource: "settings", Action: "update"},
		{Key: PermissionTopologyRead, Description: "Read topology", Resource: "topology", Action: "read"},
		{Key: PermissionTopologyUpdate, Description: "Update topology", Resource: "topology", Action: "update"},
		{Key: PermissionDevicesRead, Description: "Read devices", Resource: "devices", Action: "read"},
		{Key: PermissionDevicesCreate, Description: "Create devices", Resource: "devices", Action: "create"},
		{Key: PermissionDevicesUpdate, Description: "Update devices", Resource: "devices", Action: "update"},
		{Key: PermissionDevicesDelete, Description: "Delete devices", Resource: "devices", Action: "delete"},
		{Key: PermissionCredentialsRead, Description: "Read credential metadata", Resource: "credentials", Action: "read"},
		{Key: PermissionCredentialsUpdate, Description: "Update credentials", Resource: "credentials", Action: "update"},
		{Key: PermissionCredentialsReveal, Description: "Reveal credentials", Resource: "credentials", Action: "reveal"},
		{Key: PermissionBackupsRead, Description: "Read backups", Resource: "backups", Action: "read"},
		{Key: PermissionBackupsUpdate, Description: "Update backups", Resource: "backups", Action: "update"},
		{Key: PermissionBridgeTokenCreate, Description: "Create bridge tokens", Resource: "bridge:token", Action: "create"},
	}
}

// SystemRolePermissionKeys returns permission keys assigned to a built-in role.
func SystemRolePermissionKeys(roleName string) []string {
	switch roleName {
	case RoleSuperAdmin:
		return systemPermissionKeys()
	case RoleAdmin:
		return []string{
			PermissionUsersRead,
			PermissionUsersCreate,
			PermissionUsersUpdate,
			PermissionUsersDisable,
			PermissionRolesRead,
			PermissionRolesAssign,
			PermissionAdminDashboard,
			PermissionAuditLogsRead,
			PermissionSettingsRead,
			PermissionSettingsUpdate,
			PermissionTopologyRead,
			PermissionTopologyUpdate,
			PermissionDevicesRead,
			PermissionDevicesCreate,
			PermissionDevicesUpdate,
			PermissionDevicesDelete,
			PermissionCredentialsRead,
			PermissionCredentialsUpdate,
			PermissionBackupsRead,
			PermissionBackupsUpdate,
			PermissionBridgeTokenCreate,
		}
	case RoleManager:
		return []string{
			PermissionAdminDashboard,
			PermissionSettingsRead,
			PermissionTopologyRead,
			PermissionTopologyUpdate,
			PermissionDevicesRead,
			PermissionDevicesCreate,
			PermissionDevicesUpdate,
			PermissionBackupsRead,
			PermissionBackupsUpdate,
			PermissionBridgeTokenCreate,
		}
	case RoleUser:
		return []string{
			PermissionSettingsRead,
			PermissionTopologyRead,
			PermissionTopologyUpdate,
			PermissionDevicesRead,
			PermissionDevicesUpdate,
			PermissionBackupsRead,
		}
	case RoleViewer:
		return []string{
			PermissionSettingsRead,
			PermissionTopologyRead,
			PermissionDevicesRead,
			PermissionBackupsRead,
		}
	default:
		return nil
	}
}

func systemPermissionKeys() []string {
	permissions := SystemPermissions()
	keys := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		keys = append(keys, permission.Key)
	}
	return keys
}
