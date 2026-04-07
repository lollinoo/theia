export type DeviceType = 'router' | 'switch' | 'ap' | 'virtual' | 'unknown';

// SNMPProfile represents a reusable set of SNMP credentials.
export interface SNMPProfile {
  id: string;
  name: string;
  description: string;
  snmp: {
    version: string;
    community?: string;
    username?: string;
    auth_protocol?: string;
    auth_password?: string;
    priv_protocol?: string;
    priv_password?: string;
    security_level?: string;
  };
  created_at: string;
  updated_at: string;
}
export type DeviceStatus = 'up' | 'down' | 'probing' | 'unknown';

export interface DeviceInterface {
  id: string;
  if_index: number;
  if_name: string;
  if_descr: string;
  speed: number;
  admin_status: string;
  oper_status: string;
}

export type MetricsSource = 'prometheus' | 'snmp' | 'prometheus_snmp_fallback' | 'none';

export interface Device {
  id: string;
  hostname: string;
  ip: string;
  device_type: DeviceType;
  status: DeviceStatus;
  sys_name: string;
  sys_descr: string;
  hardware_model: string;
  vendor: string;
  managed: boolean;
  tags?: Record<string, string>;
  interfaces: DeviceInterface[];
  ssh_profile_id?: string;
  area_ids: string[];
  backup_supported: boolean;
  metrics_source: MetricsSource;
  prometheus_label_name: string;
  prometheus_label_value: string;
}

export interface Link {
  id: string;
  source_device_id: string;
  source_if_name: string;
  target_device_id: string;
  target_if_name: string;
  discovery_protocol: string;
}

export interface DevicePosition {
  device_id: string;
  x: number;
  y: number;
  pinned: boolean;
  updated_at?: string;
}

export interface InterfaceInfo {
  if_name: string;
  if_descr: string;
  speed: number;
  oper_status: string;
  admin_status: string;
  in_use: boolean;
  in_use_by?: string;
}

type APIRecord = Record<string, unknown>;

function isRecord(value: unknown): value is APIRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function readString(record: APIRecord, key: string, fallback = ''): string {
  const value = record[key];
  return typeof value === 'string' ? value : fallback;
}

function readNumber(record: APIRecord, key: string, fallback = 0): number {
  const value = record[key];
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function readBoolean(record: APIRecord, key: string, fallback = false): boolean {
  const value = record[key];
  return typeof value === 'boolean' ? value : fallback;
}

function parseDeviceType(value: unknown): DeviceType {
  switch (value) {
    case 'router':
    case 'switch':
    case 'ap':
    case 'virtual':
      return value;
    default:
      return 'unknown';
  }
}

function parseDeviceStatus(value: unknown): DeviceStatus {
  switch (value) {
    case 'up':
    case 'down':
    case 'probing':
      return value;
    default:
      return 'unknown';
  }
}

function parseDeviceInterface(value: unknown): DeviceInterface {
  if (!isRecord(value)) {
    throw new Error('invalid interface payload');
  }

  return {
    id: readString(value, 'id'),
    if_index: readNumber(value, 'if_index'),
    if_name: readString(value, 'if_name'),
    if_descr: readString(value, 'if_descr'),
    speed: readNumber(value, 'speed'),
    admin_status: readString(value, 'admin_status'),
    oper_status: readString(value, 'oper_status'),
  };
}

export function parseDevicesResponse(payload: unknown): Device[] {
  if (!isRecord(payload)) {
    throw new Error('invalid devices response');
  }

  const data = Array.isArray(payload.data) ? payload.data : [];

  return data.map((resource) => {
    if (!isRecord(resource)) {
      throw new Error('invalid device resource');
    }

    const attributes = isRecord(resource.attributes) ? resource.attributes : {};
    const tags = isRecord(attributes.tags) ? attributes.tags as Record<string, string> : {};
    const relationships = isRecord(resource.relationships) ? resource.relationships : {};
    const interfacesRelationship = isRecord(relationships.interfaces)
      ? relationships.interfaces
      : {};
    const interfacesData = Array.isArray(interfacesRelationship.data)
      ? interfacesRelationship.data
      : [];

    const rawMetricsSource = readString(attributes, 'metrics_source', 'prometheus');
    const metricsSource: MetricsSource =
      rawMetricsSource === 'snmp' ? 'snmp'
      : rawMetricsSource === 'prometheus_snmp_fallback' ? 'prometheus_snmp_fallback'
      : rawMetricsSource === 'none' ? 'none'
      : 'prometheus';

    return {
      id: readString(resource, 'id'),
      hostname: readString(attributes, 'hostname'),
      ip: readString(attributes, 'ip'),
      device_type: parseDeviceType(attributes.device_type),
      status: parseDeviceStatus(attributes.status),
      sys_name: readString(attributes, 'sys_name'),
      sys_descr: readString(attributes, 'sys_descr'),
      hardware_model: readString(attributes, 'hardware_model'),
      vendor: readString(attributes, 'vendor', 'default'),
      managed: readBoolean(attributes, 'managed', true),
      tags,
      interfaces: interfacesData.map(parseDeviceInterface),
      ssh_profile_id: typeof attributes.ssh_profile_id === 'string' ? attributes.ssh_profile_id : undefined,
      area_ids: Array.isArray(attributes.area_ids) ? (attributes.area_ids as string[]) : [],
      backup_supported: readBoolean(attributes, 'backup_supported', false),
      metrics_source: metricsSource,
      prometheus_label_name: readString(attributes, 'prometheus_label_name', 'instance'),
      prometheus_label_value: readString(attributes, 'prometheus_label_value'),
    };
  });
}

export function parseLinksResponse(payload: unknown): Link[] {
  if (!isRecord(payload)) {
    throw new Error('invalid links response');
  }

  const data = Array.isArray(payload.data) ? payload.data : [];

  return data.map((resource) => {
    if (!isRecord(resource)) {
      throw new Error('invalid link resource');
    }

    return {
      id: readString(resource, 'id'),
      source_device_id: readString(resource, 'source_device_id'),
      source_if_name: readString(resource, 'source_if_name'),
      target_device_id: readString(resource, 'target_device_id'),
      target_if_name: readString(resource, 'target_if_name'),
      discovery_protocol: readString(resource, 'discovery_protocol'),
    };
  });
}

export function parseInterfacesResponse(payload: unknown): InterfaceInfo[] {
  if (!isRecord(payload)) {
    throw new Error('invalid interfaces response');
  }

  const data = Array.isArray(payload.data) ? payload.data : [];

  return data.map((resource) => {
    if (!isRecord(resource)) {
      throw new Error('invalid interface resource');
    }

    return {
      if_name: readString(resource, 'if_name'),
      if_descr: readString(resource, 'if_descr'),
      speed: readNumber(resource, 'speed'),
      oper_status: readString(resource, 'oper_status'),
      admin_status: readString(resource, 'admin_status'),
      in_use: readBoolean(resource, 'in_use'),
      in_use_by: typeof resource['in_use_by'] === 'string' ? resource['in_use_by'] : undefined,
    };
  });
}

export function parseSNMPProfilesResponse(payload: unknown): SNMPProfile[] {
  if (!isRecord(payload)) {
    throw new Error('invalid snmp profiles response');
  }

  const data = Array.isArray(payload.data) ? payload.data : [];

  return data.map((item) => {
    if (!isRecord(item)) {
      throw new Error('invalid snmp profile item');
    }
    const snmp = isRecord(item.snmp) ? item.snmp : {};
    return {
      id: readString(item, 'id'),
      name: readString(item, 'name'),
      description: readString(item, 'description'),
      snmp: {
        version: readString(snmp, 'version', '2c'),
        community: typeof snmp.community === 'string' ? snmp.community : undefined,
        username: typeof snmp.username === 'string' ? snmp.username : undefined,
        auth_protocol: typeof snmp.auth_protocol === 'string' ? snmp.auth_protocol : undefined,
        auth_password: typeof snmp.auth_password === 'string' ? snmp.auth_password : undefined,
        priv_protocol: typeof snmp.priv_protocol === 'string' ? snmp.priv_protocol : undefined,
        priv_password: typeof snmp.priv_password === 'string' ? snmp.priv_password : undefined,
        security_level: typeof snmp.security_level === 'string' ? snmp.security_level : undefined,
      },
      created_at: readString(item, 'created_at'),
      updated_at: readString(item, 'updated_at'),
    };
  });
}

export function parseSNMPProfileResponse(payload: unknown): SNMPProfile {
  if (!isRecord(payload)) {
    throw new Error('invalid snmp profile response');
  }
  const data = isRecord(payload.data) ? payload.data : {};
  const snmp = isRecord(data.snmp) ? data.snmp : {};
  return {
    id: readString(data, 'id'),
    name: readString(data, 'name'),
    description: readString(data, 'description'),
    snmp: {
      version: readString(snmp, 'version', '2c'),
      community: typeof snmp.community === 'string' ? snmp.community : undefined,
      username: typeof snmp.username === 'string' ? snmp.username : undefined,
      auth_protocol: typeof snmp.auth_protocol === 'string' ? snmp.auth_protocol : undefined,
      auth_password: typeof snmp.auth_password === 'string' ? snmp.auth_password : undefined,
      priv_protocol: typeof snmp.priv_protocol === 'string' ? snmp.priv_protocol : undefined,
      priv_password: typeof snmp.priv_password === 'string' ? snmp.priv_password : undefined,
      security_level: typeof snmp.security_level === 'string' ? snmp.security_level : undefined,
    },
    created_at: readString(data, 'created_at'),
    updated_at: readString(data, 'updated_at'),
  };
}

// SSH profile for reusable SSH credentials shared across devices
export interface SSHProfile {
  id: string;
  name: string;
  description: string;
  username: string;
  port: number;
  auth_method: 'password' | 'key';
  created_at: string;
  updated_at: string;
}

// Area represents a grouping of devices.
export interface Area {
  id: string;
  name: string;
  description: string;
  color: string;
  device_count: number;
  created_at: string;
  updated_at: string;
}

// Backup system types
export type BackupStatus = 'pending' | 'running' | 'success' | 'failed';

export interface BackupFile {
  id: string;
  job_id: string;
  file_type: string;
  file_name: string;
  file_hash: string;
  size_bytes: number;
  created_at: string;
}

export interface BackupJob {
  id: string;
  device_id: string;
  status: BackupStatus;
  error_message: string;
  created_at: string;
  files: BackupFile[];
}

// Instance backup types
export type InstanceBackupStatus = 'running' | 'success' | 'failed';

export interface InstanceBackup {
  id: string;
  file_name: string;
  size_bytes: number;
  sha256: string;
  app_version: string;
  migration_version: number;
  status: InstanceBackupStatus;
  error_message: string;
  trigger: 'manual' | 'scheduled';
  created_at: string;
}

// Restore report from dry-run validation
export interface RestoreReport {
  valid: boolean;
  app_version: string;
  git_commit: string;
  migration_version: number;
  created_at: string;
  db_size_bytes: number;
  backup_file_count: number;
  total_size_bytes: number;
  needs_migration: boolean;
  current_migration_version: number;
  message: string;
}

// Vendor configuration
export interface VendorConfig {
  name: string;
  display_name: string;
  config: {
    vendor: { name: string; display_name: string };
    detection: {
      sys_object_id_prefixes: string[];
      sys_descr_patterns: string[];
    };
    metrics: {
      prometheus: {
        cpu: string;
        memory: string;
        temperature: string;
        uptime: string;
      };
    };
    snmp: {
      temperature_oid: string;
      temperature_scale: number;
      cpu_oid: string;
      memory_used_oid: string;
      memory_total_oid: string;
    };
    backup: {
      supported: boolean;
      methods: string[];
      default_method: string;
      ssh_commands: {
        export_running: string;
        export_compact: string;
        export_verbose: string;
        export_startup: string;
        binary_backup?: {
          save_command: string;
          remote_file_path: string;
          cleanup_command: string;
        };
      };
    };
  };
}

export function parseSSHProfilesResponse(payload: unknown): SSHProfile[] {
  if (!isRecord(payload)) {
    throw new Error('invalid ssh profiles response');
  }

  const data = Array.isArray(payload.data) ? payload.data : [];

  return data.map((item) => {
    if (!isRecord(item)) {
      throw new Error('invalid ssh profile item');
    }
    return {
      id: readString(item, 'id'),
      name: readString(item, 'name'),
      description: readString(item, 'description'),
      username: readString(item, 'username', 'admin'),
      port: readNumber(item, 'port', 22),
      auth_method: (item.auth_method === 'key' ? 'key' : 'password') as 'password' | 'key',
      created_at: readString(item, 'created_at'),
      updated_at: readString(item, 'updated_at'),
    };
  });
}

export function parseAreasResponse(payload: unknown): Area[] {
  if (!isRecord(payload)) return [];
  const data = payload.data;
  if (!Array.isArray(data)) return [];
  return data.map((item: unknown) => {
    if (!isRecord(item)) return null;
    return {
      id: readString(item, 'id', ''),
      name: readString(item, 'name', ''),
      description: readString(item, 'description', ''),
      color: readString(item, 'color', '#00E676'),
      device_count: typeof item.device_count === 'number' ? item.device_count : 0,
      created_at: readString(item, 'created_at', ''),
      updated_at: readString(item, 'updated_at', ''),
    };
  }).filter((a): a is Area => a !== null && a.id !== '');
}

export function parseAreaResponse(payload: unknown): Area {
  if (!isRecord(payload)) throw new Error('Invalid area response');
  const data = isRecord(payload.data) ? payload.data : payload;
  return {
    id: readString(data, 'id', ''),
    name: readString(data, 'name', ''),
    description: readString(data, 'description', ''),
    color: readString(data, 'color', '#00E676'),
    device_count: typeof data.device_count === 'number' ? data.device_count : 0,
    created_at: readString(data, 'created_at', ''),
    updated_at: readString(data, 'updated_at', ''),
  };
}

export function parsePositionsResponse(payload: unknown): DevicePosition[] {
  if (!isRecord(payload)) {
    throw new Error('invalid positions response');
  }

  const data = Array.isArray(payload.data) ? payload.data : [];

  return data.map((resource) => {
    if (!isRecord(resource)) {
      throw new Error('invalid position resource');
    }

    return {
      device_id: readString(resource, 'device_id'),
      x: readNumber(resource, 'x'),
      y: readNumber(resource, 'y'),
      pinned: readBoolean(resource, 'pinned'),
      updated_at: readString(resource, 'updated_at'),
    };
  });
}
