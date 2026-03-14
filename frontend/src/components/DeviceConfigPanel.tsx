import { useEffect, useRef, useState } from 'react';
import type { Device, SNMPProfile } from '../types/api';
import { checkPrometheusHealth, deleteDevice, fetchSettings, fetchSNMPProfiles, updateDevice, updateSetting } from '../api/client';

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
  onDeviceUpdated: (updated: Device) => void;
  onDeviceDeleted: () => void;
}

export function DeviceConfigPanel({ device, onDeviceUpdated, onDeviceDeleted }: DeviceConfigPanelProps) {
  const pollingKey = `polling_interval_seconds:${device.id}`;
  const grafanaKey = `grafana_dashboard_url:${device.id}`;

  const [pollingValue, setPollingValue] = useState('global');
  const [customPolling, setCustomPolling] = useState('');
  const [grafanaUrl, setGrafanaUrl] = useState('');

  const [hostname, setHostname] = useState(device.hostname);
  const [displayName, setDisplayName] = useState(device.tags?.display_name || '');
  const [ip, setIp] = useState(device.ip);
  const [snmpVersion, setSnmpVersion] = useState('2c');
  const [community, setCommunity] = useState('');
  // SNMPv3 fields
  const [username, setUsername] = useState('');
  const [securityLevel, setSecurityLevel] = useState('authPriv');
  const [authProtocol, setAuthProtocol] = useState('SHA');
  const [authPassword, setAuthPassword] = useState('');
  const [privProtocol, setPrivProtocol] = useState('AES');
  const [privPassword, setPrivPassword] = useState('');
  // Metrics source
  const [metricsSource, setMetricsSource] = useState<'prometheus' | 'snmp' | 'prometheus_snmp_fallback'>(
    (device.metrics_source as 'prometheus' | 'snmp' | 'prometheus_snmp_fallback') || 'snmp',
  );
  const [prometheusLabelName, setPrometheusLabelName] = useState(device.prometheus_label_name || 'instance');
  const [prometheusLabelValue, setPrometheusLabelValue] = useState(device.prometheus_label_value || '');
  const [editLoading, setEditLoading] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);
  const [editSaved, setEditSaved] = useState(false);

  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const [profiles, setProfiles] = useState<SNMPProfile[]>([]);
  const [prometheusAvailable, setPrometheusAvailable] = useState<boolean | null>(null);

  const [savedPolling, setSavedPolling] = useState(false);
  const [savedGrafana, setSavedGrafana] = useState(false);

  const pollingTimerRef = useRef<number | null>(null);
  const grafanaTimerRef = useRef<number | null>(null);
  const savedPollingTimerRef = useRef<number | null>(null);
  const savedGrafanaTimerRef = useRef<number | null>(null);
  const editSavedTimerRef = useRef<number | null>(null);

  useEffect(() => {
    fetchSNMPProfiles().then(setProfiles).catch(() => {/* non-fatal */});
    checkPrometheusHealth().then((result) => {
      setPrometheusAvailable(result.available);
    }).catch(() => {
      setPrometheusAvailable(false);
    });
  }, []);

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
      .catch(() => {/* non-fatal */ });
  }, [device.id, pollingKey, grafanaKey]);

  // Sync inputs when the `device` prop updates from parent
  useEffect(() => {
    setHostname(device.hostname || '');
    setDisplayName(device.tags?.display_name || '');
    setIp(device.ip || '');
    setCommunity(''); // We don't fetch credentials back from the API for security
    setUsername('');
    setAuthPassword('');
    setPrivPassword('');
    setMetricsSource((device.metrics_source as 'prometheus' | 'snmp' | 'prometheus_snmp_fallback') || 'snmp');
    setPrometheusLabelName(device.prometheus_label_name || 'instance');
    setPrometheusLabelValue(device.prometheus_label_value || '');
  }, [device]);

  function applyProfile(profileId: string) {
    const profile = profiles.find((p) => p.id === profileId);
    if (!profile) return;
    setSnmpVersion(profile.snmp.version);
    setCommunity(profile.snmp.community ?? '');
    setUsername(profile.snmp.username ?? '');
    setSecurityLevel(profile.snmp.security_level ?? 'authPriv');
    setAuthProtocol(profile.snmp.auth_protocol ?? 'SHA');
    setAuthPassword(profile.snmp.auth_password ?? '');
    setPrivProtocol(profile.snmp.priv_protocol ?? 'AES');
    setPrivPassword(profile.snmp.priv_password ?? '');
  }

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
    const isV3 = snmpVersion === '3';
    const needsAuth = securityLevel === 'authNoPriv' || securityLevel === 'authPriv';
    const needsPriv = securityLevel === 'authPriv';
    const hasSnmpChanges = isV3 ? username.trim() !== '' : community.trim() !== '';
    try {
      const usesPrometheus = metricsSource === 'prometheus' || metricsSource === 'prometheus_snmp_fallback';
      const effectiveLabelValue = prometheusLabelValue.trim() || ip.trim();
      const updated = await updateDevice(device.id, {
        hostname: hostname.trim(),
        ip: ip.trim(),
        ...(hasSnmpChanges
          ? {
              snmp: isV3
                ? {
                    version: '3',
                    username: username.trim(),
                    security_level: securityLevel,
                    ...(needsAuth ? { auth_protocol: authProtocol, auth_password: authPassword } : {}),
                    ...(needsPriv ? { priv_protocol: privProtocol, priv_password: privPassword } : {}),
                  }
                : { version: '2c', community: community.trim() },
            }
          : {}),
        tags: { ...device.tags, ...(displayName.trim() ? { display_name: displayName.trim() } : {}) },
        metrics_source: metricsSource,
        prometheus_label_name: usesPrometheus ? prometheusLabelName : undefined,
        prometheus_label_value: usesPrometheus ? effectiveLabelValue : undefined,
      });
      showSaved(setEditSaved, editSavedTimerRef);
      onDeviceUpdated(updated);
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

        {device.sys_name && (
          <div className="rounded-lg border border-border-subtle bg-bg-elevated/40 px-3 py-2">
            <p className="text-[10px] uppercase tracking-widest text-text-secondary/60 mb-0.5">Auto-discovered Hostname</p>
            <p className="text-sm font-mono text-text-primary">{device.sys_name}</p>
          </div>
        )}

        <input
          type="text"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          placeholder={device.sys_name ? `Override "${device.sys_name}"` : 'Custom name (optional)'}
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
        />

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
        {profiles.length > 0 && (
          <select
            defaultValue=""
            onChange={(e) => { applyProfile(e.target.value); e.target.value = ''; }}
            className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
          >
            <option value="" disabled>Load credentials from profile...</option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} (SNMP {p.snmp.version})
              </option>
            ))}
          </select>
        )}

        <select
          value={snmpVersion}
          onChange={(e) => setSnmpVersion(e.target.value)}
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
        >
          <option value="2c">SNMP v2c</option>
          <option value="3">SNMP v3</option>
        </select>

        {snmpVersion !== '3' && (
          <input
            type="text"
            value={community}
            onChange={(e) => setCommunity(e.target.value)}
            placeholder="SNMP Community (leave blank to keep current)"
            className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
          />
        )}

        {snmpVersion === '3' && (
          <div className="space-y-2 rounded-lg border border-border-subtle p-3">
            <p className="text-xs text-text-secondary">SNMPv3 Credentials (leave blank to keep current)</p>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="Username"
              className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
            />
            <select
              value={securityLevel}
              onChange={(e) => setSecurityLevel(e.target.value)}
              className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
            >
              <option value="noAuthNoPriv">No Auth, No Privacy</option>
              <option value="authNoPriv">Auth, No Privacy</option>
              <option value="authPriv">Auth + Privacy</option>
            </select>
            {(securityLevel === 'authNoPriv' || securityLevel === 'authPriv') && (
              <>
                <select
                  value={authProtocol}
                  onChange={(e) => setAuthProtocol(e.target.value)}
                  className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
                >
                  <option value="SHA">SHA</option>
                  <option value="MD5">MD5</option>
                </select>
                <input
                  type="password"
                  value={authPassword}
                  onChange={(e) => setAuthPassword(e.target.value)}
                  placeholder="Auth Key"
                  autoComplete="new-password"
                  className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
                />
              </>
            )}
            {securityLevel === 'authPriv' && (
              <>
                <select
                  value={privProtocol}
                  onChange={(e) => setPrivProtocol(e.target.value)}
                  className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
                >
                  <option value="AES">AES</option>
                  <option value="DES">DES</option>
                </select>
                <input
                  type="password"
                  value={privPassword}
                  onChange={(e) => setPrivPassword(e.target.value)}
                  placeholder="Encryption Key"
                  autoComplete="new-password"
                  className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
                />
              </>
            )}
          </div>
        )}

        {prometheusAvailable === false && (
          <p className="rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-xs text-yellow-400">
            Prometheus is not configured or unreachable. Only SNMP Direct is available.
          </p>
        )}

        <div className="space-y-1">
          <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">Metrics Source</label>
          <select
            value={metricsSource}
            onChange={(e) => {
              const val = e.target.value as 'prometheus' | 'snmp' | 'prometheus_snmp_fallback';
              if ((val === 'prometheus' || val === 'prometheus_snmp_fallback') && !prometheusAvailable) return;
              setMetricsSource(val);
            }}
            className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
          >
            <option value="snmp">SNMP Direct</option>
            <option value="prometheus" disabled={!prometheusAvailable}>
              Prometheus{!prometheusAvailable ? ' (unavailable)' : ''}
            </option>
            <option value="prometheus_snmp_fallback" disabled={!prometheusAvailable}>
              Prometheus + SNMP Fallback{!prometheusAvailable ? ' (unavailable)' : ''}
            </option>
          </select>
          {metricsSource === 'prometheus_snmp_fallback' && (
            <p className="text-xs text-text-secondary/70">
              Falls back to SNMP if Prometheus is unavailable or has no data for this device.
            </p>
          )}
        </div>

        {(metricsSource === 'prometheus' || metricsSource === 'prometheus_snmp_fallback') && (
          <div className="space-y-2 rounded-lg border border-border-subtle p-3">
            <p className="text-xs font-medium uppercase tracking-widest text-text-secondary">Prometheus Target</p>
            <div className="space-y-1">
              <label className="text-xs text-text-secondary">Label</label>
              <select
                value={prometheusLabelName}
                onChange={(e) => setPrometheusLabelName(e.target.value)}
                className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
              >
                <option value="instance">instance (IP address)</option>
                <option value="identity">identity</option>
                <option value="vendor">vendor</option>
              </select>
            </div>
            <div className="space-y-1">
              <label className="text-xs text-text-secondary">
                Value{prometheusLabelName === 'instance' ? ' (defaults to IP if blank)' : ''}
              </label>
              <input
                type="text"
                value={prometheusLabelValue}
                onChange={(e) => setPrometheusLabelValue(e.target.value)}
                placeholder={prometheusLabelName === 'instance' ? ip || device.ip : 'e.g. my-router'}
                className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
              />
            </div>
          </div>
        )}

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
