import { useEffect, useState } from 'react';
import { fetchDeviceInterfaces, createLink } from '../api/client';
import type { Device, InterfaceInfo } from '../types/api';

interface LinkCreatePanelProps {
  devices: Device[];
  onCreated: () => void;
  onClose: () => void;
}

function formatSpeed(bps: number): string {
  if (bps <= 0) return '';
  if (bps >= 1_000_000_000) return `${(bps / 1_000_000_000).toFixed(0)}G`;
  if (bps >= 1_000_000) return `${(bps / 1_000_000).toFixed(0)}M`;
  if (bps >= 1_000) return `${(bps / 1_000).toFixed(0)}K`;
  return `${bps}`;
}

function InterfaceSelect({
  label,
  interfaces,
  value,
  onChange,
  loading,
  placeholder,
}: {
  label: string;
  interfaces: InterfaceInfo[];
  value: string;
  onChange: (v: string) => void;
  loading: boolean;
  placeholder: string;
}) {
  const upInterfaces = interfaces.filter((i) => i.oper_status === 'up');
  const downInterfaces = interfaces.filter((i) => i.oper_status !== 'up');

  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">
        {label}
      </label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={loading || interfaces.length === 0}
        className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
      >
        <option value="">{loading ? 'Loading...' : placeholder}</option>
        {upInterfaces.map((iface) => (
          <option key={iface.if_name} value={iface.if_name}>
            {iface.if_name}
            {formatSpeed(iface.speed) ? `  ${formatSpeed(iface.speed)}` : ''}
            {'  '}up
            {iface.in_use ? `  (in use${iface.in_use_by ? ` by ${iface.in_use_by}` : ''})` : ''}
          </option>
        ))}
        {downInterfaces.length > 0 && upInterfaces.length > 0 && (
          <option disabled value="">
            ── down ──
          </option>
        )}
        {downInterfaces.map((iface) => (
          <option key={iface.if_name} value={iface.if_name} style={{ color: '#666' }}>
            {iface.if_name}
            {formatSpeed(iface.speed) ? `  ${formatSpeed(iface.speed)}` : ''}
            {'  '}down
            {iface.in_use ? `  (in use${iface.in_use_by ? ` by ${iface.in_use_by}` : ''})` : ''}
          </option>
        ))}
      </select>
    </div>
  );
}

export function LinkCreatePanel({ devices, onCreated, onClose }: LinkCreatePanelProps) {
  const [sourceDeviceId, setSourceDeviceId] = useState('');
  const [targetDeviceId, setTargetDeviceId] = useState('');
  const [sourceIfName, setSourceIfName] = useState('');
  const [targetIfName, setTargetIfName] = useState('');
  const [sourceInterfaces, setSourceInterfaces] = useState<InterfaceInfo[]>([]);
  const [targetInterfaces, setTargetInterfaces] = useState<InterfaceInfo[]>([]);
  const [sourceLoading, setSourceLoading] = useState(false);
  const [targetLoading, setTargetLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!sourceDeviceId) {
      setSourceInterfaces([]);
      setSourceIfName('');
      return;
    }
    setSourceLoading(true);
    setSourceIfName('');
    fetchDeviceInterfaces(sourceDeviceId)
      .then((ifaces) => {
        setSourceInterfaces(ifaces);
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : 'Failed to fetch source interfaces');
        setSourceInterfaces([]);
      })
      .finally(() => {
        setSourceLoading(false);
      });
  }, [sourceDeviceId]);

  useEffect(() => {
    if (!targetDeviceId) {
      setTargetInterfaces([]);
      setTargetIfName('');
      return;
    }
    setTargetLoading(true);
    setTargetIfName('');
    fetchDeviceInterfaces(targetDeviceId)
      .then((ifaces) => {
        setTargetInterfaces(ifaces);
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : 'Failed to fetch target interfaces');
        setTargetInterfaces([]);
      })
      .finally(() => {
        setTargetLoading(false);
      });
  }, [targetDeviceId]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!sourceDeviceId || !targetDeviceId || !sourceIfName || !targetIfName) {
      setError('Please select source and target device and port.');
      return;
    }
    if (sourceDeviceId === targetDeviceId && sourceIfName === targetIfName) {
      setError('Source and target port cannot be the same.');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await createLink({
        source_device_id: sourceDeviceId,
        source_if_name: sourceIfName,
        target_device_id: targetDeviceId,
        target_if_name: targetIfName,
      });
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create link.');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form
      onSubmit={(e) => {
        void handleSubmit(e);
      }}
      className="space-y-5 p-4"
    >
      {/* Source device */}
      <div className="space-y-3">
        <p className="text-xs font-medium uppercase tracking-widest text-text-secondary">
          Source
        </p>
        <div className="space-y-1.5">
          <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">
            Device
          </label>
          <select
            value={sourceDeviceId}
            onChange={(e) => setSourceDeviceId(e.target.value)}
            className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
          >
            <option value="">Select device...</option>
            {devices.map((d) => (
              <option key={d.id} value={d.id}>
                {d.tags?.display_name || d.hostname}
              </option>
            ))}
          </select>
        </div>
        <InterfaceSelect
          label="Port"
          interfaces={sourceInterfaces}
          value={sourceIfName}
          onChange={setSourceIfName}
          loading={sourceLoading}
          placeholder={sourceDeviceId ? 'Select port...' : 'Select a device first'}
        />
      </div>

      <div className="border-t border-border-subtle" />

      {/* Target device */}
      <div className="space-y-3">
        <p className="text-xs font-medium uppercase tracking-widest text-text-secondary">
          Target
        </p>
        <div className="space-y-1.5">
          <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">
            Device
          </label>
          <select
            value={targetDeviceId}
            onChange={(e) => setTargetDeviceId(e.target.value)}
            className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
          >
            <option value="">Select device...</option>
            {devices.map((d) => (
              <option key={d.id} value={d.id}>
                {d.tags?.display_name || d.hostname}
              </option>
            ))}
          </select>
        </div>
        <InterfaceSelect
          label="Port"
          interfaces={targetInterfaces}
          value={targetIfName}
          onChange={setTargetIfName}
          loading={targetLoading}
          placeholder={targetDeviceId ? 'Select port...' : 'Select a device first'}
        />
      </div>

      {error && (
        <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
          {error}
        </p>
      )}

      <div className="flex gap-2">
        <button
          type="button"
          onClick={onClose}
          className="flex-1 rounded-lg border border-border-subtle bg-bg-elevated px-4 py-2 text-sm font-medium text-text-primary transition-colors hover:bg-bg-surface"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={submitting || !sourceDeviceId || !targetDeviceId || !sourceIfName || !targetIfName}
          className="flex-1 rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {submitting ? 'Creating...' : 'Create Link'}
        </button>
      </div>
    </form>
  );
}
