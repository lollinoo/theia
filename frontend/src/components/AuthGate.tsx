import { type FormEvent, type ReactNode, useState } from 'react';
import { resetPasswordWithToken } from '../api/client';
import { useAuth } from '../contexts/AuthContext';
import { MaterialIcon } from './MaterialIcon';

interface AuthGateProps {
  children: ReactNode;
}

function messageFromError(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export function AuthGate({ children }: AuthGateProps) {
  const { status, user, error: sessionError, login, changePassword } = useAuth();
  const [mode, setMode] = useState<'login' | 'reset'>('login');
  const [identifier, setIdentifier] = useState('administrator');
  const [password, setPassword] = useState('');
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [resetToken, setResetToken] = useState('');
  const [resetNewPassword, setResetNewPassword] = useState('');
  const [resetConfirmPassword, setResetConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    setSuccess(null);
    try {
      const session = await login({ identifier: identifier.trim(), password });
      if (!session.authenticated) {
        setError('Invalid username or password');
        return;
      }
      setPassword('');
    } catch (loginError) {
      setError(messageFromError(loginError, 'Invalid username or password'));
    } finally {
      setSubmitting(false);
    }
  }

  async function handlePasswordChange(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setSuccess(null);
    if (newPassword !== confirmPassword) {
      setError('New passwords do not match');
      return;
    }

    setSubmitting(true);
    try {
      await changePassword({
        current_password: currentPassword,
        new_password: newPassword,
      });
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (changeError) {
      setError(messageFromError(changeError, 'Unable to change password'));
    } finally {
      setSubmitting(false);
    }
  }

  async function handlePasswordReset(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setSuccess(null);
    if (resetNewPassword !== resetConfirmPassword) {
      setError('New passwords do not match');
      return;
    }

    setSubmitting(true);
    try {
      await resetPasswordWithToken({
        token: resetToken.trim(),
        new_password: resetNewPassword,
      });
      setResetToken('');
      setResetNewPassword('');
      setResetConfirmPassword('');
      setMode('login');
      setSuccess('Password reset complete. Sign in with your new password.');
    } catch (resetError) {
      setError(messageFromError(resetError, 'Unable to reset password'));
    } finally {
      setSubmitting(false);
    }
  }

  if (status === 'authenticated' && user && !user.must_change_password) {
    return <>{children}</>;
  }

  if (status === 'checking') {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-bg text-on-bg">
        <div className="text-sm text-on-bg-secondary">Loading</div>
      </div>
    );
  }

  if (status === 'authenticated' && user?.must_change_password) {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-bg px-6 text-on-bg">
        <form
          className="w-full max-w-sm rounded-lg border border-outline-subtle bg-surface p-6 shadow-xl"
          onSubmit={handlePasswordChange}
        >
          <div className="mb-6 flex items-start gap-3">
            <div className="mt-0.5 rounded-md bg-warning/10 p-2 text-warning">
              <MaterialIcon name="lock_reset" size={20} />
            </div>
            <div>
              <h1 className="text-xl font-semibold">Password change required</h1>
              <p className="mt-1 text-sm text-on-bg-secondary">Set a new password to continue.</p>
            </div>
          </div>
          <label className="mb-4 block">
            <span className="mb-2 block text-sm font-medium">Current password</span>
            <input
              className="w-full rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
              autoComplete="current-password"
              type="password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
            />
          </label>
          <label className="mb-4 block">
            <span className="mb-2 block text-sm font-medium">New password</span>
            <input
              className="w-full rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
              autoComplete="new-password"
              type="password"
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
            />
          </label>
          <label className="mb-5 block">
            <span className="mb-2 block text-sm font-medium">Confirm new password</span>
            <input
              className="w-full rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
              autoComplete="new-password"
              type="password"
              value={confirmPassword}
              onChange={(event) => setConfirmPassword(event.target.value)}
            />
          </label>
          {error && <div className="mb-4 text-sm text-warning">{error}</div>}
          <button
            className="w-full rounded-md bg-primary px-4 py-2 text-sm font-semibold text-on-primary disabled:cursor-not-allowed disabled:opacity-60"
            disabled={
              submitting ||
              currentPassword.trim() === '' ||
              newPassword.trim() === '' ||
              confirmPassword.trim() === ''
            }
            type="submit"
          >
            {submitting ? 'Changing password' : 'Change password'}
          </button>
        </form>
      </div>
    );
  }

  if (mode === 'reset') {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-bg px-6 text-on-bg">
        <form
          className="w-full max-w-sm rounded-lg border border-outline-subtle bg-surface p-6 shadow-xl"
          onSubmit={handlePasswordReset}
        >
          <div className="mb-6 flex items-start gap-3">
            <div className="mt-0.5 rounded-md bg-primary/10 p-2 text-primary">
              <MaterialIcon name="key" size={20} />
            </div>
            <div>
              <h1 className="text-xl font-semibold">Reset password</h1>
              <p className="mt-1 text-sm text-on-bg-secondary">Enter your one-time reset token.</p>
            </div>
          </div>
          <label className="mb-4 block">
            <span className="mb-2 block text-sm font-medium">One-time reset token</span>
            <input
              className="w-full rounded-md border border-outline-subtle bg-bg px-3 py-2 font-mono text-sm outline-none focus:border-primary"
              autoComplete="one-time-code"
              value={resetToken}
              onChange={(event) => setResetToken(event.target.value)}
            />
          </label>
          <label className="mb-4 block">
            <span className="mb-2 block text-sm font-medium">New password</span>
            <input
              className="w-full rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
              autoComplete="new-password"
              type="password"
              value={resetNewPassword}
              onChange={(event) => setResetNewPassword(event.target.value)}
            />
          </label>
          <label className="mb-5 block">
            <span className="mb-2 block text-sm font-medium">Confirm new password</span>
            <input
              className="w-full rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
              autoComplete="new-password"
              type="password"
              value={resetConfirmPassword}
              onChange={(event) => setResetConfirmPassword(event.target.value)}
            />
          </label>
          {error && <div className="mb-4 text-sm text-warning">{error}</div>}
          <button
            className="w-full rounded-md bg-primary px-4 py-2 text-sm font-semibold text-on-primary disabled:cursor-not-allowed disabled:opacity-60"
            disabled={
              submitting ||
              resetToken.trim() === '' ||
              resetNewPassword.trim() === '' ||
              resetConfirmPassword.trim() === ''
            }
            type="submit"
          >
            {submitting ? 'Resetting password' : 'Reset password'}
          </button>
          <button
            className="mt-3 w-full rounded-md border border-outline-subtle px-4 py-2 text-sm font-medium text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg"
            type="button"
            onClick={() => {
              setMode('login');
              setError(null);
            }}
          >
            Back to sign in
          </button>
        </form>
      </div>
    );
  }

  return (
    <div className="flex h-screen w-screen items-center justify-center bg-bg px-6 text-on-bg">
      <form
        className="w-full max-w-sm rounded-lg border border-outline-subtle bg-surface p-6 shadow-xl"
        onSubmit={handleLogin}
      >
        <div className="mb-6">
          <h1 className="text-xl font-semibold">Theia</h1>
          <p className="mt-1 text-sm text-on-bg-secondary">Sign in to Theia</p>
        </div>
        <label className="mb-4 block">
          <span className="mb-2 block text-sm font-medium">Username or email</span>
          <input
            className="w-full rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
            autoComplete="username"
            value={identifier}
            onChange={(event) => setIdentifier(event.target.value)}
          />
        </label>
        <label className="mb-5 block">
          <span className="mb-2 block text-sm font-medium">Password</span>
          <input
            className="w-full rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
            autoComplete="current-password"
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>
        {(error || sessionError) && (
          <div className="mb-4 text-sm text-warning">{error ?? sessionError}</div>
        )}
        {success && <div className="mb-4 text-sm text-status-up">{success}</div>}
        <button
          className="w-full rounded-md bg-primary px-4 py-2 text-sm font-semibold text-on-primary disabled:cursor-not-allowed disabled:opacity-60"
          disabled={submitting || identifier.trim() === '' || password.trim() === ''}
          type="submit"
        >
          {submitting ? 'Signing in' : 'Sign in'}
        </button>
        <button
          className="mt-3 w-full rounded-md border border-outline-subtle px-4 py-2 text-sm font-medium text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg"
          type="button"
          onClick={() => {
            setMode('reset');
            setError(null);
            setSuccess(null);
          }}
        >
          Use reset token
        </button>
      </form>
    </div>
  );
}
