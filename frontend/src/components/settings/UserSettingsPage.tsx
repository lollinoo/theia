import { type FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import {
  type BridgeConnectorDownload,
  type BridgeCredentialMetadata,
  type UserSettingsResponse,
  changePassword,
  fetchBridgeConnectorConfig,
  fetchUserSettings,
  generateBridgeSecret,
  revokeBridgeSecret,
  rotateBridgeSecret,
  updateUserSettings,
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

const defaultConnectorDownloads: BridgeConnectorDownload[] = [
  {
    label: 'Linux x64',
    os: 'linux',
    arch: 'amd64',
    url: '/api/v1/settings/bridge/connector/download/linux/amd64',
    available: false,
  },
  {
    label: 'Windows x64',
    os: 'windows',
    arch: 'amd64',
    url: '/api/v1/settings/bridge/connector/download/windows/amd64',
    available: false,
  },
  {
    label: 'macOS Intel',
    os: 'darwin',
    arch: 'amd64',
    url: '/api/v1/settings/bridge/connector/download/darwin/amd64',
    available: false,
  },
  {
    label: 'macOS Apple Silicon',
    os: 'darwin',
    arch: 'arm64',
    url: '/api/v1/settings/bridge/connector/download/darwin/arm64',
    available: false,
  },
];

const timezoneOptions = [
  { value: 'UTC', label: 'UTC' },
  { value: 'Europe/Rome', label: 'Europe/Rome' },
  { value: 'Europe/London', label: 'Europe/London' },
  { value: 'Europe/Berlin', label: 'Europe/Berlin' },
  { value: 'Europe/Paris', label: 'Europe/Paris' },
  { value: 'America/New_York', label: 'America/New_York' },
  { value: 'America/Chicago', label: 'America/Chicago' },
  { value: 'America/Denver', label: 'America/Denver' },
  { value: 'America/Los_Angeles', label: 'America/Los_Angeles' },
  { value: 'America/Sao_Paulo', label: 'America/Sao_Paulo' },
  { value: 'Asia/Tokyo', label: 'Asia/Tokyo' },
  { value: 'Asia/Singapore', label: 'Asia/Singapore' },
  { value: 'Australia/Sydney', label: 'Australia/Sydney' },
];

const localeOptions = [
  { value: 'en-US', label: 'English (US)' },
  { value: 'en-GB', label: 'English (UK)' },
  { value: 'it-IT', label: 'Italian (Italy)' },
  { value: 'de-DE', label: 'German (Germany)' },
  { value: 'fr-FR', label: 'French (France)' },
  { value: 'es-ES', label: 'Spanish (Spain)' },
  { value: 'pt-BR', label: 'Portuguese (Brazil)' },
];

function withCurrentOption(
  options: Array<{ value: string; label: string }>,
  current: string,
): Array<{ value: string; label: string }> {
  if (!current || options.some((option) => option.value === current)) {
    return options;
  }
  return [{ value: current, label: current }, ...options];
}

export function UserSettingsPage() {
  const [settings, setSettings] = useState<UserSettingsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [oneTimeSecret, setOneTimeSecret] = useState<string | null>(null);
  const [configSnippet, setConfigSnippet] = useState('');
  const [connectorDownloads, setConnectorDownloads] = useState<BridgeConnectorDownload[]>([]);
  const [passwordForm, setPasswordForm] = useState({ current_password: '', new_password: '' });
  const [bridgeMenuOpen, setBridgeMenuOpen] = useState(false);
  const bridgeMenuRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    fetchUserSettings()
      .then(setSettings)
      .catch((err: unknown) =>
        setError(err instanceof Error ? err.message : 'Failed to load settings'),
      )
      .finally(() => setLoading(false));
    fetchBridgeConnectorConfig()
      .then((result) => setConnectorDownloads(result.downloads))
      .catch(() => setConnectorDownloads([]));
  }, []);

  useEffect(() => {
    if (!bridgeMenuOpen) return;

    const handlePointerDown = (event: MouseEvent) => {
      if (event.target instanceof Node && !bridgeMenuRef.current?.contains(event.target)) {
        setBridgeMenuOpen(false);
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setBridgeMenuOpen(false);
      }
    };

    document.addEventListener('mousedown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [bridgeMenuOpen]);

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

  const timezoneSelectOptions = useMemo(
    () => withCurrentOption(timezoneOptions, profile.timezone),
    [profile.timezone],
  );
  const localeSelectOptions = useMemo(
    () => withCurrentOption(localeOptions, profile.locale),
    [profile.locale],
  );

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

  async function saveBridgePort(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const next = await updateUserSettings({
        bridge_port: Number(form.get('bridge_port') ?? profile.bridge_port),
      });
      setSettings(next);
      setMessage('Bridge port saved');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save Bridge port');
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
    setBridgeMenuOpen(false);
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
      setConnectorDownloads(result.downloads);
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
  const bridgeStatus = credentialStatus(credential);
  const activeCredential = bridgeStatus === 'Active';
  const downloadTargets =
    connectorDownloads.length > 0 ? connectorDownloads : defaultConnectorDownloads;
  const bridgeMenuBaseClass =
    'flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring';
  const bridgeMenuItemClass = `${bridgeMenuBaseClass} text-on-bg hover:bg-surface-container`;
  const bridgeMenuDangerItemClass = `${bridgeMenuBaseClass} text-critical hover:bg-critical/10`;
  const bridgeMenuDisabledItemClass = `${bridgeMenuBaseClass} cursor-not-allowed text-on-bg-muted opacity-60`;

  return (
    <main className="min-h-full overflow-y-auto px-4 pb-10 pt-24 text-on-bg sm:px-8">
      <div className="mx-auto flex max-w-6xl flex-col gap-6">
        <header className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold">User Settings</h1>
            <p className="mt-1 text-sm text-on-bg-secondary">
              Manage your account, password, and personal Bridge Connector.
            </p>
          </div>
          <div className="rounded-full border border-outline-subtle bg-surface-container-high px-3 py-1 text-sm text-on-bg-secondary">
            {profile.timezone} / {profile.locale}
          </div>
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

        <div className="grid gap-6 lg:grid-cols-[minmax(0,1.08fr)_minmax(22rem,0.92fr)]">
          <div className="grid content-start gap-6">
            <section
              aria-labelledby="account-profile-heading"
              className="rounded-lg border border-outline-subtle bg-surface-container/80 p-5 shadow-panel"
            >
              <div className="flex items-center gap-3">
                <span
                  aria-hidden="true"
                  className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/15 text-primary"
                >
                  <MaterialIcon name="person" className="text-[20px]" />
                </span>
                <div>
                  <h2 id="account-profile-heading" className="text-lg font-semibold">
                    Account Profile
                  </h2>
                  <p className="text-sm text-on-bg-secondary">Identity and display preferences</p>
                </div>
              </div>
              <form className="mt-5 grid gap-4 sm:grid-cols-2" onSubmit={saveProfile}>
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
                  <span className="text-on-bg-secondary">Timezone</span>
                  <select name="timezone" defaultValue={profile.timezone} className="theia-input">
                    {timezoneSelectOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="grid gap-1 text-sm">
                  <span className="text-on-bg-secondary">Locale</span>
                  <select name="locale" defaultValue={profile.locale} className="theia-input">
                    {localeSelectOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>
                <div className="sm:col-span-2">
                  <button type="submit" disabled={saving} className="theia-button-primary">
                    Save Profile
                  </button>
                </div>
              </form>
            </section>

            <section
              aria-labelledby="security-heading"
              className="rounded-lg border border-outline-subtle bg-surface-container/80 p-5 shadow-panel"
            >
              <div className="flex items-center gap-3">
                <span
                  aria-hidden="true"
                  className="flex h-10 w-10 items-center justify-center rounded-lg bg-warning/15 text-warning"
                >
                  <MaterialIcon name="lock" className="text-[20px]" />
                </span>
                <div>
                  <h2 id="security-heading" className="text-lg font-semibold">
                    Security
                  </h2>
                  <p className="text-sm text-on-bg-secondary">Password and account activity</p>
                </div>
              </div>
              <div className="mt-5 grid gap-4 sm:grid-cols-2">
                <div className="grid gap-3 text-sm text-on-bg-secondary sm:border-r sm:border-outline-subtle sm:pr-4">
                  <div>
                    <span className="block text-xs uppercase text-on-bg-muted">Last login</span>
                    <span className="text-on-bg">{formatDate(settings?.user.last_login_at)}</span>
                  </div>
                  <div>
                    <span className="block text-xs uppercase text-on-bg-muted">
                      Password changed
                    </span>
                    <span className="text-on-bg">
                      {formatDate(settings?.user.password_changed_at)}
                    </span>
                  </div>
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
                      setPasswordForm((current) => ({
                        ...current,
                        new_password: event.target.value,
                      }))
                    }
                  />
                  <button type="submit" disabled={saving} className="theia-button-secondary">
                    Change Password
                  </button>
                </form>
              </div>
            </section>
          </div>

          <section
            aria-labelledby="bridge-connector-heading"
            className="rounded-lg border border-outline-subtle bg-surface-container/80 p-5 shadow-panel"
          >
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="flex min-w-0 items-center gap-3">
                <span
                  aria-hidden="true"
                  className="flex h-10 w-10 flex-none items-center justify-center rounded-lg bg-primary/15 text-primary"
                >
                  <MaterialIcon name="hub" className="text-[20px]" />
                </span>
                <div className="min-w-0">
                  <h2 id="bridge-connector-heading" className="text-lg font-semibold">
                    Bridge Connector
                  </h2>
                  <p className="text-sm text-on-bg-secondary">
                    Your local connector and Bridge Secret
                  </p>
                </div>
              </div>
              <span
                className={`rounded-full border px-3 py-1 text-sm ${
                  activeCredential
                    ? 'border-status-up/35 bg-status-up/10 text-status-up'
                    : 'border-outline-subtle bg-surface-container-high text-on-bg-secondary'
                }`}
              >
                {bridgeStatus}
              </span>
            </div>

            <form className="mt-5 border-t border-outline-subtle pt-4" onSubmit={saveBridgePort}>
              <label className="grid gap-1 text-sm">
                <span className="text-on-bg-secondary">Bridge port</span>
                <input
                  name="bridge_port"
                  type="number"
                  min={1}
                  max={65535}
                  defaultValue={profile.bridge_port}
                  className="theia-input font-mono"
                />
              </label>
              <button type="submit" disabled={saving} className="theia-button-secondary mt-3">
                Save Bridge Port
              </button>
            </form>

            <div className="mt-5 grid gap-4 border-t border-outline-subtle pt-4 text-sm sm:grid-cols-2">
              <div>
                <span className="block text-xs uppercase text-on-bg-muted">Prefix</span>
                <span className="break-all text-on-bg">
                  {credential?.secret_prefix ?? 'Not generated'}
                </span>
              </div>
              <div>
                <span className="block text-xs uppercase text-on-bg-muted">Created</span>
                <span className="text-on-bg">{formatDate(credential?.created_at)}</span>
              </div>
              <div>
                <span className="block text-xs uppercase text-on-bg-muted">Last rotated</span>
                <span className="text-on-bg">{formatDate(credential?.rotated_at)}</span>
              </div>
              <div>
                <span className="block text-xs uppercase text-on-bg-muted">Last used</span>
                <span className="text-on-bg">{formatDate(credential?.last_used_at)}</span>
              </div>
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

            <div className="mt-5 flex flex-wrap gap-2">
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
                <div ref={bridgeMenuRef} className="relative">
                  <button
                    type="button"
                    aria-haspopup="menu"
                    aria-expanded={bridgeMenuOpen}
                    aria-label="Bridge connector actions"
                    disabled={saving}
                    className="theia-button-secondary"
                    onClick={() => setBridgeMenuOpen((current) => !current)}
                  >
                    <MaterialIcon name="more_vert" className="text-[18px]" />
                    Actions
                    <MaterialIcon
                      name={bridgeMenuOpen ? 'expand_less' : 'expand_more'}
                      className="text-[18px]"
                    />
                  </button>
                  {bridgeMenuOpen && (
                    <div className="absolute right-0 z-20 mt-2 w-[min(18rem,calc(100vw-2rem))] rounded-lg border border-outline-subtle bg-surface-container-high p-1 shadow-panel">
                      <div role="menu" aria-label="Bridge connector actions">
                        <button
                          type="button"
                          role="menuitem"
                          className={bridgeMenuItemClass}
                          onClick={() => void handleSecret('rotate')}
                        >
                          <MaterialIcon name="sync" className="text-[18px]" />
                          <span className="min-w-0 flex-1 truncate">Rotate Secret</span>
                        </button>
                        <button
                          type="button"
                          role="menuitem"
                          className={bridgeMenuDangerItemClass}
                          onClick={() => void handleSecret('revoke')}
                        >
                          <MaterialIcon name="delete" className="text-[18px]" />
                          <span className="min-w-0 flex-1 truncate">Revoke Secret</span>
                        </button>
                        <div className="my-1 border-t border-outline-subtle" />
                        {downloadTargets.map((target) => {
                          const enabled = activeCredential && target.available;
                          const label = `Download ${target.label}`;
                          if (enabled) {
                            return (
                              <a
                                key={`${target.os}/${target.arch}`}
                                role="menuitem"
                                className={bridgeMenuItemClass}
                                href={target.url}
                              >
                                <MaterialIcon name="download" className="text-[18px]" />
                                <span className="min-w-0 flex-1 truncate">{label}</span>
                              </a>
                            );
                          }
                          return (
                            <button
                              key={`${target.os}/${target.arch}`}
                              type="button"
                              role="menuitem"
                              disabled
                              aria-disabled="true"
                              className={bridgeMenuDisabledItemClass}
                            >
                              <MaterialIcon name="download" className="text-[18px]" />
                              <span className="min-w-0 flex-1 truncate">{label}</span>
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  )}
                </div>
              )}
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
      </div>
    </main>
  );
}

export default UserSettingsPage;
