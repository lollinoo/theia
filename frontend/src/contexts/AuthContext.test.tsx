/**
 * Exercises auth context shared context behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
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

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return { promise, resolve };
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

  it('rechecks and clears a restored session after a backend disconnect', async () => {
    vi.mocked(fetchCurrentUser)
      .mockResolvedValueOnce({ authenticated: true, user: authUser() })
      .mockResolvedValueOnce({ authenticated: false });

    renderProbe();

    expect(await screen.findByTestId('auth-status')).toHaveTextContent('authenticated');

    act(() => {
      window.dispatchEvent(new Event('backend-session-check-required'));
    });

    await waitFor(() => {
      expect(fetchCurrentUser).toHaveBeenCalledTimes(2);
      expect(screen.getByTestId('auth-status')).toHaveTextContent('unauthenticated');
    });
    expect(screen.getByTestId('auth-user')).toHaveTextContent('none');
  });

  it('keeps the current UI session while the backend is unavailable', async () => {
    vi.mocked(fetchCurrentUser)
      .mockResolvedValueOnce({ authenticated: true, user: authUser() })
      .mockRejectedValueOnce(new Error('backend restarting'));

    renderProbe();

    expect(await screen.findByTestId('auth-status')).toHaveTextContent('authenticated');

    act(() => {
      window.dispatchEvent(new Event('backend-session-check-required'));
    });

    await waitFor(() => expect(fetchCurrentUser).toHaveBeenCalledTimes(2));
    expect(screen.getByTestId('auth-status')).toHaveTextContent('authenticated');
    expect(screen.getByTestId('auth-user')).toHaveTextContent('alice');
  });

  it('ignores a stale authenticated probe after a newer probe confirms revocation', async () => {
    const staleProbe = deferred<Awaited<ReturnType<typeof fetchCurrentUser>>>();
    vi.mocked(fetchCurrentUser)
      .mockResolvedValueOnce({ authenticated: true, user: authUser() })
      .mockReturnValueOnce(staleProbe.promise)
      .mockResolvedValueOnce({ authenticated: false });

    renderProbe();
    expect(await screen.findByTestId('auth-status')).toHaveTextContent('authenticated');

    act(() => {
      window.dispatchEvent(new Event('backend-session-check-required'));
      window.dispatchEvent(new Event('backend-session-check-required'));
    });

    await waitFor(() => {
      expect(fetchCurrentUser).toHaveBeenCalledTimes(3);
      expect(screen.getByTestId('auth-status')).toHaveTextContent('unauthenticated');
    });

    await act(async () => {
      staleProbe.resolve({ authenticated: true, user: authUser() });
      await staleProbe.promise;
    });

    expect(screen.getByTestId('auth-status')).toHaveTextContent('unauthenticated');
    expect(screen.getByTestId('auth-user')).toHaveTextContent('none');
  });
});
