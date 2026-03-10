import {
  type Device,
  type Link,
  parseDevicesResponse,
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

export interface CreateDevicePayload {
  hostname: string;
  ip: string;
  snmp: { version: string; community: string };
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
  payload: Partial<{ hostname: string; ip: string; snmp: { version: string; community: string }; tags: Record<string, string> }>,
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
