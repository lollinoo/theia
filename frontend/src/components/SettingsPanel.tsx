import { useEffect, useRef, useState } from 'react';
import { fetchSettings, updateSetting, fetchHealthVersion, type HealthVersion } from '../api/client';
import { AreaManager } from './AreaManager';
import { SNMPProfileManager } from './SNMPProfileManager';
import { SSHProfileManager } from './SSHProfileManager';

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

  const pollingTimerRef = useRef<number | null>(null);
  const grafanaTimerRef = useRef<number | null>(null);
  const prometheusTimerRef = useRef<number | null>(null);
  const savedPollingTimerRef = useRef<number | null>(null);
  const savedGrafanaTimerRef = useRef<number | null>(null);
  const savedPrometheusTimerRef = useRef<number | null>(null);
  const savedTimezoneTimerRef = useRef<number | null>(null);

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
      })
      .catch(() => {/* non-fatal */});
    fetchHealthVersion().then(setVersionInfo);
  }, []);

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
    grafanaTimerRef.current = window.setTimeout(() => {
      void updateSetting('grafana_url', value).then(() => {
        showSaved(setSavedGrafana, savedGrafanaTimerRef);
        onSettingsChange?.();
      });
    }, 500);
  }

  function schedulePrometheusUpdate(value: string) {
    if (prometheusTimerRef.current !== null) window.clearTimeout(prometheusTimerRef.current);
    prometheusTimerRef.current = window.setTimeout(() => {
      void updateSetting('prometheus_url', value).then(() =>
        showSaved(setSavedPrometheus, savedPrometheusTimerRef),
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
              className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
            />
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
            scheduleGrafanaUpdate(e.target.value);
          }}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
        />
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
            schedulePrometheusUpdate(e.target.value);
          }}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
        />
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

      <div className="mt-6">
        <AreaManager onAreasChange={onAreasChange} />
      </div>

      <div className="mt-6">
        <SNMPProfileManager />
      </div>

      <div className="mt-6">
        <SSHProfileManager />
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
