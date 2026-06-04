import { parseAuthUser, type AuthUser } from './auth';
import { permissionKeysArray, recordField, stringField } from './parsers';
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

function parseAdminAuditLog(value: unknown): AdminAuditLog {
  const record =
    typeof value === 'object' && value !== null ? (value as Record<string, unknown>) : {};
  return {
    id: stringField(record, 'id'),
    actor_user_id: stringField(record, 'actor_user_id') || undefined,
    action: stringField(record, 'action'),
    target_user_id: stringField(record, 'target_user_id') || undefined,
    resource: stringField(record, 'resource') || undefined,
    resource_id: stringField(record, 'resource_id') || undefined,
    metadata: recordField(record.metadata),
    ip_address: stringField(record, 'ip_address') || undefined,
    user_agent: stringField(record, 'user_agent') || undefined,
    created_at: stringField(record, 'created_at'),
  };
}

function parseAdminDashboard(payload: unknown): AdminDashboardResponse {
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const stats =
    typeof record.stats === 'object' && record.stats !== null
      ? (record.stats as Record<string, unknown>)
      : {};
  const logs = Array.isArray(record.recent_audit_logs) ? record.recent_audit_logs : [];
  return {
    stats: {
      total_users: typeof stats.total_users === 'number' ? stats.total_users : 0,
      active_users: typeof stats.active_users === 'number' ? stats.active_users : 0,
      disabled_users: typeof stats.disabled_users === 'number' ? stats.disabled_users : 0,
      locked_users: typeof stats.locked_users === 'number' ? stats.locked_users : 0,
      recent_logins: typeof stats.recent_logins === 'number' ? stats.recent_logins : 0,
      recent_failed_login_attempts:
        typeof stats.recent_failed_login_attempts === 'number'
          ? stats.recent_failed_login_attempts
          : 0,
    },
    recent_audit_logs: logs.map(parseAdminAuditLog),
  };
}

function parseAdminRole(value: unknown): AdminRole {
  const record =
    typeof value === 'object' && value !== null ? (value as Record<string, unknown>) : {};
  return {
    id: stringField(record, 'id'),
    name: stringField(record, 'name'),
    description: stringField(record, 'description'),
    is_system_role: record.is_system_role === true,
    permissions: permissionKeysArray(record.permissions),
  };
}

function parseAdminUserEnvelope(payload: unknown): AuthUser {
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  return parseAuthUser(record.user);
}

export async function fetchAdminDashboard(): Promise<AdminDashboardResponse> {
  return parseAdminDashboard(await requestJSON('/api/v1/admin/dashboard'));
}

export async function fetchAdminUsers(): Promise<AuthUser[]> {
  const payload = await requestJSON('/api/v1/admin/users');
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const users = Array.isArray(record.users) ? record.users : [];
  return users.map(parseAuthUser);
}

export async function fetchAdminRoles(): Promise<AdminRole[]> {
  const payload = await requestJSON('/api/v1/admin/roles');
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const roles = Array.isArray(record.roles) ? record.roles : [];
  return roles.map(parseAdminRole);
}

export async function fetchAdminPermissions(): Promise<string[]> {
  const payload = await requestJSON('/api/v1/admin/permissions');
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  return permissionKeysArray(record.permissions);
}

export async function fetchAdminAuditLogs(): Promise<AdminAuditLog[]> {
  const payload = await requestJSON('/api/v1/admin/audit-logs');
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const logs = Array.isArray(record.audit_logs) ? record.audit_logs : [];
  return logs.map(parseAdminAuditLog);
}

export async function createAdminUser(payload: CreateAdminUserPayload): Promise<AuthUser> {
  return parseAdminUserEnvelope(await requestJSONWithBody('/api/v1/admin/users', 'POST', payload));
}

export async function updateAdminUser(
  id: string,
  payload: UpdateAdminUserPayload,
): Promise<AuthUser> {
  return parseAdminUserEnvelope(
    await requestJSONWithBody(`/api/v1/admin/users/${encodeURIComponent(id)}`, 'PATCH', payload),
  );
}

export async function setAdminUserStatus(id: string, status: string): Promise<AuthUser> {
  return parseAdminUserEnvelope(
    await requestJSONWithBody(`/api/v1/admin/users/${encodeURIComponent(id)}/status`, 'PATCH', {
      status,
    }),
  );
}

export async function assignAdminUserRole(userId: string, roleId: string): Promise<AuthUser> {
  return parseAdminUserEnvelope(
    await requestJSONWithBody(`/api/v1/admin/users/${encodeURIComponent(userId)}/roles`, 'POST', {
      role_id: roleId,
    }),
  );
}

export async function removeAdminUserRole(userId: string, roleId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/admin/users/${encodeURIComponent(userId)}/roles/${encodeURIComponent(roleId)}`,
    'DELETE',
  );
}

export async function createAdminPasswordReset(
  userId: string,
): Promise<AdminPasswordResetResponse> {
  const payload = await requestJSONWithBody(
    `/api/v1/admin/users/${encodeURIComponent(userId)}/password-reset`,
    'POST',
  );
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const resetToken =
    typeof record.reset_token === 'string'
      ? record.reset_token
      : typeof record.token === 'string'
        ? record.token
        : '';
  return { reset_token: resetToken };
}
