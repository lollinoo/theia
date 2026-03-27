import { useEffect, useMemo, useRef, useState } from 'react';
import { createLink } from '../api/client';
import type { Device, InterfaceInfo, Link } from '../types/api';

interface LinkCreatePanelProps {
  devices: Device[];
  links: Link[];
  onCreated: () => void;
  onClose: () => void;
  onRefreshDevices?: () => Promise<void>;
  initialSourceDeviceId?: string;
  initialTargetDeviceId?: string;
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

function SearchableDeviceSelect({
  devices,
  value,
  onChange,
  placeholder,
}: {
  devices: Device[];
  value: string;
  onChange: (id: string) => void;
  placeholder: string;
}) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const selectedDevice = devices.find((d) => d.id === value);

  const filtered = useMemo(() => {
    if (!search.trim()) return devices;
    const q = search.toLowerCase();
    return devices.filter((d) => {
      const label = deviceLabel(d).toLowerCase();
      return label.includes(q) || d.ip.includes(q);
    });
  }, [devices, search]);

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => {
          setOpen(!open);
          setSearch('');
          setTimeout(() => inputRef.current?.focus(), 0);
        }}
        className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-left text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
      >
        {selectedDevice ? (
          <span>
            <span className="font-mono">{selectedDevice.ip}</span>
            {(selectedDevice.tags?.display_name || selectedDevice.sys_name) && (
              <span className="ml-2 text-on-bg-secondary">
                — {selectedDevice.tags?.display_name || selectedDevice.sys_name}
              </span>
            )}
          </span>
        ) : (
          <span className="text-on-bg-secondary/40">{placeholder}</span>
        )}
      </button>
      {open && (
        <div className="absolute z-50 mt-1 w-full rounded-lg border border-outline-subtle bg-elevated shadow-lg">
          <div className="p-2">
            <input
              ref={inputRef}
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search by IP or name..."
              className="w-full rounded-md border border-outline-subtle bg-bg px-2.5 py-1.5 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
            />
          </div>
          <div className="max-h-48 overflow-y-auto">
            {filtered.length === 0 ? (
              <div className="px-3 py-2 text-xs text-on-bg-secondary">No devices found</div>
            ) : (
              filtered.map((d) => (
                <button
                  key={d.id}
                  type="button"
                  onClick={() => {
                    onChange(d.id);
                    setOpen(false);
                    setSearch('');
                  }}
                  className={`w-full px-3 py-2 text-left text-sm hover:bg-surface ${d.id === value ? 'bg-primary/10 text-primary' : 'text-on-bg'}`}
                >
                  <span className="font-mono">{d.ip}</span>
                  {(d.tags?.display_name || d.sys_name) && (
                    <span className="ml-2 text-on-bg-secondary">
                      — {d.tags?.display_name || d.sys_name}
                    </span>
                  )}
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
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
      <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
        {label}
      </label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={interfaces.length === 0}
        className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
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
          <option key={iface.if_name} value={iface.if_name} style={{ color: 'var(--nt-on-bg-muted)' }}>
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

export function LinkCreatePanel({ devices, links, onCreated, onClose, onRefreshDevices, initialSourceDeviceId, initialTargetDeviceId }: LinkCreatePanelProps) {
  const [sourceDeviceId, setSourceDeviceId] = useState(initialSourceDeviceId ?? '');
  const [targetDeviceId, setTargetDeviceId] = useState(initialTargetDeviceId ?? '');
  const [sourceIfName, setSourceIfName] = useState('');
  const [targetIfName, setTargetIfName] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

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

  async function handleRefresh() {
    if (!onRefreshDevices || refreshing) return;
    setRefreshing(true);
    try {
      await onRefreshDevices();
    } finally {
      setRefreshing(false);
    }
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
      className="space-y-5 p-4 transition-colors duration-200"
    >
      {/* Refresh button */}
      {onRefreshDevices && (
        <div className="flex justify-end">
          <button
            type="button"
            onClick={() => { void handleRefresh(); }}
            disabled={refreshing}
            className="flex items-center gap-1.5 rounded-lg bg-surface-high px-2.5 py-1.5 text-xs text-on-bg-secondary transition-colors hover:bg-elevated hover:text-on-bg disabled:opacity-50"
            title="Refresh devices & interfaces"
          >
            <svg
              viewBox="0 0 16 16"
              className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`}
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
            >
              <path d="M14 8A6 6 0 1 1 8 2" strokeLinecap="round" />
              <path d="M8 0l2.5 2L8 4" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
            {refreshing ? 'Refreshing...' : 'Refresh'}
          </button>
        </div>
      )}

      {/* Source device */}
      <div className="space-y-3">
        <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
          Source
        </p>
        <div className="space-y-1.5">
          <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Device
          </label>
          <SearchableDeviceSelect
            devices={devices}
            value={sourceDeviceId}
            onChange={handleSourceDeviceChange}
            placeholder="Select device..."
          />
        </div>
        <InterfaceSelect
          label="Port"
          interfaces={sourceInterfaces}
          value={sourceIfName}
          onChange={setSourceIfName}
          placeholder={sourceDeviceId ? (sourceInterfaces.length === 0 ? 'No ports available yet' : 'Select port...') : 'Select a device first'}
        />
      </div>

      <div className="my-4" />

      {/* Target device */}
      <div className="space-y-3">
        <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
          Target
        </p>
        <div className="space-y-1.5">
          <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Device
          </label>
          <SearchableDeviceSelect
            devices={devices}
            value={targetDeviceId}
            onChange={handleTargetDeviceChange}
            placeholder="Select device..."
          />
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
          className="flex-1 rounded-lg bg-surface-high px-4 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-elevated"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={submitting || !sourceDeviceId || !targetDeviceId || !sourceIfName || !targetIfName}
          className="flex-1 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {submitting ? 'Creating...' : 'Create Link'}
        </button>
      </div>
    </form>
  );
}
