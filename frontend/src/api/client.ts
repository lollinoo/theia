import {
  type Device,
  type InterfaceInfo,
  type Link,
  parseDevicesResponse,
  parseInterfacesResponse,
  parseLinksResponse,
} from '../types/api';

type ErrorPayload = {
  error?: string;
};

async function requestJSON(path: string): Promise<unknown> {
  const response = await fetch(path, {
    headers: {
      Accept: 'application/json',
    },
  });

  const payload = (await response.json().catch(() => null)) as ErrorPayload | unknown;

  if (!response.ok) {
    const errorMessage =
      typeof payload === 'object' &&
        payload !== null &&
        'error' in payload &&
        typeof payload.error === 'string'
        ? payload.error
        : response.statusText;
    throw new Error(`${path} failed: ${response.status} ${errorMessage}`);
  }

  return payload;
}

export async function fetchDevices(): Promise<Device[]> {
  try {
    return parseDevicesResponse(await requestJSON('/api/v1/devices'));
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch devices: ${message}`);
  }
}

export async function fetchLinks(): Promise<Link[]> {
  try {
    return parseLinksResponse(await requestJSON('/api/v1/links'));
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch links: ${message}`);
  }
}

async function requestJSONWithBody(
  path: string,
  method: string,
  body?: unknown,
): Promise<unknown> {
  const response = await fetch(path, {
    method,
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  if (response.status === 204) {
    return null;
  }

  const payload = (await response.json().catch(() => null)) as ErrorPayload | unknown;

  if (!response.ok) {
    const errorMessage =
      typeof payload === 'object' &&
        payload !== null &&
        'error' in payload &&
        typeof payload.error === 'string'
        ? payload.error
        : response.statusText;
    throw new Error(`${path} failed: ${response.status} ${errorMessage}`);
  }

  return payload;
}

export async function fetchSettings(): Promise<Record<string, string>> {
  try {
    const payload = await requestJSON('/api/v1/settings');
    if (
      typeof payload === 'object' &&
      payload !== null &&
      'data' in payload &&
      typeof (payload as Record<string, unknown>).data === 'object' &&
      (payload as Record<string, unknown>).data !== null
    ) {
      const data = (payload as Record<string, unknown>).data as Record<string, unknown>;
      return Object.fromEntries(
        Object.entries(data).map(([k, v]) => [k, typeof v === 'string' ? v : String(v ?? '')]),
      );
    }
    return {};
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch settings: ${message}`);
  }
}

export async function updateSetting(key: string, value: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/settings/${encodeURIComponent(key)}`, 'PUT', { value });
}

export interface SNMPPayload {
  version: string;
  community?: string;
  // SNMPv3 fields
  username?: string;
  auth_protocol?: string;
  auth_password?: string;
  priv_protocol?: string;
  priv_password?: string;
  security_level?: string;
}

export interface CreateDevicePayload {
  hostname: string;
  ip: string;
  snmp: SNMPPayload;
  tags?: Record<string, string>;
}

export async function createDevice(payload: CreateDevicePayload): Promise<Device> {
  const response = await requestJSONWithBody('/api/v1/devices', 'POST', payload);
  const data = (response as Record<string, unknown>)?.data;
  if (!data) {
    throw new Error('Invalid create device response');
  }
  const wrapped = { data: [data] };
  const devices = parseDevicesResponse(wrapped);
  if (devices.length === 0) {
    throw new Error('No device returned from create');
  }
  return devices[0];
}

export async function updateDevice(
  id: string,
  payload: Partial<{ hostname: string; ip: string; snmp: SNMPPayload; tags: Record<string, string> }>,
): Promise<Device> {
  const response = await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(id)}`,
    'PUT',
    payload,
  );
  const data = (response as Record<string, unknown>)?.data;
  if (!data) {
    throw new Error('Invalid update device response');
  }
  const wrapped = { data: [data] };
  const devices = parseDevicesResponse(wrapped);
  if (devices.length === 0) {
    throw new Error('No device returned from update');
  }
  return devices[0];
}

export async function deleteDevice(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/devices/${encodeURIComponent(id)}`, 'DELETE');
}

export async function fetchDeviceInterfaces(deviceId: string): Promise<InterfaceInfo[]> {
  try {
    return parseInterfacesResponse(
      await requestJSON(`/api/v1/devices/${encodeURIComponent(deviceId)}/interfaces`),
    );
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch interfaces: ${message}`);
  }
}

export async function createLink(payload: {
  source_device_id: string;
  source_if_name: string;
  target_device_id: string;
  target_if_name: string;
}): Promise<Link> {
  const response = await requestJSONWithBody('/api/v1/links', 'POST', payload);
  const data = (response as Record<string, unknown>)?.data;
  if (!data || typeof data !== 'object') {
    throw new Error('Invalid create link response');
  }
  const record = data as Record<string, unknown>;
  return {
    id: typeof record.id === 'string' ? record.id : '',
    source_device_id:
      typeof record.source_device_id === 'string' ? record.source_device_id : '',
    source_if_name: typeof record.source_if_name === 'string' ? record.source_if_name : '',
    target_device_id:
      typeof record.target_device_id === 'string' ? record.target_device_id : '',
    target_if_name: typeof record.target_if_name === 'string' ? record.target_if_name : '',
    discovery_protocol:
      typeof record.discovery_protocol === 'string' ? record.discovery_protocol : 'manual',
  };
}

export async function updateLink(
  id: string,
  payload: { source_if_name: string; target_if_name: string },
): Promise<Link> {
  const response = await requestJSONWithBody(
    `/api/v1/links/${encodeURIComponent(id)}`,
    'PUT',
    payload,
  );
  const data = (response as Record<string, unknown>)?.data;
  if (!data || typeof data !== 'object') {
    throw new Error('Invalid update link response');
  }
  const record = data as Record<string, unknown>;
  return {
    id: typeof record.id === 'string' ? record.id : '',
    source_device_id:
      typeof record.source_device_id === 'string' ? record.source_device_id : '',
    source_if_name: typeof record.source_if_name === 'string' ? record.source_if_name : '',
    target_device_id:
      typeof record.target_device_id === 'string' ? record.target_device_id : '',
    target_if_name: typeof record.target_if_name === 'string' ? record.target_if_name : '',
    discovery_protocol:
      typeof record.discovery_protocol === 'string' ? record.discovery_protocol : 'manual',
  };
}

export async function deleteLink(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/links/${encodeURIComponent(id)}`, 'DELETE');
}
