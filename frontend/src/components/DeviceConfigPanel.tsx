import { useEffect, useRef, useState } from 'react';
import type { Device } from '../types/api';
import { deleteDevice, fetchSettings, updateDevice, updateSetting } from '../api/client';

const POLLING_PRESETS = [
  { label: 'Use Global', value: 'global' },
  { label: '15 seconds', value: '15' },
  { label: '30 seconds', value: '30' },
  { label: '60 seconds', value: '60' },
  { label: '2 minutes', value: '120' },
  { label: '5 minutes', value: '300' },
  { label: 'Custom...', value: 'custom' },
];

const PRESET_VALUES = new Set(
  POLLING_PRESETS.map((p) => p.value).filter((v) => v !== 'custom' && v !== 'global'),
);

interface DeviceConfigPanelProps {
  device: Device;
  onDeviceUpdated: () => void;
  onDeviceDeleted: () => void;
}

export function DeviceConfigPanel({ device, onDeviceUpdated, onDeviceDeleted }: DeviceConfigPanelProps) {
  const pollingKey = `polling_interval_seconds:${device.id}`;
  const grafanaKey = `grafana_dashboard_url:${device.id}`;

  const [pollingValue, setPollingValue] = useState('global');
  const [customPolling, setCustomPolling] = useState('');
  const [grafanaUrl, setGrafanaUrl] = useState('');

  const [hostname, setHostname] = useState(device.hostname);
  const [ip, setIp] = useState(device.ip);
  const [community, setCommunity] = useState('');
  const [editLoading, setEditLoading] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);
  const [editSaved, setEditSaved] = useState(false);

  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const [savedPolling, setSavedPolling] = useState(false);
  const [savedGrafana, setSavedGrafana] = useState(false);

  const pollingTimerRef = useRef<number | null>(null);
  const grafanaTimerRef = useRef<number | null>(null);
  const savedPollingTimerRef = useRef<number | null>(null);
  const savedGrafanaTimerRef = useRef<number | null>(null);
  const editSavedTimerRef = useRef<number | null>(null);

  useEffect(() => {
    fetchSettings()
      .then((settings) => {
        const devicePolling = settings[pollingKey];
        if (!devicePolling) {
          setPollingValue('global');
        } else if (PRESET_VALUES.has(devicePolling)) {
          setPollingValue(devicePolling);
        } else {
          setPollingValue('custom');
          setCustomPolling(devicePolling);
        }
        setGrafanaUrl(settings[grafanaKey] ?? '');
      })
      .catch(() => {/* non-fatal */});
  }, [device.id, pollingKey, grafanaKey]);

  function showSaved(
    setter: React.Dispatch<React.SetStateAction<boolean>>,
    timerRef: React.MutableRefObject<number | null>,
  ) {
    setter(true);
    if (timerRef.current !== null) window.clearTimeout(timerRef.current);
    timerRef.current = window.setTimeout(() => setter(false), 2000);
  }

  function schedulePollingUpdate(rawValue: string, isDelete = false) {
    if (pollingTimerRef.current !== null) window.clearTimeout(pollingTimerRef.current);
    pollingTimerRef.current = window.setTimeout(() => {
      const val = isDelete ? '' : rawValue;
      void updateSetting(pollingKey, val).then(() =>
        showSaved(setSavedPolling, savedPollingTimerRef),
      );
    }, 500);
  }

  function handlePollingChange(value: string) {
    setPollingValue(value);
    if (value === 'global') {
      schedulePollingUpdate('', true);
    } else if (value !== 'custom') {
      schedulePollingUpdate(value);
    }
  }

  function scheduleGrafanaUpdate(value: string) {
    if (grafanaTimerRef.current !== null) window.clearTimeout(grafanaTimerRef.current);
    grafanaTimerRef.current = window.setTimeout(() => {
      void updateSetting(grafanaKey, value).then(() =>
        showSaved(setSavedGrafana, savedGrafanaTimerRef),
      );
    }, 500);
  }

  async function handleEditSave(e: React.FormEvent) {
    e.preventDefault();
    setEditLoading(true);
    setEditError(null);
    try {
      await updateDevice(device.id, {
        hostname: hostname.trim(),
        ip: ip.trim(),
        snmp_community: community.trim() || undefined,
      });
      showSaved(setEditSaved, editSavedTimerRef);
      onDeviceUpdated();
    } catch (err) {
      setEditError(err instanceof Error ? err.message : 'Failed to update device.');
    } finally {
      setEditLoading(false);
    }
  }

  async function handleDelete() {
    setDeleteLoading(true);
    try {
      await deleteDevice(device.id);
      onDeviceDeleted();
    } catch {
      setDeleteLoading(false);
      setConfirmDelete(false);
    }
  }

  return (
    <div className="space-y-6 p-4">
      {/* Polling Override */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase tracking-widest text-text-secondary">
            Polling Override
          </p>
          <span
            className={`text-xs text-status-up transition-opacity duration-500 ${savedPolling ? 'opacity-100' : 'opacity-0'}`}
          >
            Saved
          </span>
        </div>
        <select
          value={pollingValue}
          onChange={(e) => handlePollingChange(e.target.value)}
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
        >
          {POLLING_PRESETS.map((p) => (
            <option key={p.value} value={p.value}>
              {p.label}
            </option>
          ))}
        </select>
        {pollingValue === 'custom' && (
          <input
            type="number"
            min={5}
            max={3600}
            value={customPolling}
            placeholder="Seconds (5–3600)"
            onChange={(e) => {
              setCustomPolling(e.target.value);
              schedulePollingUpdate(e.target.value);
            }}
            className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
          />
        )}
      </div>

      {/* Custom Grafana URL */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase tracking-widest text-text-secondary">
            Custom Grafana Dashboard URL
          </p>
          <span
            className={`text-xs text-status-up transition-opacity duration-500 ${savedGrafana ? 'opacity-100' : 'opacity-0'}`}
          >
            Saved
          </span>
        </div>
        <input
          type="url"
          value={grafanaUrl}
          placeholder="Leave blank to use default"
          onChange={(e) => {
            setGrafanaUrl(e.target.value);
            scheduleGrafanaUpdate(e.target.value);
          }}
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
        />
      </div>

      {/* Edit Device */}
      <form onSubmit={(e) => { void handleEditSave(e); }} className="space-y-3">
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase tracking-widest text-text-secondary">
            Edit Device
          </p>
          <span
            className={`text-xs text-status-up transition-opacity duration-500 ${editSaved ? 'opacity-100' : 'opacity-0'}`}
          >
            Saved
          </span>
        </div>

        <input
          type="text"
          value={hostname}
          onChange={(e) => setHostname(e.target.value)}
          placeholder="Hostname"
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
        />
        <input
          type="text"
          value={ip}
          onChange={(e) => setIp(e.target.value)}
          placeholder="IP Address"
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
        />
        <input
          type="text"
          value={community}
          onChange={(e) => setCommunity(e.target.value)}
          placeholder="SNMP Community (leave blank to keep current)"
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
        />

        {editError && (
          <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
            {editError}
          </p>
        )}

        <button
          type="submit"
          disabled={editLoading}
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-4 py-2 text-sm font-medium text-text-primary transition-colors hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-50"
        >
          {editLoading ? 'Saving...' : 'Save Changes'}
        </button>
      </form>

      {/* Delete Device */}
      <div className="border-t border-border-subtle pt-4 space-y-3">
        {!confirmDelete ? (
          <button
            type="button"
            onClick={() => setConfirmDelete(true)}
            className="w-full rounded-lg border border-status-down/30 bg-status-down/10 px-4 py-2 text-sm font-medium text-status-down transition-colors hover:bg-status-down/20"
          >
            Delete Device
          </button>
        ) : (
          <div className="space-y-2 rounded-lg border border-status-down/30 bg-status-down/10 p-3">
            <p className="text-sm text-status-down">Are you sure? This cannot be undone.</p>
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
                disabled={deleteLoading}
                onClick={() => { void handleDelete(); }}
                className="flex-1 rounded-lg bg-status-down px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {deleteLoading ? 'Deleting...' : 'Confirm Delete'}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
