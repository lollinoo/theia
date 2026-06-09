/**
 * Provides frontend API helpers for device endpoints.
 * Keeps request construction and backend response handling out of UI components.
 */
import {
  type Device,
  type DeviceAddressRole,
  type DeviceCredentialProfile,
  type InterfaceInfo,
  type Link,
  parseDeviceCredentialProfilesResponse,
  parseDevicesResponse,
  parseInterfacesResponse,
  parseLinksResponse,
  parseWinBoxCredentialsResponse,
  type TopologyDiscoveryMode,
  type WinBoxCredentials,
} from '../types/api';
import { requestJSON, requestJSONWithBody } from './transport';

/** Describes the bridge launch request response contract used by the frontend API boundary. */
export interface BridgeLaunchRequestResponse {
  launch_token: string;
  expires_at?: string;
}

/** Describes the snmppayload contract used by the frontend API boundary. */
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

/** Describes one device address submitted to the backend from editable forms. */
export interface DeviceAddressPayload {
  address: string;
  label?: string;
  role?: DeviceAddressRole;
  is_primary?: boolean;
  priority?: number;
  probe_ports?: number[] | null;
}

/** Describes the create device payload contract used by the frontend API boundary. */
export interface CreateDevicePayload {
  hostname: string;
  ip?: string;
  addresses?: DeviceAddressPayload[];
  probe_ports?: number[] | null;
  notes?: string | null;
  device_type?: string;
  snmp?: SNMPPayload;
  tags?: Record<string, string>;
  vendor?: string;
  metrics_source?: string;
  prometheus_label_name?: string;
  prometheus_label_value?: string;
  topology_discovery_mode?: TopologyDiscoveryMode;
  area_ids?: string[];
  skip_primary_map_membership?: boolean;
}

// fetchDevices loads all devices and keeps legacy error text used by callers.
export async function fetchDevices(): Promise<Device[]> {
  try {
    return parseDevicesResponse(await requestJSON('/api/v1/devices'));
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch devices: ${message}`);
  }
}

// fetchOrphanDevices loads devices that are not assigned to a canvas area.
export async function fetchOrphanDevices(): Promise<Device[]> {
  try {
    return parseDevicesResponse(await requestJSON('/api/v1/devices/orphans'));
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch orphan devices: ${message}`);
  }
}

// fetchLinks loads topology links and preserves the caller-facing failure message.
export async function fetchLinks(): Promise<Link[]> {
  try {
    return parseLinksResponse(await requestJSON('/api/v1/links'));
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch links: ${message}`);
  }
}

// createDevice creates a device and unwraps the single returned resource from the API envelope.
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

// updateDevice replaces editable device metadata and unwraps the single returned resource.
export async function updateDevice(
  id: string,
  payload: Partial<{
    hostname: string;
    ip: string;
    addresses: DeviceAddressPayload[];
    probe_ports: number[] | null;
    notes: string | null;
    snmp: SNMPPayload;
    tags: Record<string, string>;
    vendor: string;
    metrics_source: string;
    prometheus_label_name: string;
    prometheus_label_value: string;
    topology_discovery_mode: TopologyDiscoveryMode;
    poll_interval_override: number | null;
    polling_enabled: boolean;
    area_ids: string[];
  }>,
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

// deleteDevice removes one device by ID.
export async function deleteDevice(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/devices/${encodeURIComponent(id)}`, 'DELETE');
}

// runTopologyDiscovery triggers backend topology discovery for one device.
export async function runTopologyDiscovery(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/devices/${encodeURIComponent(id)}/topology-discovery`, 'POST');
}

// fetchDeviceInterfaces loads interface telemetry metadata for one device.
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

// createLink creates a manual topology link and preserves legacy field defaults.
export async function createLink(payload: {
  source_device_id: string;
  source_if_name: string;
  target_device_id: string;
  target_if_name: string;
  migration_source?: 'browser_localstorage';
}): Promise<Link> {
  const response = await requestJSONWithBody('/api/v1/links', 'POST', payload);
  const data = (response as Record<string, unknown>)?.data;
  if (!data || typeof data !== 'object') {
    throw new Error('Invalid create link response');
  }
  const record = data as Record<string, unknown>;
  return {
    id: typeof record.id === 'string' ? record.id : '',
    source_device_id: typeof record.source_device_id === 'string' ? record.source_device_id : '',
    source_if_name: typeof record.source_if_name === 'string' ? record.source_if_name : '',
    target_device_id: typeof record.target_device_id === 'string' ? record.target_device_id : '',
    target_if_name: typeof record.target_if_name === 'string' ? record.target_if_name : '',
    discovery_protocol:
      typeof record.discovery_protocol === 'string' ? record.discovery_protocol : 'manual',
    source_if_speed: typeof record.source_if_speed === 'number' ? record.source_if_speed : 0,
    source_if_oper_status:
      typeof record.source_if_oper_status === 'string' ? record.source_if_oper_status : '',
    target_if_speed: typeof record.target_if_speed === 'number' ? record.target_if_speed : 0,
    target_if_oper_status:
      typeof record.target_if_oper_status === 'string' ? record.target_if_oper_status : '',
  };
}

// updateLink replaces manual link endpoints and preserves legacy field defaults.
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
    source_device_id: typeof record.source_device_id === 'string' ? record.source_device_id : '',
    source_if_name: typeof record.source_if_name === 'string' ? record.source_if_name : '',
    target_device_id: typeof record.target_device_id === 'string' ? record.target_device_id : '',
    target_if_name: typeof record.target_if_name === 'string' ? record.target_if_name : '',
    discovery_protocol:
      typeof record.discovery_protocol === 'string' ? record.discovery_protocol : 'manual',
    source_if_speed: typeof record.source_if_speed === 'number' ? record.source_if_speed : 0,
    source_if_oper_status:
      typeof record.source_if_oper_status === 'string' ? record.source_if_oper_status : '',
    target_if_speed: typeof record.target_if_speed === 'number' ? record.target_if_speed : 0,
    target_if_oper_status:
      typeof record.target_if_oper_status === 'string' ? record.target_if_oper_status : '',
  };
}

// deleteLink removes one topology link by ID.
export async function deleteLink(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/links/${encodeURIComponent(id)}`, 'DELETE');
}

// testSNMPConnection runs the backend SNMP connectivity probe for one device.
export async function testSNMPConnection(deviceId: string): Promise<{
  success: boolean;
  sys_name?: string;
  sys_descr?: string;
  error?: string;
  target_ip?: string;
  snmp_version?: string;
}> {
  const response = await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/snmp-test`,
    'POST',
  );
  const data = response as Record<string, unknown>;
  return {
    success: data.success === true,
    sys_name: typeof data.sys_name === 'string' ? data.sys_name : undefined,
    sys_descr: typeof data.sys_descr === 'string' ? data.sys_descr : undefined,
    error: typeof data.error === 'string' ? data.error : undefined,
    target_ip: typeof data.target_ip === 'string' ? data.target_ip : undefined,
    snmp_version: typeof data.snmp_version === 'string' ? data.snmp_version : undefined,
  };
}

// fetchDeviceCredentialProfiles loads credential assignments for one device.
export async function fetchDeviceCredentialProfiles(
  deviceId: string,
): Promise<DeviceCredentialProfile[]> {
  const payload = await requestJSON(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/credential-profiles`,
  );
  return parseDeviceCredentialProfilesResponse(payload);
}

// assignCredentialProfile assigns an existing credential profile to one device.
export async function assignCredentialProfile(deviceId: string, profileId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/credential-profiles`,
    'POST',
    { profile_id: profileId },
  );
}

// unassignCredentialProfile removes one credential profile assignment from a device.
export async function unassignCredentialProfile(
  deviceId: string,
  profileId: string,
): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/credential-profiles/${encodeURIComponent(profileId)}`,
    'DELETE',
  );
}

// setWinBoxProfile assigns the WinBox credential profile for one device.
export async function setWinBoxProfile(deviceId: string, profileId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/winbox-profile`,
    'PUT',
    { profile_id: profileId },
  );
}

// clearWinBoxProfile removes the WinBox credential profile for one device.
export async function clearWinBoxProfile(deviceId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/winbox-profile`,
    'DELETE',
  );
}

// fetchWinBoxCredentials requests privileged WinBox credentials for one device.
export async function fetchWinBoxCredentials(deviceId: string): Promise<WinBoxCredentials> {
  const payload = await requestJSON(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/winbox-credentials`,
  );
  return parseWinBoxCredentialsResponse(payload);
}

// createBridgeLaunchRequest creates a short-lived bridge launch token for one device.
export async function createBridgeLaunchRequest(
  deviceId: string,
): Promise<BridgeLaunchRequestResponse> {
  const payload = await requestJSONWithBody(
    `/api/v1/bridge/launch-requests/${encodeURIComponent(deviceId)}`,
    'POST',
  );
  const p = payload as Record<string, unknown>;
  if (typeof p?.launch_token !== 'string' || p.launch_token === '') {
    throw new Error('invalid bridge launch response');
  }
  return {
    launch_token: p.launch_token,
    expires_at: typeof p.expires_at === 'string' ? p.expires_at : undefined,
  };
}

// testSSHConnection runs the backend SSH credential probe for one device.
export async function testSSHConnection(
  deviceId: string,
): Promise<{ success: boolean; error?: string }> {
  const response = await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/ssh-credentials/test`,
    'POST',
  );
  const data = response as Record<string, unknown>;
  return {
    success: data.success === true,
    error: typeof data.error === 'string' ? data.error : undefined,
  };
}
