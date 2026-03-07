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
