import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { type AuthUser, fetchCurrentUser, logoutUser } from '../api/client';
import { AuthProvider, useAuth } from './AuthContext';

vi.mock('../api/client', () => ({
  fetchCurrentUser: vi.fn(),
  loginUser: vi.fn(),
  logoutUser: vi.fn(),
  changePassword: vi.fn(),
}));

function authUser(overrides: Partial<AuthUser> = {}): AuthUser {
  return {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    display_name: 'Alice',
    status: 'active',
    must_change_password: false,
    roles: ['operator'],
    permissions: [],
    ...overrides,
  };
}

function AuthProbe() {
  const auth = useAuth();

  return (
    <div>
      <div data-testid="auth-status">{auth.status}</div>
      <div data-testid="auth-user">{auth.user?.username ?? 'none'}</div>
      <div data-testid="auth-error">{auth.error ?? 'none'}</div>
      <button
        type="button"
        onClick={() => {
          void auth.logout().catch(() => {});
        }}
      >
        Log out
      </button>
    </div>
  );
}

function renderProbe() {
  return render(
    <AuthProvider>
      <AuthProbe />
    </AuthProvider>,
  );
}

describe('AuthContext', () => {
  beforeEach(() => {
    vi.mocked(fetchCurrentUser).mockReset();
    vi.mocked(logoutUser).mockReset();
  });

  it('keeps the authenticated session when server logout fails', async () => {
    vi.mocked(fetchCurrentUser).mockResolvedValue({
      authenticated: true,
      user: authUser(),
    });
    vi.mocked(logoutUser).mockRejectedValue(new Error('network unavailable'));

    renderProbe();

    expect(await screen.findByTestId('auth-status')).toHaveTextContent('authenticated');
    expect(screen.getByTestId('auth-user')).toHaveTextContent('alice');

    fireEvent.click(screen.getByRole('button', { name: 'Log out' }));

    await waitFor(() => {
      expect(logoutUser).toHaveBeenCalledTimes(1);
      expect(screen.getByTestId('auth-error')).toHaveTextContent(
        'Unable to log out. Check your connection and try again.',
      );
    });
    expect(screen.getByTestId('auth-status')).toHaveTextContent('authenticated');
    expect(screen.getByTestId('auth-user')).toHaveTextContent('alice');
  });

  it('clears the authenticated session after server logout succeeds', async () => {
    vi.mocked(fetchCurrentUser).mockResolvedValue({
      authenticated: true,
      user: authUser(),
    });
    vi.mocked(logoutUser).mockResolvedValue({ authenticated: false });

    renderProbe();

    expect(await screen.findByTestId('auth-status')).toHaveTextContent('authenticated');

    fireEvent.click(screen.getByRole('button', { name: 'Log out' }));

    await waitFor(() => {
      expect(logoutUser).toHaveBeenCalledTimes(1);
      expect(screen.getByTestId('auth-status')).toHaveTextContent('unauthenticated');
    });
    expect(screen.getByTestId('auth-user')).toHaveTextContent('none');
    expect(screen.getByTestId('auth-error')).toHaveTextContent('none');
  });
});
