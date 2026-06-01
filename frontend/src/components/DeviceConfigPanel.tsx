import { useEffect, useRef, useState } from 'react';
import {
  deleteDevice,
  revealSNMPProfile,
  updateCanvasMapDeviceAreas,
  updateCanvasMapDeviceVisualColor,
  updateDevice,
} from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import type { Area, Device } from '../types/api';
import {
  MAX_STRING_LENGTH,
  validateIPOrHostname,
  validateMaxLength,
  validateRequired,
} from '../utils/validation';
import { DeviceAreasSection } from './device-config/DeviceAreasSection';
import { DeviceCredentialsSection } from './device-config/DeviceCredentialsSection';
import { DeviceGrafanaDashboardSection } from './device-config/DeviceGrafanaDashboardSection';
import { DeviceNetworkSettingsSection } from './device-config/DeviceNetworkSettingsSection';
import { DevicePollingSection } from './device-config/DevicePollingSection';
import { DeviceSnmpTestButton } from './device-config/DeviceSnmpTestButton';
import { DeviceTopologyDiscoverySection } from './device-config/DeviceTopologyDiscoverySection';
import {
  type DeviceFormModel,
  applySNMPProfile,
  createDeviceConfigFormModel,
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

  // Field-level validation errors
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const editSavedTimerRef = useRef<number | null>(null);

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

  // Sync inputs when saved configuration changes. Runtime-only updates such as
  // status changes should not reset in-progress edits.
  useEffect(() => {
    setForm(createDeviceConfigFormModel(device, Boolean(isVirtual)));
    setFieldErrors({});
  }, [deviceConfigSyncKey, isVirtual]);

  async function applyProfile(profileId: string) {
    setEditError(null);
    try {
      const revealed = await revealSNMPProfile(profileId, 'apply SNMP profile to device config');
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

          <DeviceAreasSection
            form={form}
            areas={providedAreas}
            readOnly={readOnly}
            isVirtual={isVirtual}
            mapContext={mapContext}
            onFormChange={updateForm}
            onVirtualChange={updateVirtual}
          />

          <DeviceNetworkSettingsSection
            device={device}
            form={form}
            readOnly={readOnly}
            isVirtual={isVirtual}
            fieldErrors={fieldErrors}
            onFormChange={updateForm}
            onPrometheusChange={updatePrometheus}
            onSnmpChange={updateSnmp}
            onFieldError={setFieldError}
            onSNMPProfileSelected={(profileId) => {
              void applyProfile(profileId);
            }}
          >
            <DeviceCredentialsSection
              device={device}
              readOnly={readOnly}
              isVirtual={isVirtual}
              onWinBoxAvailabilityChange={onWinBoxAvailabilityChange}
            />
          </DeviceNetworkSettingsSection>
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
      {!isVirtual && usesSNMP && <DeviceSnmpTestButton deviceId={device.id} />}

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
