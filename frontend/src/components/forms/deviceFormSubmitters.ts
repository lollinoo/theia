/**
 * Renders device form submitters UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import type { CreateDevicePayload, DeviceAddressPayload, SNMPPayload } from '../../api/client';
import type { Device } from '../../types/api';
import type { DeviceFormModel } from './deviceFormModels';

export const PROBE_PORTS_RANGE_ERROR = 'Ports must be between 1 and 65535';

/** Parses comma-separated TCP ports, deduping valid values while preserving user order. */
export function parseProbePorts(value: string | undefined): {
  ports: number[] | null;
  error: string | null;
} {
  const trimmed = (value ?? '').trim();
  if (trimmed === '') {
    return { ports: null, error: null };
  }

  const ports: number[] = [];
  const seen = new Set<number>();
  for (const part of trimmed.split(',')) {
    const segment = part.trim();
    if (!/^\d+$/.test(segment)) {
      return { ports: null, error: PROBE_PORTS_RANGE_ERROR };
    }
    const port = Number(segment);
    if (port < 1 || port > 65535) {
      return { ports: null, error: PROBE_PORTS_RANGE_ERROR };
    }
    if (!seen.has(port)) {
      seen.add(port);
      ports.push(port);
    }
  }

  return { ports, error: null };
}

/** Returns the field-level validation message for a comma-separated probe port list. */
export function validateProbePorts(value: string | undefined): string | null {
  return parseProbePorts(value).error;
}

function validProbePorts(value: string | undefined): number[] | null {
  const parsed = parseProbePorts(value);
  if (parsed.error) {
    throw new Error(parsed.error);
  }
  return parsed.ports;
}

function buildSnmpPayload(form: DeviceFormModel): SNMPPayload {
  if (form.snmp.version === '3') {
    const needsAuth =
      form.snmp.securityLevel === 'authNoPriv' || form.snmp.securityLevel === 'authPriv';
    const needsPriv = form.snmp.securityLevel === 'authPriv';

    return {
      version: '3',
      username: form.snmp.username.trim(),
      security_level: form.snmp.securityLevel,
      ...(needsAuth
        ? {
            auth_protocol: form.snmp.authProtocol,
            auth_password: form.snmp.authPassword,
          }
        : {}),
      ...(needsPriv
        ? {
            priv_protocol: form.snmp.privProtocol,
            priv_password: form.snmp.privPassword,
          }
        : {}),
    };
  }

  return {
    version: '2c',
    community: form.snmp.community.trim() || 'public',
  };
}

function buildAddressPayloads(
  form: DeviceFormModel,
  includeLegacyPrimaryOnly = false,
  includeBlankProbePorts = false,
  existingAddresses: Device['addresses'] = [],
): DeviceAddressPayload[] | undefined {
  const additionalAddresses = form.additionalAddresses
    .map((row, index) => {
      const probePorts = validProbePorts(row.probePorts);
      const clearsExistingProbePorts = existingAddresses.some(
        (address) =>
          address.address === row.address.trim() &&
          address.role === row.role &&
          (address.probe_ports?.length ?? 0) > 0,
      );
      return {
        address: row.address.trim(),
        role: row.role,
        label: row.label.trim(),
        is_primary: false,
        priority: (index + 1) * 10,
        ...(probePorts
          ? { probe_ports: probePorts }
          : includeBlankProbePorts && clearsExistingProbePorts
            ? { probe_ports: null }
            : {}),
      };
    })
    .filter((row) => row.address !== '');

  if (additionalAddresses.length === 0 && !includeLegacyPrimaryOnly) {
    return undefined;
  }

  const primaryAddress = form.ip.trim();
  if (primaryAddress === '') {
    return undefined;
  }

  return [
    {
      address: primaryAddress,
      role: 'primary',
      is_primary: true,
      priority: 0,
    },
    ...additionalAddresses.map((row) => ({
      address: row.address,
      role: row.role,
      ...(row.label ? { label: row.label } : {}),
      ...('probe_ports' in row ? { probe_ports: row.probe_ports } : {}),
      is_primary: false,
      priority: row.priority,
    })),
  ];
}

/** Builds create device payload for the UI component boundary. */
export function buildCreateDevicePayload(form: DeviceFormModel): CreateDevicePayload {
  if (form.mode === 'virtual') {
    return {
      hostname: form.displayName.trim(),
      ip: form.ip.trim() || undefined,
      device_type: 'virtual',
      tags: {
        display_name: form.displayName.trim(),
        virtual_subtype: form.virtual.subtype,
      },
      area_ids: form.areaIds.length > 0 ? form.areaIds : undefined,
    };
  }

  const usesPrometheus =
    form.metricsMode === 'prometheus' || form.metricsMode === 'prometheus_snmp_fallback';
  const labelValue = form.prometheus.labelValue.trim() || form.hostname.trim();
  const addresses = buildAddressPayloads(form);
  const probePorts = validProbePorts(form.probePorts);

  return {
    hostname: form.hostname.trim(),
    ip: form.ip.trim(),
    ...(addresses ? { addresses } : {}),
    ...(probePorts ? { probe_ports: probePorts } : {}),
    snmp: buildSnmpPayload(form),
    tags: form.displayName.trim() ? { display_name: form.displayName.trim() } : undefined,
    metrics_source: form.metricsMode,
    ...(form.vendor ? { vendor: form.vendor } : {}),
    ...(usesPrometheus
      ? {
          prometheus_label_name: form.prometheus.labelName,
          prometheus_label_value: labelValue,
        }
      : {}),
    ...(form.topologyDiscoveryMode ? { topology_discovery_mode: form.topologyDiscoveryMode } : {}),
    ...(form.areaIds.length > 0 ? { area_ids: form.areaIds } : {}),
  };
}

/** Builds update device payload for the UI component boundary. */
export function buildUpdateDevicePayload(device: Device, form: DeviceFormModel) {
  const usesPrometheus =
    form.metricsMode === 'prometheus' || form.metricsMode === 'prometheus_snmp_fallback';
  const labelValue = form.prometheus.labelValue.trim() || form.ip.trim();
  const displayName = form.displayName.trim();
  const vendor = form.vendor.trim();
  const hasSnmpChanges =
    form.snmp.version === '3'
      ? form.snmp.username.trim() !== ''
      : form.snmp.community.trim() !== '';
  const shouldSendPhysicalDisplayName =
    form.mode === 'physical' && (device.tags?.display_name !== undefined || displayName !== '');
  const shouldSendVendor = device.vendor !== '' || vendor !== '';
  const hasExistingAdditionalAddresses = (device.addresses ?? []).some(
    (address) => !address.is_primary && address.role !== 'primary',
  );
  const addresses =
    form.mode === 'physical'
      ? buildAddressPayloads(form, hasExistingAdditionalAddresses, true, device.addresses)
      : undefined;
  const probePorts = validProbePorts(form.probePorts);
  const shouldSendProbePorts =
    form.mode === 'physical' &&
    (form.probePorts.trim() !== '' || (device.probe_ports?.length ?? 0) > 0);

  return {
    hostname: device.hostname,
    ip: form.ip.trim(),
    ...(addresses ? { addresses } : {}),
    ...(shouldSendProbePorts ? { probe_ports: probePorts } : {}),
    notes: form.notes.trim() === '' ? null : form.notes.trim(),
    ...(hasSnmpChanges ? { snmp: buildSnmpPayload(form) } : {}),
    tags:
      form.mode === 'virtual'
        ? {
            ...device.tags,
            display_name: displayName,
            virtual_subtype: form.virtual.subtype,
          }
        : {
            ...device.tags,
            ...(shouldSendPhysicalDisplayName ? { display_name: displayName } : {}),
          },
    area_ids: form.areaIds,
    metrics_source: form.metricsMode,
    ...(shouldSendVendor ? { vendor } : {}),
    ...(usesPrometheus
      ? {
          prometheus_label_name: form.prometheus.labelName,
          prometheus_label_value: labelValue,
        }
      : {}),
    ...(form.mode === 'physical' ? { topology_discovery_mode: form.topologyDiscoveryMode } : {}),
  };
}
