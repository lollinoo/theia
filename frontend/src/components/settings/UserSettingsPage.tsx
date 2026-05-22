import { useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  changePassword,
  fetchBridgeConnectorConfig,
  fetchUserSettings,
  generateBridgeSecret,
  revokeBridgeSecret,
  rotateBridgeSecret,
  updateUserSettings,
  type BridgeCredentialMetadata,
  type UserSettingsResponse,
} from '../../api/client';
import { MaterialIcon } from '../MaterialIcon';

function formatDate(value?: string): string {
  if (!value) return 'Never';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function credentialStatus(credential?: BridgeCredentialMetadata): string {
  if (!credential) return 'Not configured';
  if (credential.revoked_at || credential.status === 'revoked') return 'Revoked';
  if (credential.expires_at && new Date(credential.expires_at).getTime() <= Date.now()) {
    return 'Expired';
  }
  return 'Active';
}

export function UserSettingsPage() {
  const [settings, setSettings] = useState<UserSettingsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [oneTimeSecret, setOneTimeSecret] = useState<string | null>(null);
  const [configSnippet, setConfigSnippet] = useState('');
  const [passwordForm, setPasswordForm] = useState({ current_password: '', new_password: '' });

  useEffect(() => {
    fetchUserSettings()
      .then(setSettings)
      .catch((err: unknown) =>
        setError(err instanceof Error ? err.message : 'Failed to load settings'),
      )
      .finally(() => setLoading(false));
  }, []);

  const profile = useMemo(() => {
    return {
      display_name: settings?.user.display_name ?? '',
      username: settings?.user.username ?? '',
      email: settings?.user.email ?? '',
      timezone: settings?.preferences.timezone ?? 'UTC',
      locale: settings?.preferences.locale ?? 'en-US',
      bridge_port: settings?.preferences.bridge_port ?? 1337,
    };
  }, [settings]);

  async function reload() {
    const next = await fetchUserSettings();
    setSettings(next);
    return next;
  }

  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const next = await updateUserSettings({
        display_name: String(form.get('display_name') ?? ''),
        username: String(form.get('username') ?? ''),
        email: String(form.get('email') ?? ''),
        timezone: String(form.get('timezone') ?? ''),
        locale: String(form.get('locale') ?? ''),
        bridge_port: Number(form.get('bridge_port') ?? 1337),
      });
      setSettings(next);
      setMessage('Settings saved');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save settings');
    } finally {
      setSaving(false);
    }
  }

  async function submitPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      await changePassword(passwordForm);
      setPasswordForm({ current_password: '', new_password: '' });
      setMessage('Password changed');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to change password');
    } finally {
      setSaving(false);
    }
  }

  async function handleSecret(action: 'generate' | 'rotate' | 'revoke') {
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      if (action === 'generate') {
        const result = await generateBridgeSecret();
        setOneTimeSecret(result.secret);
      } else if (action === 'rotate') {
        const result = await rotateBridgeSecret();
        setOneTimeSecret(result.secret);
      } else {
        await revokeBridgeSecret();
        setOneTimeSecret(null);
      }
      await reload();
      setMessage(
        action === 'revoke'
          ? 'Bridge Secret revoked'
          : 'Copy the Bridge Secret now. You will not be able to see it again.',
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Bridge Secret operation failed');
    } finally {
      setSaving(false);
    }
  }

  async function loadConfigSnippet() {
    try {
      const result = await fetchBridgeConnectorConfig();
      setConfigSnippet(JSON.stringify(result.config, null, 2));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load connector config');
    }
  }

  if (loading) {
    return (
      <main className="min-h-full px-4 pb-10 pt-24 text-on-bg sm:px-8">
        <div className="mx-auto max-w-5xl text-sm text-on-bg-secondary">Loading settings</div>
      </main>
    );
  }

  const credential = settings?.bridge.credential;
  const configured = settings?.bridge.configured === true;
  const activeCredential = credentialStatus(credential) === 'Active';
  const downloadClass = activeCredential
    ? 'theia-button-secondary'
    : 'theia-button-secondary pointer-events-none opacity-50';

  return (
    <main className="min-h-full overflow-y-auto px-4 pb-10 pt-24 text-on-bg sm:px-8">
      <div className="mx-auto flex max-w-5xl flex-col gap-6">
        <header>
          <h1 className="text-2xl font-semibold">User Settings</h1>
          <p className="mt-1 text-sm text-on-bg-secondary">
            Manage your account, password, and personal Bridge Connector.
          </p>
        </header>

        {(message || error) && (
          <div
            className={`rounded-lg border px-4 py-3 text-sm ${
              error
                ? 'border-critical/40 bg-critical/10 text-critical'
                : 'border-primary/40 bg-primary/10 text-on-bg'
            }`}
          >
            {error ?? message}
          </div>
        )}

        <section className="rounded-lg border border-outline-subtle bg-surface-container/70 p-4">
          <h2 className="text-lg font-semibold">Account Profile</h2>
          <form className="mt-4 grid gap-4 sm:grid-cols-2" onSubmit={saveProfile}>
            <label className="grid gap-1 text-sm">
              <span className="text-on-bg-secondary">Display name</span>
              <input
                name="display_name"
                defaultValue={profile.display_name}
                className="theia-input"
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-on-bg-secondary">Username</span>
              <input name="username" defaultValue={profile.username} className="theia-input" />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-on-bg-secondary">Email</span>
              <input
                name="email"
                type="email"
                defaultValue={profile.email}
                className="theia-input"
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-on-bg-secondary">Bridge port</span>
              <input
                name="bridge_port"
                type="number"
                min={1}
                max={65535}
                defaultValue={profile.bridge_port}
                className="theia-input"
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-on-bg-secondary">Timezone</span>
              <input name="timezone" defaultValue={profile.timezone} className="theia-input" />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-on-bg-secondary">Locale</span>
              <input name="locale" defaultValue={profile.locale} className="theia-input" />
            </label>
            <div className="sm:col-span-2">
              <button type="submit" disabled={saving} className="theia-button-primary">
                Save Profile
              </button>
            </div>
          </form>
        </section>

        <section className="rounded-lg border border-outline-subtle bg-surface-container/70 p-4">
          <h2 className="text-lg font-semibold">Security</h2>
          <div className="mt-3 grid gap-4 sm:grid-cols-2">
            <div className="text-sm text-on-bg-secondary">
              <div>Last login: {formatDate(settings?.user.last_login_at)}</div>
              <div>Password changed: {formatDate(settings?.user.password_changed_at)}</div>
            </div>
            <form className="grid gap-3" onSubmit={submitPassword}>
              <input
                className="theia-input"
                type="password"
                placeholder="Current password"
                value={passwordForm.current_password}
                onChange={(event) =>
                  setPasswordForm((current) => ({
                    ...current,
                    current_password: event.target.value,
                  }))
                }
              />
              <input
                className="theia-input"
                type="password"
                placeholder="New password"
                value={passwordForm.new_password}
                onChange={(event) =>
                  setPasswordForm((current) => ({ ...current, new_password: event.target.value }))
                }
              />
              <button type="submit" disabled={saving} className="theia-button-secondary">
                Change Password
              </button>
            </form>
          </div>
        </section>

        <section className="rounded-lg border border-outline-subtle bg-surface-container/70 p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold">Bridge Connector</h2>
              <p className="mt-1 text-sm text-on-bg-secondary">
                Your Bridge Secret connects your local Bridge Connector to your Theia account.
              </p>
            </div>
            <span className="rounded-full border border-outline-subtle px-3 py-1 text-sm">
              {credentialStatus(credential)}
            </span>
          </div>

          <div className="mt-4 grid gap-3 text-sm sm:grid-cols-2">
            <div>Prefix: {credential?.secret_prefix ?? 'Not generated'}</div>
            <div>Created: {formatDate(credential?.created_at)}</div>
            <div>Last rotated: {formatDate(credential?.rotated_at)}</div>
            <div>Last used: {formatDate(credential?.last_used_at)}</div>
          </div>

          {oneTimeSecret && (
            <div className="mt-4 rounded-lg border border-warning/50 bg-warning/10 p-3">
              <div className="text-sm font-semibold">
                Copy this now. You will not be able to see it again.
              </div>
              <div className="mt-2 flex gap-2">
                <code className="min-w-0 flex-1 overflow-x-auto rounded bg-surface-container-high px-3 py-2 text-sm">
                  {oneTimeSecret}
                </code>
                <button
                  type="button"
                  className="theia-button-secondary"
                  aria-label="Copy Bridge Secret"
                  onClick={() => void navigator.clipboard?.writeText(oneTimeSecret)}
                >
                  <MaterialIcon name="content_copy" className="text-[18px]" />
                </button>
              </div>
            </div>
          )}

          <div className="mt-4 flex flex-wrap gap-2">
            {!configured && (
              <button
                type="button"
                disabled={saving}
                className="theia-button-primary"
                onClick={() => void handleSecret('generate')}
              >
                Generate Bridge Secret
              </button>
            )}
            {configured && (
              <>
                <button
                  type="button"
                  disabled={saving}
                  className="theia-button-secondary"
                  onClick={() => void handleSecret('rotate')}
                >
                  Rotate Secret
                </button>
                <button
                  type="button"
                  disabled={saving}
                  className="theia-button-danger"
                  onClick={() => void handleSecret('revoke')}
                >
                  Revoke Secret
                </button>
              </>
            )}
            <a
              className={downloadClass}
              aria-disabled={!activeCredential}
              href={
                activeCredential
                  ? '/api/v1/settings/bridge/connector/download/linux/amd64'
                  : undefined
              }
            >
              Download Linux
            </a>
            <a
              className={downloadClass}
              aria-disabled={!activeCredential}
              href={
                activeCredential
                  ? '/api/v1/settings/bridge/connector/download/windows/amd64'
                  : undefined
              }
            >
              Download Windows
            </a>
            <button
              type="button"
              className="theia-button-secondary"
              onClick={() => void loadConfigSnippet()}
            >
              Show Config
            </button>
          </div>

          {configSnippet && (
            <pre className="mt-4 overflow-x-auto rounded-lg bg-surface-container-high p-3 text-xs">
              {configSnippet}
            </pre>
          )}
        </section>
      </div>
    </main>
  );
}

export default UserSettingsPage;
