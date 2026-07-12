/**
 * Renders settings panel UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import { useEffect, useState } from 'react';
import {
  fetchHealthRuntime,
  fetchSettingsWithMetadata,
  type HealthRuntime,
  updateSetting,
} from '../api/client';
import { useSettingAutosave } from '../hooks/useSettingAutosave';
import type { TopologyDiscoveryMode } from '../types/api';
import {
  formatTopologyDiscoveryMode,
  TOPOLOGY_DISCOVERY_DEFAULT_OPTIONS,
} from '../utils/topologyDiscovery';
import {
  validateIntervalAllowlist,
  validateRetentionCount,
  validateURL,
} from '../utils/validation';
import { CredentialProfileManager } from './CredentialProfileManager';
import { GrafanaDashboardProfileManager } from './GrafanaDashboardProfileManager';
import { InstanceBackupManager } from './InstanceBackupManager';
import { MaterialIcon } from './MaterialIcon';
import { SNMPProfileManager } from './SNMPProfileManager';
import { BridgeSettingsSection } from './settings-panel/BridgeSettingsSection';
import { DeviceBackupSettingsSection } from './settings-panel/DeviceBackupSettingsSection';
import { PollingSettingsSection } from './settings-panel/PollingSettingsSection';
import { PrometheusSettingsSection } from './settings-panel/PrometheusSettingsSection';
import { SavedIndicator } from './settings-panel/SavedIndicator';
import { SettingsSection } from './settings-panel/SettingsSection';
import { SNMPDebugSettingsSection } from './settings-panel/SNMPDebugSettingsSection';
import {
  DEFAULT_SNMP_DEBUG_SETTINGS,
  DEFAULT_WORKER_SETTINGS,
  PRESET_VALUES,
  SNMP_DEBUG_SETTINGS,
  type SNMPDebugSetting,
  type SNMPDebugSettingKey,
  WORKER_SETTINGS,
  type WorkerSetting,
  type WorkerSettingKey,
} from './settings-panel/settingsConstants';
import { controlClass, fieldLabelClass } from './settings-panel/settingsPanelStyles';

const DEFAULT_NETWORK_PROBE_PORTS = '22,8291,80,443';
const WORKER_SETTING_KEYS = new Set<string>(WORKER_SETTINGS.map((setting) => setting.key));

/** Props for the admin settings container; changes notify parents that runtime config may need refresh. */
interface SettingsPanelProps {
  onSettingsChange?: (changed?: { timezone?: string }) => void;
}

function normalizeNetworkProbePorts(value: string): { value: string; error: string | null } {
  const trimmed = value.trim();
  if (trimmed === '') {
    return { value: '', error: 'Ports must be between 1 and 65535' };
  }

  const ports: number[] = [];
  const seen = new Set<number>();
  for (const part of trimmed.split(',')) {
    const segment = part.trim();
    if (!/^\d+$/.test(segment)) {
      return { value: '', error: 'Ports must be between 1 and 65535' };
    }
    const port = Number(segment);
    if (port < 1 || port > 65535) {
      return { value: '', error: 'Ports must be between 1 and 65535' };
    }
    if (!seen.has(port)) {
      seen.add(port);
      ports.push(port);
    }
  }

  return { value: ports.join(','), error: null };
}

function isWorkerSettingKey(key: SNMPDebugSettingKey): key is WorkerSettingKey {
  return WORKER_SETTING_KEYS.has(key);
}

/**
 * Renders admin-level settings and owns fetch, validation, debounced autosave, and saved indicators.
 * Profile managers and section components handle presentation while this container persists setting keys.
 */
export function SettingsPanel({ onSettingsChange }: SettingsPanelProps) {
  const [pollingValue, setPollingValue] = useState('60');
  const [customPolling, setCustomPolling] = useState('');
  const [networkProbePorts, setNetworkProbePorts] = useState(DEFAULT_NETWORK_PROBE_PORTS);
  const [prometheusUrl, setPrometheusUrl] = useState('');
  const [timezone, setTimezone] = useState('UTC');
  const [topologyDiscoveryDefaultMode, setTopologyDiscoveryDefaultMode] =
    useState<TopologyDiscoveryMode>('lldp_cdp');
  const [runtimeInfo, setRuntimeInfo] = useState<HealthRuntime | null>(null);
  const [runtimeError, setRuntimeError] = useState(false);
  const [backupSectionOpen, setBackupSectionOpen] = useState(false);
  const [deviceBackupSectionOpen, setDeviceBackupSectionOpen] = useState(false);
  const [deviceBackupInterval, setDeviceBackupInterval] = useState('0');
  const [deviceBackupRetention, setDeviceBackupRetention] = useState('5');
  const [bridgePort, setBridgePort] = useState('1337');
  const [workerSectionOpen, setWorkerSectionOpen] = useState(false);
  const [snmpDebugSectionOpen, setSNMPDebugSectionOpen] = useState(false);
  const [workerSettings, setWorkerSettings] =
    useState<Record<WorkerSettingKey, string>>(DEFAULT_WORKER_SETTINGS);
  const [snmpDebugSettings, setSNMPDebugSettings] = useState<Record<SNMPDebugSettingKey, string>>(
    DEFAULT_SNMP_DEBUG_SETTINGS,
  );
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const autosave = useSettingAutosave(updateSetting);

  const isSaved = (key: string) => autosave.states[key]?.status === 'saved';
  const saveError = Object.values(autosave.states).find((state) => state.error)?.error;
  const savedWorkerSettings = Object.fromEntries(
    WORKER_SETTINGS.map((setting) => [setting.key, isSaved(setting.key)]),
  ) as Record<WorkerSettingKey, boolean>;
  const savedSNMPDebugSettings = Object.fromEntries(
    SNMP_DEBUG_SETTINGS.map((setting) => [setting.key, isSaved(setting.key)]),
  ) as Record<SNMPDebugSettingKey, boolean>;

  useEffect(() => {
    let active = true;
    fetchSettingsWithMetadata()
      .then(({ data: settings }) => {
        if (!active) return;
        const interval = settings['polling_interval_seconds'] ?? '60';
        if (PRESET_VALUES.has(interval)) {
          setPollingValue(interval);
        } else {
          setPollingValue('custom');
          setCustomPolling(interval);
        }
        setNetworkProbePorts(settings['network_probe_ports'] ?? DEFAULT_NETWORK_PROBE_PORTS);
        setPrometheusUrl(settings['prometheus_url'] ?? '');
        setTimezone(settings['timezone'] || 'UTC');
        setTopologyDiscoveryDefaultMode(
          (settings['topology_discovery_default_mode'] as TopologyDiscoveryMode | undefined) ??
            'lldp_cdp',
        );
        setDeviceBackupInterval(settings['device_backup_interval_hours'] ?? '0');
        setDeviceBackupRetention(settings['device_backup_retention_count'] ?? '5');
        setBridgePort(settings['bridge_port'] ?? '1337');
        setWorkerSettings((prev) => {
          const next = { ...prev };
          for (const setting of WORKER_SETTINGS) {
            next[setting.key] = settings[setting.key] ?? setting.defaultValue;
          }
          return next;
        });
        setSNMPDebugSettings((prev) => {
          const next = { ...prev };
          for (const setting of SNMP_DEBUG_SETTINGS) {
            next[setting.key] = settings[setting.key] ?? setting.defaultValue;
          }
          return next;
        });
      })
      .catch(() => {
        /* non-fatal */
      });
    fetchHealthRuntime().then(
      (runtime) => {
        if (active) setRuntimeInfo(runtime);
      },
      () => {
        if (active) setRuntimeError(true);
      },
    );
    return () => {
      active = false;
    };
  }, []);

  /** Stores validation errors by stable field key and removes entries when fields become valid. */
  function setFieldError(field: string, error: string | null) {
    setFieldErrors((prev) => {
      if (error) return { ...prev, [field]: error };
      const next = { ...prev };
      delete next[field];
      return next;
    });
  }

  /** Validates worker numeric settings before scheduling an autosave request. */
  function validateIntegerRange(value: string, min: number, max: number): string | null {
    const trimmed = value.trim();
    if (!/^\d+$/.test(trimmed)) return 'Must be a valid integer';
    const parsed = parseInt(trimmed, 10);
    if (parsed < min || parsed > max) return `Must be between ${min} and ${max}`;
    return null;
  }

  /** Debounces worker setting persistence and keeps invalid values local until corrected. */
  function handleWorkerSettingChange(key: WorkerSettingKey, value: string) {
    const setting = WORKER_SETTINGS.find((candidate) => candidate.key === key);
    if (!setting) return;
    setWorkerSettings((prev) => ({ ...prev, [key]: value }));
    setSNMPDebugSettings((prev) => ({ ...prev, [key]: value }));
    const err = validateIntegerRange(value, setting.min, setting.max);
    if (err) {
      autosave.cancel(key);
      setFieldError(key, err);
      return;
    }

    setFieldError(key, null);
    const normalized = String(parseInt(value, 10));
    autosave.save(key, normalized);
  }

  /** Debounces SNMP debug setting persistence and keeps duplicate worker controls synchronized. */
  function handleSNMPDebugSettingChange(key: SNMPDebugSettingKey, value: string) {
    const setting = SNMP_DEBUG_SETTINGS.find((candidate) => candidate.key === key);
    if (!setting) return;
    setSNMPDebugSettings((prev) => ({ ...prev, [key]: value }));
    if (isWorkerSettingKey(key)) {
      setWorkerSettings((prev) => ({ ...prev, [key]: value }));
    }

    const err = validateIntegerRange(value, setting.min, setting.max);
    if (err) {
      autosave.cancel(key);
      setFieldError(key, err);
      return;
    }

    setFieldError(key, null);
    const normalized = String(parseInt(value, 10));
    autosave.save(key, normalized);
  }

  /** Debounces polling interval persistence after enforcing the global allowed range. */
  function schedulePollingUpdate(rawValue: string) {
    const trimmed = rawValue.trim();
    const numVal = parseInt(trimmed, 10);
    if (!/^\d+$/.test(trimmed) || numVal < 5 || numVal > 3600) {
      autosave.cancel('polling_interval_seconds');
      return;
    }
    autosave.save('polling_interval_seconds', String(numVal));
  }

  /** Debounces Prometheus URL persistence while treating an empty URL as a valid disabled state. */
  function schedulePrometheusUpdate(value: string) {
    // Gate auto-save: if value is non-empty and fails URL validation, set error and skip save
    if (value.trim() !== '') {
      const err = validateURL(value, 'Prometheus URL');
      if (err) {
        autosave.cancel('prometheus_url');
        setFieldError('prometheusUrl', err);
        return;
      }
    }
    autosave.save('prometheus_url', value);
  }

  /** Persists only supported device-backup interval values so the scheduler receives known cadences. */
  function handleDeviceIntervalChange(value: string) {
    const err = validateIntervalAllowlist(value);
    if (err) {
      autosave.cancel('device_backup_interval_hours');
      setFieldError('deviceBackupInterval', err);
      setDeviceBackupInterval(value);
      return;
    }
    setFieldError('deviceBackupInterval', null);
    setDeviceBackupInterval(value);
    autosave.save('device_backup_interval_hours', value);
  }

  /** Debounces device-backup retention persistence after normalizing to an integer string. */
  function handleDeviceRetentionChange(value: string) {
    const err = validateRetentionCount(value);
    if (err) {
      autosave.cancel('device_backup_retention_count');
      setFieldError('deviceBackupRetention', err);
      setDeviceBackupRetention(value);
      return;
    }
    setFieldError('deviceBackupRetention', null);
    setDeviceBackupRetention(value);
    const num = parseInt(value, 10);
    autosave.save('device_backup_retention_count', String(num));
  }

  /** Debounces bridge port persistence and keeps invalid port text visible for correction. */
  function handleBridgePortChange(value: string) {
    setBridgePort(value);
    setFieldError('bridgePort', null);
    const trimmed = value.trim();
    const num = parseInt(trimmed, 10);
    if (!/^\d+$/.test(trimmed) || num < 1 || num > 65535) {
      autosave.cancel('bridge_port');
      setFieldError('bridgePort', 'Bridge port must be an integer between 1 and 65535');
      return;
    }
    autosave.save('bridge_port', String(num));
  }

  /** Applies a preset polling cadence immediately while custom values wait for the custom input. */
  function handlePollingPresetChange(value: string) {
    setPollingValue(value);
    if (value !== 'custom') {
      schedulePollingUpdate(value);
    } else {
      autosave.cancel('polling_interval_seconds');
    }
  }

  function handleCustomPollingChange(value: string) {
    setCustomPolling(value);
    setFieldError('customPolling', null);
    schedulePollingUpdate(value);
  }

  function handleNetworkProbePortsChange(value: string) {
    setNetworkProbePorts(value);
    setFieldError('networkProbePorts', null);
    autosave.cancel('network_probe_ports');
  }

  function handleNetworkProbePortsBlur() {
    const normalized = normalizeNetworkProbePorts(networkProbePorts);
    if (normalized.error) {
      setFieldError('networkProbePorts', normalized.error);
      return;
    }
    setFieldError('networkProbePorts', null);
    setNetworkProbePorts(normalized.value);
    autosave.save('network_probe_ports', normalized.value, { delayMs: 0 });
  }

  /** Reports custom polling validation on blur without changing the debounced save contract. */
  function handleCustomPollingBlur() {
    const trimmed = customPolling.trim();
    const num = parseInt(trimmed, 10);
    if (!/^\d+$/.test(trimmed) || num < 5 || num > 3600) {
      setFieldError('customPolling', 'Polling interval must be between 5 and 3600 seconds');
    } else {
      setFieldError('customPolling', null);
    }
  }

  function handlePrometheusChange(value: string) {
    setPrometheusUrl(value);
    setFieldError('prometheusUrl', null);
    schedulePrometheusUpdate(value);
  }

  function handlePrometheusBlur() {
    setFieldError('prometheusUrl', validateURL(prometheusUrl, 'Prometheus URL'));
  }

  /** Persists timezone immediately because the select only exposes valid IANA timezone values. */
  function handleTimezoneChange(value: string) {
    setTimezone(value);
    autosave.save('timezone', value, {
      delayMs: 0,
      onSuccess: () => onSettingsChange?.({ timezone: value }),
    });
  }

  function handleBridgePortBlur() {
    const trimmed = bridgePort.trim();
    const num = parseInt(trimmed, 10);
    if (!/^\d+$/.test(trimmed) || num < 1 || num > 65535) {
      setFieldError('bridgePort', 'Bridge port must be an integer between 1 and 65535');
    } else {
      setFieldError('bridgePort', null);
    }
  }

  function handleWorkerSettingBlur(setting: WorkerSetting) {
    setFieldError(
      setting.key,
      validateIntegerRange(workerSettings[setting.key], setting.min, setting.max),
    );
  }

  function handleSNMPDebugSettingBlur(setting: SNMPDebugSetting) {
    setFieldError(
      setting.key,
      validateIntegerRange(snmpDebugSettings[setting.key], setting.min, setting.max),
    );
  }

  return (
    <div
      data-testid="settings-panel-layout"
      className="grid min-w-0 items-start gap-6 p-4 transition-colors duration-200 lg:grid-cols-2"
    >
      <div data-testid="settings-panel-left-column" className="grid min-w-0 content-start gap-6">
        {saveError && (
          <p
            role="alert"
            className="rounded-md bg-status-down/10 px-3 py-2 text-sm text-status-down"
          >
            {saveError}
          </p>
        )}
        <SettingsSection
          id="settings-polling-heading"
          title="Polling"
          description="Global collection cadence and worker capacity."
          icon="speed"
          accent="primary"
        >
          <PollingSettingsSection
            pollingValue={pollingValue}
            customPolling={customPolling}
            networkProbePorts={networkProbePorts}
            savedPolling={isSaved('polling_interval_seconds')}
            savedNetworkProbePorts={isSaved('network_probe_ports')}
            customPollingError={fieldErrors.customPolling}
            networkProbePortsError={fieldErrors.networkProbePorts}
            workerSectionOpen={workerSectionOpen}
            workerSettings={workerSettings}
            savedWorkerSettings={savedWorkerSettings}
            fieldErrors={fieldErrors}
            onPollingPresetChange={handlePollingPresetChange}
            onCustomPollingChange={handleCustomPollingChange}
            onCustomPollingBlur={handleCustomPollingBlur}
            onNetworkProbePortsChange={handleNetworkProbePortsChange}
            onNetworkProbePortsBlur={handleNetworkProbePortsBlur}
            onWorkerSectionToggle={() => setWorkerSectionOpen((prev) => !prev)}
            onWorkerSettingChange={handleWorkerSettingChange}
            onWorkerSettingBlur={handleWorkerSettingBlur}
          />
        </SettingsSection>

        <SettingsSection
          id="settings-snmp-debug-heading"
          title="SNMP Debug"
          description="Runtime SNMP collection timing and concurrency controls."
          icon="tune"
          accent="warning"
        >
          <SNMPDebugSettingsSection
            open={snmpDebugSectionOpen}
            settings={snmpDebugSettings}
            savedSettings={savedSNMPDebugSettings}
            fieldErrors={fieldErrors}
            onToggle={() => setSNMPDebugSectionOpen((prev) => !prev)}
            onSettingChange={handleSNMPDebugSettingChange}
            onSettingBlur={handleSNMPDebugSettingBlur}
          />
        </SettingsSection>

        <SettingsSection
          id="settings-topology-heading"
          title="Topology"
          description="Default discovery behavior for devices that inherit global policy."
          icon="account_tree"
          accent="secondary"
        >
          <label className="grid gap-1 text-sm" htmlFor="topology-discovery-default">
            <span className="flex items-center justify-between gap-3">
              <span className={fieldLabelClass}>Topology Discovery Default</span>
              <SavedIndicator visible={isSaved('topology_discovery_default_mode')} />
            </span>
            <select
              id="topology-discovery-default"
              aria-label="Topology Discovery Default"
              value={topologyDiscoveryDefaultMode}
              onChange={(e) => {
                const nextValue = e.target.value as TopologyDiscoveryMode;
                setTopologyDiscoveryDefaultMode(nextValue);
                autosave.save('topology_discovery_default_mode', nextValue, {
                  delayMs: 0,
                  onSuccess: () => onSettingsChange?.(),
                });
              }}
              className={controlClass()}
            >
              {TOPOLOGY_DISCOVERY_DEFAULT_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
          <p className="mt-2 text-xs text-on-bg-secondary">
            Applies to devices using the per-device{' '}
            <span className="font-medium">Use global default</span> mode. Current default:{' '}
            <span className="font-medium">
              {formatTopologyDiscoveryMode(topologyDiscoveryDefaultMode)}
            </span>
            .
          </p>
        </SettingsSection>

        <SettingsSection
          id="settings-integrations-heading"
          title="Integrations"
          description="External observability endpoints used by metrics integrations."
          icon="hub"
          accent="primary"
        >
          <PrometheusSettingsSection
            prometheusUrl={prometheusUrl}
            savedPrometheus={isSaved('prometheus_url')}
            prometheusError={fieldErrors.prometheusUrl}
            onPrometheusChange={handlePrometheusChange}
            onPrometheusBlur={handlePrometheusBlur}
          />
        </SettingsSection>
      </div>

      <div data-testid="settings-panel-right-column" className="grid min-w-0 content-start gap-6">
        <SettingsSection
          id="settings-bridge-heading"
          title="Bridge & Time"
          description="Local WinBox bridge listener and display timezone."
          icon="settings_ethernet"
          accent="warning"
        >
          <BridgeSettingsSection
            timezone={timezone}
            bridgePort={bridgePort}
            savedTimezone={isSaved('timezone')}
            savedBridgePort={isSaved('bridge_port')}
            bridgePortError={fieldErrors.bridgePort}
            onTimezoneChange={handleTimezoneChange}
            onBridgePortChange={handleBridgePortChange}
            onBridgePortBlur={handleBridgePortBlur}
          />
        </SettingsSection>

        <SettingsSection
          id="settings-profiles-heading"
          title="Profiles"
          description="Credential, SNMP, and Grafana profiles available to managed devices."
          icon="badge"
          accent="primary"
        >
          <div className="grid gap-5">
            <div
              data-testid="snmp-profile-well"
              className="rounded-lg bg-surface-container-high p-3"
            >
              <SNMPProfileManager />
            </div>
            <div
              data-testid="credential-profile-well"
              className="rounded-lg bg-surface-container-high p-3"
            >
              <CredentialProfileManager />
            </div>
            <div
              data-testid="grafana-profile-well"
              className="min-w-0 overflow-hidden rounded-lg bg-surface-container-high p-3"
            >
              <GrafanaDashboardProfileManager />
            </div>
          </div>
        </SettingsSection>

        <SettingsSection
          id="settings-backups-heading"
          title="Backups"
          description="Device configuration retention and full instance backup tools."
          icon="backup"
          accent="secondary"
        >
          <div className="grid gap-3">
            <DeviceBackupSettingsSection
              open={deviceBackupSectionOpen}
              deviceBackupInterval={deviceBackupInterval}
              deviceBackupRetention={deviceBackupRetention}
              savedDeviceInterval={isSaved('device_backup_interval_hours')}
              savedDeviceRetention={isSaved('device_backup_retention_count')}
              retentionError={fieldErrors.deviceBackupRetention}
              onToggle={() => setDeviceBackupSectionOpen((prev) => !prev)}
              onDeviceIntervalChange={handleDeviceIntervalChange}
              onDeviceRetentionChange={handleDeviceRetentionChange}
            />

            <div className="rounded-lg bg-surface-container-high p-3">
              <button
                type="button"
                aria-expanded={backupSectionOpen}
                onClick={() => setBackupSectionOpen((prev) => !prev)}
                className="flex w-full items-center justify-between gap-3 rounded-md px-1 py-1 text-left transition-colors hover:text-on-bg"
              >
                <span>
                  <span className="block text-sm font-semibold text-on-bg">Instance Backups</span>
                  <span className="block text-xs text-on-bg-secondary">
                    Export and restore application-level backup archives.
                  </span>
                </span>
                <MaterialIcon
                  name={backupSectionOpen ? 'expand_less' : 'expand_more'}
                  className="text-on-bg-secondary"
                />
              </button>
              {backupSectionOpen && (
                <div className="mt-4">
                  <InstanceBackupManager />
                </div>
              )}
            </div>
          </div>
        </SettingsSection>

        <SettingsSection
          id="settings-runtime-heading"
          title="Runtime"
          description="Deployment environment for this Theia instance."
          icon="info"
          accent={import.meta.env.DEV ? 'warning' : 'status-up'}
        >
          {runtimeInfo ? (
            <div className="grid gap-3 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span
                  className={`rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider ${
                    runtimeInfo.environment === 'development'
                      ? 'bg-warning/15 text-warning'
                      : 'bg-status-up/15 text-status-up'
                  }`}
                >
                  {runtimeInfo.environment}
                </span>
              </div>
            </div>
          ) : runtimeError ? (
            <p className="text-sm text-status-down">Runtime information unavailable</p>
          ) : (
            <p className="text-sm text-on-bg-secondary">Loading runtime information</p>
          )}
        </SettingsSection>
      </div>
    </div>
  );
}
