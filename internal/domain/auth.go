package domain

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

// SystemPermission describes a built-in RBAC permission.
type SystemPermission struct {
	Key         string
	Description string
	Resource    string
	Action      string
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
			PermissionUsersRead,
			PermissionRolesRead,
			PermissionAdminDashboard,
			PermissionSettingsRead,
			PermissionTopologyRead,
			PermissionTopologyUpdate,
			PermissionDevicesRead,
			PermissionDevicesCreate,
			PermissionDevicesUpdate,
			PermissionCredentialsRead,
			PermissionCredentialsUpdate,
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
			PermissionBridgeTokenCreate,
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
