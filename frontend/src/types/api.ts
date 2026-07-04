/**
 * Defines api type contracts shared across frontend modules.
 * Keeps backend-facing domain shapes explicit at compile time.
 */
import { parseSnapshotPayload, type SnapshotPayload } from './metrics';

/** DeviceType mirrors backend device categories used for visualization and polling defaults. */
export type DeviceType = 'router' | 'switch' | 'ap' | 'firewall' | 'virtual' | 'unknown';
/** DevicePollClass controls scheduler cadence classes assigned by backend device classification. */
export type DevicePollClass = 'core' | 'standard' | 'low';
/** TopologyDiscoveryMode controls whether LLDP/CDP discovery is inherited, disabled, or forced per device. */
export type TopologyDiscoveryMode = 'inherit' | 'off' | 'lldp' | 'lldp_cdp' | 'bootstrap_once';
/** TopologyBootstrapState tracks the one-time discovery lifecycle for newly added devices. */
export type TopologyBootstrapState = 'idle' | 'pending' | 'followup_scheduled' | 'completed';

/** SNMPProfile represents a reusable set of SNMP credentials. */
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
/** DeviceStatus is the persisted reachability state stored with the canonical device record. */
export type DeviceStatus = 'up' | 'down' | 'probing' | 'unknown';

/** DeviceAddressRole describes how the backend should use one saved device address. */
export type DeviceAddressRole = 'primary' | 'management' | 'backup' | 'monitoring' | 'other';

/** DeviceAddress is one persisted address attached to the canonical device record. */
export interface DeviceAddress {
  id: string;
  device_id: string;
  address: string;
  label: string;
  role: DeviceAddressRole;
  is_primary: boolean;
  priority: number;
  probe_ports: number[] | null;
  created_at?: string;
  updated_at?: string;
}

/** DeviceInterface is the last known interface inventory for one device. */
export interface DeviceInterface {
  id: string;
  if_index: number;
  if_name: string;
  if_descr: string;
  speed: number;
  admin_status: string;
  oper_status: string;
}

/** MetricsSource identifies which collector path currently owns runtime telemetry for a device. */
export type MetricsSource = 'prometheus' | 'snmp' | 'prometheus_snmp_fallback' | 'none';

/** Device is the canonical API representation used by topology, inventory, and settings flows. */
export interface Device {
  id: string;
  hostname: string;
  ip: string;
  addresses: DeviceAddress[];
  probe_ports: number[] | null;
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

/** Link is a canonical topology edge enriched with endpoint interface metadata for display. */
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

/** DevicePosition stores user-owned canvas coordinates for one device in a map context. */
export interface DevicePosition {
  device_id: string;
  x: number;
  y: number;
  pinned: boolean;
  updated_at?: string;
}

/** CanvasMapFilter describes a saved projection over the canonical topology graph. */
export interface CanvasMapFilter {
  area_id?: string | null;
  device_ids?: string[];
  include_cross_area_links?: boolean;
  include_ghost_devices?: boolean;
  tags?: Record<string, string>;
}

/** CanvasMap is saved topology map metadata plus aggregate counts for hub/list views. */
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

/** CanvasTopologyCapabilities advertises optional server features for topology clients. */
export interface CanvasTopologyCapabilities {
  supports_topology_delta: boolean;
  supports_position_revision: boolean;
  supports_area_filtering: boolean;
}

/** CanvasTopologyResponse is the canonical HTTP bootstrap payload for the canvas. */
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

/** InterfaceInfo describes selectable interfaces when creating or editing manual links. */
export interface InterfaceInfo {
  if_name: string;
  if_descr: string;
  speed: number;
  oper_status: string;
  admin_status: string;
  in_use: boolean;
  in_use_by?: string;
}

/** GrafanaVariableSource selects which Theia value fills dashboard URL templates. */
export type GrafanaVariableSource = 'hostname' | 'ip' | 'map_name' | 'map_id';

/** GrafanaDashboardProfile is a reusable dashboard URL template. */
export interface GrafanaDashboardProfile {
  id: string;
  name: string;
  url_template: string;
  variable_source: GrafanaVariableSource;
  created_at?: string;
  updated_at?: string;
}

/** GrafanaDeviceDashboardOverride stores per-device dashboard customization. */
export interface GrafanaDeviceDashboardOverride {
  profile_id: string | null;
  custom_url: string;
  updated_at?: string;
}

/** GrafanaDashboardConfig groups profiles, the default profile, and device overrides. */
export interface GrafanaDashboardConfig {
  profiles: GrafanaDashboardProfile[];
  default_profile_id: string;
  device_overrides: Record<string, GrafanaDeviceDashboardOverride>;
}

type APIRecord = Record<string, unknown>;

// isRecord narrows unknown payloads to non-array objects before DTO parsing.
function isRecord(value: unknown): value is APIRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

// readString reads optional string fields while preserving the caller's fallback.
function readString(record: APIRecord, key: string, fallback = ''): string {
  const value = record[key];
  return typeof value === 'string' ? value : fallback;
}

// readNumber reads finite numeric fields while preserving existing zero-style defaults.
function readNumber(record: APIRecord, key: string, fallback = 0): number {
  const value = record[key];
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

// readNullableNumber preserves null for absent or non-finite numeric fields.
function readNullableNumber(record: APIRecord, key: string): number | null {
  const value = record[key];
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

function readNullableNumberArray(record: APIRecord, key: string): number[] | null {
  const value = record[key];
  if (!Array.isArray(value)) {
    return null;
  }
  if (!value.every((item) => typeof item === 'number' && Number.isInteger(item))) {
    return null;
  }
  return [...value];
}

// readNullableString preserves null for absent or non-string fields.
function readNullableString(record: APIRecord, key: string): string | null {
  const value = record[key];
  return typeof value === 'string' ? value : null;
}

// readBoolean reads optional booleans while preserving the caller's default.
function readBoolean(record: APIRecord, key: string, fallback = false): boolean {
  const value = record[key];
  return typeof value === 'boolean' ? value : fallback;
}

// parseDeviceType normalizes legacy access_point values and unknown device types.
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

// parseDevicePollClass normalizes unknown poll classes to the standard class.
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

// parseDeviceStatus normalizes unknown device status values for canvas rendering.
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

// parseTopologyDiscoveryMode validates discovery modes with a caller-selected fallback.
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

// parseTopologyBootstrapState normalizes missing bootstrap state to idle.
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

// parseDeviceAddressRole validates address role values with a conservative fallback.
function parseDeviceAddressRole(value: unknown): DeviceAddressRole {
  switch (value) {
    case 'primary':
    case 'management':
    case 'backup':
    case 'monitoring':
    case 'other':
      return value;
    default:
      return 'other';
  }
}

function primaryAddressFromIP(deviceID: string, ip: string): DeviceAddress {
  return {
    id: '',
    device_id: deviceID,
    address: ip,
    label: '',
    role: 'primary',
    is_primary: true,
    priority: 0,
    probe_ports: null,
  };
}

function parseDeviceAddress(
  value: unknown,
  fallbackDeviceID: string,
  fallbackPriority: number,
): DeviceAddress | null {
  if (!isRecord(value)) {
    return null;
  }

  const address = readString(value, 'address').trim();
  if (address === '') {
    return null;
  }

  const role = parseDeviceAddressRole(value.role);
  return {
    id: readString(value, 'id'),
    device_id: readString(value, 'device_id', fallbackDeviceID),
    address,
    label: readString(value, 'label'),
    role,
    is_primary: readBoolean(value, 'is_primary', role === 'primary'),
    priority: readNumber(value, 'priority', fallbackPriority),
    probe_ports: readNullableNumberArray(value, 'probe_ports'),
    created_at: readString(value, 'created_at') || undefined,
    updated_at: readString(value, 'updated_at') || undefined,
  };
}

function parseDeviceAddresses(
  attributes: APIRecord,
  deviceID: string,
  ip: string,
): DeviceAddress[] {
  const rawAddresses = attributes.addresses;
  if (!Array.isArray(rawAddresses)) {
    return ip === '' ? [] : [primaryAddressFromIP(deviceID, ip)];
  }

  const addresses = rawAddresses
    .map((address, index) => parseDeviceAddress(address, deviceID, index))
    .filter((address): address is DeviceAddress => address !== null);

  return addresses.length > 0 ? addresses : ip === '' ? [] : [primaryAddressFromIP(deviceID, ip)];
}

// parseDeviceInterface validates one embedded device interface resource.
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

// parseDevicesResponse converts JSON:API device resources while preserving legacy defaults.
/**
 * Parses the devices collection response and normalizes legacy/partial fields into frontend defaults.
 * Invalid non-object entries are skipped so one malformed row does not break the entire inventory view.
 */
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

    const deviceID = readString(resource, 'id');
    const ip = readString(attributes, 'ip');

    return {
      id: deviceID,
      hostname: readString(attributes, 'hostname'),
      ip,
      addresses: parseDeviceAddresses(attributes, deviceID, ip),
      probe_ports: readNullableNumberArray(attributes, 'probe_ports'),
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

// parseLinksResponse converts link resources while preserving empty-string and zero defaults.
/** Parses canonical topology links and preserves optional endpoint enrichment when present. */
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

// parsePositionResource converts one canvas position and can fall back to the map key ID.
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

// parseCanvasTopologyPositions accepts both array and keyed-object position payloads.
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

// parseCanvasMapFilter validates saved-map filters without adding missing optional keys.
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

// parseCanvasMapResponse parses a single saved-map DTO from wrapped or direct payloads.
/** Parses one saved map and normalizes absent filters/counts to stable empty defaults. */
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

// parseCanvasMapsResponse parses saved-map list responses and rejects malformed envelopes.
/** Parses the saved-map list endpoint and rejects non-array payloads. */
export function parseCanvasMapsResponse(payload: unknown): CanvasMap[] {
  if (!isRecord(payload) || !Array.isArray(payload.data)) {
    throw new Error('invalid canvas maps response');
  }

  return payload.data.map(parseCanvasMapResponse);
}

// parseCanvasTopologyResponse parses topology payloads including runtime, positions, and map metadata.
/**
 * Parses the canvas bootstrap payload that seeds canonical graph, map metadata, positions, and runtime state.
 * Positions and runtime snapshot are optional so clients can tolerate older or degraded backend responses.
 */
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

// parseInterfacesResponse converts interface list responses for device detail screens.
/** Parses interface selection responses for manual link and device-detail UI. */
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

// parseSNMPProfilesResponse parses SNMP profiles while preserving secret-set booleans.
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

// parseGrafanaVariableSource keeps unknown variable sources on the hostname default.
function parseGrafanaVariableSource(value: unknown): GrafanaVariableSource {
  return value === 'ip' || value === 'map_name' || value === 'map_id' ? value : 'hostname';
}

// parseGrafanaDashboardConfigResponse parses dashboard profiles and per-device overrides.
export function parseGrafanaDashboardConfigResponse(payload: unknown): GrafanaDashboardConfig {
  if (!isRecord(payload)) {
    throw new Error('invalid grafana dashboard config response');
  }
  const data = isRecord(payload.data) ? payload.data : {};
  const profilesData = Array.isArray(data.profiles) ? data.profiles : [];
  const profiles = profilesData.map((item) => {
    if (!isRecord(item)) {
      throw new Error('invalid grafana dashboard profile item');
    }
    return {
      id: readString(item, 'id'),
      name: readString(item, 'name'),
      url_template: readString(item, 'url_template'),
      variable_source: parseGrafanaVariableSource(item.variable_source),
      created_at: readString(item, 'created_at') || undefined,
      updated_at: readString(item, 'updated_at') || undefined,
    };
  });

  const rawOverrides = isRecord(data.device_overrides) ? data.device_overrides : {};
  const device_overrides = Object.fromEntries(
    Object.entries(rawOverrides).flatMap(([deviceId, value]) => {
      if (!isRecord(value)) {
        return [];
      }
      return [
        [
          deviceId,
          {
            profile_id: readNullableString(value, 'profile_id'),
            custom_url: readString(value, 'custom_url'),
            updated_at: readString(value, 'updated_at') || undefined,
          },
        ],
      ];
    }),
  );

  return {
    profiles,
    default_profile_id: readString(data, 'default_profile_id'),
    device_overrides,
  };
}

// parseSNMPProfileResponse parses one SNMP profile and keeps omitted secrets undefined.
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

/** CredentialProfile describes reusable SSH credentials shared across devices. */
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

/** DeviceCredentialProfile represents one credential assignment for a device, including WinBox role. */
export interface DeviceCredentialProfile {
  profile_id: string;
  name: string;
  role: string;
  is_winbox: boolean;
}

/** WinBoxCredentials holds the resolved credentials needed to launch WinBox. */
export interface WinBoxCredentials {
  ip: string;
  username: string;
  password: string;
}

/** Area represents a named device grouping used for filtering and map projections. */
export interface Area {
  id: string;
  name: string;
  description: string;
  color: string;
  device_count: number;
  created_at: string;
  updated_at: string;
}

/** BackupStatus is the lifecycle state for a single-device configuration backup job. */
export type BackupStatus = 'pending' | 'running' | 'success' | 'failed';

/** BackupFile is metadata for a generated backup artifact; file paths are intentionally not exposed. */
export interface BackupFile {
  id: string;
  job_id: string;
  file_type: string;
  file_name: string;
  file_hash: string;
  size_bytes: number;
  created_at: string;
}

/** BackupFileContent is the API response for inline preview or delegated download of a backup artifact. */
export interface BackupFileContent {
  content: string;
  inline: boolean;
  download_url: string;
  reason?: 'unsupported_type' | 'too_large' | string;
  size_bytes: number;
  max_inline_size_bytes: number;
}

/** BackupJob is one device backup operation and the files it produced. */
export interface BackupJob {
  id: string;
  device_id: string;
  status: BackupStatus;
  error_message: string;
  error_code?: string;
  created_at: string;
  files: BackupFile[];
}

/** BulkBackupRunStatus is the durable lifecycle state of a multi-device backup run. */
export type BulkBackupRunStatus =
  | 'running'
  | 'pausing'
  | 'paused'
  | 'cancelling'
  | 'success'
  | 'partial'
  | 'failed'
  | 'cancelled';

/** BulkBackupRunItemStatus is one device's lifecycle state inside a bulk backup run. */
export type BulkBackupRunItemStatus =
  | 'checking'
  | 'skipped'
  | 'active'
  | 'queued'
  | 'running'
  | 'success'
  | 'failed'
  | 'cancelled';

/** BulkBackupRunItem is one device row in the durable bulk backup run timeline. */
export interface BulkBackupRunItem {
  id: string;
  run_id: string;
  device_id: string;
  device_name: string;
  status: BulkBackupRunItemStatus;
  reason?: string;
  backup_job_id?: string;
  file_count: number;
  byte_count: number;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

/** BulkBackupRun summarizes durable multi-device backup progress and embeds item state. */
export interface BulkBackupRun {
  id: string;
  status: BulkBackupRunStatus;
  batch_size: number;
  total_count: number;
  queued_count: number;
  running_count: number;
  completed_count: number;
  success_count: number;
  failed_count: number;
  skipped_count: number;
  cancelled_count: number;
  file_count: number;
  byte_count: number;
  current_device_id?: string;
  current_device_name?: string;
  current_job_id?: string;
  error_message: string;
  cancel_requested: boolean;
  created_by: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  items: BulkBackupRunItem[];
}

/** BulkOperationStatus advertises backend quota and concurrency limits for bulk actions. */
export interface BulkOperationStatus {
  bulk_backup_run: {
    /** 0 means persistent bulk backup runs have no enforced selected-device cap. */
    max_devices: number;
    /** 0 means persistent bulk backup runs have no enforced queued-item cap. */
    max_queued_jobs: number;
    batch_size: number;
    max_active_runs: number;
    configurable_concurrency: boolean;
    distributed: boolean;
    distributed_max_active_runs: number;
    can_pause: boolean;
    can_resume: boolean;
    can_cancel: boolean;
  };
  bulk_download: {
    max_devices: number;
    max_files: number;
    max_bytes: number;
    max_concurrent_per_actor: number;
    max_concurrent_global: number;
    distributed: boolean;
    distributed_max_concurrent_per_actor: number;
    distributed_max_concurrent_global: number;
  };
}

/** InstanceBackupStatus is the lifecycle state for full application backup archives. */
export type InstanceBackupStatus = 'running' | 'success' | 'failed' | 'cancelled';

/** InstanceBackupProgress reports best-effort phase progress for a running archive job. */
export interface InstanceBackupProgress {
  phase: string;
  message: string;
  current: number;
  total: number;
}

/** InstanceBackup is metadata for a full Theia instance archive. */
export interface InstanceBackup {
  id: string;
  file_name: string;
  size_bytes: number;
  sha256: string;
  migration_version: number;
  status: InstanceBackupStatus;
  error_message: string;
  progress?: InstanceBackupProgress;
  trigger: 'manual' | 'scheduled';
  created_at: string;
}

/** RestoreReport is returned after dry-run validation or staging of an instance backup archive. */
export interface RestoreReport {
  valid: boolean;
  migration_version: number;
  created_at: string;
  db_size_bytes: number;
  backup_file_count: number;
  total_size_bytes: number;
  needs_migration: boolean;
  current_migration_version: number;
  message: string;
}

/** RestoreStatusPhase is the restart-handoff lifecycle for staged restores. */
export type RestoreStatusPhase =
  | 'validation_passed'
  | 'staged_restart_pending'
  | 'startup_restore_detected'
  | 'applying_postgres'
  | 'postgres_applied'
  | 'verifying_keyring'
  | 'running_credential_rewrap'
  | 'completed'
  | 'failed_retryable'
  | 'failed_operator_action_required';

/** RestoreStatus describes a persisted restore operation across process restarts. */
export interface RestoreStatus {
  operation_id: string;
  phase: RestoreStatusPhase;
  attempt_count: number;
  last_error: string;
  missing_key_id: string;
  created_at: string;
  updated_at: string;
}

/** VendorConfig is a parsed vendor capability profile used for metrics and backup behavior. */
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

// parseCredentialProfilesResponse parses reusable credential profiles with legacy defaults.
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

// parseCredentialProfileResponse parses one credential profile response envelope.
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

// parseDeviceCredentialProfilesResponse parses credential profile assignments for one device.
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

// parseWinBoxCredentialsResponse preserves empty strings when privileged credentials are absent.
export function parseWinBoxCredentialsResponse(payload: unknown): WinBoxCredentials {
  if (!isRecord(payload)) return { ip: '', username: '', password: '' };
  return {
    ip: readString(payload, 'ip'),
    username: readString(payload, 'username'),
    password: readString(payload, 'password'),
  };
}

// parseAreasResponse parses area lists and drops malformed or ID-less entries.
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

// parseAreaResponse parses one area from wrapped or direct payloads.
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

// parsePositionsResponse parses persisted canvas position rows.
export function parsePositionsResponse(payload: unknown): DevicePosition[] {
  if (!isRecord(payload)) {
    throw new Error('invalid positions response');
  }

  const data = Array.isArray(payload.data) ? payload.data : [];

  return data.map((resource) => parsePositionResource(resource));
}
