import { useEffect, useRef, useState } from 'react';
import {
  type HealthVersion,
  fetchHealthVersion,
  fetchSettingsWithMetadata,
  updateSetting,
} from '../api/client';
import type { TopologyDiscoveryMode } from '../types/api';
import {
  TOPOLOGY_DISCOVERY_DEFAULT_OPTIONS,
  formatTopologyDiscoveryMode,
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
import {
  DEFAULT_WORKER_SETTINGS,
  PRESET_VALUES,
  WORKER_SETTINGS,
  type WorkerSetting,
  type WorkerSettingKey,
  createWorkerSavedFlags,
  createWorkerTimerRefs,
} from './settings-panel/settingsConstants';
import { controlClass, fieldLabelClass } from './settings-panel/settingsPanelStyles';

interface SettingsPanelProps {
  onSettingsChange?: () => void;
}

export function SettingsPanel({ onSettingsChange }: SettingsPanelProps) {
  const [pollingValue, setPollingValue] = useState('60');
  const [customPolling, setCustomPolling] = useState('');
  const [prometheusUrl, setPrometheusUrl] = useState('');
  const [timezone, setTimezone] = useState('UTC');
  const [topologyDiscoveryDefaultMode, setTopologyDiscoveryDefaultMode] =
    useState<TopologyDiscoveryMode>('lldp_cdp');
  const [savedPolling, setSavedPolling] = useState(false);
  const [savedPrometheus, setSavedPrometheus] = useState(false);
  const [savedTimezone, setSavedTimezone] = useState(false);
  const [savedTopologyDiscovery, setSavedTopologyDiscovery] = useState(false);
  const [versionInfo, setVersionInfo] = useState<HealthVersion | null>(null);
  const [backupSectionOpen, setBackupSectionOpen] = useState(false);
  const [deviceBackupSectionOpen, setDeviceBackupSectionOpen] = useState(false);
  const [deviceBackupInterval, setDeviceBackupInterval] = useState('0');
  const [deviceBackupRetention, setDeviceBackupRetention] = useState('5');
  const [savedDeviceInterval, setSavedDeviceInterval] = useState(false);
  const [savedDeviceRetention, setSavedDeviceRetention] = useState(false);
  const [bridgePort, setBridgePort] = useState('1337');
  const [savedBridgePort, setSavedBridgePort] = useState(false);
  const [workerSectionOpen, setWorkerSectionOpen] = useState(false);
  const [workerSettings, setWorkerSettings] =
    useState<Record<WorkerSettingKey, string>>(DEFAULT_WORKER_SETTINGS);
  const [savedWorkerSettings, setSavedWorkerSettings] =
    useState<Record<WorkerSettingKey, boolean>>(createWorkerSavedFlags);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const pollingTimerRef = useRef<number | null>(null);
  const prometheusTimerRef = useRef<number | null>(null);
  const savedPollingTimerRef = useRef<number | null>(null);
  const savedPrometheusTimerRef = useRef<number | null>(null);
  const savedTimezoneTimerRef = useRef<number | null>(null);
  const savedTopologyDiscoveryTimerRef = useRef<number | null>(null);
  const deviceIntervalTimerRef = useRef<number | null>(null);
  const deviceRetentionTimerRef = useRef<number | null>(null);
  const savedDeviceIntervalTimerRef = useRef<number | null>(null);
  const savedDeviceRetentionTimerRef = useRef<number | null>(null);
  const bridgePortTimerRef = useRef<number | null>(null);
  const savedBridgePortTimerRef = useRef<number | null>(null);
  const workerTimerRefs = useRef<Record<WorkerSettingKey, number | null>>(createWorkerTimerRefs());
  const savedWorkerTimerRefs = useRef<Record<WorkerSettingKey, number | null>>(
    createWorkerTimerRefs(),
  );

  useEffect(() => {
    fetchSettingsWithMetadata()
      .then(({ data: settings }) => {
        const interval = settings['polling_interval_seconds'] ?? '60';
        if (PRESET_VALUES.has(interval)) {
          setPollingValue(interval);
        } else {
          setPollingValue('custom');
          setCustomPolling(interval);
        }
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
      })
      .catch(() => {
        /* non-fatal */
      });
    fetchHealthVersion().then(setVersionInfo);
  }, []);

  function setFieldError(field: string, error: string | null) {
    setFieldErrors((prev) => {
      if (error) return { ...prev, [field]: error };
      const next = { ...prev };
      delete next[field];
      return next;
    });
  }

  function showSaved(
    setter: React.Dispatch<React.SetStateAction<boolean>>,
    timerRef: React.MutableRefObject<number | null>,
  ) {
    setter(true);
    if (timerRef.current !== null) window.clearTimeout(timerRef.current);
    timerRef.current = window.setTimeout(() => setter(false), 2000);
  }

  function showWorkerSaved(key: WorkerSettingKey) {
    setSavedWorkerSettings((prev) => ({ ...prev, [key]: true }));
    if (savedWorkerTimerRefs.current[key] !== null) {
      window.clearTimeout(savedWorkerTimerRefs.current[key]);
    }
    savedWorkerTimerRefs.current[key] = window.setTimeout(() => {
      setSavedWorkerSettings((prev) => ({ ...prev, [key]: false }));
    }, 2000);
  }

  function validateIntegerRange(value: string, min: number, max: number): string | null {
    const trimmed = value.trim();
    if (!/^\d+$/.test(trimmed)) return 'Must be a valid integer';
    const parsed = parseInt(trimmed, 10);
    if (parsed < min || parsed > max) return `Must be between ${min} and ${max}`;
    return null;
  }

  function handleWorkerSettingChange(key: WorkerSettingKey, value: string) {
    const setting = WORKER_SETTINGS.find((candidate) => candidate.key === key);
    if (!setting) return;
    setWorkerSettings((prev) => ({ ...prev, [key]: value }));
    if (workerTimerRefs.current[key] !== null) {
      window.clearTimeout(workerTimerRefs.current[key]);
    }

    const err = validateIntegerRange(value, setting.min, setting.max);
    if (err) {
      setFieldError(key, err);
      return;
    }

    setFieldError(key, null);
    const normalized = String(parseInt(value, 10));
    workerTimerRefs.current[key] = window.setTimeout(() => {
      void updateSetting(key, normalized).then(() => showWorkerSaved(key));
    }, 500);
  }

  function schedulePollingUpdate(rawValue: string) {
    if (pollingTimerRef.current !== null) window.clearTimeout(pollingTimerRef.current);
    const trimmed = rawValue.trim();
    const numVal = parseInt(trimmed, 10);
    if (!/^\d+$/.test(trimmed) || numVal < 5 || numVal > 3600) return;
    pollingTimerRef.current = window.setTimeout(() => {
      void updateSetting('polling_interval_seconds', String(numVal)).then(() =>
        showSaved(setSavedPolling, savedPollingTimerRef),
      );
    }, 500);
  }

  function schedulePrometheusUpdate(value: string) {
    if (prometheusTimerRef.current !== null) window.clearTimeout(prometheusTimerRef.current);
    // Gate auto-save: if value is non-empty and fails URL validation, set error and skip save
    if (value.trim() !== '') {
      const err = validateURL(value, 'Prometheus URL');
      if (err) {
        setFieldError('prometheusUrl', err);
        return;
      }
    }
    prometheusTimerRef.current = window.setTimeout(() => {
      void updateSetting('prometheus_url', value).then(() =>
        showSaved(setSavedPrometheus, savedPrometheusTimerRef),
      );
    }, 500);
  }

  function handleDeviceIntervalChange(value: string) {
    const err = validateIntervalAllowlist(value);
    if (err) {
      setFieldError('deviceBackupInterval', err);
      setDeviceBackupInterval(value);
      return;
    }
    setFieldError('deviceBackupInterval', null);
    setDeviceBackupInterval(value);
    if (deviceIntervalTimerRef.current !== null)
      window.clearTimeout(deviceIntervalTimerRef.current);
    deviceIntervalTimerRef.current = window.setTimeout(() => {
      void updateSetting('device_backup_interval_hours', value).then(() =>
        showSaved(setSavedDeviceInterval, savedDeviceIntervalTimerRef),
      );
    }, 500);
  }

  function handleDeviceRetentionChange(value: string) {
    const err = validateRetentionCount(value);
    if (err) {
      setFieldError('deviceBackupRetention', err);
      setDeviceBackupRetention(value);
      return;
    }
    setFieldError('deviceBackupRetention', null);
    setDeviceBackupRetention(value);
    if (deviceRetentionTimerRef.current !== null)
      window.clearTimeout(deviceRetentionTimerRef.current);
    const num = parseInt(value, 10);
    deviceRetentionTimerRef.current = window.setTimeout(() => {
      void updateSetting('device_backup_retention_count', String(num)).then(() =>
        showSaved(setSavedDeviceRetention, savedDeviceRetentionTimerRef),
      );
    }, 500);
  }

  function handleBridgePortChange(value: string) {
    setBridgePort(value);
    setFieldError('bridgePort', null);
    if (bridgePortTimerRef.current !== null) window.clearTimeout(bridgePortTimerRef.current);
    const trimmed = value.trim();
    const num = parseInt(trimmed, 10);
    if (!/^\d+$/.test(trimmed) || num < 1 || num > 65535) {
      setFieldError('bridgePort', 'Bridge port must be an integer between 1 and 65535');
      return;
    }
    bridgePortTimerRef.current = window.setTimeout(() => {
      void updateSetting('bridge_port', String(num)).then(() =>
        showSaved(setSavedBridgePort, savedBridgePortTimerRef),
      );
    }, 500);
  }

  function handlePollingPresetChange(value: string) {
    setPollingValue(value);
    if (value !== 'custom') {
      schedulePollingUpdate(value);
    }
  }

  function handleCustomPollingChange(value: string) {
    setCustomPolling(value);
    setFieldError('customPolling', null);
    schedulePollingUpdate(value);
  }

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

  function handleTimezoneChange(value: string) {
    setTimezone(value);
    void updateSetting('timezone', value).then(() =>
      showSaved(setSavedTimezone, savedTimezoneTimerRef),
    );
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

  return (
    <div
      data-testid="settings-panel-layout"
      className="grid min-w-0 items-start gap-6 p-4 transition-colors duration-200 lg:grid-cols-2"
    >
      <div data-testid="settings-panel-left-column" className="grid min-w-0 content-start gap-6">
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
            savedPolling={savedPolling}
            customPollingError={fieldErrors.customPolling}
            workerSectionOpen={workerSectionOpen}
            workerSettings={workerSettings}
            savedWorkerSettings={savedWorkerSettings}
            fieldErrors={fieldErrors}
            onPollingPresetChange={handlePollingPresetChange}
            onCustomPollingChange={handleCustomPollingChange}
            onCustomPollingBlur={handleCustomPollingBlur}
            onWorkerSectionToggle={() => setWorkerSectionOpen((prev) => !prev)}
            onWorkerSettingChange={handleWorkerSettingChange}
            onWorkerSettingBlur={handleWorkerSettingBlur}
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
              <SavedIndicator visible={savedTopologyDiscovery} />
            </span>
            <select
              id="topology-discovery-default"
              aria-label="Topology Discovery Default"
              value={topologyDiscoveryDefaultMode}
              onChange={(e) => {
                const nextValue = e.target.value as TopologyDiscoveryMode;
                setTopologyDiscoveryDefaultMode(nextValue);
                void updateSetting('topology_discovery_default_mode', nextValue).then(() => {
                  showSaved(setSavedTopologyDiscovery, savedTopologyDiscoveryTimerRef);
                  onSettingsChange?.();
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
            savedPrometheus={savedPrometheus}
            prometheusError={fieldErrors.prometheusUrl}
            onPrometheusChange={handlePrometheusChange}
            onPrometheusBlur={handlePrometheusBlur}
          />
        </SettingsSection>
      </div>

      <div data-testid="settings-panel-right-column" className="grid min-w-0 content-start gap-6">
        <SettingsSection
          id="settings-bridge-heading"
          title="Bridge"
          description="Local WinBox bridge listener and timestamp preferences."
          icon="settings_ethernet"
          accent="warning"
        >
          <BridgeSettingsSection
            timezone={timezone}
            bridgePort={bridgePort}
            savedTimezone={savedTimezone}
            savedBridgePort={savedBridgePort}
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
              savedDeviceInterval={savedDeviceInterval}
              savedDeviceRetention={savedDeviceRetention}
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
          id="settings-about-heading"
          title="About"
          description="Installed application version and build metadata."
          icon="info"
          accent={import.meta.env.DEV ? 'warning' : 'status-up'}
        >
          {versionInfo ? (
            <div className="grid gap-3 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-semibold text-on-bg">Theia v{versionInfo.version}</span>
                <span
                  className={`rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider ${
                    import.meta.env.DEV
                      ? 'bg-warning/15 text-warning'
                      : 'bg-status-up/15 text-status-up'
                  }`}
                >
                  {import.meta.env.DEV ? 'dev' : 'production'}
                </span>
              </div>
              <div className="grid gap-1 text-xs text-on-bg-secondary">
                <p className="break-all">Commit: {versionInfo.git_commit}</p>
                <p>
                  Built:{' '}
                  {versionInfo.build_date === 'unknown'
                    ? 'unknown'
                    : new Date(versionInfo.build_date).toLocaleString()}
                </p>
              </div>
            </div>
          ) : (
            <p className="text-sm text-on-bg-secondary">Loading version information</p>
          )}
        </SettingsSection>
      </div>
    </div>
  );
}
