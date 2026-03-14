import { useMemo, useState } from 'react';
import { createLink } from '../api/client';
import type { Device, InterfaceInfo, Link } from '../types/api';

interface LinkCreatePanelProps {
  devices: Device[];
  links: Link[];
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

function deviceLabel(d: Device): string {
  const name = d.tags?.display_name || d.sys_name;
  return name ? `${d.ip} — ${name}` : d.ip;
}

function getDeviceInterfaces(
  device: Device | undefined,
  deviceId: string,
  links: Link[],
): InterfaceInfo[] {
  if (!device || !device.interfaces?.length) return [];

  const inUseIfaces = new Set<string>();
  for (const link of links) {
    if (link.source_device_id === deviceId) inUseIfaces.add(link.source_if_name);
    if (link.target_device_id === deviceId) inUseIfaces.add(link.target_if_name);
  }

  return device.interfaces
    .filter((i) => {
      if (!i.if_name) return false;
      const lower = i.if_name.toLowerCase();
      return !lower.startsWith('lo') && lower !== 'null' && !lower.startsWith('null');
    })
    .sort((a, b) => {
      const aUp = a.oper_status === 'up';
      const bUp = b.oper_status === 'up';
      if (aUp !== bUp) return aUp ? -1 : 1;
      return a.if_name.localeCompare(b.if_name);
    })
    .map((i) => ({
      if_name: i.if_name,
      if_descr: i.if_descr,
      speed: i.speed,
      oper_status: i.oper_status,
      admin_status: i.admin_status,
      in_use: inUseIfaces.has(i.if_name),
    }));
}

function InterfaceSelect({
  label,
  interfaces,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  interfaces: InterfaceInfo[];
  value: string;
  onChange: (v: string) => void;
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
        disabled={interfaces.length === 0}
        className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
      >
        <option value="">{placeholder}</option>
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

export function LinkCreatePanel({ devices, links, onCreated, onClose }: LinkCreatePanelProps) {
  const [sourceDeviceId, setSourceDeviceId] = useState('');
  const [targetDeviceId, setTargetDeviceId] = useState('');
  const [sourceIfName, setSourceIfName] = useState('');
  const [targetIfName, setTargetIfName] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const sourceDevice = devices.find((d) => d.id === sourceDeviceId);
  const targetDevice = devices.find((d) => d.id === targetDeviceId);

  const sourceInterfaces = useMemo(
    () => getDeviceInterfaces(sourceDevice, sourceDeviceId, links),
    [sourceDevice, sourceDeviceId, links],
  );

  const targetInterfaces = useMemo(
    () => getDeviceInterfaces(targetDevice, targetDeviceId, links),
    [targetDevice, targetDeviceId, links],
  );

  function handleSourceDeviceChange(id: string) {
    setSourceDeviceId(id);
    setSourceIfName('');
  }

  function handleTargetDeviceChange(id: string) {
    setTargetDeviceId(id);
    setTargetIfName('');
  }

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
            onChange={(e) => handleSourceDeviceChange(e.target.value)}
            className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
          >
            <option value="">Select device...</option>
            {devices.map((d) => (
              <option key={d.id} value={d.id}>
                {deviceLabel(d)}
              </option>
            ))}
          </select>
        </div>
        <InterfaceSelect
          label="Port"
          interfaces={sourceInterfaces}
          value={sourceIfName}
          onChange={setSourceIfName}
          placeholder={sourceDeviceId ? (sourceInterfaces.length === 0 ? 'No ports available yet' : 'Select port...') : 'Select a device first'}
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
            onChange={(e) => handleTargetDeviceChange(e.target.value)}
            className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
          >
            <option value="">Select device...</option>
            {devices.map((d) => (
              <option key={d.id} value={d.id}>
                {deviceLabel(d)}
              </option>
            ))}
          </select>
        </div>
        <InterfaceSelect
          label="Port"
          interfaces={targetInterfaces}
          value={targetIfName}
          onChange={setTargetIfName}
          placeholder={targetDeviceId ? (targetInterfaces.length === 0 ? 'No ports available yet' : 'Select port...') : 'Select a device first'}
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
