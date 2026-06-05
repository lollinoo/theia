import { type FormEvent, type ReactNode, useId, useState } from 'react';
import { resetPasswordWithToken } from '../api/client';
import { useAuth } from '../contexts/AuthContext';
import { MaterialIcon } from './MaterialIcon';

interface AuthGateProps {
  children: ReactNode;
}

function messageFromError(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

type PasswordPolicyResult = {
  length: boolean;
  uppercase: boolean;
  lowercase: boolean;
  number: boolean;
  special: boolean;
  valid: boolean;
};

function evaluatePasswordPolicy(password: string): PasswordPolicyResult {
  const trimmed = password.trim();
  const characters = Array.from(trimmed);

  const result = {
    length: characters.length >= 10 && characters.length <= 24,
    uppercase: /\p{Lu}/u.test(trimmed),
    lowercase: /\p{Ll}/u.test(trimmed),
    number: /\p{Nd}/u.test(trimmed),
    special: /[\p{P}\p{S}]/u.test(trimmed),
  };

  return {
    ...result,
    valid: result.length && result.uppercase && result.lowercase && result.number && result.special,
  };
}

function PasswordRequirementItem({ met, children }: { met: boolean; children: ReactNode }) {
  return (
    <li className="flex items-center justify-between gap-3">
      <span>{children}</span>
      <span className={met ? 'font-medium text-status-up' : 'font-medium text-on-bg-secondary'}>
        {met ? 'Met' : 'Not met'}
      </span>
    </li>
  );
}

function PasswordRequirementsList({
  policy,
  confirmationMatches,
  differentFromCurrent,
}: {
  policy: PasswordPolicyResult;
  confirmationMatches: boolean;
  differentFromCurrent?: boolean;
}) {
  return (
    <section className="mb-4 rounded-md border border-outline-subtle bg-bg/60 p-3 text-xs text-on-bg-secondary">
      <h2 className="mb-2 text-sm font-medium text-on-bg">Password requirements</h2>
      <ul className="space-y-1">
        <PasswordRequirementItem met={policy.length}>10 to 24 characters</PasswordRequirementItem>
        <PasswordRequirementItem met={policy.uppercase}>
          At least one uppercase letter
        </PasswordRequirementItem>
        <PasswordRequirementItem met={policy.lowercase}>
          At least one lowercase letter
        </PasswordRequirementItem>
        <PasswordRequirementItem met={policy.number}>At least one number</PasswordRequirementItem>
        <PasswordRequirementItem met={policy.special}>
          At least one special character
        </PasswordRequirementItem>
        <PasswordRequirementItem met={confirmationMatches}>Passwords match</PasswordRequirementItem>
        {differentFromCurrent !== undefined && (
          <PasswordRequirementItem met={differentFromCurrent}>
            Different from current password
          </PasswordRequirementItem>
        )}
      </ul>
    </section>
  );
}

function PasswordInput({
  label,
  revealLabel,
  autoComplete,
  value,
  onChange,
  className = 'mb-4',
}: {
  label: string;
  revealLabel: string;
  autoComplete: string;
  value: string;
  onChange: (value: string) => void;
  className?: string;
}) {
  const inputId = useId();
  const [visible, setVisible] = useState(false);
  const action = visible ? 'Hide' : 'Show';

  return (
    <div className={`${className} block`}>
      <label htmlFor={inputId} className="mb-2 block text-sm font-medium">
        {label}
      </label>
      <div className="flex rounded-md border border-outline-subtle bg-bg focus-within:border-primary">
        <input
          id={inputId}
          className="min-w-0 flex-1 rounded-l-md bg-transparent px-3 py-2 text-sm outline-none"
          autoComplete={autoComplete}
          type={visible ? 'text' : 'password'}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
        <button
          aria-label={`${action} ${revealLabel}`}
          className="flex shrink-0 items-center rounded-r-md border-l border-outline-subtle px-3 py-2 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg"
          type="button"
          onClick={() => setVisible((current) => !current)}
        >
          <MaterialIcon name={visible ? 'visibility_off' : 'visibility'} size={16} />
        </button>
      </div>
    </div>
  );
}

export function AuthGate({ children }: AuthGateProps) {
  const { status, user, error: sessionError, login, changePassword } = useAuth();
  const [mode, setMode] = useState<'login' | 'reset'>('login');
  const [identifier, setIdentifier] = useState('');
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
  const forcedPasswordPolicy = evaluatePasswordPolicy(newPassword);
  const forcedPasswordConfirmationMatches =
    newPassword !== '' && confirmPassword !== '' && newPassword === confirmPassword;
  const forcedPasswordDifferentFromCurrent =
    currentPassword !== '' && newPassword !== '' && currentPassword !== newPassword;
  const canSubmitForcedPasswordChange =
    !submitting &&
    currentPassword.trim() !== '' &&
    forcedPasswordPolicy.valid &&
    forcedPasswordConfirmationMatches &&
    forcedPasswordDifferentFromCurrent;
  const resetPasswordPolicy = evaluatePasswordPolicy(resetNewPassword);
  const resetPasswordConfirmationMatches =
    resetNewPassword !== '' &&
    resetConfirmPassword !== '' &&
    resetNewPassword === resetConfirmPassword;
  const canSubmitPasswordReset =
    !submitting &&
    resetToken.trim() !== '' &&
    resetPasswordPolicy.valid &&
    resetPasswordConfirmationMatches;

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
    if (!canSubmitForcedPasswordChange) {
      return;
    }
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
    if (!canSubmitPasswordReset) {
      return;
    }
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
          <PasswordInput
            label="Current password"
            revealLabel="current password"
            autoComplete="current-password"
            value={currentPassword}
            onChange={setCurrentPassword}
          />
          <PasswordInput
            label="New password"
            revealLabel="new password"
            autoComplete="new-password"
            value={newPassword}
            onChange={setNewPassword}
          />
          <PasswordRequirementsList
            policy={forcedPasswordPolicy}
            confirmationMatches={forcedPasswordConfirmationMatches}
            differentFromCurrent={forcedPasswordDifferentFromCurrent}
          />
          <PasswordInput
            label="Confirm new password"
            revealLabel="confirm password"
            autoComplete="new-password"
            value={confirmPassword}
            onChange={setConfirmPassword}
            className="mb-5"
          />
          {error && <div className="mb-4 text-sm text-warning">{error}</div>}
          <button
            className="w-full rounded-md bg-primary px-4 py-2 text-sm font-semibold text-on-primary disabled:cursor-not-allowed disabled:opacity-60"
            disabled={!canSubmitForcedPasswordChange}
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
          <PasswordInput
            label="New password"
            revealLabel="new password"
            autoComplete="new-password"
            value={resetNewPassword}
            onChange={setResetNewPassword}
          />
          <PasswordRequirementsList
            policy={resetPasswordPolicy}
            confirmationMatches={resetPasswordConfirmationMatches}
          />
          <PasswordInput
            label="Confirm new password"
            revealLabel="confirm password"
            autoComplete="new-password"
            value={resetConfirmPassword}
            onChange={setResetConfirmPassword}
            className="mb-5"
          />
          {error && <div className="mb-4 text-sm text-warning">{error}</div>}
          <button
            className="w-full rounded-md bg-primary px-4 py-2 text-sm font-semibold text-on-primary disabled:cursor-not-allowed disabled:opacity-60"
            disabled={!canSubmitPasswordReset}
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
        <PasswordInput
          label="Password"
          revealLabel="password"
          autoComplete="current-password"
          value={password}
          onChange={setPassword}
          className="mb-5"
        />
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
