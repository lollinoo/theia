import type { Device, SNMPProfile } from '../../types/api';

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
  };
}

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
    },
  };
}

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
    },
  };
}

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
      virtual: { subtype: form.virtual.subtype },
    };
  }

  return {
    ...createAddDeviceFormModel(),
    mode: 'physical',
  };
}

export function applySNMPProfile(form: DeviceFormModel, profile: SNMPProfile): DeviceFormModel {
  return {
    ...form,
    snmp: {
      version: profile.snmp.version === '3' ? '3' : '2c',
      community: profile.snmp.community ?? 'public',
      username: profile.snmp.username ?? '',
      securityLevel: profile.snmp.security_level ?? 'authPriv',
      authProtocol: profile.snmp.auth_protocol ?? 'SHA',
      authPassword: profile.snmp.auth_password ?? '',
      privProtocol: profile.snmp.priv_protocol ?? 'AES',
      privPassword: profile.snmp.priv_password ?? '',
    },
  };
}
