/**
 * Renders device form submitters UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import type { CreateDevicePayload, DeviceAddressPayload, SNMPPayload } from '../../api/client';
import type { Device } from '../../types/api';
import type { DeviceFormModel } from './deviceFormModels';

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

function buildAddressPayloads(form: DeviceFormModel): DeviceAddressPayload[] | undefined {
  const additionalAddresses = form.additionalAddresses
    .map((row, index) => ({
      address: row.address.trim(),
      role: row.role,
      label: row.label.trim(),
      is_primary: false,
      priority: (index + 1) * 10,
    }))
    .filter((row) => row.address !== '');

  if (additionalAddresses.length === 0) {
    return undefined;
  }

  return [
    {
      address: form.ip.trim(),
      role: 'primary',
      is_primary: true,
      priority: 0,
    },
    ...additionalAddresses.map((row) => ({
      address: row.address,
      role: row.role,
      ...(row.label ? { label: row.label } : {}),
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

  return {
    hostname: form.hostname.trim(),
    ip: form.ip.trim(),
    ...(addresses ? { addresses } : {}),
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
  const addresses = form.mode === 'physical' ? buildAddressPayloads(form) : undefined;

  return {
    hostname: device.hostname,
    ip: form.ip.trim(),
    ...(addresses ? { addresses } : {}),
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
