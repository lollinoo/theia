import { useEffect, useState } from 'react';
import { fetchDeviceInterfaces, updateLink, deleteLink } from '../api/client';
import type { Device, InterfaceInfo, Link } from '../types/api';

interface LinkDetailsPanelProps {
  link: Link;
  devices: Device[];
  onUpdated: () => void;
  onDeleted: () => void;
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
}: {
  label: string;
  interfaces: InterfaceInfo[];
  value: string;
  onChange: (v: string) => void;
  loading: boolean;
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
        disabled={loading}
        className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
      >
        <option value="">{loading ? 'Loading...' : 'Select port...'}</option>
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

export function LinkDetailsPanel({
  link,
  devices,
  onUpdated,
  onDeleted,
  onClose: _onClose,
}: LinkDetailsPanelProps) {
  const deviceMap = new Map(devices.map((d) => [d.id, d]));
  const sourceDevice = deviceMap.get(link.source_device_id);
  const targetDevice = deviceMap.get(link.target_device_id);

  const [editing, setEditing] = useState(false);
  const [sourceIfName, setSourceIfName] = useState(link.source_if_name);
  const [targetIfName, setTargetIfName] = useState(link.target_if_name);
  const [sourceInterfaces, setSourceInterfaces] = useState<InterfaceInfo[]>([]);
  const [targetInterfaces, setTargetInterfaces] = useState<InterfaceInfo[]>([]);
  const [sourceLoading, setSourceLoading] = useState(false);
  const [targetLoading, setTargetLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);

  // Load interfaces when entering edit mode
  useEffect(() => {
    if (!editing) return;

    if (link.source_device_id) {
      setSourceLoading(true);
      fetchDeviceInterfaces(link.source_device_id)
        .then((ifaces) => setSourceInterfaces(ifaces))
        .catch(() => setSourceInterfaces([]))
        .finally(() => setSourceLoading(false));
    }

    if (link.target_device_id) {
      setTargetLoading(true);
      fetchDeviceInterfaces(link.target_device_id)
        .then((ifaces) => setTargetInterfaces(ifaces))
        .catch(() => setTargetInterfaces([]))
        .finally(() => setTargetLoading(false));
    }
  }, [editing, link.source_device_id, link.target_device_id]);

  // Reset edit state when link changes
  useEffect(() => {
    setEditing(false);
    setSourceIfName(link.source_if_name);
    setTargetIfName(link.target_if_name);
    setSaveError(null);
    setConfirmDelete(false);
  }, [link.id, link.source_if_name, link.target_if_name]);

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setSaveError(null);
    try {
      await updateLink(link.id, {
        source_if_name: sourceIfName,
        target_if_name: targetIfName,
      });
      onUpdated();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to update link.');
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    setDeleting(true);
    try {
      await deleteLink(link.id);
      onDeleted();
    } catch {
      setDeleting(false);
      setConfirmDelete(false);
    }
  }

  const protocolBadgeColor =
    link.discovery_protocol === 'lldp'
      ? 'bg-accent/20 text-accent border-accent/30'
      : link.discovery_protocol === 'cdp'
        ? 'bg-status-up/20 text-status-up border-status-up/30'
        : 'bg-bg-elevated text-text-secondary border-border-subtle';

  return (
    <div className="space-y-5 p-4">
      {/* Link summary */}
      <div className="rounded-lg border border-border-subtle bg-bg-elevated p-3 space-y-2">
        <div className="space-y-0.5">
          <p className="text-sm font-medium text-text-primary truncate">
            {sourceDevice?.tags?.display_name || sourceDevice?.hostname || link.source_device_id}
            <span className="text-text-secondary font-normal">:{link.source_if_name || '—'}</span>
          </p>
          <p className="text-xs text-text-secondary px-1">↕</p>
          <p className="text-sm font-medium text-text-primary truncate">
            {targetDevice?.tags?.display_name || targetDevice?.hostname || link.target_device_id}
            <span className="text-text-secondary font-normal">:{link.target_if_name || '—'}</span>
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span
            className={`inline-block rounded border px-2 py-0.5 text-xs font-medium ${protocolBadgeColor}`}
          >
            {link.discovery_protocol || 'manual'}
          </span>
        </div>
      </div>

      {!editing ? (
        /* View mode */
        <button
          type="button"
          onClick={() => setEditing(true)}
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-4 py-2 text-sm font-medium text-text-primary transition-colors hover:bg-bg-surface"
        >
          Edit Ports
        </button>
      ) : (
        /* Edit mode */
        <form
          onSubmit={(e) => {
            void handleSave(e);
          }}
          className="space-y-4"
        >
          <p className="text-xs font-medium uppercase tracking-widest text-text-secondary">
            Edit Port Assignments
          </p>

          <div className="space-y-1">
            <p className="text-xs text-text-secondary">
              Source: {sourceDevice?.tags?.display_name || sourceDevice?.hostname || link.source_device_id}
            </p>
            <InterfaceSelect
              label="Source Port"
              interfaces={sourceInterfaces}
              value={sourceIfName}
              onChange={setSourceIfName}
              loading={sourceLoading}
            />
          </div>

          <div className="space-y-1">
            <p className="text-xs text-text-secondary">
              Target: {targetDevice?.tags?.display_name || targetDevice?.hostname || link.target_device_id}
            </p>
            <InterfaceSelect
              label="Target Port"
              interfaces={targetInterfaces}
              value={targetIfName}
              onChange={setTargetIfName}
              loading={targetLoading}
            />
          </div>

          {saveError && (
            <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
              {saveError}
            </p>
          )}

          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => {
                setEditing(false);
                setSourceIfName(link.source_if_name);
                setTargetIfName(link.target_if_name);
                setSaveError(null);
              }}
              className="flex-1 rounded-lg border border-border-subtle bg-bg-elevated px-4 py-2 text-sm font-medium text-text-primary transition-colors hover:bg-bg-surface"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={saving || !sourceIfName || !targetIfName}
              className="flex-1 rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {saving ? 'Saving...' : 'Save'}
            </button>
          </div>
        </form>
      )}

      {/* Delete section */}
      <div className="border-t border-border-subtle pt-4 space-y-3">
        {!confirmDelete ? (
          <button
            type="button"
            onClick={() => setConfirmDelete(true)}
            className="w-full rounded-lg border border-status-down/30 bg-status-down/10 px-4 py-2 text-sm font-medium text-status-down transition-colors hover:bg-status-down/20"
          >
            Delete Link
          </button>
        ) : (
          <div className="space-y-2 rounded-lg border border-status-down/30 bg-status-down/10 p-3">
            <p className="text-sm text-status-down">Delete this link?</p>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setConfirmDelete(false)}
                className="flex-1 rounded-lg border border-border-subtle bg-bg-elevated px-3 py-1.5 text-xs text-text-primary hover:bg-bg-surface"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={deleting}
                onClick={() => {
                  void handleDelete();
                }}
                className="flex-1 rounded-lg bg-status-down px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {deleting ? 'Deleting...' : 'Confirm Delete'}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
