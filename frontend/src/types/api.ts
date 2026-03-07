export type DeviceType = 'router' | 'switch' | 'ap' | 'unknown';
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

export interface Device {
  id: string;
  hostname: string;
  ip: string;
  device_type: DeviceType;
  status: DeviceStatus;
  sys_name: string;
  sys_descr: string;
  hardware_model: string;
  managed: boolean;
  interfaces: DeviceInterface[];
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
  if (!isRecord(payload) || !Array.isArray(payload.data)) {
    throw new Error('invalid devices response');
  }

  return payload.data.map((resource) => {
    if (!isRecord(resource)) {
      throw new Error('invalid device resource');
    }

    const attributes = isRecord(resource.attributes) ? resource.attributes : {};
    const relationships = isRecord(resource.relationships) ? resource.relationships : {};
    const interfacesRelationship = isRecord(relationships.interfaces)
      ? relationships.interfaces
      : {};
    const interfacesData = Array.isArray(interfacesRelationship.data)
      ? interfacesRelationship.data
      : [];

    return {
      id: readString(resource, 'id'),
      hostname: readString(attributes, 'hostname'),
      ip: readString(attributes, 'ip'),
      device_type: parseDeviceType(attributes.device_type),
      status: parseDeviceStatus(attributes.status),
      sys_name: readString(attributes, 'sys_name'),
      sys_descr: readString(attributes, 'sys_descr'),
      hardware_model: readString(attributes, 'hardware_model'),
      managed: readBoolean(attributes, 'managed', true),
      interfaces: interfacesData.map(parseDeviceInterface),
    };
  });
}

export function parseLinksResponse(payload: unknown): Link[] {
  if (!isRecord(payload) || !Array.isArray(payload.data)) {
    throw new Error('invalid links response');
  }

  return payload.data.map((resource) => {
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

export function parsePositionsResponse(payload: unknown): DevicePosition[] {
  if (!isRecord(payload) || !Array.isArray(payload.data)) {
    throw new Error('invalid positions response');
  }

  return payload.data.map((resource) => {
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
