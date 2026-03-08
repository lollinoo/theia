import { useState } from 'react';
import { createDevice } from '../api/client';

interface AddDevicePanelProps {
  onDeviceAdded: () => void;
}

export function AddDevicePanel({ onDeviceAdded }: AddDevicePanelProps) {
  const [hostname, setHostname] = useState('');
  const [community, setCommunity] = useState('public');
  const [version, setVersion] = useState('v2c');
  const [displayName, setDisplayName] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
        snmp_community: community.trim() || 'public',
        snmp_version: version,
        display_name: displayName.trim() || undefined,
      });
      onDeviceAdded();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add device.');
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={(e) => { void handleSubmit(e); }} className="space-y-4 p-4">
      <div className="space-y-2">
        <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">
          Hostname / IP <span className="text-status-down">*</span>
        </label>
        <input
          type="text"
          value={hostname}
          onChange={(e) => setHostname(e.target.value)}
          placeholder="192.168.1.1 or router.local"
          required
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
        />
      </div>

      <div className="space-y-2">
        <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">
          SNMP Community
        </label>
        <input
          type="text"
          value={community}
          onChange={(e) => setCommunity(e.target.value)}
          placeholder="public"
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
        />
      </div>

      <div className="space-y-2">
        <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">
          SNMP Version
        </label>
        <select
          value={version}
          onChange={(e) => setVersion(e.target.value)}
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
        >
          <option value="v2c">v2c</option>
          <option value="v3">v3</option>
        </select>
      </div>

      <div className="space-y-2">
        <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">
          Display Name <span className="text-text-secondary/50">(optional)</span>
        </label>
        <input
          type="text"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          placeholder="My Router"
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
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
