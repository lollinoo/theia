import { permissionKeysArray, stringArray, stringField } from './parsers';
import { requestJSON, requestJSONWithBody } from './transport';

export interface AuthUser {
  id: string;
  username: string;
  email: string;
  display_name: string;
  status: string;
  must_change_password: boolean;
  roles: string[];
  permissions: string[];
}

export interface AuthSession {
  authenticated: boolean;
  user?: AuthUser;
}

export interface LoginPayload {
  identifier: string;
  password: string;
}

export interface ChangePasswordPayload {
  current_password: string;
  new_password: string;
}

export interface ResetPasswordPayload {
  token: string;
  new_password: string;
}

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

function parseAuthSession(payload: unknown): AuthSession {
  const record =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const authenticated = record.authenticated === true;
  const user = authenticated && record.user !== undefined ? parseAuthUser(record.user) : undefined;
  return user ? { authenticated, user } : { authenticated };
}

export async function fetchCurrentUser(): Promise<AuthSession> {
  return parseAuthSession(await requestJSON('/api/v1/auth/me'));
}

export async function loginUser(payload: LoginPayload): Promise<AuthSession> {
  return parseAuthSession(await requestJSONWithBody('/api/v1/auth/login', 'POST', payload));
}

export async function logoutUser(): Promise<AuthSession> {
  return parseAuthSession(await requestJSONWithBody('/api/v1/auth/logout', 'POST'));
}

export async function changePassword(payload: ChangePasswordPayload): Promise<AuthSession> {
  return parseAuthSession(
    await requestJSONWithBody('/api/v1/auth/password/change', 'POST', payload),
  );
}

export async function resetPasswordWithToken(payload: ResetPasswordPayload): Promise<void> {
  await requestJSONWithBody('/api/v1/auth/password/reset', 'POST', payload);
}
