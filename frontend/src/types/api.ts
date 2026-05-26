import { type SnapshotPayload, parseSnapshotPayload } from './metrics';

export type DeviceType = 'router' | 'switch' | 'ap' | 'firewall' | 'virtual' | 'unknown';
export type DevicePollClass = 'core' | 'standard' | 'low';
export type TopologyDiscoveryMode = 'inherit' | 'off' | 'lldp' | 'lldp_cdp' | 'bootstrap_once';
export type TopologyBootstrapState = 'idle' | 'pending' | 'followup_scheduled' | 'completed';

// SNMPProfile represents a reusable set of SNMP credentials.
export interface SNMPProfile {
  id: string;
  name: string;
  description: string;
  snmp: {
    version: string;
    community?: string;
    community_set?: boolean;
    username?: string;
    auth_protocol?: string;
    auth_password?: string;
    auth_password_set?: boolean;
    priv_protocol?: string;
    priv_password?: string;
    priv_password_set?: boolean;
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
  notes?: string | null;
  device_type: DeviceType;
  poll_class: DevicePollClass;
  poll_interval_override: number | null;
  polling_enabled: boolean;
  status: DeviceStatus;
  sys_name: string;
  sys_descr: string;
  hardware_model: string;
  os_version?: string;
  vendor: string;
  managed: boolean;
  tags?: Record<string, string>;
  interfaces: DeviceInterface[];
  area_ids: string[];
  backup_supported: boolean;
  metrics_source: MetricsSource;
  prometheus_label_name: string;
  prometheus_label_value: string;
  topology_discovery_mode?: TopologyDiscoveryMode;
  effective_topology_discovery_mode?: TopologyDiscoveryMode;
  topology_bootstrap_state?: TopologyBootstrapState;
  last_topology_discovery_at?: string | null;
  last_topology_discovery_result?: string;
  map_visual_color?: string | null;
}

export interface Link {
  id: string;
  source_device_id: string;
  source_if_name: string;
  target_device_id: string;
  target_if_name: string;
  discovery_protocol: string;
  /** Interface speed in bps, enriched by the links list endpoint. */
  source_if_speed: number;
  /** Interface operational status, enriched by the links list endpoint. */
  source_if_oper_status: string;
  /** Interface speed in bps, enriched by the links list endpoint. */
  target_if_speed: number;
  /** Interface operational status, enriched by the links list endpoint. */
  target_if_oper_status: string;
}

export interface DevicePosition {
  device_id: string;
  x: number;
  y: number;
  pinned: boolean;
  updated_at?: string;
}

export interface CanvasMapFilter {
  area_id?: string | null;
  device_ids?: string[];
  include_cross_area_links?: boolean;
  include_ghost_devices?: boolean;
  tags?: Record<string, string>;
}

export interface CanvasMap {
  id: string;
  name: string;
  description: string;
  source_area_id: string | null;
  filter: CanvasMapFilter;
  is_default: boolean;
  device_count: number;
  link_count: number;
  position_count: number;
  created_at: string;
  updated_at: string;
}

export interface CanvasTopologyCapabilities {
  supports_topology_delta: boolean;
  supports_position_revision: boolean;
  supports_area_filtering: boolean;
}

export interface CanvasTopologyResponse {
  schema_version: 1;
  topology_version: string;
  runtime_version?: number;
  runtime_identity?: string;
  runtime_snapshot?: SnapshotPayload;
  generated_at: string;
  map?: CanvasMap;
  devices: Device[];
  links: Link[];
  positions: Record<string, DevicePosition>;
  areas: Area[];
  capabilities: CanvasTopologyCapabilities;
  settings: {
    layout: {
      version: number;
    };
  };
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

function readNullableNumber(record: APIRecord, key: string): number | null {
  const value = record[key];
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

function readNullableString(record: APIRecord, key: string): string | null {
  const value = record[key];
  return typeof value === 'string' ? value : null;
}

function readBoolean(record: APIRecord, key: string, fallback = false): boolean {
  const value = record[key];
  return typeof value === 'boolean' ? value : fallback;
}

function parseDeviceType(value: unknown): DeviceType {
  switch (value) {
    case 'access_point':
      return 'ap';
    case 'router':
    case 'switch':
    case 'ap':
    case 'firewall':
    case 'virtual':
      return value;
    default:
      return 'unknown';
  }
}

function parseDevicePollClass(value: unknown): DevicePollClass {
  switch (value) {
    case 'core':
    case 'standard':
    case 'low':
      return value;
    default:
      return 'standard';
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

function parseTopologyDiscoveryMode(
  value: unknown,
  fallback: TopologyDiscoveryMode = 'inherit',
): TopologyDiscoveryMode {
  switch (value) {
    case 'inherit':
    case 'off':
    case 'lldp':
    case 'lldp_cdp':
    case 'bootstrap_once':
      return value;
    default:
      return fallback;
  }
}

function parseTopologyBootstrapState(value: unknown): TopologyBootstrapState {
  switch (value) {
    case 'pending':
    case 'followup_scheduled':
    case 'completed':
      return value;
    default:
      return 'idle';
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
    const tags = isRecord(attributes.tags) ? (attributes.tags as Record<string, string>) : {};
    const relationships = isRecord(resource.relationships) ? resource.relationships : {};
    const interfacesRelationship = isRecord(relationships.interfaces)
      ? relationships.interfaces
      : {};
    const interfacesData = Array.isArray(interfacesRelationship.data)
      ? interfacesRelationship.data
      : [];

    const rawMetricsSource = readString(attributes, 'metrics_source', 'prometheus');
    const metricsSource: MetricsSource =
      rawMetricsSource === 'snmp'
        ? 'snmp'
        : rawMetricsSource === 'prometheus_snmp_fallback'
          ? 'prometheus_snmp_fallback'
          : rawMetricsSource === 'none'
            ? 'none'
            : 'prometheus';

    return {
      id: readString(resource, 'id'),
      hostname: readString(attributes, 'hostname'),
      ip: readString(attributes, 'ip'),
      notes: readNullableString(attributes, 'notes'),
      device_type: parseDeviceType(attributes.device_type),
      poll_class: parseDevicePollClass(attributes.poll_class),
      poll_interval_override: readNullableNumber(attributes, 'poll_interval_override'),
      polling_enabled: readBoolean(attributes, 'polling_enabled', true),
      status: parseDeviceStatus(attributes.status),
      sys_name: readString(attributes, 'sys_name'),
      sys_descr: readString(attributes, 'sys_descr'),
      hardware_model: readString(attributes, 'hardware_model'),
      os_version: readString(attributes, 'os_version'),
      vendor: readString(attributes, 'vendor', 'default'),
      managed: readBoolean(attributes, 'managed', true),
      tags,
      interfaces: interfacesData.map(parseDeviceInterface),
      area_ids: Array.isArray(attributes.area_ids) ? (attributes.area_ids as string[]) : [],
      backup_supported: readBoolean(attributes, 'backup_supported', false),
      metrics_source: metricsSource,
      prometheus_label_name: readString(attributes, 'prometheus_label_name', 'instance'),
      prometheus_label_value: readString(attributes, 'prometheus_label_value'),
      topology_discovery_mode: parseTopologyDiscoveryMode(
        attributes.topology_discovery_mode,
        'inherit',
      ),
      effective_topology_discovery_mode: parseTopologyDiscoveryMode(
        attributes.effective_topology_discovery_mode,
        'off',
      ),
      topology_bootstrap_state: parseTopologyBootstrapState(attributes.topology_bootstrap_state),
      last_topology_discovery_at: readNullableString(attributes, 'last_topology_discovery_at'),
      last_topology_discovery_result: readString(attributes, 'last_topology_discovery_result'),
      map_visual_color: readNullableString(attributes, 'map_visual_color'),
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
      source_if_speed: readNumber(resource, 'source_if_speed'),
      source_if_oper_status: readString(resource, 'source_if_oper_status'),
      target_if_speed: readNumber(resource, 'target_if_speed'),
      target_if_oper_status: readString(resource, 'target_if_oper_status'),
    };
  });
}

function parsePositionResource(resource: unknown, fallbackDeviceId = ''): DevicePosition {
  if (!isRecord(resource)) {
    throw new Error('invalid position resource');
  }

  return {
    device_id: readString(resource, 'device_id', fallbackDeviceId),
    x: readNumber(resource, 'x'),
    y: readNumber(resource, 'y'),
    pinned: readBoolean(resource, 'pinned'),
    updated_at: readString(resource, 'updated_at'),
  };
}

function parseCanvasTopologyPositions(payload: unknown): Record<string, DevicePosition> {
  if (Array.isArray(payload)) {
    return Object.fromEntries(
      payload.map((resource) => {
        const position = parsePositionResource(resource);
        return [position.device_id, position];
      }),
    );
  }

  if (!isRecord(payload)) {
    return {};
  }

  return Object.fromEntries(
    Object.entries(payload).map(([deviceId, resource]) => {
      const position = parsePositionResource(resource, deviceId);
      return [position.device_id || deviceId, position];
    }),
  );
}

function parseCanvasMapFilter(value: unknown): CanvasMapFilter {
  if (value === undefined || value === null) {
    return {};
  }

  if (!isRecord(value)) {
    throw new Error('invalid canvas map filter');
  }

  const filter: CanvasMapFilter = {};

  if ('area_id' in value) {
    if (value.area_id !== null && typeof value.area_id !== 'string') {
      throw new Error('invalid canvas map filter');
    }
    filter.area_id = value.area_id;
  }

  if ('device_ids' in value) {
    if (
      !Array.isArray(value.device_ids) ||
      !value.device_ids.every((id) => typeof id === 'string')
    ) {
      throw new Error('invalid canvas map filter');
    }
    filter.device_ids = value.device_ids;
  }

  if ('include_cross_area_links' in value) {
    if (typeof value.include_cross_area_links !== 'boolean') {
      throw new Error('invalid canvas map filter');
    }
    filter.include_cross_area_links = value.include_cross_area_links;
  }

  if ('include_ghost_devices' in value) {
    if (typeof value.include_ghost_devices !== 'boolean') {
      throw new Error('invalid canvas map filter');
    }
    filter.include_ghost_devices = value.include_ghost_devices;
  }

  if ('tags' in value) {
    if (!isRecord(value.tags) || Object.values(value.tags).some((tag) => typeof tag !== 'string')) {
      throw new Error('invalid canvas map filter');
    }
    filter.tags = value.tags as Record<string, string>;
  }

  return filter;
}

export function parseCanvasMapResponse(payload: unknown): CanvasMap {
  const resource = isRecord(payload) && isRecord(payload.data) ? payload.data : payload;

  if (!isRecord(resource)) {
    throw new Error('invalid canvas map payload');
  }

  const id = readString(resource, 'id');
  const name = readString(resource, 'name');

  if (id === '' || name === '') {
    throw new Error('invalid canvas map identity');
  }

  return {
    id,
    name,
    description: readString(resource, 'description'),
    source_area_id: readNullableString(resource, 'source_area_id'),
    filter: parseCanvasMapFilter(resource.filter),
    is_default: readBoolean(resource, 'is_default'),
    device_count: readNumber(resource, 'device_count'),
    link_count: readNumber(resource, 'link_count'),
    position_count: readNumber(resource, 'position_count'),
    created_at: readString(resource, 'created_at'),
    updated_at: readString(resource, 'updated_at'),
  };
}

export function parseCanvasMapsResponse(payload: unknown): CanvasMap[] {
  if (!isRecord(payload) || !Array.isArray(payload.data)) {
    throw new Error('invalid canvas maps response');
  }

  return payload.data.map(parseCanvasMapResponse);
}

export function parseCanvasTopologyResponse(payload: unknown): CanvasTopologyResponse {
  if (!isRecord(payload)) {
    throw new Error('invalid canvas topology response');
  }

  const capabilities = isRecord(payload.capabilities) ? payload.capabilities : {};
  const settings = isRecord(payload.settings) ? payload.settings : {};
  const layout = isRecord(settings.layout) ? settings.layout : {};

  return {
    schema_version: 1,
    topology_version: readString(payload, 'topology_version'),
    runtime_version:
      typeof payload.runtime_version === 'number' && Number.isFinite(payload.runtime_version)
        ? payload.runtime_version
        : undefined,
    runtime_identity:
      typeof payload.runtime_identity === 'string' ? payload.runtime_identity : undefined,
    runtime_snapshot:
      payload.runtime_snapshot === undefined
        ? undefined
        : parseSnapshotPayload(payload.runtime_snapshot),
    generated_at: readString(payload, 'generated_at'),
    map: payload.map === undefined ? undefined : parseCanvasMapResponse(payload.map),
    devices: parseDevicesResponse({
      data: Array.isArray(payload.devices) ? payload.devices : [],
    }),
    links: parseLinksResponse({
      data: Array.isArray(payload.links) ? payload.links : [],
    }),
    positions: parseCanvasTopologyPositions(payload.positions),
    areas: parseAreasResponse({
      data: Array.isArray(payload.areas) ? payload.areas : [],
    }),
    capabilities: {
      supports_topology_delta: readBoolean(capabilities, 'supports_topology_delta'),
      supports_position_revision: readBoolean(capabilities, 'supports_position_revision'),
      supports_area_filtering: readBoolean(capabilities, 'supports_area_filtering'),
    },
    settings: {
      layout: {
        version: readNumber(layout, 'version', 1),
      },
    },
  };
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
        community_set: readBoolean(snmp, 'community_set'),
        username: typeof snmp.username === 'string' ? snmp.username : undefined,
        auth_protocol: typeof snmp.auth_protocol === 'string' ? snmp.auth_protocol : undefined,
        auth_password: typeof snmp.auth_password === 'string' ? snmp.auth_password : undefined,
        auth_password_set: readBoolean(snmp, 'auth_password_set'),
        priv_protocol: typeof snmp.priv_protocol === 'string' ? snmp.priv_protocol : undefined,
        priv_password: typeof snmp.priv_password === 'string' ? snmp.priv_password : undefined,
        priv_password_set: readBoolean(snmp, 'priv_password_set'),
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
      community_set: readBoolean(snmp, 'community_set'),
      username: typeof snmp.username === 'string' ? snmp.username : undefined,
      auth_protocol: typeof snmp.auth_protocol === 'string' ? snmp.auth_protocol : undefined,
      auth_password: typeof snmp.auth_password === 'string' ? snmp.auth_password : undefined,
      auth_password_set: readBoolean(snmp, 'auth_password_set'),
      priv_protocol: typeof snmp.priv_protocol === 'string' ? snmp.priv_protocol : undefined,
      priv_password: typeof snmp.priv_password === 'string' ? snmp.priv_password : undefined,
      priv_password_set: readBoolean(snmp, 'priv_password_set'),
      security_level: typeof snmp.security_level === 'string' ? snmp.security_level : undefined,
    },
    created_at: readString(data, 'created_at'),
    updated_at: readString(data, 'updated_at'),
  };
}

// CredentialProfile for reusable SSH credentials shared across devices
export interface CredentialProfile {
  id: string;
  name: string;
  description: string;
  username: string;
  port: number;
  auth_method: 'password' | 'key';
  role: string;
  created_at: string;
  updated_at: string;
}

// DeviceCredentialProfile represents a credential profile assigned to a device
export interface DeviceCredentialProfile {
  profile_id: string;
  name: string;
  role: string;
  is_winbox: boolean;
}

// WinBoxCredentials holds the resolved credentials needed to launch WinBox
export interface WinBoxCredentials {
  ip: string;
  username: string;
  password: string;
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

export interface BackupFileContent {
  content: string;
  inline: boolean;
  download_url: string;
  reason?: 'unsupported_type' | 'too_large' | string;
  size_bytes: number;
  max_inline_size_bytes: number;
}

export interface BackupJob {
  id: string;
  device_id: string;
  status: BackupStatus;
  error_message: string;
  created_at: string;
  files: BackupFile[];
}

export type BulkBackupRunStatus =
  | 'running'
  | 'pausing'
  | 'paused'
  | 'cancelling'
  | 'success'
  | 'partial'
  | 'failed'
  | 'cancelled';

export type BulkBackupRunItemStatus =
  | 'checking'
  | 'skipped'
  | 'active'
  | 'queued'
  | 'running'
  | 'success'
  | 'failed'
  | 'cancelled';

export interface BulkBackupRunItem {
  id: string;
  run_id: string;
  device_id: string;
  device_name: string;
  status: BulkBackupRunItemStatus;
  reason?: string;
  backup_job_id?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

export interface BulkBackupRun {
  id: string;
  status: BulkBackupRunStatus;
  batch_size: number;
  total_count: number;
  queued_count: number;
  success_count: number;
  failed_count: number;
  skipped_count: number;
  cancelled_count: number;
  error_message: string;
  cancel_requested: boolean;
  created_by: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  items: BulkBackupRunItem[];
}

// Instance backup types
export type InstanceBackupStatus = 'running' | 'success' | 'failed' | 'cancelled';

export interface InstanceBackupProgress {
  phase: string;
  message: string;
  current: number;
  total: number;
}

export interface InstanceBackup {
  id: string;
  file_name: string;
  size_bytes: number;
  sha256: string;
  app_version: string;
  migration_version: number;
  status: InstanceBackupStatus;
  error_message: string;
  progress?: InstanceBackupProgress;
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

export function parseCredentialProfilesResponse(payload: unknown): CredentialProfile[] {
  if (!isRecord(payload)) {
    throw new Error('invalid credential profiles response');
  }

  const data = Array.isArray(payload.data) ? payload.data : [];

  return data.map((item) => {
    if (!isRecord(item)) {
      throw new Error('invalid credential profile item');
    }
    return {
      id: readString(item, 'id'),
      name: readString(item, 'name'),
      description: readString(item, 'description'),
      username: readString(item, 'username', 'admin'),
      port: readNumber(item, 'port', 22),
      auth_method: (item.auth_method === 'key' ? 'key' : 'password') as 'password' | 'key',
      role: readString(item, 'role'),
      created_at: readString(item, 'created_at'),
      updated_at: readString(item, 'updated_at'),
    };
  });
}

export function parseCredentialProfileResponse(payload: unknown): CredentialProfile {
  if (!isRecord(payload)) {
    throw new Error('invalid credential profile response');
  }
  const data = isRecord(payload.data) ? payload.data : {};
  return {
    id: readString(data, 'id'),
    name: readString(data, 'name'),
    description: readString(data, 'description'),
    username: readString(data, 'username', 'admin'),
    port: readNumber(data, 'port', 22),
    auth_method: (data.auth_method === 'key' ? 'key' : 'password') as 'password' | 'key',
    role: readString(data, 'role'),
    created_at: readString(data, 'created_at'),
    updated_at: readString(data, 'updated_at'),
  };
}

export function parseDeviceCredentialProfilesResponse(payload: unknown): DeviceCredentialProfile[] {
  if (!isRecord(payload)) {
    throw new Error('invalid device credential profiles response');
  }

  const data = Array.isArray(payload.data) ? payload.data : [];

  return data.map((item) => {
    if (!isRecord(item)) {
      throw new Error('invalid device credential profile item');
    }
    return {
      // Backend serializes profile ID as "id" (assignedProfileResponse.ID)
      profile_id: readString(item, 'id'),
      name: readString(item, 'name'),
      role: readString(item, 'role'),
      is_winbox: typeof item.is_winbox === 'boolean' ? item.is_winbox : false,
    };
  });
}

export function parseWinBoxCredentialsResponse(payload: unknown): WinBoxCredentials {
  if (!isRecord(payload)) return { ip: '', username: '', password: '' };
  return {
    ip: readString(payload, 'ip'),
    username: readString(payload, 'username'),
    password: readString(payload, 'password'),
  };
}

export function parseAreasResponse(payload: unknown): Area[] {
  if (!isRecord(payload)) return [];
  const data = payload.data;
  if (!Array.isArray(data)) return [];
  return data
    .map((item: unknown) => {
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
    })
    .filter((a): a is Area => a !== null && a.id !== '');
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

  return data.map((resource) => parsePositionResource(resource));
}
