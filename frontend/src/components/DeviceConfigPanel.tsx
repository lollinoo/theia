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
  revealSNMPProfile,
  setWinBoxProfile,
  testSNMPConnection,
  unassignCredentialProfile,
  updateCanvasMapDeviceAreas,
  updateCanvasMapDeviceVisualColor,
  updateDevice,
} from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import type {
  Area,
  CredentialProfile,
  Device,
  DeviceCredentialProfile,
  MetricsSource,
  SNMPProfile,
} from '../types/api';
import {
  MAX_STRING_LENGTH,
  validateIPOrHostname,
  validateMaxLength,
  validateRequired,
} from '../utils/validation';
import { MaterialIcon } from './MaterialIcon';
import { DeviceGrafanaDashboardSection } from './device-config/DeviceGrafanaDashboardSection';
import { DevicePollingSection } from './device-config/DevicePollingSection';
import { DeviceTopologyDiscoverySection } from './device-config/DeviceTopologyDiscoverySection';
import {
  type DeviceFormModel,
  applySNMPProfile,
  createDeviceConfigFormModel,
  defaultVirtualNodeColor,
  normalizeVirtualNodeColor,
} from './forms/deviceFormModels';
import { buildUpdateDevicePayload } from './forms/deviceFormSubmitters';

type DeviceUpdatePayload = Parameters<typeof updateDevice>[1];

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
    mapVisualColor: device.map_visual_color ?? null,
  });
}

function sameAreaIds(first: string[], second: string[]): boolean {
  if (first.length !== second.length) return false;
  const sortedFirst = [...first].sort();
  const sortedSecond = [...second].sort();
  return sortedFirst.every((value, index) => value === sortedSecond[index]);
}

function sameStringRecord(
  first: Record<string, string> | undefined,
  second: Record<string, string> | undefined,
): boolean {
  const firstEntries = Object.entries(first ?? {}).sort(([left], [right]) =>
    left.localeCompare(right),
  );
  const secondEntries = Object.entries(second ?? {}).sort(([left], [right]) =>
    left.localeCompare(right),
  );
  if (firstEntries.length !== secondEntries.length) return false;
  return firstEntries.every(
    ([key, value], index) =>
      secondEntries[index]?.[0] === key && secondEntries[index]?.[1] === value,
  );
}

function deviceConfigGlobalPayloadHasChanges(
  device: Device,
  payload: DeviceUpdatePayload,
): boolean {
  if (payload.hostname !== undefined && payload.hostname !== device.hostname) return true;
  if (payload.ip !== undefined && payload.ip !== device.ip) return true;
  if (payload.notes !== undefined && payload.notes !== (device.notes ?? null)) return true;
  if (payload.snmp !== undefined) return true;
  if (payload.tags !== undefined && !sameStringRecord(payload.tags, device.tags)) return true;
  if (payload.vendor !== undefined && payload.vendor !== device.vendor) return true;
  if (payload.metrics_source !== undefined && payload.metrics_source !== device.metrics_source) {
    return true;
  }
  if (
    payload.prometheus_label_name !== undefined &&
    payload.prometheus_label_name !== device.prometheus_label_name
  ) {
    return true;
  }
  if (
    payload.prometheus_label_value !== undefined &&
    payload.prometheus_label_value !== device.prometheus_label_value
  ) {
    return true;
  }
  if (
    payload.topology_discovery_mode !== undefined &&
    payload.topology_discovery_mode !== device.topology_discovery_mode
  ) {
    return true;
  }
  if (payload.area_ids !== undefined && !sameAreaIds(payload.area_ids, device.area_ids ?? [])) {
    return true;
  }
  return false;
}

function normalizeMapVisualColor(color: string | null | undefined): string | null {
  return color ? normalizeVirtualNodeColor(color) : null;
}

interface DeviceConfigPanelProps {
  device: Device;
  readOnly?: boolean;
  areas?: Area[];
  mapContext?: {
    mapId: string;
    mapName: string;
  };
  onDeviceUpdated: (updated: Device) => void;
  onDeviceDeleted: () => void;
  onRemoveFromMap?: (deviceId: string) => void | Promise<void>;
  onSettingsChange?: () => void;
  onWinBoxAvailabilityChange?: (hasWinboxProfile: boolean) => void;
  isVirtual?: boolean;
}

export function DeviceConfigPanel({
  device,
  readOnly = false,
  areas: providedAreas,
  mapContext,
  onDeviceUpdated,
  onDeviceDeleted,
  onRemoveFromMap,
  onSettingsChange,
  onWinBoxAvailabilityChange,
  isVirtual,
}: DeviceConfigPanelProps) {
  const [form, setForm] = useState(() => createDeviceConfigFormModel(device, Boolean(isVirtual)));
  const [editLoading, setEditLoading] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);
  const [editSaved, setEditSaved] = useState(false);

  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [removeFromMapLoading, setRemoveFromMapLoading] = useState(false);

  const [profiles, setProfiles] = useState<SNMPProfile[]>([]);
  const [credentialProfiles, setCredentialProfiles] = useState<CredentialProfile[]>([]);
  const [assignments, setAssignments] = useState<DeviceCredentialProfile[]>([]);
  const [assignmentsLoading, setAssignmentsLoading] = useState(false);
  const [showAddSelect, setShowAddSelect] = useState(false);
  const [removingId, setRemovingId] = useState<string | null>(null);
  const [loadedAreas, setLoadedAreas] = useState<Area[]>([]);
  const [prometheusAvailable, setPrometheusAvailable] = useState<boolean | null>(null);

  // Field-level validation errors
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const editSavedTimerRef = useRef<number | null>(null);
  const winBoxAvailabilityCallbackRef = useRef(onWinBoxAvailabilityChange);

  const usesPrometheus =
    form.metricsMode === 'prometheus' || form.metricsMode === 'prometheus_snmp_fallback';
  const usesSNMP = form.metricsMode === 'snmp' || form.metricsMode === 'prometheus_snmp_fallback';
  const deviceConfigSyncKey = buildDeviceConfigSyncKey(device, Boolean(isVirtual));
  const areas = providedAreas ?? loadedAreas;

  function updateForm(update: Partial<DeviceFormModel>) {
    setForm((current) => ({ ...current, ...update }));
  }

  function updateSnmp(update: Partial<DeviceFormModel['snmp']>) {
    setForm((current) => ({ ...current, snmp: { ...current.snmp, ...update } }));
  }

  function updatePrometheus(update: Partial<DeviceFormModel['prometheus']>) {
    setForm((current) => ({ ...current, prometheus: { ...current.prometheus, ...update } }));
  }

  function updateVirtual(update: Partial<DeviceFormModel['virtual']>) {
    setForm((current) => ({ ...current, virtual: { ...current.virtual, ...update } }));
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
    checkPrometheusHealth()
      .then((result) => {
        setPrometheusAvailable(result.enabled !== false && result.available);
      })
      .catch(() => {
        setPrometheusAvailable(false);
      });
  }, []);

  useEffect(() => {
    if (providedAreas) {
      setLoadedAreas([]);
      return;
    }

    fetchAreas()
      .then(setLoadedAreas)
      .catch(() => {
        /* non-fatal */
      });
  }, [providedAreas]);

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

  // Sync inputs when saved configuration changes. Runtime-only updates such as
  // status changes should not reset in-progress edits.
  useEffect(() => {
    setForm(createDeviceConfigFormModel(device, Boolean(isVirtual)));
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
      const payload = buildUpdateDevicePayload(device, form);
      const mapScopedAreaEdit = Boolean(mapContext);
      const nextVisualColor = normalizeMapVisualColor(form.virtual.visualColor);
      const currentVisualColor = normalizeMapVisualColor(device.map_visual_color);
      const mapScopedVisualColorEdit =
        Boolean(isVirtual && mapContext) && nextVisualColor !== currentVisualColor;
      const { area_ids: _areaIds, ...payloadWithoutAreaIds } = payload;
      const globalPayload = mapScopedAreaEdit ? payloadWithoutAreaIds : payload;
      const shouldUpdateGlobal =
        !mapScopedVisualColorEdit || deviceConfigGlobalPayloadHasChanges(device, globalPayload);
      const updatedGlobal = shouldUpdateGlobal
        ? await updateDevice(device.id, globalPayload)
        : device;
      if (mapScopedAreaEdit && mapContext && !sameAreaIds(device.area_ids ?? [], form.areaIds)) {
        await updateCanvasMapDeviceAreas(mapContext.mapId, {
          device_ids: [device.id],
          area_ids: form.areaIds,
        });
      }
      if (mapScopedVisualColorEdit && mapContext) {
        await updateCanvasMapDeviceVisualColor(mapContext.mapId, device.id, {
          visual_color: nextVisualColor,
        });
      }
      const updated = mapScopedAreaEdit
        ? { ...updatedGlobal, area_ids: [...form.areaIds], map_visual_color: nextVisualColor }
        : updatedGlobal;
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

  async function handleRemoveFromMap() {
    if (readOnly || !mapContext || !onRemoveFromMap) return;
    setRemoveFromMapLoading(true);
    try {
      await onRemoveFromMap(device.id);
    } finally {
      setRemoveFromMapLoading(false);
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

  return (
    <div className="space-y-6 p-4 transition-colors duration-200">
      {/* Polling Override — physical devices only */}
      {!isVirtual && (
        <DevicePollingSection
          device={device}
          readOnly={readOnly}
          resetKey={deviceConfigSyncKey}
          onDeviceUpdated={onDeviceUpdated}
        />
      )}

      <DeviceTopologyDiscoverySection
        device={device}
        topologyDiscoveryMode={form.topologyDiscoveryMode}
        metricsMode={form.metricsMode}
        ip={form.ip}
        readOnly={readOnly}
        resetKey={deviceConfigSyncKey}
        isVirtual={isVirtual}
        onTopologyDiscoveryModeChange={(topologyDiscoveryMode) =>
          updateForm({ topologyDiscoveryMode })
        }
      />

      <DeviceGrafanaDashboardSection
        device={device}
        readOnly={readOnly}
        isVirtual={isVirtual}
        onSettingsChange={onSettingsChange}
      />

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

          {isVirtual && mapContext && (
            <div className="space-y-1">
              <label
                htmlFor="device-virtual-node-color"
                className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary"
              >
                Virtual node color
              </label>
              <div className="flex items-center gap-2">
                <input
                  id="device-virtual-node-color"
                  aria-label="Virtual node color"
                  type="color"
                  value={form.virtual.visualColor ?? defaultVirtualNodeColor}
                  onChange={(e) =>
                    updateVirtual({ visualColor: normalizeVirtualNodeColor(e.target.value) })
                  }
                  className="h-10 w-12 shrink-0 cursor-pointer rounded-lg border border-outline-subtle bg-elevated p-1"
                />
                <button
                  type="button"
                  onClick={() => updateVirtual({ visualColor: null })}
                  className="rounded-lg bg-surface-high px-3 py-2 text-xs font-medium text-on-bg-secondary transition-colors hover:text-on-bg"
                >
                  Use area/default color
                </button>
              </div>
            </div>
          )}

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

      {mapContext && onRemoveFromMap && (
        <div className="mt-6 space-y-2 rounded-lg border border-outline-subtle bg-surface-container px-3 py-3">
          <p className="text-xs text-on-bg-secondary">
            Removes this device only from {mapContext.mapName}. Inventory and other maps are kept.
          </p>
          <button
            type="button"
            disabled={readOnly || removeFromMapLoading}
            onClick={() => {
              void handleRemoveFromMap();
            }}
            className="w-full rounded-lg bg-surface-high px-4 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
          >
            {removeFromMapLoading ? 'Removing...' : 'Remove from this map'}
          </button>
        </div>
      )}

      {/* Delete Device Everywhere */}
      <div className="mt-6 space-y-3">
        {!confirmDelete ? (
          <button
            type="button"
            disabled={readOnly}
            onClick={() => setConfirmDelete(true)}
            className="w-full rounded-lg border border-status-down/30 bg-status-down/10 px-4 py-2 text-sm font-medium text-status-down transition-colors hover:bg-status-down/20 disabled:cursor-not-allowed disabled:opacity-50"
          >
            Delete device everywhere
          </button>
        ) : (
          <div className="space-y-2 rounded-lg border border-status-down/30 bg-status-down/10 p-3">
            <p className="text-sm text-status-down">
              Are you sure? This deletes the device everywhere and cannot be undone.
            </p>
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
