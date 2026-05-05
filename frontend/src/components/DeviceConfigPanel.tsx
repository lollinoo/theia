import { useEffect, useRef, useState } from 'react';
import {
  assignCredentialProfile,
  checkPrometheusHealth,
  clearWinBoxProfile,
  deleteDevice,
  fetchAreas,
  fetchCredentialProfiles,
  fetchDeviceCredentialProfiles,
  fetchSNMPProfiles,
  fetchSettings,
  revealSNMPProfile,
  runTopologyDiscovery,
  setWinBoxProfile,
  testSNMPConnection,
  unassignCredentialProfile,
  updateDevice,
  updateSetting,
} from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import type {
  Area,
  CredentialProfile,
  Device,
  DeviceCredentialProfile,
  DevicePollClass,
  MetricsSource,
  SNMPProfile,
  TopologyDiscoveryMode,
} from '../types/api';
import {
  TOPOLOGY_DISCOVERY_MODE_OPTIONS,
  formatTopologyBootstrapState,
  formatTopologyDiscoveryMode,
  formatTopologyDiscoveryResult,
  formatTopologyDiscoveryTimestamp,
  formatTopologyFollowupExpectation,
} from '../utils/topologyDiscovery';
import {
  MAX_STRING_LENGTH,
  validateIPOrHostname,
  validateMaxLength,
  validateRequired,
  validateURL,
} from '../utils/validation';
import { MaterialIcon } from './MaterialIcon';
import {
  type DeviceFormModel,
  applySNMPProfile,
  createDeviceConfigFormModel,
} from './forms/deviceFormModels';
import { buildUpdateDevicePayload } from './forms/deviceFormSubmitters';

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

function buildDeviceConfigSyncKey(device: Device, isVirtual: boolean): string {
  return JSON.stringify({
    id: device.id,
    isVirtual,
    hostname: device.hostname,
    ip: device.ip,
    displayName: device.tags?.display_name ?? '',
    notes: device.notes ?? '',
    vendor: device.vendor ?? '',
    metricsSource: device.metrics_source ?? 'snmp',
    topologyDiscoveryMode: device.topology_discovery_mode ?? 'inherit',
    areaIds: device.area_ids ?? [],
    prometheusLabelName: device.prometheus_label_name || 'instance',
    prometheusLabelValue: device.prometheus_label_value || '',
    virtualSubtype: device.tags?.virtual_subtype ?? 'internet',
    pollIntervalOverride: device.poll_interval_override ?? null,
    pollingEnabled: device.polling_enabled !== false,
  });
}

interface DeviceConfigPanelProps {
  device: Device;
  readOnly?: boolean;
  onDeviceUpdated: (updated: Device) => void;
  onDeviceDeleted: () => void;
  onSettingsChange?: () => void;
  onWinBoxAvailabilityChange?: (hasWinboxProfile: boolean) => void;
  isVirtual?: boolean;
}

export function DeviceConfigPanel({
  device,
  readOnly = false,
  onDeviceUpdated,
  onDeviceDeleted,
  onSettingsChange,
  onWinBoxAvailabilityChange,
  isVirtual,
}: DeviceConfigPanelProps) {
  const grafanaKey = `grafana_dashboard_url:${device.id}`;

  const [pollingValue, setPollingValue] = useState('default');
  const [customPolling, setCustomPolling] = useState('');
  const [pollingEnabled, setPollingEnabled] = useState(device.polling_enabled !== false);
  const [grafanaUrl, setGrafanaUrl] = useState('');

  const [form, setForm] = useState(() => createDeviceConfigFormModel(device, Boolean(isVirtual)));
  const [editLoading, setEditLoading] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);
  const [editSaved, setEditSaved] = useState(false);

  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const [profiles, setProfiles] = useState<SNMPProfile[]>([]);
  const [credentialProfiles, setCredentialProfiles] = useState<CredentialProfile[]>([]);
  const [assignments, setAssignments] = useState<DeviceCredentialProfile[]>([]);
  const [assignmentsLoading, setAssignmentsLoading] = useState(false);
  const [showAddSelect, setShowAddSelect] = useState(false);
  const [removingId, setRemovingId] = useState<string | null>(null);
  const [areas, setAreas] = useState<Area[]>([]);
  const [prometheusAvailable, setPrometheusAvailable] = useState<boolean | null>(null);

  const [savedPolling, setSavedPolling] = useState(false);
  const [savedGrafana, setSavedGrafana] = useState(false);
  const [topologyDiscoveryDefaultMode, setTopologyDiscoveryDefaultMode] =
    useState<TopologyDiscoveryMode>('lldp_cdp');
  const [topologyDiscoveryMessage, setTopologyDiscoveryMessage] = useState<string | null>(null);
  const [topologyDiscoveryError, setTopologyDiscoveryError] = useState<string | null>(null);
  const [topologyDiscoveryRunning, setTopologyDiscoveryRunning] = useState(false);

  // Field-level validation errors
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const pollingTimerRef = useRef<number | null>(null);
  const grafanaTimerRef = useRef<number | null>(null);
  const savedPollingTimerRef = useRef<number | null>(null);
  const savedGrafanaTimerRef = useRef<number | null>(null);
  const editSavedTimerRef = useRef<number | null>(null);
  const winBoxAvailabilityCallbackRef = useRef(onWinBoxAvailabilityChange);

  const usesPrometheus =
    form.metricsMode === 'prometheus' || form.metricsMode === 'prometheus_snmp_fallback';
  const usesSNMP = form.metricsMode === 'snmp' || form.metricsMode === 'prometheus_snmp_fallback';
  const deviceConfigSyncKey = buildDeviceConfigSyncKey(device, Boolean(isVirtual));

  function updateForm(update: Partial<DeviceFormModel>) {
    setForm((current) => ({ ...current, ...update }));
  }

  function updateSnmp(update: Partial<DeviceFormModel['snmp']>) {
    setForm((current) => ({ ...current, snmp: { ...current.snmp, ...update } }));
  }

  function updatePrometheus(update: Partial<DeviceFormModel['prometheus']>) {
    setForm((current) => ({ ...current, prometheus: { ...current.prometheus, ...update } }));
  }

  function syncPollingState(pollIntervalOverride: number | null | undefined) {
    if (pollIntervalOverride === null || pollIntervalOverride === undefined) {
      setPollingValue('default');
      setCustomPolling('');
      return;
    }

    const overrideValue = String(pollIntervalOverride);
    if (PRESET_VALUES.has(overrideValue)) {
      setPollingValue(overrideValue);
      setCustomPolling('');
      return;
    }

    setPollingValue('custom');
    setCustomPolling(overrideValue);
  }

  function setFieldError(field: string, err: string | null) {
    setFieldErrors((prev) => {
      if (err) return { ...prev, [field]: err };
      const next = { ...prev };
      delete next[field];
      return next;
    });
  }

  function handleBlur(field: string, validator: () => string | null) {
    return () => {
      const err = validator();
      setFieldError(field, err);
    };
  }

  function validateDisplayNameField(value: string): string | null {
    return (
      (isVirtual ? validateRequired(value, 'Display Name') : null) ??
      validateMaxLength(value, MAX_STRING_LENGTH, 'Display name')
    );
  }

  useEffect(() => {
    winBoxAvailabilityCallbackRef.current = onWinBoxAvailabilityChange;
  }, [onWinBoxAvailabilityChange]);

  async function loadAssignments(deviceId = device.id) {
    setAssignmentsLoading(true);
    try {
      const nextAssignments = await fetchDeviceCredentialProfiles(deviceId);
      setAssignments(nextAssignments);
      winBoxAvailabilityCallbackRef.current?.(
        nextAssignments.some((assignment) => assignment.is_winbox),
      );
    } catch {
      // non-fatal — section shows empty
    } finally {
      setAssignmentsLoading(false);
    }
  }

  useEffect(() => {
    if (!isVirtual) {
      fetchSNMPProfiles()
        .then(setProfiles)
        .catch(() => {
          /* non-fatal */
        });
      fetchCredentialProfiles()
        .then(setCredentialProfiles)
        .catch(() => {
          /* non-fatal */
        });
    }
    fetchAreas()
      .then(setAreas)
      .catch(() => {
        /* non-fatal */
      });
    checkPrometheusHealth()
      .then((result) => {
        setPrometheusAvailable(result.enabled !== false && result.available);
      })
      .catch(() => {
        setPrometheusAvailable(false);
      });
  }, []);

  useEffect(() => {
    let cancelled = false;
    setShowAddSelect(false);
    setRemovingId(null);

    if (isVirtual) {
      setAssignments([]);
      setAssignmentsLoading(false);
      winBoxAvailabilityCallbackRef.current?.(false);
      return () => {
        cancelled = true;
      };
    }

    setAssignments([]);
    setAssignmentsLoading(true);
    fetchDeviceCredentialProfiles(device.id)
      .then((nextAssignments) => {
        if (cancelled) return;
        setAssignments(nextAssignments);
        winBoxAvailabilityCallbackRef.current?.(
          nextAssignments.some((assignment) => assignment.is_winbox),
        );
      })
      .catch(() => {
        if (!cancelled) setAssignments([]);
      })
      .finally(() => {
        if (!cancelled) setAssignmentsLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [device.id, isVirtual]);

  useEffect(() => {
    fetchSettings()
      .then((settings) => {
        setGrafanaUrl(settings[grafanaKey] ?? '');
        setTopologyDiscoveryDefaultMode(
          (settings['topology_discovery_default_mode'] as TopologyDiscoveryMode | undefined) ??
            'lldp_cdp',
        );
      })
      .catch(() => {
        /* non-fatal */
      });
  }, [device.id, grafanaKey]);

  // Sync inputs when saved configuration changes. Runtime-only updates such as
  // status changes should not reset in-progress edits.
  useEffect(() => {
    setForm(createDeviceConfigFormModel(device, Boolean(isVirtual)));
    syncPollingState(device.poll_interval_override);
    setPollingEnabled(device.polling_enabled !== false);
    setTopologyDiscoveryMessage(null);
    setTopologyDiscoveryError(null);
    setTopologyDiscoveryRunning(false);
    setFieldErrors({});
  }, [deviceConfigSyncKey, isVirtual]);

  async function applyProfile(profileId: string) {
    const profile = profiles.find((p) => p.id === profileId);
    if (!profile) return;
    setEditError(null);
    try {
      const revealed = await revealSNMPProfile(profile.id, 'apply SNMP profile to device config');
      setForm((current) => applySNMPProfile(current, revealed));
    } catch (err) {
      setEditError(err instanceof Error ? err.message : 'Failed to reveal SNMP profile.');
    }
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
    if (readOnly) return;
    if (!pollingEnabled) return;
    if (pollingTimerRef.current !== null) window.clearTimeout(pollingTimerRef.current);
    pollingTimerRef.current = window.setTimeout(() => {
      const pollIntervalOverride = isDelete ? null : Number.parseInt(rawValue, 10);
      void updateDevice(device.id, { poll_interval_override: pollIntervalOverride })
        .then((updated) => {
          setFieldError('polling', null);
          showSaved(setSavedPolling, savedPollingTimerRef);
          onDeviceUpdated(updated);
        })
        .catch((error) => {
          if (error instanceof ValidationError || error instanceof ServerError) {
            setFieldError('polling', error.message);
            return;
          }
          setFieldError(
            'polling',
            error instanceof Error ? error.message : 'Failed to update polling override',
          );
        });
    }, 500);
  }

  async function handlePollingEnabledChange(enabled: boolean) {
    if (readOnly) return;
    if (pollingTimerRef.current !== null) {
      window.clearTimeout(pollingTimerRef.current);
      pollingTimerRef.current = null;
    }
    const previous = pollingEnabled;
    setPollingEnabled(enabled);
    setFieldError('polling', null);
    try {
      const updated = await updateDevice(device.id, { polling_enabled: enabled });
      showSaved(setSavedPolling, savedPollingTimerRef);
      onDeviceUpdated(updated);
    } catch (error) {
      setPollingEnabled(previous);
      if (error instanceof ValidationError || error instanceof ServerError) {
        setFieldError('polling', error.message);
        return;
      }
      setFieldError(
        'polling',
        error instanceof Error ? error.message : 'Failed to update polling state',
      );
    }
  }

  function handlePollingChange(value: string) {
    if (readOnly) return;
    setPollingValue(value);
    setFieldError('polling', null);
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
      if (pollingTimerRef.current !== null) window.clearTimeout(pollingTimerRef.current);
      setFieldError('polling', POLLING_OVERRIDE_ERROR);
      return;
    }

    const parsedValue = Number.parseInt(rawValue, 10);
    if (!Number.isInteger(parsedValue) || parsedValue < 5 || parsedValue > 3600) {
      if (pollingTimerRef.current !== null) window.clearTimeout(pollingTimerRef.current);
      setFieldError('polling', POLLING_OVERRIDE_ERROR);
      return;
    }

    setFieldError('polling', null);
    schedulePollingUpdate(rawValue);
  }

  function scheduleGrafanaUpdate(value: string) {
    if (readOnly) return;
    if (grafanaTimerRef.current !== null) window.clearTimeout(grafanaTimerRef.current);
    grafanaTimerRef.current = null;

    const err = value.trim() === '' ? null : validateURL(value, 'Grafana URL');
    setFieldError('grafanaUrl', err);
    if (err) {
      return;
    }

    grafanaTimerRef.current = window.setTimeout(() => {
      void updateSetting(grafanaKey, value).then(() => {
        showSaved(setSavedGrafana, savedGrafanaTimerRef);
        onSettingsChange?.();
      });
    }, 500);
  }

  async function handleEditSave(e: React.FormEvent) {
    e.preventDefault();
    if (readOnly) return;

    // Validate before API call
    const errors: Record<string, string> = {};
    const trimmedIP = form.ip.trim();
    if (!(isVirtual && trimmedIP === '')) {
      const ipErr = validateIPOrHostname(trimmedIP);
      if (ipErr) errors['ip'] = ipErr;
    }
    const displayNameErr = validateDisplayNameField(form.displayName);
    if (displayNameErr) errors['displayName'] = displayNameErr;
    if (usesPrometheus) {
      const labelValueErr = validateMaxLength(
        form.prometheus.labelValue,
        MAX_STRING_LENGTH,
        'Label value',
      );
      if (labelValueErr) errors['prometheusLabelValue'] = labelValueErr;
    }
    const isV3 = form.snmp.version === '3';
    if (!isV3 && form.snmp.community.trim()) {
      const communityErr = validateMaxLength(
        form.snmp.community,
        MAX_STRING_LENGTH,
        'Community string',
      );
      if (communityErr) errors['community'] = communityErr;
    }
    if (isV3 && form.snmp.username.trim()) {
      const usernameErr = validateMaxLength(form.snmp.username, MAX_STRING_LENGTH, 'Username');
      if (usernameErr) errors['username'] = usernameErr;
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors);
      return;
    }

    setEditLoading(true);
    setEditError(null);
    try {
      const updated = await updateDevice(device.id, buildUpdateDevicePayload(device, form));
      showSaved(setEditSaved, editSavedTimerRef);
      onDeviceUpdated(updated);
    } catch (err) {
      if (err instanceof ServerError) {
        setEditError(
          err.correlationId
            ? `Something went wrong (ref: ${err.correlationId})`
            : 'Something went wrong',
        );
      } else if (err instanceof ValidationError) {
        setEditError(err.message);
      } else {
        setEditError(err instanceof Error ? err.message : 'Failed to update device.');
      }
    } finally {
      setEditLoading(false);
    }
  }

  async function handleDelete() {
    if (readOnly) return;
    setDeleteLoading(true);
    try {
      await deleteDevice(device.id);
      onDeviceDeleted();
    } catch {
      setDeleteLoading(false);
      setConfirmDelete(false);
    }
  }

  async function handleRunTopologyDiscovery() {
    if (readOnly) return;
    setTopologyDiscoveryRunning(true);
    setTopologyDiscoveryError(null);
    setTopologyDiscoveryMessage(null);
    try {
      await runTopologyDiscovery(device.id);
      setTopologyDiscoveryMessage(
        'Topology discovery started. Links and ports will refresh when the SNMP pass completes.',
      );
    } catch (err) {
      if (err instanceof ServerError || err instanceof ValidationError) {
        setTopologyDiscoveryError(err.message);
      } else {
        setTopologyDiscoveryError(
          err instanceof Error ? err.message : 'Failed to start topology discovery.',
        );
      }
    } finally {
      setTopologyDiscoveryRunning(false);
    }
  }

  async function handleAssign(profileId: string) {
    if (readOnly) return;
    try {
      await assignCredentialProfile(device.id, profileId);
      setShowAddSelect(false);
      void loadAssignments(device.id);
    } catch {
      // non-fatal
    }
  }

  async function handleUnassign(profileId: string) {
    if (readOnly) return;
    try {
      await unassignCredentialProfile(device.id, profileId);
      setRemovingId(null);
      void loadAssignments(device.id);
    } catch {
      // non-fatal
    }
  }

  async function handleToggleWinBox(profileId: string, currentlyDesignated: boolean) {
    if (readOnly) return;
    try {
      if (currentlyDesignated) {
        await clearWinBoxProfile(device.id);
      } else {
        await setWinBoxProfile(device.id, profileId);
      }
      void loadAssignments(device.id);
    } catch {
      // non-fatal
    }
  }

  const pollClass = device.poll_class || 'standard';
  const defaultPollingDuration = DEFAULT_POLLING_DURATION_BY_CLASS[pollClass];
  const discoveryState = device.topology_bootstrap_state || 'idle';
  const discoveryBusy =
    topologyDiscoveryRunning ||
    discoveryState === 'pending' ||
    discoveryState === 'followup_scheduled';
  const discoveryRunDisabled =
    readOnly || discoveryBusy || form.metricsMode === 'prometheus' || form.ip.trim() === '';
  const effectiveTopologyDiscoveryMode = device.effective_topology_discovery_mode || 'off';
  const configuredTopologyDiscoveryMode =
    form.topologyDiscoveryMode === 'inherit'
      ? `Use global default (${formatTopologyDiscoveryMode(topologyDiscoveryDefaultMode)})`
      : formatTopologyDiscoveryMode(form.topologyDiscoveryMode);
  const nextTopologyFollowup = formatTopologyFollowupExpectation(
    discoveryState,
    device.last_topology_discovery_at,
  );

  return (
    <div className="space-y-6 p-4 transition-colors duration-200">
      {/* Polling Override — physical devices only */}
      {!isVirtual && (
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
          {fieldErrors['polling'] && (
            <p className="text-xs text-status-down">{fieldErrors['polling']}</p>
          )}
        </div>
      )}

      {!isVirtual && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
              Topology Discovery
            </p>
            <span className="text-xs text-on-bg-secondary">
              Effective: {formatTopologyDiscoveryMode(effectiveTopologyDiscoveryMode)}
            </span>
          </div>
          <select
            id="device-topology-discovery-mode"
            aria-label="Topology Discovery"
            value={form.topologyDiscoveryMode}
            disabled={readOnly}
            onChange={(e) =>
              updateForm({ topologyDiscoveryMode: e.target.value as TopologyDiscoveryMode })
            }
            className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
          >
            {TOPOLOGY_DISCOVERY_MODE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <div className="space-y-2 rounded-lg bg-surface-high p-3">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs uppercase tracking-widest text-on-bg-secondary">
                Device Setting
              </span>
              <span className="text-sm text-on-bg">{configuredTopologyDiscoveryMode}</span>
            </div>
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs uppercase tracking-widest text-on-bg-secondary">
                Bootstrap State
              </span>
              <span className="text-sm text-on-bg">
                {formatTopologyBootstrapState(device.topology_bootstrap_state)}
              </span>
            </div>
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs uppercase tracking-widest text-on-bg-secondary">
                Last Discovery
              </span>
              <span className="text-sm text-on-bg">
                {formatTopologyDiscoveryTimestamp(device.last_topology_discovery_at)}
              </span>
            </div>
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs uppercase tracking-widest text-on-bg-secondary">
                Last Result
              </span>
              <span className="text-sm text-on-bg">
                {formatTopologyDiscoveryResult(device.last_topology_discovery_result)}
              </span>
            </div>
            {nextTopologyFollowup && (
              <div className="flex items-center justify-between gap-2">
                <span className="text-xs uppercase tracking-widest text-on-bg-secondary">
                  Next Follow-up
                </span>
                <span className="text-sm text-on-bg">{nextTopologyFollowup}</span>
              </div>
            )}
          </div>
          <button
            type="button"
            onClick={() => {
              void handleRunTopologyDiscovery();
            }}
            disabled={discoveryRunDisabled}
            className="w-full rounded-lg bg-surface-high px-4 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
          >
            {discoveryBusy ? 'Topology discovery running...' : 'Run Topology Discovery Now'}
          </button>
          <p className="text-xs text-on-bg-secondary">
            {form.metricsMode === 'prometheus'
              ? 'Prometheus-only devices cannot run SNMP topology discovery until SNMP or fallback mode is enabled.'
              : form.ip.trim() === ''
                ? 'Topology discovery requires a device IP.'
                : 'Bootstrap once opens a short discovery window, may queue one follow-up to fill missing ports, then returns the device to Off.'}
          </p>
          {topologyDiscoveryMessage && (
            <p className="rounded-lg border border-status-up/30 bg-status-up/10 px-3 py-2 text-xs text-status-up">
              {topologyDiscoveryMessage}
            </p>
          )}
          {topologyDiscoveryError && (
            <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
              {topologyDiscoveryError}
            </p>
          )}
        </div>
      )}

      {/* Custom Grafana URL — only for devices with IP */}
      {(!isVirtual || device.ip) && (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
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
            disabled={readOnly}
            onChange={(e) => {
              setGrafanaUrl(e.target.value);
              scheduleGrafanaUpdate(e.target.value);
            }}
            onBlur={handleBlur('grafanaUrl', () => validateURL(grafanaUrl, 'Grafana URL'))}
            className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60${fieldErrors['grafanaUrl'] ? ' border-status-down' : ' border-outline-subtle'}`}
          />
          {fieldErrors['grafanaUrl'] && (
            <p className="mt-1 text-xs text-status-down">{fieldErrors['grafanaUrl']}</p>
          )}
        </div>
      )}

      {/* Edit Device */}
      <form
        noValidate
        onSubmit={(e) => {
          void handleEditSave(e);
        }}
        className="space-y-3"
      >
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Edit Device
          </p>
          <span
            className={`text-xs text-status-up transition-opacity duration-500 ${editSaved ? 'opacity-100' : 'opacity-0'}`}
          >
            Saved
          </span>
        </div>

        {device.sys_name && (
          <div className="bg-surface-high rounded-lg px-3 py-2">
            <p className="text-[10px] uppercase tracking-widest text-on-bg-secondary mb-0.5">
              Auto-discovered Hostname
            </p>
            <p className="text-sm font-mono text-on-bg">{device.sys_name}</p>
          </div>
        )}

        <fieldset disabled={readOnly} className="space-y-3 disabled:opacity-70">
          <input
            type="text"
            value={form.displayName}
            onChange={(e) => {
              updateForm({ displayName: e.target.value });
              setFieldError('displayName', null);
            }}
            onBlur={handleBlur('displayName', () => validateDisplayNameField(form.displayName))}
            placeholder={
              isVirtual
                ? 'e.g. ISP Gateway'
                : device.sys_name
                  ? `Override "${device.sys_name}"`
                  : 'Custom name (optional)'
            }
            required={isVirtual}
            className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed${fieldErrors['displayName'] ? ' border-status-down' : ' border-outline-subtle'}`}
          />
          {fieldErrors['displayName'] && (
            <p className="mt-1 text-xs text-status-down">{fieldErrors['displayName']}</p>
          )}

          <input
            type="text"
            value={form.ip}
            onChange={(e) => {
              updateForm({ ip: e.target.value });
              setFieldError('ip', null);
            }}
            onBlur={handleBlur('ip', () => validateIPOrHostname(form.ip.trim()))}
            placeholder="IP Address"
            className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors['ip'] ? ' border-status-down' : ' border-outline-subtle'}`}
          />
          {fieldErrors['ip'] && (
            <p className="mt-1 text-xs text-status-down">{fieldErrors['ip']}</p>
          )}

          <div className="space-y-1">
            <label
              htmlFor="device-notes"
              className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary"
            >
              Device Notes
            </label>
            <textarea
              id="device-notes"
              value={form.notes}
              onChange={(e) => updateForm({ notes: e.target.value })}
              rows={5}
              placeholder="Add internal notes for this device (optional)"
              className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
            />
          </div>

          <div className="space-y-1">
            <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
              Areas
            </label>
            {form.areaIds.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {form.areaIds.map((id) => {
                  const area = areas.find((a) => a.id === id);
                  if (!area) return null;
                  return (
                    <span
                      key={id}
                      className="inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium text-on-bg"
                      style={{
                        backgroundColor: `${area.color}25`,
                        border: `1px solid ${area.color}60`,
                      }}
                    >
                      <span
                        className="inline-block h-2 w-2 rounded-full"
                        style={{ backgroundColor: area.color }}
                      />
                      {area.name}
                      <button
                        type="button"
                        onClick={() =>
                          updateForm({ areaIds: form.areaIds.filter((areaId) => areaId !== id) })
                        }
                        className="ml-0.5 text-on-bg-secondary hover:text-on-bg"
                      >
                        <svg
                          className="w-3 h-3"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M6 18L18 6M6 6l12 12"
                          />
                        </svg>
                      </button>
                    </span>
                  );
                })}
              </div>
            )}
            <select
              value=""
              disabled={areas.filter((a) => !form.areaIds.includes(a.id)).length === 0}
              onChange={(e) => {
                if (e.target.value) {
                  updateForm({ areaIds: [...form.areaIds, e.target.value] });
                }
              }}
              className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:opacity-50"
            >
              <option value="">
                {areas.length === 0
                  ? 'No areas created'
                  : areas.filter((a) => !form.areaIds.includes(a.id)).length === 0
                    ? 'All areas assigned'
                    : form.areaIds.length === 0
                      ? 'Unassigned - select area...'
                      : 'Add another area...'}
              </option>
              {areas
                .filter((a) => !form.areaIds.includes(a.id))
                .map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
            </select>
          </div>

          {/* Vendor, SSH, Metrics Source, Prometheus, SNMP — physical devices only */}
          {!isVirtual && (
            <>
              <div className="space-y-1">
                <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
                  Vendor
                </label>
                <select
                  value={form.vendor}
                  onChange={(e) => updateForm({ vendor: e.target.value })}
                  className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                >
                  <option value="">— Select vendor —</option>
                  <option value="mikrotik">MikroTik</option>
                </select>
                <p className="text-xs text-on-bg-secondary">
                  Vendor tag determines backup commands and metric queries.
                </p>
              </div>

              {/* Credentials section */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
                    Credentials
                  </p>
                  <button
                    type="button"
                    onClick={() => setShowAddSelect((v) => !v)}
                    className="px-2 py-1 text-xs rounded bg-surface-high text-on-bg-secondary hover:text-on-bg"
                  >
                    + Add
                  </button>
                </div>

                {showAddSelect && (
                  <div className="flex items-center gap-2">
                    <select
                      defaultValue=""
                      onChange={(e) => {
                        if (e.target.value) {
                          void handleAssign(e.target.value);
                        }
                      }}
                      className="flex-1 rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                    >
                      <option value="" disabled>
                        Select a profile...
                      </option>
                      {credentialProfiles
                        .filter((p) => !assignments.some((a) => a.profile_id === p.id))
                        .map((p) => (
                          <option key={p.id} value={p.id}>
                            {p.name}
                          </option>
                        ))}
                    </select>
                    <button
                      type="button"
                      onClick={() => setShowAddSelect(false)}
                      className="px-2 py-1 text-xs rounded bg-surface-high text-on-bg-secondary hover:text-on-bg"
                    >
                      Dismiss
                    </button>
                  </div>
                )}

                {assignmentsLoading && (
                  <p className="text-xs text-on-bg-secondary">Loading credentials...</p>
                )}

                {!assignmentsLoading && assignments.length === 0 && (
                  <p className="text-xs text-on-bg-secondary">
                    No credentials assigned. Add a profile to enable WinBox launch.
                  </p>
                )}

                {!assignmentsLoading &&
                  assignments.map((assignment) => (
                    <div key={assignment.profile_id} className="rounded-lg bg-surface-high p-3">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="text-sm font-medium text-on-bg truncate">
                            {assignment.name}
                          </span>
                          <span className="text-xs font-medium px-2 py-0.5 bg-surface rounded-full text-on-bg-secondary shrink-0">
                            {assignment.role}
                          </span>
                        </div>
                        <div className="flex items-center gap-1 shrink-0 ml-2">
                          {/* WinBox key toggle */}
                          <button
                            type="button"
                            title={
                              assignment.is_winbox
                                ? 'Clear WinBox designation'
                                : 'Designate as WinBox profile'
                            }
                            onClick={() => {
                              void handleToggleWinBox(assignment.profile_id, assignment.is_winbox);
                            }}
                            className={`p-1 rounded-md transition-colors${assignment.is_winbox ? ' text-primary' : ' text-on-bg-secondary hover:text-on-bg'}`}
                          >
                            <MaterialIcon name="key" size={18} />
                          </button>
                          {/* Remove button */}
                          <button
                            type="button"
                            title="Remove assignment"
                            onClick={() => setRemovingId(assignment.profile_id)}
                            className="p-1 rounded-md text-on-bg-secondary hover:text-status-down transition-colors"
                          >
                            <MaterialIcon name="remove" size={18} />
                          </button>
                        </div>
                      </div>

                      {/* Inline removal confirmation */}
                      {removingId === assignment.profile_id && (
                        <div className="mt-2 border border-status-down/30 bg-status-down/10 rounded-lg px-3 py-2 flex items-center justify-between">
                          <p className="text-xs text-status-down">Delete this profile?</p>
                          <div className="flex gap-2">
                            <button
                              type="button"
                              onClick={() => setRemovingId(null)}
                              className="px-2 py-1 text-xs rounded bg-surface-high text-on-bg hover:bg-elevated"
                            >
                              Keep Profile
                            </button>
                            <button
                              type="button"
                              onClick={() => {
                                void handleUnassign(assignment.profile_id);
                              }}
                              className="px-2 py-1 text-xs rounded bg-status-down text-white hover:opacity-90"
                            >
                              Delete
                            </button>
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
              </div>

              {prometheusAvailable === false && (
                <p className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">
                  Prometheus is not configured or unreachable. Only SNMP Direct is available.
                </p>
              )}

              <div className="space-y-1">
                <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
                  Metrics Source
                </label>
                <select
                  value={form.metricsMode}
                  onChange={(e) => {
                    const val = e.target.value as
                      | 'prometheus'
                      | 'snmp'
                      | 'prometheus_snmp_fallback';
                    if (
                      (val === 'prometheus' || val === 'prometheus_snmp_fallback') &&
                      !prometheusAvailable
                    )
                      return;
                    updateForm({ metricsMode: val as MetricsSource });
                  }}
                  className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                >
                  <option value="snmp">SNMP Direct</option>
                  <option value="prometheus" disabled={!prometheusAvailable}>
                    Prometheus{!prometheusAvailable ? ' (unavailable)' : ''}
                  </option>
                  <option value="prometheus_snmp_fallback" disabled={!prometheusAvailable}>
                    Prometheus + SNMP Fallback{!prometheusAvailable ? ' (unavailable)' : ''}
                  </option>
                </select>
                {form.metricsMode === 'prometheus' && (
                  <p className="text-xs text-on-bg-secondary">
                    Metrics from Prometheus only. No fallback if Prometheus is unreachable.
                  </p>
                )}
                {form.metricsMode === 'prometheus_snmp_fallback' && (
                  <p className="text-xs text-on-bg-secondary">
                    Falls back to SNMP if Prometheus is unavailable or has no data for this device.
                  </p>
                )}
              </div>

              {/* Prometheus Target — visible when metrics source uses Prometheus */}
              {usesPrometheus && (
                <div className="space-y-2 bg-surface-high rounded-lg p-3">
                  <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
                    Prometheus Target
                  </p>
                  <div className="space-y-1">
                    <label className="text-xs text-on-bg-secondary">Label</label>
                    <select
                      value={form.prometheus.labelName}
                      onChange={(e) => updatePrometheus({ labelName: e.target.value })}
                      className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                    >
                      <option value="instance">instance (IP address)</option>
                      <option value="identity">identity</option>
                      <option value="vendor">vendor</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs text-on-bg-secondary">
                      Value
                      {form.prometheus.labelName === 'instance' ? ' (defaults to IP if blank)' : ''}
                    </label>
                    <input
                      type="text"
                      value={form.prometheus.labelValue}
                      onChange={(e) => {
                        updatePrometheus({ labelValue: e.target.value });
                        setFieldError('prometheusLabelValue', null);
                      }}
                      onBlur={handleBlur('prometheusLabelValue', () =>
                        validateMaxLength(
                          form.prometheus.labelValue,
                          MAX_STRING_LENGTH,
                          'Label value',
                        ),
                      )}
                      placeholder={
                        form.prometheus.labelName === 'instance'
                          ? form.ip || device.ip
                          : 'e.g. my-router'
                      }
                      className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors['prometheusLabelValue'] ? ' border-status-down' : ' border-outline-subtle'}`}
                    />
                    {fieldErrors['prometheusLabelValue'] && (
                      <p className="mt-1 text-xs text-status-down">
                        {fieldErrors['prometheusLabelValue']}
                      </p>
                    )}
                  </div>
                </div>
              )}

              {/* SNMP Credentials — visible when metrics source uses SNMP */}
              {usesSNMP && (
                <>
                  {profiles.length > 0 && (
                    <select
                      defaultValue=""
                      onChange={(e) => {
                        void applyProfile(e.target.value);
                        e.target.value = '';
                      }}
                      className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                    >
                      <option value="" disabled>
                        Load credentials from profile...
                      </option>
                      {profiles.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name} (SNMP {p.snmp.version})
                        </option>
                      ))}
                    </select>
                  )}

                  <select
                    value={form.snmp.version}
                    onChange={(e) =>
                      updateSnmp({ version: e.target.value as DeviceFormModel['snmp']['version'] })
                    }
                    className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                  >
                    <option value="2c">SNMP v2c</option>
                    <option value="3">SNMP v3</option>
                  </select>

                  {form.snmp.version !== '3' && (
                    <>
                      <input
                        type="text"
                        value={form.snmp.community}
                        onChange={(e) => {
                          updateSnmp({ community: e.target.value });
                          setFieldError('community', null);
                        }}
                        onBlur={handleBlur('community', () =>
                          validateMaxLength(
                            form.snmp.community,
                            MAX_STRING_LENGTH,
                            'Community string',
                          ),
                        )}
                        placeholder="SNMP Community (leave blank to keep current)"
                        className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors['community'] ? ' border-status-down' : ' border-outline-subtle'}`}
                      />
                      {fieldErrors['community'] && (
                        <p className="mt-1 text-xs text-status-down">{fieldErrors['community']}</p>
                      )}
                    </>
                  )}

                  {form.snmp.version === '3' && (
                    <div className="space-y-2 bg-surface-high rounded-lg p-3">
                      <p className="text-xs text-on-bg-secondary">
                        SNMPv3 Credentials (leave blank to keep current)
                      </p>
                      <input
                        type="text"
                        value={form.snmp.username}
                        onChange={(e) => {
                          updateSnmp({ username: e.target.value });
                          setFieldError('username', null);
                        }}
                        onBlur={handleBlur('username', () =>
                          validateMaxLength(form.snmp.username, MAX_STRING_LENGTH, 'Username'),
                        )}
                        placeholder="Username"
                        className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors['username'] ? ' border-status-down' : ' border-outline-subtle'}`}
                      />
                      {fieldErrors['username'] && (
                        <p className="mt-1 text-xs text-status-down">{fieldErrors['username']}</p>
                      )}
                      <select
                        value={form.snmp.securityLevel}
                        onChange={(e) => updateSnmp({ securityLevel: e.target.value })}
                        className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                      >
                        <option value="noAuthNoPriv">No Auth, No Privacy</option>
                        <option value="authNoPriv">Auth, No Privacy</option>
                        <option value="authPriv">Auth + Privacy</option>
                      </select>
                      {(form.snmp.securityLevel === 'authNoPriv' ||
                        form.snmp.securityLevel === 'authPriv') && (
                        <>
                          <select
                            value={form.snmp.authProtocol}
                            onChange={(e) => updateSnmp({ authProtocol: e.target.value })}
                            className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                          >
                            <option value="SHA">SHA</option>
                            <option value="MD5">MD5</option>
                            <option value="SHA-224">SHA-224</option>
                            <option value="SHA-256">SHA-256</option>
                            <option value="SHA-384">SHA-384</option>
                            <option value="SHA-512">SHA-512</option>
                          </select>
                          <input
                            type="password"
                            value={form.snmp.authPassword}
                            onChange={(e) => updateSnmp({ authPassword: e.target.value })}
                            placeholder="Auth Key"
                            autoComplete="new-password"
                            className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                          />
                        </>
                      )}
                      {form.snmp.securityLevel === 'authPriv' && (
                        <>
                          <select
                            value={form.snmp.privProtocol}
                            onChange={(e) => updateSnmp({ privProtocol: e.target.value })}
                            className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                          >
                            <option value="AES">AES</option>
                            <option value="DES">DES</option>
                          </select>
                          <input
                            type="password"
                            value={form.snmp.privPassword}
                            onChange={(e) => updateSnmp({ privPassword: e.target.value })}
                            placeholder="Encryption Key"
                            autoComplete="new-password"
                            className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
                          />
                        </>
                      )}
                    </div>
                  )}
                </>
              )}
            </>
          )}
        </fieldset>

        {editError && (
          <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
            {editError}
          </p>
        )}

        <button
          type="submit"
          disabled={readOnly || editLoading}
          className="w-full rounded-lg bg-surface-high px-4 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
        >
          {editLoading ? 'Saving...' : 'Save Changes'}
        </button>
      </form>

      {/* SNMP Test — visible when metrics source uses SNMP (physical only) */}
      {!isVirtual && usesSNMP && <SNMPTestButton deviceId={device.id} />}

      {/* Delete Device */}
      <div className="mt-6 space-y-3">
        {!confirmDelete ? (
          <button
            type="button"
            disabled={readOnly}
            onClick={() => setConfirmDelete(true)}
            className="w-full rounded-lg border border-status-down/30 bg-status-down/10 px-4 py-2 text-sm font-medium text-status-down transition-colors hover:bg-status-down/20 disabled:cursor-not-allowed disabled:opacity-50"
          >
            Delete Device
          </button>
        ) : (
          <div className="space-y-2 rounded-lg border border-status-down/30 bg-status-down/10 p-3">
            <p className="text-sm text-status-down">Are you sure? This cannot be undone.</p>
            <div className="flex gap-2">
              <button
                type="button"
                disabled={readOnly}
                onClick={() => setConfirmDelete(false)}
                className="flex-1 rounded-lg bg-surface-high px-3 py-1.5 text-xs text-on-bg hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={readOnly || deleteLoading}
                onClick={() => {
                  void handleDelete();
                }}
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

function SNMPTestButton({ deviceId }: { deviceId: string }) {
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<{
    success: boolean;
    sys_name?: string;
    sys_descr?: string;
    error?: string;
    target_ip?: string;
    snmp_version?: string;
  } | null>(null);

  async function handleTest() {
    setTesting(true);
    setResult(null);
    try {
      const r = await testSNMPConnection(deviceId);
      setResult(r);
    } catch (err) {
      setResult({ success: false, error: err instanceof Error ? err.message : 'Test failed' });
    } finally {
      setTesting(false);
    }
  }

  return (
    <div className="space-y-2">
      <button
        type="button"
        onClick={() => {
          void handleTest();
        }}
        disabled={testing}
        className="w-full rounded-lg bg-surface-high px-4 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
      >
        {testing ? 'Testing SNMP...' : 'Test SNMP Connectivity'}
      </button>
      {result && (
        <div
          className={`rounded-lg border px-3 py-2 text-xs ${
            result.success
              ? 'border-status-up/30 bg-status-up/10 text-status-up'
              : 'border-status-down/30 bg-status-down/10 text-status-down'
          }`}
        >
          {result.success ? (
            <div className="space-y-1">
              <div className="font-medium">SNMP OK</div>
              {result.sys_name && <div>sysName: {result.sys_name}</div>}
              {result.sys_descr && <div className="truncate">sysDescr: {result.sys_descr}</div>}
            </div>
          ) : (
            <div className="space-y-1">
              <div className="font-medium">SNMP Failed</div>
              <div className="break-words">{result.error}</div>
              {result.target_ip && (
                <div className="text-on-bg-secondary">Target: {result.target_ip}:161 (UDP)</div>
              )}
              {result.snmp_version && (
                <div className="text-on-bg-secondary">Version: {result.snmp_version}</div>
              )}
              <div className="text-on-bg-secondary mt-1">
                Check: SNMP enabled on device, community/credentials correct, UDP/161 reachable from
                container
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
