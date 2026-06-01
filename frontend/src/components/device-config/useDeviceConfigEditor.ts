import {
  type Dispatch,
  type FormEvent,
  type MutableRefObject,
  type SetStateAction,
  useEffect,
  useRef,
  useState,
} from 'react';

import {
  revealSNMPProfile,
  updateCanvasMapDeviceAreas,
  updateCanvasMapDeviceVisualColor,
  updateDevice,
} from '../../api/client';
import { ServerError, ValidationError } from '../../api/errors';
import type { Device } from '../../types/api';
import {
  MAX_STRING_LENGTH,
  validateIPOrHostname,
  validateMaxLength,
  validateRequired,
} from '../../utils/validation';
import {
  type DeviceFormModel,
  applySNMPProfile,
  createDeviceConfigFormModel,
  normalizeVirtualNodeColor,
} from '../forms/deviceFormModels';
import { buildUpdateDevicePayload } from '../forms/deviceFormSubmitters';

type DeviceUpdatePayload = Parameters<typeof updateDevice>[1];

interface DeviceConfigEditorOptions {
  device: Device;
  readOnly: boolean;
  mapContext?: {
    mapId: string;
    mapName: string;
  };
  onDeviceUpdated: (updated: Device) => void;
  isVirtual?: boolean;
}

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
    areaIds: [...(device.area_ids ?? [])].sort(),
    prometheusLabelName: device.prometheus_label_name || 'instance',
    prometheusLabelValue: device.prometheus_label_value || '',
    virtualSubtype: device.tags?.virtual_subtype ?? 'internet',
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

function showSaved(
  setter: Dispatch<SetStateAction<boolean>>,
  timerRef: MutableRefObject<number | null>,
) {
  setter(true);
  if (timerRef.current !== null) window.clearTimeout(timerRef.current);
  timerRef.current = window.setTimeout(() => setter(false), 2000);
}

export function useDeviceConfigEditor({
  device,
  readOnly,
  mapContext,
  onDeviceUpdated,
  isVirtual,
}: DeviceConfigEditorOptions) {
  const [form, setForm] = useState(() => createDeviceConfigFormModel(device, Boolean(isVirtual)));
  const [editLoading, setEditLoading] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);
  const [editSaved, setEditSaved] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const editSavedTimerRef = useRef<number | null>(null);
  const applyProfileGenerationRef = useRef(0);
  const saveGenerationRef = useRef(0);

  const usesPrometheus =
    form.metricsMode === 'prometheus' || form.metricsMode === 'prometheus_snmp_fallback';
  const usesSNMP = form.metricsMode === 'snmp' || form.metricsMode === 'prometheus_snmp_fallback';
  const deviceConfigSyncKey = buildDeviceConfigSyncKey(device, Boolean(isVirtual));
  const deviceConfigSyncKeyRef = useRef(deviceConfigSyncKey);
  deviceConfigSyncKeyRef.current = deviceConfigSyncKey;

  useEffect(() => {
    return () => {
      if (editSavedTimerRef.current !== null) {
        window.clearTimeout(editSavedTimerRef.current);
        editSavedTimerRef.current = null;
      }
    };
  }, []);

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

  function validateIPField(value: string): string | null {
    return validateIPOrHostname(value.trim());
  }

  useEffect(() => {
    saveGenerationRef.current += 1;
    setForm(createDeviceConfigFormModel(device, Boolean(isVirtual)));
    setFieldErrors({});
    setEditLoading(false);
  }, [deviceConfigSyncKey, isVirtual]);

  async function applyProfile(profileId: string) {
    const revealSyncKey = deviceConfigSyncKey;
    const revealGeneration = applyProfileGenerationRef.current + 1;
    applyProfileGenerationRef.current = revealGeneration;
    setEditError(null);
    try {
      const revealed = await revealSNMPProfile(profileId, 'apply SNMP profile to device config');
      if (
        deviceConfigSyncKeyRef.current !== revealSyncKey ||
        applyProfileGenerationRef.current !== revealGeneration
      ) {
        return;
      }
      setForm((current) => applySNMPProfile(current, revealed));
    } catch (err) {
      if (
        deviceConfigSyncKeyRef.current !== revealSyncKey ||
        applyProfileGenerationRef.current !== revealGeneration
      ) {
        return;
      }
      setEditError(err instanceof Error ? err.message : 'Failed to reveal SNMP profile.');
    }
  }

  async function handleEditSave(e: FormEvent) {
    e.preventDefault();
    if (readOnly) return;

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

    const saveSyncKey = deviceConfigSyncKey;
    const saveGeneration = saveGenerationRef.current + 1;
    saveGenerationRef.current = saveGeneration;
    const isCurrentSave = () =>
      deviceConfigSyncKeyRef.current === saveSyncKey &&
      saveGenerationRef.current === saveGeneration;

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
      if (!isCurrentSave()) return;
      if (mapScopedAreaEdit && mapContext && !sameAreaIds(device.area_ids ?? [], form.areaIds)) {
        await updateCanvasMapDeviceAreas(mapContext.mapId, {
          device_ids: [device.id],
          area_ids: form.areaIds,
        });
        if (!isCurrentSave()) return;
      }
      if (mapScopedVisualColorEdit && mapContext) {
        await updateCanvasMapDeviceVisualColor(mapContext.mapId, device.id, {
          visual_color: nextVisualColor,
        });
        if (!isCurrentSave()) return;
      }
      const updated = mapScopedAreaEdit
        ? { ...updatedGlobal, area_ids: [...form.areaIds], map_visual_color: nextVisualColor }
        : updatedGlobal;
      showSaved(setEditSaved, editSavedTimerRef);
      onDeviceUpdated(updated);
    } catch (err) {
      if (!isCurrentSave()) return;
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
      if (isCurrentSave()) {
        setEditLoading(false);
      }
    }
  }

  return {
    form,
    fieldErrors,
    editLoading,
    editError,
    editSaved,
    usesSNMP,
    deviceConfigSyncKey,
    updateForm,
    updateSnmp,
    updatePrometheus,
    updateVirtual,
    setFieldError,
    handleBlur,
    validateIPField,
    validateDisplayNameField,
    applyProfile,
    handleEditSave,
  };
}
