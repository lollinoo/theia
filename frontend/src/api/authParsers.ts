import { permissionKeysArray, stringArray, stringField } from './parsers';
import type { AuthSession, AuthUser } from './auth';

// parseAuthUser normalizes auth user payloads while dropping any secret-bearing fields.
export function parseAuthUser(value: unknown): AuthUser {
  const record =
    typeof value === 'object' && value !== null ? (value as Record<string, unknown>) : {};
  return {
    id: stringField(record, 'id'),
    username: stringField(record, 'username'),
    email: stringField(record, 'email'),
    display_name: stringField(record, 'display_name'),
    status: stringField(record, 'status') || 'unknown',
    must_change_password: record.must_change_password === true,
    roles: stringArray(record.roles),
    permissions: permissionKeysArray(record.permissions),
  };
}

// parseAuthSession preserves unauthenticated-session defaults and parses users only when present.
export function parseAuthSession(payload: unknown): AuthSession {
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const authenticated = record.authenticated === true;
  const user = authenticated && record.user !== undefined ? parseAuthUser(record.user) : undefined;
  return user ? { authenticated, user } : { authenticated };
}
