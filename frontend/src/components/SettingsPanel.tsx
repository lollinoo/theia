import { useEffect, useRef, useState } from 'react';
import { fetchSettings, updateSetting } from '../api/client';

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

export function SettingsPanel() {
  const [pollingValue, setPollingValue] = useState('60');
  const [customPolling, setCustomPolling] = useState('');
  const [grafanaUrl, setGrafanaUrl] = useState('');
  const [prometheusUrl, setPrometheusUrl] = useState('');
  const [savedPolling, setSavedPolling] = useState(false);
  const [savedGrafana, setSavedGrafana] = useState(false);
  const [savedPrometheus, setSavedPrometheus] = useState(false);

  const pollingTimerRef = useRef<number | null>(null);
  const grafanaTimerRef = useRef<number | null>(null);
  const prometheusTimerRef = useRef<number | null>(null);
  const savedPollingTimerRef = useRef<number | null>(null);
  const savedGrafanaTimerRef = useRef<number | null>(null);
  const savedPrometheusTimerRef = useRef<number | null>(null);

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
      })
      .catch(() => {/* non-fatal */});
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
      void updateSetting('grafana_url', value).then(() =>
        showSaved(setSavedGrafana, savedGrafanaTimerRef),
      );
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
    <div className="space-y-6 p-4">
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">
            Polling Interval
          </label>
          <SavedIndicator visible={savedPolling} />
        </div>
        <select
          value={pollingValue}
          onChange={(e) => handlePollingPresetChange(e.target.value)}
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
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
              className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none"
            />
            <span className="text-xs text-text-secondary">sec</span>
          </div>
        )}
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">
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
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
        />
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium uppercase tracking-widest text-text-secondary">
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
          className="w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none"
        />
      </div>
    </div>
  );
}
