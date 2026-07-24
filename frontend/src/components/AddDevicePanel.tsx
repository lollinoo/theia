/**
 * Renders add device panel UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import { useEffect, useState } from 'react';
import {
  addDeviceToCanvasMap,
  assignCredentialProfile,
  checkPrometheusHealth,
  createDevice,
  fetchAreas,
  fetchCredentialProfiles,
  fetchDevices,
  fetchSNMPProfiles,
  revealSNMPProfile,
  setWinBoxProfile,
  updateCanvasMapDeviceAreas,
  updateCanvasMapDeviceVisualColor,
} from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import type {
  Area,
  CredentialProfile,
  Device,
  SNMPProfile,
  TopologyDiscoveryMode,
} from '../types/api';
import {
  formatTopologyDiscoveryMode,
  TOPOLOGY_DISCOVERY_MODE_OPTIONS,
} from '../utils/topologyDiscovery';
import {
  MAX_STRING_LENGTH,
  validateIPOrHostname,
  validateMaxLength,
  validateRequired,
} from '../utils/validation';
import {
  applySNMPProfile,
  createAddDeviceFormModel,
  createEmptyDeviceAddressFormRow,
  type DeviceFormModel,
  defaultVirtualNodeColor,
  normalizeVirtualNodeColor,
  resetDeviceFormMode,
  type SecondaryDeviceAddressRole,
} from './forms/deviceFormModels';
import { buildCreateDevicePayload, validateProbePorts } from './forms/deviceFormSubmitters';
import { MaterialIcon } from './MaterialIcon';

interface AddDevicePanelProps {
  onDeviceAdded: (deviceId: string) => void;
  areas?: Area[];
  devices?: Device[];
  mapContext?: {
    mapId: string;
  };
}

type MetricsMode = 'snmp' | 'prometheus' | 'prometheus_snmp_fallback';
type DuplicateDeviceAddResult =
  | { status: 'not-handled' }
  | { status: 'added'; deviceId: string }
  | { status: 'error' };

function normalizeDeviceLookupValue(value: string | undefined): string {
  return (value ?? '').trim().toLowerCase();
}

function deviceAddressLookupValues(device: Device): string[] {
  return [device.ip, ...(device.addresses ?? []).map((address) => address.address)]
    .map(normalizeDeviceLookupValue)
    .filter(Boolean);
}

function deviceAddressFormRowKey(address: DeviceFormModel['additionalAddresses'][number]): string {
  return address.formId ?? `${address.address}-${address.role}-${address.label}`;
}

function duplicateMapDeviceAddressMessage(address: string): string {
  const trimmed = address.trim();
  return trimmed
    ? `a device with IP/host "${trimmed}" already exists in this map`
    : 'a device with that address already exists in this map';
}

function isDuplicateDeviceValidationError(err: unknown): err is ValidationError {
  return err instanceof ValidationError && /device.*already exists/i.test(err.message);
}

/** Renders the AddDevicePanel component within the UI component boundary. */
export function AddDevicePanel({
  onDeviceAdded,
  areas: providedAreas,
  devices = [],
  mapContext,
}: AddDevicePanelProps) {
  const [form, setForm] = useState(createAddDeviceFormModel);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Prometheus availability
  const [prometheusAvailable, setPrometheusAvailable] = useState<boolean | null>(null);
  const [prometheusCheckDone, setPrometheusCheckDone] = useState(false);

  // profiles
  const [profiles, setProfiles] = useState<SNMPProfile[]>([]);
  const [credentialProfiles, setCredentialProfiles] = useState<CredentialProfile[]>([]);
  const [selectedCredentialProfileIds, setSelectedCredentialProfileIds] = useState<string[]>([]);
  const [winboxCredentialProfileId, setWinboxCredentialProfileId] = useState('');

  // areas
  const [loadedAreas, setLoadedAreas] = useState<Area[]>([]);

  // Field-level validation errors
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const isVirtual = form.mode === 'virtual';
  const isV3 = form.snmp.version === '3';
  const needsAuth =
    form.snmp.securityLevel === 'authNoPriv' || form.snmp.securityLevel === 'authPriv';
  const needsPriv = form.snmp.securityLevel === 'authPriv';
  const usesPrometheus =
    form.metricsMode === 'prometheus' || form.metricsMode === 'prometheus_snmp_fallback';
  const usesSNMP = form.metricsMode === 'snmp' || form.metricsMode === 'prometheus_snmp_fallback';
  const areas = providedAreas ?? loadedAreas;

  function updateForm(update: Partial<DeviceFormModel>) {
    setForm((current) => ({ ...current, ...update }));
  }

  function updateSnmp(update: Partial<DeviceFormModel['snmp']>) {
    setForm((current) => ({
      ...current,
      snmp: { ...current.snmp, ...update },
    }));
  }

  function updatePrometheus(update: Partial<DeviceFormModel['prometheus']>) {
    setForm((current) => ({
      ...current,
      prometheus: { ...current.prometheus, ...update },
    }));
  }

  function updateVirtual(update: Partial<DeviceFormModel['virtual']>) {
    setForm((current) => ({
      ...current,
      virtual: { ...current.virtual, ...update },
    }));
  }

  function addAdditionalAddress() {
    setForm((current) => ({
      ...current,
      additionalAddresses: [...current.additionalAddresses, createEmptyDeviceAddressFormRow()],
    }));
  }

  function updateAdditionalAddress(
    index: number,
    update: Partial<DeviceFormModel['additionalAddresses'][number]>,
  ) {
    setForm((current) => ({
      ...current,
      additionalAddresses: current.additionalAddresses.map((address, addressIndex) =>
        addressIndex === index ? { ...address, ...update } : address,
      ),
    }));
  }

  function removeAdditionalAddress(index: number) {
    setForm((current) => ({
      ...current,
      additionalAddresses: current.additionalAddresses.filter(
        (_address, addressIndex) => addressIndex !== index,
      ),
    }));
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

  function handleModeSwitch(mode: DeviceFormModel['mode']) {
    setForm((current) => resetDeviceFormMode(current, mode));
    if (mode === 'virtual') {
      setSelectedCredentialProfileIds([]);
      setWinboxCredentialProfileId('');
    }
    setError(null);
    setFieldErrors({});
  }

  useEffect(() => {
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
    checkPrometheusHealth()
      .then((result) => {
        const nextAvailable = result.enabled !== false && result.available;
        setPrometheusAvailable(nextAvailable);
        setPrometheusCheckDone(true);
        // If prometheus is unavailable, force SNMP mode
        if (!nextAvailable) {
          setForm((current) => ({ ...current, metricsMode: 'snmp' }));
        }
      })
      .catch(() => {
        setPrometheusAvailable(false);
        setPrometheusCheckDone(true);
        setForm((current) => ({ ...current, metricsMode: 'snmp' }));
      });
  }, []);

  useEffect(() => {
    if (!providedAreas) {
      fetchAreas()
        .then(setLoadedAreas)
        .catch(() => {
          /* non-fatal */
        });
    }
  }, [providedAreas]);

  function buildCreatePayloadForCurrentScope() {
    const payload = buildCreateDevicePayload(form);
    if (!mapContext) {
      return payload;
    }
    const { area_ids: _areaIds, ...payloadWithoutAreaIds } = payload;
    return { ...payloadWithoutAreaIds, skip_primary_map_membership: true };
  }

  function currentMapAddressConflictMessage(
    payload: ReturnType<typeof buildCreatePayloadForCurrentScope>,
  ) {
    if (!mapContext) {
      return null;
    }
    const payloadAddresses = (payload.addresses ?? [{ address: payload.ip ?? '' }])
      .map((address) => ({
        raw: address.address,
        normalized: normalizeDeviceLookupValue(address.address),
      }))
      .filter((address) => address.normalized !== '');
    if (payloadAddresses.length === 0) {
      return null;
    }
    const existingAddresses = devices.flatMap(deviceAddressLookupValues);
    const conflict = payloadAddresses.find((address) =>
      existingAddresses.includes(address.normalized),
    );
    return conflict ? duplicateMapDeviceAddressMessage(conflict.raw) : null;
  }

  async function addDeviceToCurrentMap(deviceId: string) {
    if (!mapContext) {
      return;
    }
    await addDeviceToCanvasMap(mapContext.mapId, deviceId, {
      include_connected_links: true,
    });
    if (form.areaIds.length > 0) {
      await updateCanvasMapDeviceAreas(mapContext.mapId, {
        device_ids: [deviceId],
        area_ids: form.areaIds,
      });
    }
    if (form.mode === 'virtual' && form.virtual.visualColor) {
      await updateCanvasMapDeviceVisualColor(mapContext.mapId, deviceId, {
        visual_color: normalizeVirtualNodeColor(form.virtual.visualColor),
      });
    }
  }

  async function addExistingDeviceToCurrentMapOnDuplicate(
    err: unknown,
    payload: ReturnType<typeof buildCreatePayloadForCurrentScope>,
  ): Promise<DuplicateDeviceAddResult> {
    if (!mapContext || !isDuplicateDeviceValidationError(err)) {
      return { status: 'not-handled' };
    }

    const lookupValues = new Set(
      [payload.ip, payload.hostname, ...(payload.addresses ?? []).map((address) => address.address)]
        .map(normalizeDeviceLookupValue)
        .filter(Boolean),
    );
    if (lookupValues.size === 0) {
      return { status: 'not-handled' };
    }

    const existingDevice = (await fetchDevices()).find(
      (device) =>
        lookupValues.has(normalizeDeviceLookupValue(device.ip)) ||
        lookupValues.has(normalizeDeviceLookupValue(device.hostname)) ||
        deviceAddressLookupValues(device).some((address) => lookupValues.has(address)),
    );
    if (!existingDevice) {
      return { status: 'not-handled' };
    }
    const requestedVirtual = form.mode === 'virtual';
    if ((existingDevice.device_type === 'virtual') !== requestedVirtual) {
      return { status: 'not-handled' };
    }

    try {
      await addDeviceToCurrentMap(existingDevice.id);
      return { status: 'added', deviceId: existingDevice.id };
    } catch (mapErr) {
      if (mapErr instanceof ServerError) {
        setError(
          mapErr.correlationId
            ? `Something went wrong (ref: ${mapErr.correlationId})`
            : 'Something went wrong',
        );
      } else if (mapErr instanceof ValidationError) {
        setError(mapErr.message);
      } else {
        setError(
          mapErr instanceof Error ? mapErr.message : 'Failed to add existing device to map.',
        );
      }
      return { status: 'error' };
    }
  }

  async function applyProfile(profileId: string) {
    const profile = profiles.find((p) => p.id === profileId);
    if (!profile) return;
    setError(null);
    try {
      const revealed = await revealSNMPProfile(profile.id, 'apply SNMP profile to device form');
      setForm((current) => applySNMPProfile(current, revealed));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reveal SNMP profile.');
    }
  }

  function handleMetricsModeChange(value: MetricsMode) {
    if ((value === 'prometheus' || value === 'prometheus_snmp_fallback') && !prometheusAvailable) {
      return; // guard against selecting unavailable option
    }
    updateForm({ metricsMode: value });
  }

  function toggleCredentialProfile(profileId: string) {
    setSelectedCredentialProfileIds((current) => {
      if (current.includes(profileId)) {
        if (winboxCredentialProfileId === profileId) {
          setWinboxCredentialProfileId('');
        }
        return current.filter((id) => id !== profileId);
      }
      return [...current, profileId];
    });
  }

  function validateAdditionalAddressRows(primaryAddress: string): Record<string, string> {
    const errors: Record<string, string> = {};
    const seen = new Set<string>();
    const primary = normalizeDeviceLookupValue(primaryAddress);
    if (primary) {
      seen.add(primary);
    }

    form.additionalAddresses.forEach((address, index) => {
      const probePortsErr = validateProbePorts(address.probePorts);
      if (probePortsErr) {
        errors[`additionalAddressProbePorts${index}`] = probePortsErr;
      }

      const trimmed = address.address.trim();
      if (trimmed === '') {
        return;
      }
      const validationError = validateIPOrHostname(trimmed);
      if (validationError) {
        errors[`additionalAddress${index}`] = validationError;
        return;
      }
      const normalized = normalizeDeviceLookupValue(trimmed);
      if (seen.has(normalized)) {
        errors[`additionalAddress${index}`] = 'Duplicate device address';
        return;
      }
      seen.add(normalized);
    });

    return errors;
  }

  async function assignSelectedCredentials(deviceId: string) {
    if (selectedCredentialProfileIds.length === 0) return;

    for (const profileId of selectedCredentialProfileIds) {
      await assignCredentialProfile(deviceId, profileId);
    }
    if (
      winboxCredentialProfileId &&
      selectedCredentialProfileIds.includes(winboxCredentialProfileId)
    ) {
      await setWinBoxProfile(deviceId, winboxCredentialProfileId);
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (isVirtual) {
      // Validate virtual mode fields
      const errors: Record<string, string> = {};
      const displayNameErr =
        validateRequired(form.displayName, 'Display Name') ??
        validateMaxLength(form.displayName, MAX_STRING_LENGTH, 'Display name');
      if (displayNameErr) errors['displayName'] = displayNameErr;
      if (form.ip.trim()) {
        const virtualIpErr = validateIPOrHostname(form.ip.trim());
        if (virtualIpErr) errors['virtualIp'] = virtualIpErr;
      }
      if (Object.keys(errors).length > 0) {
        setFieldErrors(errors);
        return;
      }
      setError(null);
      const payload = buildCreatePayloadForCurrentScope();
      const mapAddressConflict = currentMapAddressConflictMessage(payload);
      if (mapAddressConflict) {
        setError(mapAddressConflict);
        return;
      }
      setLoading(true);
      try {
        const created = await createDevice(payload);
        await addDeviceToCurrentMap(created.id);
        onDeviceAdded(created.id);
      } catch (err) {
        const duplicateAddResult = await addExistingDeviceToCurrentMapOnDuplicate(err, payload);
        if (duplicateAddResult.status === 'added') {
          onDeviceAdded(duplicateAddResult.deviceId);
          return;
        }
        if (duplicateAddResult.status === 'error') return;
        if (err instanceof ServerError) {
          setError(
            err.correlationId
              ? `Something went wrong (ref: ${err.correlationId})`
              : 'Something went wrong',
          );
        } else if (err instanceof ValidationError) {
          setError(err.message);
        } else {
          setError(err instanceof Error ? err.message : 'Failed to add virtual node.');
        }
      } finally {
        setLoading(false);
      }
      return;
    }

    // Validate physical mode fields
    const errors: Record<string, string> = {};
    const hostnameErr = validateIPOrHostname(form.hostname.trim());
    if (hostnameErr) errors['hostname'] = hostnameErr;
    const probePortsErr = validateProbePorts(form.probePorts);
    if (probePortsErr) errors['probePorts'] = probePortsErr;
    Object.assign(errors, validateAdditionalAddressRows(form.hostname.trim()));
    const displayNameErr = validateMaxLength(form.displayName, MAX_STRING_LENGTH, 'Display name');
    if (displayNameErr) errors['displayName'] = displayNameErr;
    if (usesPrometheus) {
      const labelValueErr = validateMaxLength(
        form.prometheus.labelValue,
        MAX_STRING_LENGTH,
        'Label value',
      );
      if (labelValueErr) errors['prometheusLabelValue'] = labelValueErr;
    }
    if (!isV3) {
      const communityErr = validateMaxLength(
        form.snmp.community,
        MAX_STRING_LENGTH,
        'Community string',
      );
      if (communityErr) errors['community'] = communityErr;
    } else {
      const usernameErr = validateMaxLength(form.snmp.username, MAX_STRING_LENGTH, 'Username');
      if (usernameErr) errors['username'] = usernameErr;
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors);
      return;
    }

    setError(null);
    const payload = buildCreatePayloadForCurrentScope();
    const mapAddressConflict = currentMapAddressConflictMessage(payload);
    if (mapAddressConflict) {
      setError(mapAddressConflict);
      return;
    }
    setLoading(true);
    try {
      const created = await createDevice(payload);
      await assignSelectedCredentials(created.id);
      await addDeviceToCurrentMap(created.id);
      onDeviceAdded(created.id);
    } catch (err) {
      const duplicateAddResult = await addExistingDeviceToCurrentMapOnDuplicate(err, payload);
      if (duplicateAddResult.status === 'added') {
        onDeviceAdded(duplicateAddResult.deviceId);
        return;
      }
      if (duplicateAddResult.status === 'error') return;
      if (err instanceof ServerError) {
        setError(
          err.correlationId
            ? `Something went wrong (ref: ${err.correlationId})`
            : 'Something went wrong',
        );
      } else if (err instanceof ValidationError) {
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : 'Failed to add device.');
      }
    } finally {
      setLoading(false);
    }
  }

  const inputClass =
    'w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none';
  const selectClass =
    'w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none';
  const labelClass = 'text-xs font-medium uppercase tracking-widest text-on-bg-secondary';

  return (
    <form
      onSubmit={(e) => {
        void handleSubmit(e);
      }}
      className="space-y-4 p-4 transition-colors duration-200"
    >
      {/* Device mode toggle */}
      <div className="flex rounded-lg bg-surface p-0.5">
        <button
          type="button"
          className={`flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
            !isVirtual ? 'bg-primary text-white' : 'text-on-bg-secondary hover:text-on-bg'
          }`}
          onClick={() => handleModeSwitch('physical')}
        >
          Physical Device
        </button>
        <button
          type="button"
          className={`flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
            isVirtual ? 'bg-primary text-white' : 'text-on-bg-secondary hover:text-on-bg'
          }`}
          onClick={() => handleModeSwitch('virtual')}
        >
          Virtual Node
        </button>
      </div>

      {isVirtual ? (
        <div className="space-y-4">
          {/* Display Name (required) */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
              Display Name <span className="text-status-down">*</span>
            </label>
            <input
              type="text"
              value={form.displayName}
              onChange={(e) => {
                updateForm({ displayName: e.target.value });
                setFieldError('displayName', null);
              }}
              onBlur={handleBlur(
                'displayName',
                () =>
                  validateRequired(form.displayName, 'Display Name') ??
                  validateMaxLength(form.displayName, MAX_STRING_LENGTH, 'Display name'),
              )}
              placeholder="e.g. ISP Gateway"
              className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder:text-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors['displayName'] ? ' border-status-down' : ' border-outline-subtle'}`}
              required
            />
            {fieldErrors['displayName'] && (
              <p className="mt-1 text-xs text-status-down">{fieldErrors['displayName']}</p>
            )}
          </div>

          {/* Subtype 2x2 grid */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
              Type
            </label>
            <div className="grid grid-cols-2 gap-2">
              {(
                [
                  { value: 'internet', label: 'Internet', icon: 'language' },
                  { value: 'cloud', label: 'Cloud', icon: 'cloud' },
                  { value: 'server', label: 'Server', icon: 'dns' },
                  { value: 'generic', label: 'Generic', icon: 'hub' },
                ] as const
              ).map((st) => (
                <button
                  key={st.value}
                  type="button"
                  onClick={() => updateVirtual({ subtype: st.value })}
                  className={`flex flex-col items-center gap-1.5 rounded-lg border-2 px-3 py-3 transition-colors ${
                    form.virtual.subtype === st.value
                      ? 'border-primary bg-primary/10'
                      : 'border-outline-subtle bg-elevated hover:border-outline'
                  }`}
                >
                  <MaterialIcon
                    name={st.icon}
                    size={24}
                    className={
                      form.virtual.subtype === st.value ? 'text-primary' : 'text-on-bg-secondary'
                    }
                  />
                  <span
                    className={`text-xs font-medium ${
                      form.virtual.subtype === st.value ? 'text-primary' : 'text-on-bg-secondary'
                    }`}
                  >
                    {st.label}
                  </span>
                </button>
              ))}
            </div>
          </div>

          {mapContext && (
            <div className="space-y-1.5">
              <label
                htmlFor="virtual-node-color"
                className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary"
              >
                Virtual node color
              </label>
              <div className="flex items-center gap-2">
                <input
                  id="virtual-node-color"
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

          {/* IP Address (optional) */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
              IP Address{' '}
              <span className="normal-case tracking-normal text-on-bg-muted">(optional)</span>
            </label>
            <input
              type="text"
              value={form.ip}
              onChange={(e) => {
                updateForm({ ip: e.target.value });
                setFieldError('virtualIp', null);
              }}
              onBlur={handleBlur('virtualIp', () =>
                form.ip.trim() ? validateIPOrHostname(form.ip.trim()) : null,
              )}
              placeholder="e.g. 203.0.113.1"
              className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder:text-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors['virtualIp'] ? ' border-status-down' : ' border-outline-subtle'}`}
            />
            {fieldErrors['virtualIp'] && (
              <p className="mt-1 text-xs text-status-down">{fieldErrors['virtualIp']}</p>
            )}
          </div>
        </div>
      ) : (
        <>
          {/* Prometheus unavailable warning */}
          {prometheusCheckDone && !prometheusAvailable && (
            <div className="rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-xs text-yellow-400">
              Prometheus is not configured or unreachable. Only SNMP Direct is available.
            </div>
          )}

          <div className="space-y-2">
            <label className={labelClass}>
              IP Address <span className="text-status-down">*</span>
            </label>
            <input
              type="text"
              value={form.hostname}
              onChange={(e) => {
                updateForm({ hostname: e.target.value, ip: e.target.value });
                setFieldError('hostname', null);
              }}
              onBlur={handleBlur('hostname', () => validateIPOrHostname(form.hostname.trim()))}
              placeholder="192.168.1.1"
              required
              className={`${inputClass}${fieldErrors['hostname'] ? ' border-status-down' : ''}`}
            />
            {fieldErrors['hostname'] && (
              <p className="mt-1 text-xs text-status-down">{fieldErrors['hostname']}</p>
            )}
          </div>

          <div className="space-y-2">
            <label htmlFor="probe-ports" className={labelClass}>
              Probe ports
            </label>
            <input
              id="probe-ports"
              aria-label="Probe ports"
              type="text"
              value={form.probePorts}
              onChange={(e) => {
                updateForm({ probePorts: e.target.value });
                setFieldError('probePorts', null);
              }}
              onBlur={handleBlur('probePorts', () => validateProbePorts(form.probePorts))}
              placeholder="22,8291"
              className={`${inputClass}${fieldErrors['probePorts'] ? ' border-status-down' : ''}`}
            />
            {fieldErrors['probePorts'] && (
              <p className="mt-1 text-xs text-status-down">{fieldErrors['probePorts']}</p>
            )}
          </div>

          <div className="space-y-3 rounded-lg bg-surface-high p-3">
            <div className="flex items-center justify-between gap-3">
              <p className={labelClass}>Additional addresses</p>
              <button
                type="button"
                onClick={addAdditionalAddress}
                className="rounded-lg bg-elevated px-3 py-1.5 text-xs font-medium text-on-bg-secondary transition-colors hover:text-on-bg"
              >
                Add address
              </button>
            </div>
            {form.additionalAddresses.map((address, index) => (
              <div
                key={deviceAddressFormRowKey(address)}
                data-testid={`additional-address-row-${index + 1}`}
                className="space-y-3 rounded-lg bg-elevated p-3"
              >
                <div className="space-y-1">
                  <span className="text-xs text-on-bg-secondary">Address</span>
                  <input
                    id={`additional-address-${index}`}
                    aria-label={`Additional address ${index + 1}`}
                    type="text"
                    value={address.address}
                    onChange={(e) => {
                      updateAdditionalAddress(index, { address: e.target.value });
                      setFieldError(`additionalAddress${index}`, null);
                    }}
                    onBlur={handleBlur(`additionalAddress${index}`, () => {
                      const trimmed = address.address.trim();
                      return trimmed ? validateIPOrHostname(trimmed) : null;
                    })}
                    placeholder="192.0.2.10 or oob-router"
                    className={`${inputClass}${fieldErrors[`additionalAddress${index}`] ? ' border-status-down' : ''}`}
                  />
                  {fieldErrors[`additionalAddress${index}`] && (
                    <p className="mt-1 text-xs text-status-down">
                      {fieldErrors[`additionalAddress${index}`]}
                    </p>
                  )}
                </div>
                <div className="space-y-2">
                  <div className="space-y-1">
                    <span className="text-xs text-on-bg-secondary">Role</span>
                    <select
                      id={`additional-address-role-${index}`}
                      aria-label={`Address role ${index + 1}`}
                      value={address.role}
                      onChange={(e) =>
                        updateAdditionalAddress(index, {
                          role: e.target.value as SecondaryDeviceAddressRole,
                        })
                      }
                      className={selectClass}
                    >
                      <option value="management">Management</option>
                      <option value="backup">Backup</option>
                      <option value="monitoring">Monitoring</option>
                      <option value="other">Other</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <span className="text-xs text-on-bg-secondary">Label</span>
                    <input
                      id={`additional-address-label-${index}`}
                      aria-label={`Address label ${index + 1}`}
                      type="text"
                      value={address.label}
                      onChange={(e) => updateAdditionalAddress(index, { label: e.target.value })}
                      placeholder="OOB"
                      className={inputClass}
                    />
                  </div>
                  <div className="space-y-1">
                    <span className="text-xs text-on-bg-secondary">Probe ports</span>
                    <input
                      id={`additional-address-probe-ports-${index}`}
                      aria-label={`Address probe ports ${index + 1}`}
                      type="text"
                      value={address.probePorts}
                      onChange={(e) => {
                        updateAdditionalAddress(index, { probePorts: e.target.value });
                        setFieldError(`additionalAddressProbePorts${index}`, null);
                      }}
                      onBlur={handleBlur(`additionalAddressProbePorts${index}`, () =>
                        validateProbePorts(address.probePorts),
                      )}
                      placeholder="2222"
                      className={`${inputClass}${fieldErrors[`additionalAddressProbePorts${index}`] ? ' border-status-down' : ''}`}
                    />
                    {fieldErrors[`additionalAddressProbePorts${index}`] && (
                      <p className="mt-1 text-xs text-status-down">
                        {fieldErrors[`additionalAddressProbePorts${index}`]}
                      </p>
                    )}
                  </div>
                  <div className="flex justify-end">
                    <button
                      type="button"
                      onClick={() => removeAdditionalAddress(index)}
                      aria-label={`Remove address ${index + 1}`}
                      className="rounded-lg bg-surface px-3 py-2 text-xs font-medium text-on-bg-secondary transition-colors hover:text-on-bg"
                    >
                      Remove
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>

          {/* Metrics & Collection Mode */}
          <div className="space-y-2">
            <label className={labelClass}>Metrics Source</label>
            <select
              value={form.metricsMode}
              onChange={(e) => handleMetricsModeChange(e.target.value as MetricsMode)}
              className={selectClass}
            >
              <option value="snmp">SNMP Direct</option>
              <option value="prometheus" disabled={!prometheusAvailable}>
                Prometheus{!prometheusAvailable ? ' (unavailable)' : ''}
              </option>
              <option value="prometheus_snmp_fallback" disabled={!prometheusAvailable}>
                Prometheus + SNMP Fallback
                {!prometheusAvailable ? ' (unavailable)' : ''}
              </option>
            </select>
            {form.metricsMode === 'prometheus' && (
              <p className="text-xs text-on-bg-secondary/70">
                Metrics from Prometheus only. No fallback if Prometheus is unreachable.
              </p>
            )}
            {form.metricsMode === 'prometheus_snmp_fallback' && (
              <p className="text-xs text-on-bg-secondary/70">
                Metrics from Prometheus. Falls back to SNMP if Prometheus is unavailable or has no
                data.
              </p>
            )}
          </div>

          <div className="space-y-2">
            <label htmlFor="topology-discovery-mode" className={labelClass}>
              Topology Discovery
            </label>
            <select
              id="topology-discovery-mode"
              value={form.topologyDiscoveryMode}
              onChange={(e) =>
                updateForm({
                  topologyDiscoveryMode: e.target.value as TopologyDiscoveryMode,
                })
              }
              className={selectClass}
            >
              {TOPOLOGY_DISCOVERY_MODE_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            <p className="text-xs text-on-bg-secondary/70">
              Selected mode:{' '}
              <span className="font-medium">
                {formatTopologyDiscoveryMode(form.topologyDiscoveryMode)}
              </span>
              .
              {form.metricsMode === 'prometheus'
                ? ' Prometheus-only devices skip SNMP topology discovery until SNMP or fallback mode is enabled.'
                : ' Bootstrap once runs an initial discovery window, may queue one follow-up to fill missing ports, then auto-disables.'}
            </p>
          </div>

          {/* Prometheus label config */}
          {usesPrometheus && (
            <div className="space-y-2 bg-surface-high rounded-lg p-3">
              <p className={labelClass}>Prometheus Target</p>
              <div className="space-y-1">
                <label className="text-xs text-on-bg-secondary">Label</label>
                <select
                  value={form.prometheus.labelName}
                  onChange={(e) => updatePrometheus({ labelName: e.target.value })}
                  className={selectClass}
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
                    validateMaxLength(form.prometheus.labelValue, MAX_STRING_LENGTH, 'Label value'),
                  )}
                  placeholder={
                    form.prometheus.labelName === 'instance'
                      ? form.hostname || '192.168.1.1'
                      : 'e.g. my-router'
                  }
                  className={`${inputClass}${fieldErrors['prometheusLabelValue'] ? ' border-status-down' : ''}`}
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
            <div className="space-y-3 bg-surface-high rounded-lg p-3">
              <p className={labelClass}>SNMP Credentials</p>

              {profiles.length > 0 && (
                <div className="space-y-1">
                  <label className="text-xs text-on-bg-secondary">Load from Profile</label>
                  <select
                    defaultValue=""
                    onChange={(e) => {
                      void applyProfile(e.target.value);
                      e.target.value = '';
                    }}
                    className={selectClass}
                  >
                    <option value="" disabled>
                      Select a credential profile...
                    </option>
                    {profiles.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name} (SNMP {p.snmp.version})
                      </option>
                    ))}
                  </select>
                </div>
              )}

              <div className="space-y-1">
                <label className="text-xs text-on-bg-secondary">Version</label>
                <select
                  value={form.snmp.version}
                  onChange={(e) =>
                    updateSnmp({
                      version: e.target.value as DeviceFormModel['snmp']['version'],
                    })
                  }
                  className={selectClass}
                >
                  <option value="2c">v2c</option>
                  <option value="3">v3</option>
                </select>
              </div>

              {!isV3 && (
                <div className="space-y-1">
                  <label className="text-xs text-on-bg-secondary">Community</label>
                  <input
                    type="text"
                    value={form.snmp.community}
                    onChange={(e) => {
                      updateSnmp({ community: e.target.value });
                      setFieldError('community', null);
                    }}
                    onBlur={handleBlur('community', () =>
                      validateMaxLength(form.snmp.community, MAX_STRING_LENGTH, 'Community string'),
                    )}
                    placeholder="public"
                    className={`${inputClass}${fieldErrors['community'] ? ' border-status-down' : ''}`}
                  />
                  {fieldErrors['community'] && (
                    <p className="mt-1 text-xs text-status-down">{fieldErrors['community']}</p>
                  )}
                </div>
              )}

              {isV3 && (
                <div className="space-y-2">
                  <div className="space-y-1">
                    <label className="text-xs text-on-bg-secondary">Username</label>
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
                      placeholder="snmpv3user"
                      className={`${inputClass}${fieldErrors['username'] ? ' border-status-down' : ''}`}
                    />
                    {fieldErrors['username'] && (
                      <p className="mt-1 text-xs text-status-down">{fieldErrors['username']}</p>
                    )}
                  </div>

                  <div className="space-y-1">
                    <label className="text-xs text-on-bg-secondary">Security Level</label>
                    <select
                      value={form.snmp.securityLevel}
                      onChange={(e) => updateSnmp({ securityLevel: e.target.value })}
                      className={selectClass}
                    >
                      <option value="noAuthNoPriv">No Auth, No Privacy</option>
                      <option value="authNoPriv">Auth, No Privacy</option>
                      <option value="authPriv">Auth + Privacy</option>
                    </select>
                  </div>

                  {needsAuth && (
                    <>
                      <div className="space-y-1">
                        <label className="text-xs text-on-bg-secondary">Auth Protocol</label>
                        <select
                          value={form.snmp.authProtocol}
                          onChange={(e) => updateSnmp({ authProtocol: e.target.value })}
                          className={selectClass}
                        >
                          <option value="SHA">SHA</option>
                          <option value="MD5">MD5</option>
                          <option value="SHA-224">SHA-224</option>
                          <option value="SHA-256">SHA-256</option>
                          <option value="SHA-384">SHA-384</option>
                          <option value="SHA-512">SHA-512</option>
                        </select>
                      </div>
                      <div className="space-y-1">
                        <label className="text-xs text-on-bg-secondary">Auth Key</label>
                        <input
                          type="password"
                          value={form.snmp.authPassword}
                          onChange={(e) => updateSnmp({ authPassword: e.target.value })}
                          placeholder="Authentication passphrase"
                          autoComplete="new-password"
                          className={inputClass}
                        />
                      </div>
                    </>
                  )}

                  {needsPriv && (
                    <>
                      <div className="space-y-1">
                        <label className="text-xs text-on-bg-secondary">Encryption Protocol</label>
                        <select
                          value={form.snmp.privProtocol}
                          onChange={(e) => updateSnmp({ privProtocol: e.target.value })}
                          className={selectClass}
                        >
                          <option value="AES">AES</option>
                          <option value="DES">DES</option>
                        </select>
                      </div>
                      <div className="space-y-1">
                        <label className="text-xs text-on-bg-secondary">Encryption Key</label>
                        <input
                          type="password"
                          value={form.snmp.privPassword}
                          onChange={(e) => updateSnmp({ privPassword: e.target.value })}
                          placeholder="Privacy passphrase"
                          autoComplete="new-password"
                          className={inputClass}
                        />
                      </div>
                    </>
                  )}
                </div>
              )}
            </div>
          )}

          <div className="space-y-2">
            <label className={labelClass}>
              Custom Name <span className="text-on-bg-secondary/50">(optional)</span>
            </label>
            <input
              type="text"
              value={form.displayName}
              onChange={(e) => {
                updateForm({ displayName: e.target.value });
                setFieldError('displayName', null);
              }}
              onBlur={handleBlur('displayName', () =>
                validateMaxLength(form.displayName, MAX_STRING_LENGTH, 'Display name'),
              )}
              placeholder="Auto-discovered from SNMP / Prometheus"
              className={`${inputClass}${fieldErrors['displayName'] ? ' border-status-down' : ''}`}
            />
            {fieldErrors['displayName'] && (
              <p className="mt-1 text-xs text-status-down">{fieldErrors['displayName']}</p>
            )}
          </div>

          <div className="space-y-2">
            <label className={labelClass}>
              Vendor <span className="text-on-bg-secondary/50">(optional)</span>
            </label>
            <select
              value={form.vendor}
              onChange={(e) => updateForm({ vendor: e.target.value })}
              className={selectClass}
            >
              <option value="">— Select vendor —</option>
              <option value="mikrotik">MikroTik</option>
            </select>
            <p className="text-xs text-on-bg-secondary/70">
              Vendor tag determines backup commands and metric queries.
            </p>
          </div>

          <div className="space-y-3 bg-surface-high rounded-lg p-3">
            <p className={labelClass}>Credentials</p>
            {credentialProfiles.length === 0 ? (
              <p className="text-xs text-on-bg-secondary">
                No credential profiles available. Create one in Settings to assign it here.
              </p>
            ) : (
              <div className="space-y-2">
                {credentialProfiles.map((profile) => {
                  const selected = selectedCredentialProfileIds.includes(profile.id);
                  return (
                    <div key={profile.id} className="rounded-lg bg-elevated px-3 py-2">
                      <label className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={selected}
                          onChange={() => toggleCredentialProfile(profile.id)}
                          aria-label={`Assign ${profile.name}`}
                          className="h-4 w-4 accent-primary"
                        />
                        <span className="min-w-0 flex-1 truncate text-sm font-medium text-on-bg">
                          {profile.name}
                        </span>
                        <span className="shrink-0 rounded-full bg-surface px-2 py-0.5 text-xs text-on-bg-secondary">
                          {profile.role}
                        </span>
                      </label>
                      {selected && (
                        <label className="mt-2 flex items-center gap-2 pl-6 text-xs text-on-bg-secondary">
                          <input
                            type="radio"
                            name="add-device-winbox-profile"
                            checked={winboxCredentialProfileId === profile.id}
                            onChange={() => setWinboxCredentialProfileId(profile.id)}
                            aria-label={`Use ${profile.name} for WinBox`}
                            className="h-3.5 w-3.5 accent-primary"
                          />
                          WinBox
                        </label>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </>
      )}

      {/* Areas multi-select -- shared between both modes */}
      {areas.length > 0 && (
        <div className="space-y-2">
          <label className={labelClass}>
            Area <span className="text-on-bg-secondary/50">(optional)</span>
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
                        updateForm({
                          areaIds: form.areaIds.filter((areaId) => areaId !== id),
                        })
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
          {areas.filter((a) => !form.areaIds.includes(a.id)).length > 0 && (
            <select
              value=""
              onChange={(e) => {
                if (e.target.value) {
                  updateForm({ areaIds: [...form.areaIds, e.target.value] });
                }
              }}
              className={selectClass}
            >
              <option value="">
                {form.areaIds.length === 0 ? 'Unassigned - select area...' : 'Add another area...'}
              </option>
              {areas
                .filter((a) => !form.areaIds.includes(a.id))
                .map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
            </select>
          )}
        </div>
      )}

      {error && (
        <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={loading}
        className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {loading ? 'Adding...' : isVirtual ? 'Add Virtual Node' : 'Add Device'}
      </button>
    </form>
  );
}
