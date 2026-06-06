/**
 * Normalizes backend admin payloads into frontend-safe shapes.
 * Keeps API boundary validation close to the transport helpers that consume it.
 */
import type {
  AdminAuditLog,
  AdminDashboardResponse,
  AdminPasswordResetResponse,
  AdminRole,
} from './admin';
import { type AuthUser, parseAuthUser } from './auth';
import { permissionKeysArray, recordField, stringField } from './parsers';

// parseAdminAuditLog normalizes audit log records while preserving optional metadata fields.
export function parseAdminAuditLog(value: unknown): AdminAuditLog {
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

// parseAdminDashboard normalizes dashboard counters and embedded recent audit logs.
export function parseAdminDashboard(payload: unknown): AdminDashboardResponse {
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

// parseAdminRole normalizes role metadata and permission arrays from admin responses.
export function parseAdminRole(value: unknown): AdminRole {
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

// parseAdminUsersEnvelope extracts safe users from the admin users envelope.
export function parseAdminUsersEnvelope(payload: unknown): AuthUser[] {
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const users = Array.isArray(record.users) ? record.users : [];
  return users.map(parseAuthUser);
}

// parseAdminRolesEnvelope extracts role rows from the admin roles envelope.
export function parseAdminRolesEnvelope(payload: unknown): AdminRole[] {
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const roles = Array.isArray(record.roles) ? record.roles : [];
  return roles.map(parseAdminRole);
}

// parseAdminPermissionsEnvelope extracts permission keys from the admin permissions envelope.
export function parseAdminPermissionsEnvelope(payload: unknown): string[] {
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  return permissionKeysArray(record.permissions);
}

// parseAdminAuditLogsEnvelope extracts audit log rows from the admin audit logs envelope.
export function parseAdminAuditLogsEnvelope(payload: unknown): AdminAuditLog[] {
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const logs = Array.isArray(record.audit_logs) ? record.audit_logs : [];
  return logs.map(parseAdminAuditLog);
}

// parseAdminUserEnvelope extracts one safe user from mutation responses.
export function parseAdminUserEnvelope(payload: unknown): AuthUser {
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  return parseAuthUser(record.user);
}

// parseAdminPasswordResetResponse preserves reset_token and legacy token fallback behavior.
export function parseAdminPasswordResetResponse(payload: unknown): AdminPasswordResetResponse {
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
