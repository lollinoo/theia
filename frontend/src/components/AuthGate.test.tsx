import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  type AuthUser,
  changePassword,
  fetchCurrentUser,
  loginUser,
  logoutUser,
} from '../api/client';
import { AuthProvider } from '../contexts/AuthContext';
import { AuthGate } from './AuthGate';

vi.mock('../api/client', () => ({
  fetchCurrentUser: vi.fn(),
  loginUser: vi.fn(),
  logoutUser: vi.fn(),
  changePassword: vi.fn(),
}));

function renderGate() {
  return render(
    <AuthProvider>
      <AuthGate>
        <div>secured app</div>
      </AuthGate>
    </AuthProvider>,
  );
}

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

describe('AuthGate', () => {
  beforeEach(() => {
    vi.mocked(fetchCurrentUser).mockReset();
    vi.mocked(loginUser).mockReset();
    vi.mocked(logoutUser).mockReset();
    vi.mocked(changePassword).mockReset();
  });

  it('renders children when a password session already exists', async () => {
    vi.mocked(fetchCurrentUser).mockResolvedValue({
      authenticated: true,
      user: authUser(),
    });

    renderGate();

    expect(await screen.findByText('secured app')).toBeInTheDocument();
  });

  it('logs in with identifier and password before rendering children', async () => {
    vi.mocked(fetchCurrentUser).mockResolvedValue({ authenticated: false });
    vi.mocked(loginUser).mockResolvedValue({
      authenticated: true,
      user: authUser({ username: 'administrator' }),
    });

    renderGate();

    fireEvent.change(await screen.findByLabelText('Username or email'), {
      target: { value: 'administrator' },
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'theia' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    await waitFor(() => {
      expect(loginUser).toHaveBeenCalledWith({
        identifier: 'administrator',
        password: 'theia',
      });
    });
    expect(await screen.findByText('secured app')).toBeInTheDocument();
  });

  it('blocks the app until a required password change completes', async () => {
    vi.mocked(fetchCurrentUser).mockResolvedValue({
      authenticated: true,
      user: authUser({ must_change_password: true }),
    });
    vi.mocked(changePassword).mockResolvedValue({
      authenticated: true,
      user: authUser({ must_change_password: false }),
    });

    renderGate();

    expect(await screen.findByText('Password change required')).toBeInTheDocument();
    expect(screen.queryByText('secured app')).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'old-password' },
    });
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'new-password-123' },
    });
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'new-password-123' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Change password' }));

    await waitFor(() => {
      expect(changePassword).toHaveBeenCalledWith({
        current_password: 'old-password',
        new_password: 'new-password-123',
      });
    });
    expect(await screen.findByText('secured app')).toBeInTheDocument();
  });

  it('keeps the app behind the login form when the session probe fails', async () => {
    vi.mocked(fetchCurrentUser).mockRejectedValue(new Error('network unavailable'));

    renderGate();

    expect(await screen.findByText('Sign in to Theia')).toBeInTheDocument();
    expect(screen.queryByText('secured app')).not.toBeInTheDocument();
  });
});
