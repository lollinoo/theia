/**
 * Renders device grafana dashboard section controls within the device configuration workflow.
 * Keeps this section focused on one editable device responsibility.
 */
import { useEffect, useRef, useState } from 'react';
import {
  fetchGrafanaDashboardConfig,
  fetchSettings,
  saveDeviceGrafanaDashboardOverride,
} from '../../api/client';
import type { Device, GrafanaDashboardProfile } from '../../types/api';
import { validateURL } from '../../utils/validation';

interface DeviceGrafanaDashboardSectionProps {
  device: Device;
  readOnly?: boolean;
  isVirtual?: boolean;
  onSettingsChange?: () => void;
}

/** Renders the DeviceGrafanaDashboardSection component within the device configuration workflow. */
export function DeviceGrafanaDashboardSection({
  device,
  readOnly = false,
  isVirtual,
  onSettingsChange,
}: DeviceGrafanaDashboardSectionProps) {
  const [grafanaUrl, setGrafanaUrl] = useState('');
  const [grafanaProfileId, setGrafanaProfileId] = useState('');
  const [grafanaProfiles, setGrafanaProfiles] = useState<GrafanaDashboardProfile[]>([]);
  const [grafanaError, setGrafanaError] = useState<string | null>(null);
  const [savedGrafana, setSavedGrafana] = useState(false);

  const grafanaTimerRef = useRef<number | null>(null);
  const savedGrafanaTimerRef = useRef<number | null>(null);
  const grafanaUrlRef = useRef('');
  const grafanaProfileIdRef = useRef('');
  const saveGenerationRef = useRef(0);
  const isHidden = isVirtual && !device.ip;

  function clearGrafanaTimer() {
    if (grafanaTimerRef.current !== null) {
      window.clearTimeout(grafanaTimerRef.current);
      grafanaTimerRef.current = null;
    }
  }

  function clearSavedGrafanaTimer() {
    if (savedGrafanaTimerRef.current !== null) {
      window.clearTimeout(savedGrafanaTimerRef.current);
      savedGrafanaTimerRef.current = null;
    }
  }

  function invalidateGrafanaWork() {
    saveGenerationRef.current += 1;
    clearGrafanaTimer();
    clearSavedGrafanaTimer();
  }

  useEffect(() => {
    let cancelled = false;
    setGrafanaError(null);
    setSavedGrafana(false);
    Promise.all([fetchSettings(), fetchGrafanaDashboardConfig()])
      .then(([rawSettings, grafanaConfig]) => {
        if (cancelled) return;
        setGrafanaProfiles(grafanaConfig.profiles);
        const override = grafanaConfig.device_overrides[device.id];
        const nextProfileId = override?.profile_id ?? '';
        const nextGrafanaUrl =
          override?.custom_url ?? rawSettings[`grafana_dashboard_url:${device.id}`] ?? '';
        grafanaProfileIdRef.current = nextProfileId;
        grafanaUrlRef.current = nextGrafanaUrl;
        setGrafanaProfileId(nextProfileId);
        setGrafanaUrl(nextGrafanaUrl);
      })
      .catch(() => {
        /* non-fatal */
      });

    return () => {
      cancelled = true;
      invalidateGrafanaWork();
    };
  }, [device.id]);

  useEffect(() => {
    return () => {
      invalidateGrafanaWork();
    };
  }, []);

  useEffect(() => {
    if (readOnly) {
      invalidateGrafanaWork();
      setSavedGrafana(false);
    }
  }, [readOnly]);

  useEffect(() => {
    if (isHidden) {
      invalidateGrafanaWork();
      setSavedGrafana(false);
    }
  }, [isHidden]);

  function showSaved() {
    setSavedGrafana(true);
    clearSavedGrafanaTimer();
    savedGrafanaTimerRef.current = window.setTimeout(() => setSavedGrafana(false), 2000);
  }

  async function saveGrafanaOverride(profileId: string, customUrl: string) {
    const saveGeneration = saveGenerationRef.current;
    const nextConfig = await saveDeviceGrafanaDashboardOverride(device.id, {
      profile_id: profileId || null,
      custom_url: customUrl.trim(),
    });
    if (saveGeneration !== saveGenerationRef.current) {
      return;
    }
    setGrafanaProfiles(nextConfig.profiles);
    showSaved();
    onSettingsChange?.();
  }

  function scheduleGrafanaUpdate(value: string, profileId: string) {
    if (readOnly) return;
    clearGrafanaTimer();

    const err = value.trim() === '' ? null : validateURL(value, 'Grafana URL');
    setGrafanaError(err);
    if (err) {
      return;
    }

    grafanaTimerRef.current = window.setTimeout(() => {
      void saveGrafanaOverride(profileId, value);
    }, 500);
  }

  function handleGrafanaProfileChange(profileId: string) {
    if (readOnly) return;
    grafanaProfileIdRef.current = profileId;
    setGrafanaProfileId(profileId);
    setGrafanaError(null);
    clearGrafanaTimer();
    void saveGrafanaOverride(profileId, grafanaUrlRef.current);
  }

  if (isHidden) {
    return null;
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
          Grafana Dashboard
        </p>
        <span
          className={`text-xs text-status-up transition-opacity duration-500 ${savedGrafana ? 'opacity-100' : 'opacity-0'}`}
        >
          Saved
        </span>
      </div>
      <label className="grid gap-1 text-sm">
        <span className="text-xs text-on-bg-secondary">Dashboard Profile</span>
        <select
          value={grafanaProfileId}
          disabled={readOnly}
          onChange={(e) => handleGrafanaProfileChange(e.target.value)}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
        >
          <option value="">Use global default</option>
          {grafanaProfiles.map((profile) => (
            <option key={profile.id} value={profile.id}>
              {profile.name}
            </option>
          ))}
        </select>
      </label>
      <input
        type="url"
        value={grafanaUrl}
        placeholder="Optional custom URL override"
        disabled={readOnly}
        onChange={(e) => {
          const nextUrl = e.target.value;
          grafanaUrlRef.current = nextUrl;
          setGrafanaUrl(nextUrl);
          scheduleGrafanaUpdate(nextUrl, grafanaProfileIdRef.current);
        }}
        onBlur={() => setGrafanaError(validateURL(grafanaUrl, 'Grafana URL'))}
        className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60${grafanaError ? ' border-status-down' : ' border-outline-subtle'}`}
      />
      {grafanaError && <p className="mt-1 text-xs text-status-down">{grafanaError}</p>}
    </div>
  );
}
