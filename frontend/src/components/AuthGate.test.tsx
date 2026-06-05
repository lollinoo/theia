import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  type AuthUser,
  changePassword,
  fetchCurrentUser,
  loginUser,
  logoutUser,
  resetPasswordWithToken,
} from '../api/client';
import { AuthProvider } from '../contexts/AuthContext';
import { AuthGate } from './AuthGate';

vi.mock('../api/client', () => ({
  fetchCurrentUser: vi.fn(),
  loginUser: vi.fn(),
  logoutUser: vi.fn(),
  changePassword: vi.fn(),
  resetPasswordWithToken: vi.fn(),
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

async function renderForcedPasswordChange() {
  vi.mocked(fetchCurrentUser).mockResolvedValue({
    authenticated: true,
    user: authUser({ must_change_password: true }),
  });

  renderGate();

  expect(await screen.findByText('Password change required')).toBeInTheDocument();
}

describe('AuthGate', () => {
  beforeEach(() => {
    vi.mocked(fetchCurrentUser).mockReset();
    vi.mocked(loginUser).mockReset();
    vi.mocked(logoutUser).mockReset();
    vi.mocked(changePassword).mockReset();
    vi.mocked(resetPasswordWithToken).mockReset();
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

  it('does not prefill the sign-in identifier', async () => {
    vi.mocked(fetchCurrentUser).mockResolvedValue({ authenticated: false });

    renderGate();

    expect(await screen.findByLabelText('Username or email')).toHaveValue('');
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
      target: { value: 'NewPass123!' },
    });
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'NewPass123!' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Change password' }));

    await waitFor(() => {
      expect(changePassword).toHaveBeenCalledWith({
        current_password: 'old-password',
        new_password: 'NewPass123!',
      });
    });
    expect(await screen.findByText('secured app')).toBeInTheDocument();
  });

  it('shows password requirements during a required password change', async () => {
    await renderForcedPasswordChange();

    expect(screen.getByText('Password requirements')).toBeInTheDocument();
    expect(screen.getByText('10 to 24 characters')).toBeInTheDocument();
    expect(screen.getByText('At least one uppercase letter')).toBeInTheDocument();
    expect(screen.getByText('At least one lowercase letter')).toBeInTheDocument();
    expect(screen.getByText('At least one number')).toBeInTheDocument();
    expect(screen.getByText('At least one special character')).toBeInTheDocument();
    expect(screen.getByText('Passwords match')).toBeInTheDocument();
  });

  it('keeps password change disabled for invalid new passwords', async () => {
    await renderForcedPasswordChange();

    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'old-password' },
    });
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'short' },
    });
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'short' },
    });

    expect(screen.getByRole('button', { name: 'Change password' })).toBeDisabled();
    expect(changePassword).not.toHaveBeenCalled();
  });

  it('enables password change for a valid new password and matching confirmation', async () => {
    await renderForcedPasswordChange();

    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'old-password' },
    });
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'NewPass123!' },
    });
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'NewPass123!' },
    });

    expect(screen.getByRole('button', { name: 'Change password' })).not.toBeDisabled();
  });

  it('keeps password change disabled when confirmation does not match', async () => {
    await renderForcedPasswordChange();

    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'old-password' },
    });
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'NewPass123!' },
    });
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'OtherPass1!' },
    });

    expect(screen.getByRole('button', { name: 'Change password' })).toBeDisabled();
    expect(screen.getByText('Passwords match').closest('li')).toHaveTextContent('Not met');
  });

  it('keeps password change disabled when composition requirements are not met', async () => {
    await renderForcedPasswordChange();

    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'old-password' },
    });
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'nouppercase1!' },
    });
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'nouppercase1!' },
    });

    expect(screen.getByRole('button', { name: 'Change password' })).toBeDisabled();
    expect(screen.getByText('At least one uppercase letter').closest('li')).toHaveTextContent(
      'Not met',
    );
  });

  it('shows password reveal controls on the required password change form even when empty', async () => {
    await renderForcedPasswordChange();

    const currentPasswordInput = screen.getByLabelText('Current password');
    expect(screen.getByRole('button', { name: 'Show current password' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show new password' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show confirm password' })).toBeInTheDocument();
    expect(currentPasswordInput).toHaveAttribute('type', 'password');

    fireEvent.click(screen.getByRole('button', { name: 'Show current password' }));

    expect(currentPasswordInput).toHaveAttribute('type', 'text');
    expect(screen.getByRole('button', { name: 'Hide current password' })).toBeInTheDocument();
  });

  it('keeps the app behind the login form when the session probe fails', async () => {
    vi.mocked(fetchCurrentUser).mockRejectedValue(new Error('network unavailable'));

    renderGate();

    expect(await screen.findByText('Sign in to Theia')).toBeInTheDocument();
    expect(screen.queryByText('secured app')).not.toBeInTheDocument();
  });

  it('completes a one-time reset token before returning to sign in', async () => {
    vi.mocked(fetchCurrentUser).mockResolvedValue({ authenticated: false });
    vi.mocked(resetPasswordWithToken).mockResolvedValue(undefined);

    renderGate();

    fireEvent.click(await screen.findByRole('button', { name: 'Use reset token' }));
    fireEvent.change(screen.getByLabelText('One-time reset token'), {
      target: { value: ' reset-token-1 ' },
    });
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'NewPass123!' },
    });
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'NewPass123!' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Reset password' }));

    await waitFor(() => {
      expect(resetPasswordWithToken).toHaveBeenCalledWith({
        token: 'reset-token-1',
        new_password: 'NewPass123!',
      });
    });
    expect(loginUser).not.toHaveBeenCalled();
    expect(
      await screen.findByText('Password reset complete. Sign in with your new password.'),
    ).toBeInTheDocument();
  });
});
