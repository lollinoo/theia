/**
 * Renders device form models UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import type { Device, SNMPProfile } from '../../types/api';

/** Defines default virtual node color constants and helper contracts for the UI component boundary. */
export const defaultVirtualNodeColor = '#00E676';

/** Normalizes virtual node color for the UI component boundary. */
export function normalizeVirtualNodeColor(color: string): string {
  const trimmed = color.trim();
  if (/^#[0-9a-fA-F]{6}$/.test(trimmed)) {
    return trimmed.toUpperCase();
  }
  return defaultVirtualNodeColor;
}

/** Describes the device form model contract used by the UI component boundary. */
export interface DeviceFormModel {
  mode: 'physical' | 'virtual';
  hostname: string;
  ip: string;
  displayName: string;
  notes: string;
  vendor: string;
  metricsMode: Device['metrics_source'];
  topologyDiscoveryMode: NonNullable<Device['topology_discovery_mode']>;
  areaIds: string[];
  snmp: {
    version: '2c' | '3';
    community: string;
    username: string;
    securityLevel: string;
    authProtocol: string;
    authPassword: string;
    privProtocol: string;
    privPassword: string;
  };
  prometheus: {
    labelName: string;
    labelValue: string;
  };
  virtual: {
    subtype: string;
    visualColor: string | null;
  };
}

/** Creates add device form model for the UI component boundary. */
export function createAddDeviceFormModel(): DeviceFormModel {
  return {
    mode: 'physical',
    hostname: '',
    ip: '',
    displayName: '',
    notes: '',
    vendor: '',
    metricsMode: 'snmp',
    topologyDiscoveryMode: 'inherit',
    areaIds: [],
    snmp: {
      version: '2c',
      community: 'public',
      username: '',
      securityLevel: 'authPriv',
      authProtocol: 'SHA',
      authPassword: '',
      privProtocol: 'AES',
      privPassword: '',
    },
    prometheus: {
      labelName: 'instance',
      labelValue: '',
    },
    virtual: {
      subtype: 'internet',
      visualColor: null,
    },
  };
}

/** Creates device config form model for the UI component boundary. */
export function createDeviceConfigFormModel(device: Device, isVirtual: boolean): DeviceFormModel {
  return {
    ...createAddDeviceFormModel(),
    mode: isVirtual ? 'virtual' : 'physical',
    hostname: device.hostname,
    ip: device.ip,
    displayName: device.tags?.display_name ?? '',
    notes: device.notes ?? '',
    vendor: device.vendor ?? '',
    metricsMode: device.metrics_source ?? 'snmp',
    topologyDiscoveryMode: device.topology_discovery_mode ?? 'inherit',
    areaIds: [...(device.area_ids ?? [])],
    snmp: {
      ...createAddDeviceFormModel().snmp,
      version: '2c',
      community: '',
      username: '',
      authPassword: '',
      privPassword: '',
    },
    prometheus: {
      labelName: device.prometheus_label_name || 'instance',
      labelValue: device.prometheus_label_value || '',
    },
    virtual: {
      subtype: device.tags?.virtual_subtype || 'internet',
      visualColor: isVirtual ? (device.map_visual_color ?? null) : null,
    },
  };
}

/** Resets device form mode for the UI component boundary. */
export function resetDeviceFormMode(
  form: DeviceFormModel,
  nextMode: DeviceFormModel['mode'],
): DeviceFormModel {
  if (nextMode === 'virtual') {
    return {
      ...createAddDeviceFormModel(),
      mode: 'virtual',
      displayName: form.displayName,
      areaIds: form.areaIds,
      virtual: { subtype: form.virtual.subtype, visualColor: form.virtual.visualColor },
    };
  }

  return {
    ...createAddDeviceFormModel(),
    mode: 'physical',
  };
}

/** Applies SNMP profile for the UI component boundary. */
export function applySNMPProfile(form: DeviceFormModel, profile: SNMPProfile): DeviceFormModel {
  const currentDefaults = createAddDeviceFormModel().snmp;
  return {
    ...form,
    snmp: {
      version: profile.snmp.version === '3' ? '3' : '2c',
      community: profile.snmp.community ?? form.snmp.community,
      username: profile.snmp.username ?? '',
      securityLevel: profile.snmp.security_level ?? currentDefaults.securityLevel,
      authProtocol: profile.snmp.auth_protocol ?? currentDefaults.authProtocol,
      authPassword: profile.snmp.auth_password ?? form.snmp.authPassword,
      privProtocol: profile.snmp.priv_protocol ?? currentDefaults.privProtocol,
      privPassword: profile.snmp.priv_password ?? form.snmp.privPassword,
    },
  };
}
