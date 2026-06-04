import { parseAuthSession } from './authParsers';
import { requestJSON, requestJSONWithBody } from './transport';
export { parseAuthUser } from './authParsers';

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

// fetchCurrentUser loads the current password session without adding legacy bearer headers.
export async function fetchCurrentUser(): Promise<AuthSession> {
  return parseAuthSession(await requestJSON('/api/v1/auth/me'));
}

// loginUser starts a password session using the public login endpoint.
export async function loginUser(payload: LoginPayload): Promise<AuthSession> {
  return parseAuthSession(await requestJSONWithBody('/api/v1/auth/login', 'POST', payload));
}

// logoutUser ends the current password session and returns the unauthenticated session envelope.
export async function logoutUser(): Promise<AuthSession> {
  return parseAuthSession(await requestJSONWithBody('/api/v1/auth/logout', 'POST'));
}

// changePassword updates the current password and preserves the returned session contract.
export async function changePassword(payload: ChangePasswordPayload): Promise<AuthSession> {
  return parseAuthSession(
    await requestJSONWithBody('/api/v1/auth/password/change', 'POST', payload),
  );
}

// resetPasswordWithToken completes a public password reset token flow.
export async function resetPasswordWithToken(payload: ResetPasswordPayload): Promise<void> {
  await requestJSONWithBody('/api/v1/auth/password/reset', 'POST', payload);
}
