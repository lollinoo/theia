import { useEffect, useRef, useState } from 'react';
import { updateDevice } from '../../api/client';
import { ServerError, ValidationError } from '../../api/errors';
import type { Device, DevicePollClass } from '../../types/api';

const POLLING_PRESETS = [
  { label: 'Use device default', value: 'default' },
  { label: '15 seconds', value: '15' },
  { label: '30 seconds', value: '30' },
  { label: '60 seconds', value: '60' },
  { label: '5 minutes', value: '300' },
  { label: 'Custom...', value: 'custom' },
];

const PRESET_VALUES = new Set(
  POLLING_PRESETS.map((p) => p.value).filter((v) => v !== 'custom' && v !== 'default'),
);

const DEFAULT_POLLING_DURATION_BY_CLASS: Record<DevicePollClass, string> = {
  core: '30s',
  standard: '1m',
  low: '5m',
};

const POLLING_OVERRIDE_ERROR = 'Polling override must be an integer between 5 and 3600 seconds';

interface DevicePollingSectionProps {
  device: Device;
  readOnly?: boolean;
  resetKey?: string;
  onDeviceUpdated: (updated: Device) => void;
}

function pollingStateFromOverride(pollIntervalOverride: number | null | undefined): {
  pollingValue: string;
  customPolling: string;
} {
  if (pollIntervalOverride === null || pollIntervalOverride === undefined) {
    return { pollingValue: 'default', customPolling: '' };
  }

  const overrideValue = String(pollIntervalOverride);
  if (PRESET_VALUES.has(overrideValue)) {
    return { pollingValue: overrideValue, customPolling: '' };
  }

  return { pollingValue: 'custom', customPolling: overrideValue };
}

export function DevicePollingSection({
  device,
  readOnly = false,
  resetKey,
  onDeviceUpdated,
}: DevicePollingSectionProps) {
  const initialPollingState = pollingStateFromOverride(device.poll_interval_override);
  const [pollingValue, setPollingValue] = useState(initialPollingState.pollingValue);
  const [customPolling, setCustomPolling] = useState(initialPollingState.customPolling);
  const [pollingEnabled, setPollingEnabled] = useState(device.polling_enabled !== false);
  const [savedPolling, setSavedPolling] = useState(false);
  const [pollingError, setPollingError] = useState<string | null>(null);

  const pollingTimerRef = useRef<number | null>(null);
  const savedPollingTimerRef = useRef<number | null>(null);
  const pollingSaveGenerationRef = useRef(0);
  const pollingContextKey = JSON.stringify({
    deviceId: device.id,
    readOnly,
    resetKey: resetKey ?? '',
  });
  const pollingContextKeyRef = useRef(pollingContextKey);
  pollingContextKeyRef.current = pollingContextKey;

  function beginPollingSave() {
    pollingSaveGenerationRef.current += 1;
    return {
      generation: pollingSaveGenerationRef.current,
      contextKey: pollingContextKey,
    };
  }

  function isCurrentPollingSave(save: { generation: number; contextKey: string }): boolean {
    return (
      pollingSaveGenerationRef.current === save.generation &&
      pollingContextKeyRef.current === save.contextKey
    );
  }

  function clearPendingPollingUpdate() {
    if (pollingTimerRef.current !== null) {
      window.clearTimeout(pollingTimerRef.current);
      pollingTimerRef.current = null;
    }
  }

  useEffect(() => {
    const nextPollingState = pollingStateFromOverride(device.poll_interval_override);
    setPollingValue(nextPollingState.pollingValue);
    setCustomPolling(nextPollingState.customPolling);
    setPollingEnabled(device.polling_enabled !== false);
    setPollingError(null);
  }, [device.id, device.poll_interval_override, device.polling_enabled, resetKey]);

  useEffect(() => {
    return () => clearPendingPollingUpdate();
  }, [device.id, readOnly, resetKey]);

  function showSaved() {
    setSavedPolling(true);
    if (savedPollingTimerRef.current !== null) window.clearTimeout(savedPollingTimerRef.current);
    savedPollingTimerRef.current = window.setTimeout(() => setSavedPolling(false), 2000);
  }

  function schedulePollingUpdate(rawValue: string, isDelete = false) {
    if (readOnly) return;
    if (!pollingEnabled) return;
    clearPendingPollingUpdate();
    pollingTimerRef.current = window.setTimeout(() => {
      const save = beginPollingSave();
      const pollIntervalOverride = isDelete ? null : Number.parseInt(rawValue, 10);
      void updateDevice(device.id, { poll_interval_override: pollIntervalOverride })
        .then((updated) => {
          if (!isCurrentPollingSave(save)) return;
          setPollingError(null);
          showSaved();
          onDeviceUpdated(updated);
        })
        .catch((error) => {
          if (!isCurrentPollingSave(save)) return;
          if (error instanceof ValidationError || error instanceof ServerError) {
            setPollingError(error.message);
            return;
          }
          setPollingError(
            error instanceof Error ? error.message : 'Failed to update polling override',
          );
        });
    }, 500);
  }

  async function handlePollingEnabledChange(enabled: boolean) {
    if (readOnly) return;
    clearPendingPollingUpdate();
    const previous = pollingEnabled;
    setPollingEnabled(enabled);
    setPollingError(null);
    const save = beginPollingSave();
    try {
      const updated = await updateDevice(device.id, { polling_enabled: enabled });
      if (!isCurrentPollingSave(save)) return;
      showSaved();
      onDeviceUpdated(updated);
    } catch (error) {
      if (!isCurrentPollingSave(save)) return;
      setPollingEnabled(previous);
      if (error instanceof ValidationError || error instanceof ServerError) {
        setPollingError(error.message);
        return;
      }
      setPollingError(error instanceof Error ? error.message : 'Failed to update polling state');
    }
  }

  function handlePollingChange(value: string) {
    if (readOnly) return;
    setPollingValue(value);
    setPollingError(null);
    if (value === 'default') {
      setCustomPolling('');
      schedulePollingUpdate('', true);
    } else if (value !== 'custom') {
      setCustomPolling('');
      schedulePollingUpdate(value);
    }
  }

  function handleCustomPollingChange(rawValue: string) {
    if (readOnly) return;
    setCustomPolling(rawValue);

    if (!/^\d+$/.test(rawValue)) {
      clearPendingPollingUpdate();
      setPollingError(POLLING_OVERRIDE_ERROR);
      return;
    }

    const parsedValue = Number.parseInt(rawValue, 10);
    if (!Number.isInteger(parsedValue) || parsedValue < 5 || parsedValue > 3600) {
      clearPendingPollingUpdate();
      setPollingError(POLLING_OVERRIDE_ERROR);
      return;
    }

    setPollingError(null);
    schedulePollingUpdate(rawValue);
  }

  const pollClass = device.poll_class || 'standard';
  const defaultPollingDuration = DEFAULT_POLLING_DURATION_BY_CLASS[pollClass];

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
          Polling Override
        </p>
        <span
          className={`text-xs text-status-up transition-opacity duration-500 ${savedPolling ? 'opacity-100' : 'opacity-0'}`}
        >
          Saved
        </span>
      </div>
      <div className="rounded-lg bg-surface-high px-3 py-2">
        <p className="text-sm text-on-bg">
          Default cadence: every {defaultPollingDuration} ({pollClass} class)
        </p>
      </div>
      <label className="flex items-center justify-between gap-3 rounded-lg bg-surface-high px-3 py-2">
        <span className="min-w-0">
          <span className="block text-sm font-medium text-on-bg">Continuous Polling</span>
          <span className="block text-xs text-on-bg-secondary">
            {pollingEnabled
              ? 'Backend recurring polling is active.'
              : 'Continuous polling is suspended for this device.'}
          </span>
        </span>
        <input
          type="checkbox"
          role="switch"
          aria-label="Continuous Polling"
          aria-checked={pollingEnabled}
          checked={pollingEnabled}
          disabled={readOnly}
          onChange={(e) => {
            void handlePollingEnabledChange(e.target.checked);
          }}
          className="h-5 w-9 cursor-pointer accent-primary disabled:cursor-not-allowed disabled:opacity-60"
        />
      </label>
      <select
        value={pollingValue}
        disabled={readOnly || !pollingEnabled}
        onChange={(e) => handlePollingChange(e.target.value)}
        className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
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
          placeholder="Seconds (5-3600)"
          disabled={readOnly || !pollingEnabled}
          onChange={(e) => handleCustomPollingChange(e.target.value)}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
        />
      )}
      {pollingError && <p className="text-xs text-status-down">{pollingError}</p>}
    </div>
  );
}
