import type { AuthUser } from './auth';
import {
  parseAdminAuditLogsEnvelope,
  parseAdminDashboard,
  parseAdminPasswordResetResponse,
  parseAdminPermissionsEnvelope,
  parseAdminRolesEnvelope,
  parseAdminUserEnvelope,
  parseAdminUsersEnvelope,
} from './adminParsers';
import { requestJSON, requestJSONWithBody } from './transport';

export interface AdminDashboardStats {
  total_users: number;
  active_users: number;
  disabled_users: number;
  locked_users: number;
  recent_logins: number;
  recent_failed_login_attempts: number;
}

export interface AdminAuditLog {
  id: string;
  actor_user_id?: string;
  action: string;
  target_user_id?: string;
  resource?: string;
  resource_id?: string;
  metadata?: Record<string, unknown>;
  ip_address?: string;
  user_agent?: string;
  created_at: string;
}

export interface AdminDashboardResponse {
  stats: AdminDashboardStats;
  recent_audit_logs: AdminAuditLog[];
}

export interface AdminRole {
  id: string;
  name: string;
  description: string;
  is_system_role: boolean;
  permissions: string[];
}

export interface CreateAdminUserPayload {
  username: string;
  password: string;
  email?: string;
  display_name?: string;
  must_change_password?: boolean;
  role_ids?: string[];
}

export interface UpdateAdminUserPayload {
  username?: string;
  email?: string;
  display_name?: string;
  must_change_password?: boolean;
}

export interface AdminPasswordResetResponse {
  reset_token: string;
}

// fetchAdminDashboard loads admin summary counters and recent audit logs.
export async function fetchAdminDashboard(): Promise<AdminDashboardResponse> {
  return parseAdminDashboard(await requestJSON('/api/v1/admin/dashboard'));
}

// fetchAdminUsers loads safe admin user rows without exposing secret-bearing fields.
export async function fetchAdminUsers(): Promise<AuthUser[]> {
  return parseAdminUsersEnvelope(await requestJSON('/api/v1/admin/users'));
}

// fetchAdminRoles loads available roles and their permission keys.
export async function fetchAdminRoles(): Promise<AdminRole[]> {
  return parseAdminRolesEnvelope(await requestJSON('/api/v1/admin/roles'));
}

// fetchAdminPermissions loads global admin permission keys.
export async function fetchAdminPermissions(): Promise<string[]> {
  return parseAdminPermissionsEnvelope(await requestJSON('/api/v1/admin/permissions'));
}

// fetchAdminAuditLogs loads recent admin audit logs.
export async function fetchAdminAuditLogs(): Promise<AdminAuditLog[]> {
  return parseAdminAuditLogsEnvelope(await requestJSON('/api/v1/admin/audit-logs'));
}

// createAdminUser creates an admin-managed user and parses the safe response user.
export async function createAdminUser(payload: CreateAdminUserPayload): Promise<AuthUser> {
  return parseAdminUserEnvelope(await requestJSONWithBody('/api/v1/admin/users', 'POST', payload));
}

// updateAdminUser patches admin-managed user profile fields.
export async function updateAdminUser(
  id: string,
  payload: UpdateAdminUserPayload,
): Promise<AuthUser> {
  return parseAdminUserEnvelope(
    await requestJSONWithBody(`/api/v1/admin/users/${encodeURIComponent(id)}`, 'PATCH', payload),
  );
}

// setAdminUserStatus updates account status and parses the safe response user.
export async function setAdminUserStatus(id: string, status: string): Promise<AuthUser> {
  return parseAdminUserEnvelope(
    await requestJSONWithBody(`/api/v1/admin/users/${encodeURIComponent(id)}/status`, 'PATCH', {
      status,
    }),
  );
}

// assignAdminUserRole assigns one role to a user and parses the safe response user.
export async function assignAdminUserRole(userId: string, roleId: string): Promise<AuthUser> {
  return parseAdminUserEnvelope(
    await requestJSONWithBody(`/api/v1/admin/users/${encodeURIComponent(userId)}/roles`, 'POST', {
      role_id: roleId,
    }),
  );
}

// removeAdminUserRole removes one role assignment from a user.
export async function removeAdminUserRole(userId: string, roleId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/admin/users/${encodeURIComponent(userId)}/roles/${encodeURIComponent(roleId)}`,
    'DELETE',
  );
}

// createAdminPasswordReset creates a reset token while preserving the legacy token fallback.
export async function createAdminPasswordReset(
  userId: string,
): Promise<AdminPasswordResetResponse> {
  return parseAdminPasswordResetResponse(
    await requestJSONWithBody(
      `/api/v1/admin/users/${encodeURIComponent(userId)}/password-reset`,
      'POST',
    ),
  );
}
