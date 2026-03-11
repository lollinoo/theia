import { useState } from 'react';
import { createDevice } from '../api/client';

interface AddDevicePanelProps {
  onDeviceAdded: () => void;
}

export function AddDevicePanel({ onDeviceAdded }: AddDevicePanelProps) {
  const [hostname, setHostname] = useState('');
  const [version, setVersion] = useState('2c');
  const [displayName, setDisplayName] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // v2c
  const [community, setCommunity] = useState('public');

  // v3
  const [username, setUsername] = useState('');
  const [securityLevel, setSecurityLevel] = useState('authPriv');
  const [authProtocol, setAuthProtocol] = useState('SHA');
  const [authPassword, setAuthPassword] = useState('');
  const [privProtocol, setPrivProtocol] = useState('AES');
  const [privPassword, setPrivPassword] = useState('');

  const isV3 = version === '3';
  const needsAuth = securityLevel === 'authNoPriv' || securityLevel === 'authPriv';
  const needsPriv = securityLevel === 'authPriv';

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!hostname.trim()) {
      setError('Hostname or IP is required.');
      return;
    }
    setLoading(true);
    setError(null);
    try {
      await createDevice({
        hostname: hostname.trim(),
        ip: hostname.trim(),
        snmp: isV3
          ? {
              version: '3',
              username: username.trim(),
              security_level: securityLevel,
              ...(needsAuth ? { auth_protocol: authProtocol, auth_password: authPassword } : {}),
              ...(needsPriv ? { priv_protocol: privProtocol, priv_password: privPassword } : {}),
            }
          : {
              version: version,
              community: community.trim() || 'public',
            },
        tags: displayName.trim() ? { display_name: displayName.trim() } : undefined,
      });
      onDeviceAdded();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add device.');
    } finally {
      setLoading(false);
    }
  }

  const inputClass =
    'w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none';
  const selectClass =
    'w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none';
  const labelClass = 'text-xs font-medium uppercase tracking-widest text-text-secondary';

  return (
    <form onSubmit={(e) => { void handleSubmit(e); }} className="space-y-4 p-4">
      <div className="space-y-2">
        <label className={labelClass}>
          Hostname / IP <span className="text-status-down">*</span>
        </label>
        <input
          type="text"
          value={hostname}
          onChange={(e) => setHostname(e.target.value)}
          placeholder="192.168.1.1 or router.local"
          required
          className={inputClass}
        />
      </div>

      <div className="space-y-2">
        <label className={labelClass}>SNMP Version</label>
        <select
          value={version}
          onChange={(e) => setVersion(e.target.value)}
          className={selectClass}
        >
          <option value="2c">v2c</option>
          <option value="3">v3</option>
        </select>
      </div>

      {!isV3 && (
        <div className="space-y-2">
          <label className={labelClass}>SNMP Community</label>
          <input
            type="text"
            value={community}
            onChange={(e) => setCommunity(e.target.value)}
            placeholder="public"
            className={inputClass}
          />
        </div>
      )}

      {isV3 && (
        <div className="space-y-3 rounded-lg border border-border-subtle p-3">
          <p className={labelClass}>SNMPv3 Credentials</p>

          <div className="space-y-2">
            <label className="text-xs text-text-secondary">Username</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="snmpv3user"
              className={inputClass}
            />
          </div>

          <div className="space-y-2">
            <label className="text-xs text-text-secondary">Security Level</label>
            <select
              value={securityLevel}
              onChange={(e) => setSecurityLevel(e.target.value)}
              className={selectClass}
            >
              <option value="noAuthNoPriv">No Auth, No Privacy</option>
              <option value="authNoPriv">Auth, No Privacy</option>
              <option value="authPriv">Auth + Privacy</option>
            </select>
          </div>

          {needsAuth && (
            <>
              <div className="space-y-2">
                <label className="text-xs text-text-secondary">Auth Protocol</label>
                <select
                  value={authProtocol}
                  onChange={(e) => setAuthProtocol(e.target.value)}
                  className={selectClass}
                >
                  <option value="SHA">SHA</option>
                  <option value="MD5">MD5</option>
                </select>
              </div>
              <div className="space-y-2">
                <label className="text-xs text-text-secondary">Auth Key</label>
                <input
                  type="password"
                  value={authPassword}
                  onChange={(e) => setAuthPassword(e.target.value)}
                  placeholder="Authentication passphrase"
                  autoComplete="new-password"
                  className={inputClass}
                />
              </div>
            </>
          )}

          {needsPriv && (
            <>
              <div className="space-y-2">
                <label className="text-xs text-text-secondary">Encryption Protocol</label>
                <select
                  value={privProtocol}
                  onChange={(e) => setPrivProtocol(e.target.value)}
                  className={selectClass}
                >
                  <option value="AES">AES</option>
                  <option value="DES">DES</option>
                </select>
              </div>
              <div className="space-y-2">
                <label className="text-xs text-text-secondary">Encryption Key</label>
                <input
                  type="password"
                  value={privPassword}
                  onChange={(e) => setPrivPassword(e.target.value)}
                  placeholder="Privacy passphrase"
                  autoComplete="new-password"
                  className={inputClass}
                />
              </div>
            </>
          )}
        </div>
      )}

      <div className="space-y-2">
        <label className={labelClass}>
          Display Name <span className="text-text-secondary/50">(optional)</span>
        </label>
        <input
          type="text"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          placeholder="My Router"
          className={inputClass}
        />
      </div>

      {error && (
        <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={loading}
        className="w-full rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {loading ? 'Adding...' : 'Add Device'}
      </button>
    </form>
  );
}
