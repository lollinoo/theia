import { useEffect, useRef, useState } from 'react';
import { fetchSettings, updateSetting, fetchHealthVersion, type HealthVersion } from '../api/client';
import { validateURL, validateIntervalAllowlist, validateRetentionCount } from '../utils/validation';
import { AreaManager } from './AreaManager';
import { SNMPProfileManager } from './SNMPProfileManager';
import { CredentialProfileManager } from './CredentialProfileManager';
import { InstanceBackupManager } from './InstanceBackupManager';

const TIMEZONES = [
  { label: 'UTC', value: 'UTC' },
  { label: 'Europe/London (GMT/BST)', value: 'Europe/London' },
  { label: 'Europe/Paris (CET/CEST)', value: 'Europe/Paris' },
  { label: 'Europe/Berlin (CET/CEST)', value: 'Europe/Berlin' },
  { label: 'Europe/Rome (CET/CEST)', value: 'Europe/Rome' },
  { label: 'Europe/Madrid (CET/CEST)', value: 'Europe/Madrid' },
  { label: 'Europe/Amsterdam (CET/CEST)', value: 'Europe/Amsterdam' },
  { label: 'Europe/Zurich (CET/CEST)', value: 'Europe/Zurich' },
  { label: 'Europe/Vienna (CET/CEST)', value: 'Europe/Vienna' },
  { label: 'Europe/Brussels (CET/CEST)', value: 'Europe/Brussels' },
  { label: 'Europe/Stockholm (CET/CEST)', value: 'Europe/Stockholm' },
  { label: 'Europe/Warsaw (CET/CEST)', value: 'Europe/Warsaw' },
  { label: 'Europe/Prague (CET/CEST)', value: 'Europe/Prague' },
  { label: 'Europe/Helsinki (EET/EEST)', value: 'Europe/Helsinki' },
  { label: 'Europe/Bucharest (EET/EEST)', value: 'Europe/Bucharest' },
  { label: 'Europe/Athens (EET/EEST)', value: 'Europe/Athens' },
  { label: 'Europe/Istanbul (TRT)', value: 'Europe/Istanbul' },
  { label: 'Europe/Moscow (MSK)', value: 'Europe/Moscow' },
  { label: 'Asia/Dubai (GST)', value: 'Asia/Dubai' },
  { label: 'Asia/Kolkata (IST)', value: 'Asia/Kolkata' },
  { label: 'Asia/Singapore (SGT)', value: 'Asia/Singapore' },
  { label: 'Asia/Shanghai (CST)', value: 'Asia/Shanghai' },
  { label: 'Asia/Tokyo (JST)', value: 'Asia/Tokyo' },
  { label: 'Asia/Seoul (KST)', value: 'Asia/Seoul' },
  { label: 'Australia/Sydney (AEST/AEDT)', value: 'Australia/Sydney' },
  { label: 'Pacific/Auckland (NZST/NZDT)', value: 'Pacific/Auckland' },
  { label: 'America/New_York (EST/EDT)', value: 'America/New_York' },
  { label: 'America/Chicago (CST/CDT)', value: 'America/Chicago' },
  { label: 'America/Denver (MST/MDT)', value: 'America/Denver' },
  { label: 'America/Los_Angeles (PST/PDT)', value: 'America/Los_Angeles' },
  { label: 'America/Anchorage (AKST/AKDT)', value: 'America/Anchorage' },
  { label: 'Pacific/Honolulu (HST)', value: 'Pacific/Honolulu' },
  { label: 'America/Sao_Paulo (BRT)', value: 'America/Sao_Paulo' },
  { label: 'America/Argentina/Buenos_Aires (ART)', value: 'America/Argentina/Buenos_Aires' },
  { label: 'America/Toronto (EST/EDT)', value: 'America/Toronto' },
  { label: 'America/Vancouver (PST/PDT)', value: 'America/Vancouver' },
  { label: 'America/Mexico_City (CST/CDT)', value: 'America/Mexico_City' },
  { label: 'Africa/Cairo (EET)', value: 'Africa/Cairo' },
  { label: 'Africa/Johannesburg (SAST)', value: 'Africa/Johannesburg' },
  { label: 'Africa/Lagos (WAT)', value: 'Africa/Lagos' },
];

const POLLING_PRESETS = [
  { label: '15 seconds', value: '15' },
  { label: '30 seconds', value: '30' },
  { label: '60 seconds (default)', value: '60' },
  { label: '2 minutes', value: '120' },
  { label: '5 minutes', value: '300' },
  { label: 'Custom...', value: 'custom' },
];

const PRESET_VALUES = new Set(POLLING_PRESETS.map((p) => p.value).filter((v) => v !== 'custom'));

interface SavedIndicatorProps {
  visible: boolean;
}

function SavedIndicator({ visible }: SavedIndicatorProps) {
  return (
    <span
      className={`text-xs text-status-up transition-opacity duration-500 ${visible ? 'opacity-100' : 'opacity-0'}`}
    >
      Saved
    </span>
  );
}

interface SettingsPanelProps {
  onAreasChange?: () => void;
  onSettingsChange?: () => void;
}

export function SettingsPanel({ onAreasChange, onSettingsChange }: SettingsPanelProps) {
  const [pollingValue, setPollingValue] = useState('60');
  const [customPolling, setCustomPolling] = useState('');
  const [grafanaUrl, setGrafanaUrl] = useState('');
  const [prometheusUrl, setPrometheusUrl] = useState('');
  const [timezone, setTimezone] = useState('UTC');
  const [savedPolling, setSavedPolling] = useState(false);
  const [savedGrafana, setSavedGrafana] = useState(false);
  const [savedPrometheus, setSavedPrometheus] = useState(false);
  const [savedTimezone, setSavedTimezone] = useState(false);
  const [versionInfo, setVersionInfo] = useState<HealthVersion | null>(null);
  const [backupSectionOpen, setBackupSectionOpen] = useState(false);
  const [deviceBackupSectionOpen, setDeviceBackupSectionOpen] = useState(false);
  const [deviceBackupInterval, setDeviceBackupInterval] = useState('0');
  const [deviceBackupRetention, setDeviceBackupRetention] = useState('5');
  const [savedDeviceInterval, setSavedDeviceInterval] = useState(false);
  const [savedDeviceRetention, setSavedDeviceRetention] = useState(false);
  const [bridgeSecret, setBridgeSecret] = useState('');
  const [savedBridgeSecret, setSavedBridgeSecret] = useState(false);
  const [bridgePort, setBridgePort] = useState('1337');
  const [savedBridgePort, setSavedBridgePort] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const pollingTimerRef = useRef<number | null>(null);
  const grafanaTimerRef = useRef<number | null>(null);
  const prometheusTimerRef = useRef<number | null>(null);
  const savedPollingTimerRef = useRef<number | null>(null);
  const savedGrafanaTimerRef = useRef<number | null>(null);
  const savedPrometheusTimerRef = useRef<number | null>(null);
  const savedTimezoneTimerRef = useRef<number | null>(null);
  const deviceIntervalTimerRef = useRef<number | null>(null);
  const deviceRetentionTimerRef = useRef<number | null>(null);
  const savedDeviceIntervalTimerRef = useRef<number | null>(null);
  const savedDeviceRetentionTimerRef = useRef<number | null>(null);
  const bridgeSecretTimerRef = useRef<number | null>(null);
  const savedBridgeSecretTimerRef = useRef<number | null>(null);
  const bridgePortTimerRef = useRef<number | null>(null);
  const savedBridgePortTimerRef = useRef<number | null>(null);

  useEffect(() => {
    fetchSettings()
      .then((settings) => {
        const interval = settings['polling_interval_seconds'] ?? '60';
        if (PRESET_VALUES.has(interval)) {
          setPollingValue(interval);
        } else {
          setPollingValue('custom');
          setCustomPolling(interval);
        }
        setGrafanaUrl(settings['grafana_url'] ?? '');
        setPrometheusUrl(settings['prometheus_url'] ?? '');
        setTimezone(settings['timezone'] || 'UTC');
        setDeviceBackupInterval(settings['device_backup_interval_hours'] ?? '0');
        setDeviceBackupRetention(settings['device_backup_retention_count'] ?? '5');
        setBridgeSecret(settings['bridge_secret'] ?? '');
        setBridgePort(settings['bridge_port'] ?? '1337');
      })
      .catch(() => {/* non-fatal */});
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

  function schedulePollingUpdate(rawValue: string) {
    if (pollingTimerRef.current !== null) window.clearTimeout(pollingTimerRef.current);
    const numVal = parseInt(rawValue, 10);
    if (!Number.isFinite(numVal) || numVal < 5 || numVal > 3600) return;
    pollingTimerRef.current = window.setTimeout(() => {
      void updateSetting('polling_interval_seconds', String(numVal)).then(() =>
        showSaved(setSavedPolling, savedPollingTimerRef),
      );
    }, 500);
  }

  function scheduleGrafanaUpdate(value: string) {
    if (grafanaTimerRef.current !== null) window.clearTimeout(grafanaTimerRef.current);
    // Gate auto-save: if value is non-empty and fails URL validation, set error and skip save
    if (value.trim() !== '') {
      const err = validateURL(value, 'Grafana URL');
      if (err) {
        setFieldError('grafanaUrl', err);
        return;
      }
    }
    grafanaTimerRef.current = window.setTimeout(() => {
      void updateSetting('grafana_url', value).then(() => {
        showSaved(setSavedGrafana, savedGrafanaTimerRef);
        onSettingsChange?.();
      });
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
    if (err) { setFieldError('deviceBackupInterval', err); setDeviceBackupInterval(value); return; }
    setFieldError('deviceBackupInterval', null);
    setDeviceBackupInterval(value);
    if (deviceIntervalTimerRef.current !== null) window.clearTimeout(deviceIntervalTimerRef.current);
    deviceIntervalTimerRef.current = window.setTimeout(() => {
      void updateSetting('device_backup_interval_hours', value).then(() =>
        showSaved(setSavedDeviceInterval, savedDeviceIntervalTimerRef),
      );
    }, 500);
  }

  function handleDeviceRetentionChange(value: string) {
    const err = validateRetentionCount(value);
    if (err) { setFieldError('deviceBackupRetention', err); setDeviceBackupRetention(value); return; }
    setFieldError('deviceBackupRetention', null);
    setDeviceBackupRetention(value);
    if (deviceRetentionTimerRef.current !== null) window.clearTimeout(deviceRetentionTimerRef.current);
    const num = parseInt(value, 10);
    deviceRetentionTimerRef.current = window.setTimeout(() => {
      void updateSetting('device_backup_retention_count', String(num)).then(() =>
        showSaved(setSavedDeviceRetention, savedDeviceRetentionTimerRef),
      );
    }, 500);
  }

  function computeDeviceNextBackupText(): string {
    const intervalHours = parseInt(deviceBackupInterval, 10);
    if (!intervalHours || intervalHours <= 0) return 'Scheduling disabled';
    return 'Backups run every ' + formatDeviceInterval(intervalHours);
  }

  function formatDeviceInterval(hours: number): string {
    if (hours >= 168) return '7 days';
    if (hours >= 48) return '48 hours';
    if (hours >= 24) return '24 hours';
    return hours + ' hours';
  }

  function handleBridgeSecretChange(value: string) {
    setBridgeSecret(value);
    if (bridgeSecretTimerRef.current !== null) window.clearTimeout(bridgeSecretTimerRef.current);
    bridgeSecretTimerRef.current = window.setTimeout(() => {
      void updateSetting('bridge_secret', value).then(() =>
        showSaved(setSavedBridgeSecret, savedBridgeSecretTimerRef),
      );
    }, 500);
  }

  function handleBridgePortChange(value: string) {
    setBridgePort(value);
    setFieldError('bridgePort', null);
    if (bridgePortTimerRef.current !== null) window.clearTimeout(bridgePortTimerRef.current);
    const num = parseInt(value, 10);
    if (!Number.isFinite(num) || num < 1 || num > 65535) {
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

  return (
    <div className="space-y-6 p-4 transition-colors duration-200">
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Polling Interval
          </label>
          <SavedIndicator visible={savedPolling} />
        </div>
        <select
          value={pollingValue}
          onChange={(e) => handlePollingPresetChange(e.target.value)}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
        >
          {POLLING_PRESETS.map((preset) => (
            <option key={preset.value} value={preset.value}>
              {preset.label}
            </option>
          ))}
        </select>
        {pollingValue === 'custom' && (
          <div className="flex items-center gap-2">
            <div className="w-full">
              <input
                type="number"
                min={5}
                max={3600}
                value={customPolling}
                placeholder="Seconds (5–3600)"
                onChange={(e) => {
                  setCustomPolling(e.target.value);
                  setFieldError('customPolling', null);
                  schedulePollingUpdate(e.target.value);
                }}
                onBlur={() => {
                  const num = parseInt(customPolling, 10);
                  if (!Number.isFinite(num) || num < 5 || num > 3600) {
                    setFieldError('customPolling', 'Polling interval must be between 5 and 3600 seconds');
                  } else {
                    setFieldError('customPolling', null);
                  }
                }}
                className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors.customPolling ? ' border-status-down' : ' border-outline-subtle'}`}
              />
              {fieldErrors.customPolling && (
                <p className="mt-1 text-xs text-status-down">{fieldErrors.customPolling}</p>
              )}
            </div>
            <span className="text-xs text-on-bg-secondary">sec</span>
          </div>
        )}
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Grafana URL
          </label>
          <SavedIndicator visible={savedGrafana} />
        </div>
        <input
          type="url"
          value={grafanaUrl}
          placeholder="http://localhost:3001"
          onChange={(e) => {
            setGrafanaUrl(e.target.value);
            setFieldError('grafanaUrl', null);
            scheduleGrafanaUpdate(e.target.value);
          }}
          onBlur={() => setFieldError('grafanaUrl', validateURL(grafanaUrl, 'Grafana URL'))}
          className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors.grafanaUrl ? ' border-status-down' : ' border-outline-subtle'}`}
        />
        {fieldErrors.grafanaUrl && (
          <p className="mt-1 text-xs text-status-down">{fieldErrors.grafanaUrl}</p>
        )}
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Prometheus URL
          </label>
          <SavedIndicator visible={savedPrometheus} />
        </div>
        <input
          type="url"
          value={prometheusUrl}
          placeholder="http://localhost:9090"
          onChange={(e) => {
            setPrometheusUrl(e.target.value);
            setFieldError('prometheusUrl', null);
            schedulePrometheusUpdate(e.target.value);
          }}
          onBlur={() => setFieldError('prometheusUrl', validateURL(prometheusUrl, 'Prometheus URL'))}
          className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors.prometheusUrl ? ' border-status-down' : ' border-outline-subtle'}`}
        />
        {fieldErrors.prometheusUrl && (
          <p className="mt-1 text-xs text-status-down">{fieldErrors.prometheusUrl}</p>
        )}
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Timezone
          </label>
          <SavedIndicator visible={savedTimezone} />
        </div>
        <select
          value={timezone}
          onChange={(e) => {
            setTimezone(e.target.value);
            void updateSetting('timezone', e.target.value).then(() =>
              showSaved(setSavedTimezone, savedTimezoneTimerRef),
            );
          }}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
        >
          {TIMEZONES.map((tz) => (
            <option key={tz.value} value={tz.value}>
              {tz.label}
            </option>
          ))}
        </select>
        <p className="text-xs text-on-bg-secondary/70">
          Affects backup filenames and zip timestamps.
        </p>
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            WinBox Bridge Secret
          </label>
          <SavedIndicator visible={savedBridgeSecret} />
        </div>
        <input
          type="text"
          value={bridgeSecret}
          placeholder="Paste 64-char hex key from config.json"
          onChange={(e) => handleBridgeSecretChange(e.target.value)}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none font-mono"
        />
        <p className="text-xs text-on-bg-secondary/70">
          Found in <span className="font-mono">~/.config/winbox-bridge/config.json</span> → <span className="font-mono">bridge_secret</span> field. Required to launch WinBox from Theia.
        </p>
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            WinBox Bridge Port
          </label>
          <SavedIndicator visible={savedBridgePort} />
        </div>
        <input
          type="number"
          min={1}
          max={65535}
          value={bridgePort}
          placeholder="1337"
          onChange={(e) => handleBridgePortChange(e.target.value)}
          onBlur={() => {
            const num = parseInt(bridgePort, 10);
            if (!Number.isFinite(num) || num < 1 || num > 65535) {
              setFieldError('bridgePort', 'Bridge port must be an integer between 1 and 65535');
            } else {
              setFieldError('bridgePort', null);
            }
          }}
          className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none font-mono${fieldErrors.bridgePort ? ' border-status-down' : ' border-outline-subtle'}`}
        />
        {fieldErrors.bridgePort && (
          <p className="mt-1 text-xs text-status-down">{fieldErrors.bridgePort}</p>
        )}
        <p className="text-xs text-on-bg-secondary/70">
          TCP port the WinBox bridge listens on. Default is <span className="font-mono">1337</span>.
          Must match <span className="font-mono">ListenPort</span> in the bridge&apos;s config.json.
        </p>
      </div>

      <div className="mt-6">
        <AreaManager onAreasChange={onAreasChange} />
      </div>

      <div className="mt-6">
        <SNMPProfileManager />
      </div>

      <div className="mt-6">
        <CredentialProfileManager />
      </div>

      {/* Device Backup section (collapsible, collapsed by default) */}
      <div className="mt-6">
        <button
          type="button"
          onClick={() => setDeviceBackupSectionOpen((prev) => !prev)}
          className="flex w-full items-center justify-between rounded-lg bg-surface-high px-3 py-2.5 text-left transition-colors hover:bg-elevated/50"
        >
          <span className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Device Backups
          </span>
          <svg
            className={`w-4 h-4 text-on-bg-secondary transition-transform duration-200 ${deviceBackupSectionOpen ? 'rotate-180' : ''}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        {deviceBackupSectionOpen && (
          <div className="mt-2 space-y-3 transition-colors duration-200">
            {/* Schedule & Retention Settings */}
            <div className="rounded-lg bg-surface-high p-3 space-y-3">
              {/* Schedule Interval Dropdown */}
              <div className="space-y-1">
                <div className="flex items-center justify-between">
                  <label className="text-[11px] font-medium text-on-bg-secondary">
                    Automatic Backup Schedule
                  </label>
                  {savedDeviceInterval && (
                    <span className="text-[10px] text-status-up font-medium">Saved</span>
                  )}
                </div>
                <select
                  value={deviceBackupInterval}
                  onChange={(e) => handleDeviceIntervalChange(e.target.value)}
                  className="w-full rounded-lg border border-outline-subtle bg-elevated px-2.5 py-1.5 text-xs text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                >
                  <option value="0">Disabled</option>
                  <option value="6">Every 6 hours</option>
                  <option value="12">Every 12 hours</option>
                  <option value="24">Every 24 hours</option>
                  <option value="48">Every 48 hours</option>
                  <option value="168">Every 7 days</option>
                </select>
                {/* Next backup helper text (per D-08) */}
                <p className="text-[10px] text-on-bg-muted">
                  {computeDeviceNextBackupText()}
                </p>
              </div>

              {/* Retention Count Input (per D-04, D-05) */}
              <div className="space-y-1">
                <div className="flex items-center justify-between">
                  <label className="text-[11px] font-medium text-on-bg-secondary">
                    Keep last N backups per device
                  </label>
                  {savedDeviceRetention && (
                    <span className="text-[10px] text-status-up font-medium">Saved</span>
                  )}
                </div>
                <input
                  type="number"
                  min={1}
                  max={50}
                  value={deviceBackupRetention}
                  onChange={(e) => handleDeviceRetentionChange(e.target.value)}
                  className={`w-full rounded-lg border bg-elevated px-2.5 py-1.5 text-xs text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors.deviceBackupRetention ? ' border-status-down' : ' border-outline-subtle'}`}
                />
                {fieldErrors.deviceBackupRetention && (
                  <p className="mt-0.5 text-[10px] text-status-down">{fieldErrors.deviceBackupRetention}</p>
                )}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Instance Backup section (collapsible, collapsed by default) */}
      <div className="mt-6">
        <button
          type="button"
          onClick={() => setBackupSectionOpen((prev) => !prev)}
          className="flex w-full items-center justify-between rounded-lg bg-surface-high px-3 py-2.5 text-left transition-colors hover:bg-elevated/50"
        >
          <span className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Instance Backups
          </span>
          <svg
            className={`w-4 h-4 text-on-bg-secondary transition-transform duration-200 ${backupSectionOpen ? 'rotate-180' : ''}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        {backupSectionOpen && (
          <div className="mt-2">
            <InstanceBackupManager />
          </div>
        )}
      </div>

      {versionInfo && (
        <div className="mt-6 space-y-2">
          <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            About
          </label>
          <div className="flex items-center gap-2">
            <span className="text-sm text-on-bg font-medium">
              Theia v{versionInfo.version}
            </span>
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
          <div className="space-y-0.5 text-xs text-on-bg-secondary/70">
            <p>Commit: {versionInfo.git_commit}</p>
            <p>Built: {versionInfo.build_date === 'unknown' ? 'unknown' : new Date(versionInfo.build_date).toLocaleString()}</p>
          </div>
        </div>
      )}

    </div>
  );
}
