import {
  type Area,
  type BackupFile,
  type BackupFileContent,
  type BackupJob,
  type BackupStatus,
  type CanvasMap,
  type CanvasMapFilter,
  type CanvasTopologyResponse,
  type CredentialProfile,
  type Device,
  type DeviceCredentialProfile,
  type InstanceBackup,
  type InstanceBackupProgress,
  type InstanceBackupStatus,
  type InterfaceInfo,
  type Link,
  type RestoreReport,
  type SNMPProfile,
  type TopologyDiscoveryMode,
  type VendorConfig,
  type WinBoxCredentials,
  parseAreaResponse,
  parseAreasResponse,
  parseCanvasMapResponse,
  parseCanvasMapsResponse,
  parseCanvasTopologyResponse,
  parseCredentialProfileResponse,
  parseCredentialProfilesResponse,
  parseDeviceCredentialProfilesResponse,
  parseDevicesResponse,
  parseInterfacesResponse,
  parseLinksResponse,
  parseSNMPProfileResponse,
  parseSNMPProfilesResponse,
  parseWinBoxCredentialsResponse,
} from '../types/api';
import { ServerError, ValidationError } from './errors';

export { ValidationError, ServerError };

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

export class CanvasTopologyFetchError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = 'CanvasTopologyFetchError';
    this.status = status;
  }
}

export type CanvasTopologyFetchResult =
  | {
      status: 'ok';
      topology: CanvasTopologyResponse;
      etag?: string;
    }
  | {
      status: 'not-modified';
      etag?: string;
    };

const canvasBootstrapReuseWindowMs = 2000;

type CanvasBootstrapCacheKey = string;

const defaultCanvasBootstrapCacheKey = '__default__';

let canvasBootstrapRequests = new Map<
  CanvasBootstrapCacheKey,
  Promise<{ topology: CanvasTopologyResponse }>
>();
let recentCanvasBootstraps = new Map<
  CanvasBootstrapCacheKey,
  { value: { topology: CanvasTopologyResponse }; expiresAt: number }
>();

type FetchCanvasBootstrapOptions = {
  force?: boolean;
};

export function resetCanvasBootstrapRequestCache(): void {
  canvasBootstrapRequests.clear();
  recentCanvasBootstraps.clear();
}

export async function fetchCanvasBootstrap(
  options: FetchCanvasBootstrapOptions = {},
): Promise<{ topology: CanvasTopologyResponse }> {
  return fetchCanvasBootstrapWithCache(
    `default:${defaultCanvasBootstrapCacheKey}`,
    '/api/v1/canvas',
    options,
  );
}

function fetchCanvasBootstrapWithCache(
  cacheKey: CanvasBootstrapCacheKey,
  path: string,
  options: FetchCanvasBootstrapOptions = {},
): Promise<{ topology: CanvasTopologyResponse }> {
  const recentBootstrap = recentCanvasBootstraps.get(cacheKey);
  if (options.force !== true && recentBootstrap && Date.now() < recentBootstrap.expiresAt) {
    return Promise.resolve(recentBootstrap.value);
  }

  const pendingRequest = canvasBootstrapRequests.get(cacheKey);
  if (options.force !== true && pendingRequest) {
    return pendingRequest;
  }

  const request = fetchCanvasBootstrapUncached(path)
    .then((result) => {
      recentCanvasBootstraps.set(cacheKey, {
        value: result,
        expiresAt: Date.now() + canvasBootstrapReuseWindowMs,
      });
      return result;
    })
    .finally(() => {
      if (canvasBootstrapRequests.get(cacheKey) === request) {
        canvasBootstrapRequests.delete(cacheKey);
      }
    });
  canvasBootstrapRequests.set(cacheKey, request);
  return request;
}

async function fetchCanvasBootstrapUncached(
  path: string,
): Promise<{ topology: CanvasTopologyResponse }> {
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
    throw new CanvasTopologyFetchError(
      response.status,
      `${path} failed: ${response.status} ${errorMessage}`,
    );
  }

  return {
    topology: parseCanvasTopologyResponse(payload),
  };
}

export async function fetchCanvasTopology(
  ifNoneMatch?: string,
): Promise<CanvasTopologyFetchResult> {
  return fetchCanvasTopologyFromPath('/api/v1/topology/canvas', ifNoneMatch);
}

async function fetchCanvasTopologyFromPath(
  path: string,
  ifNoneMatch?: string,
): Promise<CanvasTopologyFetchResult> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
  };
  if (ifNoneMatch) {
    headers['If-None-Match'] = ifNoneMatch;
  }

  const response = await fetch(path, { headers });
  const etag = response.headers.get('ETag') ?? undefined;

  if (response.status === 304) {
    return {
      status: 'not-modified',
      etag,
    };
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
    throw new CanvasTopologyFetchError(
      response.status,
      `${path} failed: ${response.status} ${errorMessage}`,
    );
  }

  return {
    status: 'ok',
    topology: parseCanvasTopologyResponse(payload),
    etag,
  };
}

export async function fetchCanvasMaps(): Promise<CanvasMap[]> {
  return parseCanvasMapsResponse(await requestJSON('/api/v1/canvas/maps'));
}

export async function createCanvasMap(payload: {
  name: string;
  description?: string;
  source_area_id?: string | null;
  filter?: CanvasMapFilter;
}): Promise<CanvasMap> {
  return parseCanvasMapResponse(await requestJSONWithBody('/api/v1/canvas/maps', 'POST', payload));
}

export async function updateCanvasMap(
  id: string,
  payload: Partial<{
    name: string;
    description: string;
    source_area_id: string | null;
    filter: CanvasMapFilter;
  }>,
): Promise<CanvasMap> {
  return parseCanvasMapResponse(
    await requestJSONWithBody(`/api/v1/canvas/maps/${encodeURIComponent(id)}`, 'PATCH', payload),
  );
}

export async function deleteCanvasMap(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/canvas/maps/${encodeURIComponent(id)}`, 'DELETE');
}

export async function duplicateCanvasMap(
  id: string,
  payload: { name: string },
): Promise<CanvasMap> {
  return parseCanvasMapResponse(
    await requestJSONWithBody(
      `/api/v1/canvas/maps/${encodeURIComponent(id)}/duplicate`,
      'POST',
      payload,
    ),
  );
}

export async function fetchCanvasMapBootstrap(
  mapId: string,
  options: FetchCanvasBootstrapOptions = {},
): Promise<{ topology: CanvasTopologyResponse }> {
  return fetchCanvasBootstrapWithCache(
    `map:${mapId}`,
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/bootstrap`,
    options,
  );
}

export async function fetchCanvasMapTopology(
  mapId: string,
  ifNoneMatch?: string,
): Promise<CanvasTopologyFetchResult> {
  return fetchCanvasTopologyFromPath(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/topology`,
    ifNoneMatch,
  );
}

export interface HealthVersion {
  version: string;
  git_commit: string;
  build_date: string;
}

export async function fetchHealthVersion(): Promise<HealthVersion> {
  try {
    const payload = await requestJSON('/api/v1/health');
    const p = payload as Record<string, unknown>;
    const v = p.version as Record<string, unknown> | undefined;
    return {
      version: typeof v?.version === 'string' ? v.version : 'unknown',
      git_commit: typeof v?.git_commit === 'string' ? v.git_commit : 'unknown',
      build_date: typeof v?.build_date === 'string' ? v.build_date : 'unknown',
    };
  } catch {
    return { version: 'unknown', git_commit: 'unknown', build_date: 'unknown' };
  }
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

async function requestJSONWithBody(path: string, method: string, body?: unknown): Promise<unknown> {
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

    if (response.status === 400 || response.status === 409) {
      throw new ValidationError(errorMessage);
    }

    if (response.status === 500) {
      // Extract correlation ID from backend error message (e.g. "internal error, ref: abc123")
      const refMatch = /ref:\s*([a-zA-Z0-9-]+)/.exec(errorMessage);
      const correlationId = refMatch ? refMatch[1] : undefined;
      const userMessage = correlationId
        ? `Something went wrong (ref: ${correlationId})`
        : 'Something went wrong';
      throw new ServerError(userMessage, correlationId);
    }

    throw new Error(`${path} failed: ${response.status} ${errorMessage}`);
  }

  return payload;
}

export interface SettingSecretState {
  present: boolean;
  redacted: boolean;
}

export interface SettingsWithMetadata {
  data: Record<string, string>;
  secrets: Record<string, SettingSecretState>;
}

function parseSettingsPayload(payload: unknown): SettingsWithMetadata {
  const result: SettingsWithMetadata = { data: {}, secrets: {} };
  if (typeof payload !== 'object' || payload === null) {
    return result;
  }

  const record = payload as Record<string, unknown>;
  if (typeof record.data === 'object' && record.data !== null) {
    result.data = Object.fromEntries(
      Object.entries(record.data as Record<string, unknown>).map(([key, value]) => [
        key,
        typeof value === 'string' ? value : String(value ?? ''),
      ]),
    );
  }

  const meta = record.meta;
  if (typeof meta === 'object' && meta !== null) {
    const secrets = (meta as Record<string, unknown>).secrets;
    if (typeof secrets === 'object' && secrets !== null) {
      result.secrets = Object.fromEntries(
        Object.entries(secrets as Record<string, unknown>).flatMap(([key, value]) => {
          if (typeof value !== 'object' || value === null) {
            return [];
          }
          const secret = value as Record<string, unknown>;
          return [
            [
              key,
              {
                present: secret.present === true,
                redacted: secret.redacted === true,
              },
            ],
          ];
        }),
      );
    }
  }

  return result;
}

export async function fetchSettingsWithMetadata(): Promise<SettingsWithMetadata> {
  try {
    const payload = await requestJSON('/api/v1/settings');
    return parseSettingsPayload(payload);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch settings: ${message}`);
  }
}

export async function fetchSettings(): Promise<Record<string, string>> {
  const settings = await fetchSettingsWithMetadata();
  return settings.data;
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
  ip?: string;
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
  payload: Partial<{
    hostname: string;
    ip: string;
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

export async function deleteDevice(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/devices/${encodeURIComponent(id)}`, 'DELETE');
}

export async function runTopologyDiscovery(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/devices/${encodeURIComponent(id)}/topology-discovery`, 'POST');
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

export async function deleteLink(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/links/${encodeURIComponent(id)}`, 'DELETE');
}

export interface SNMPProfilePayload {
  name: string;
  description?: string;
  snmp: SNMPPayload;
}

export interface PrometheusHealthResult {
  enabled?: boolean;
  available: boolean;
  url: string;
  error?: string;
}

export async function checkPrometheusHealth(): Promise<PrometheusHealthResult> {
  try {
    const payload = await requestJSON('/api/v1/prometheus/health');
    if (typeof payload === 'object' && payload !== null) {
      const p = payload as Record<string, unknown>;
      return {
        enabled: typeof p.enabled === 'boolean' ? p.enabled : undefined,
        available: p.available === true,
        url: typeof p.url === 'string' ? p.url : '',
        error: typeof p.error === 'string' ? p.error : undefined,
      };
    }
    return { available: false, url: '', error: 'invalid response' };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    return { available: false, url: '', error: message };
  }
}

export async function fetchSNMPProfiles(): Promise<SNMPProfile[]> {
  return parseSNMPProfilesResponse(await requestJSON('/api/v1/snmp-profiles'));
}

export async function createSNMPProfile(payload: SNMPProfilePayload): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody('/api/v1/snmp-profiles', 'POST', payload),
  );
}

export async function updateSNMPProfile(
  id: string,
  payload: SNMPProfilePayload,
): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}`, 'PUT', payload),
  );
}

export async function deleteSNMPProfile(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}`, 'DELETE');
}

// --- SNMP Test ---

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

// --- Credential Profiles ---

export async function fetchCredentialProfiles(): Promise<CredentialProfile[]> {
  return parseCredentialProfilesResponse(await requestJSON('/api/v1/credential-profiles'));
}

export interface CredentialProfilePayload {
  name: string;
  description?: string;
  username: string;
  port: number;
  auth_method: string;
  secret: string;
  role: string;
}

export async function createCredentialProfile(
  payload: CredentialProfilePayload,
): Promise<CredentialProfile> {
  return parseCredentialProfileResponse(
    await requestJSONWithBody('/api/v1/credential-profiles', 'POST', payload),
  );
}

export async function updateCredentialProfile(
  id: string,
  payload: CredentialProfilePayload,
): Promise<CredentialProfile> {
  return parseCredentialProfileResponse(
    await requestJSONWithBody(
      `/api/v1/credential-profiles/${encodeURIComponent(id)}`,
      'PUT',
      payload,
    ),
  );
}

export async function deleteCredentialProfile(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/credential-profiles/${encodeURIComponent(id)}`, 'DELETE');
}

// --- Device Credential Profile Assignments ---

export async function fetchDeviceCredentialProfiles(
  deviceId: string,
): Promise<DeviceCredentialProfile[]> {
  const payload = await requestJSON(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/credential-profiles`,
  );
  return parseDeviceCredentialProfilesResponse(payload);
}

export async function assignCredentialProfile(deviceId: string, profileId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/credential-profiles`,
    'POST',
    { profile_id: profileId },
  );
}

export async function unassignCredentialProfile(
  deviceId: string,
  profileId: string,
): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/credential-profiles/${encodeURIComponent(profileId)}`,
    'DELETE',
  );
}

export async function setWinBoxProfile(deviceId: string, profileId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/winbox-profile`,
    'PUT',
    { profile_id: profileId },
  );
}

export async function clearWinBoxProfile(deviceId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/winbox-profile`,
    'DELETE',
  );
}

export async function fetchWinBoxCredentials(deviceId: string): Promise<WinBoxCredentials> {
  const payload = await requestJSON(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/winbox-credentials`,
  );
  return parseWinBoxCredentialsResponse(payload);
}

export async function revealSNMPProfile(id: string, reason: string): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}/reveal`, 'POST', {
      reason,
    }),
  );
}

// fetchBridgeToken requests an AES-GCM encrypted credential token from the backend.
// The backend reads the stored bridge secret server-side, so the browser never receives or replays
// that shared secret. The plaintext credentials only appear inside the encrypted local bridge token.
export async function fetchBridgeToken(deviceId: string): Promise<string> {
  const payload = await requestJSONWithBody(
    `/api/v1/bridge/token/${encodeURIComponent(deviceId)}`,
    'POST',
  );
  const p = payload as Record<string, unknown>;
  if (typeof p?.token !== 'string' || p.token === '') {
    throw new Error('invalid bridge token response');
  }
  return p.token;
}

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

// --- Areas ---

export async function fetchAreas(): Promise<Area[]> {
  return parseAreasResponse(await requestJSON('/api/v1/areas'));
}

export async function createArea(payload: {
  name: string;
  description: string;
  color: string;
}): Promise<Area> {
  return parseAreaResponse(await requestJSONWithBody('/api/v1/areas', 'POST', payload));
}

export async function updateArea(
  id: string,
  payload: { name: string; description: string; color: string },
): Promise<Area> {
  return parseAreaResponse(
    await requestJSONWithBody(`/api/v1/areas/${encodeURIComponent(id)}`, 'PUT', payload),
  );
}

export async function deleteArea(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/areas/${encodeURIComponent(id)}`, 'DELETE');
}

// --- Backup Jobs ---

function parseBackupFile(data: Record<string, unknown>): BackupFile {
  return {
    id: typeof data.id === 'string' ? data.id : '',
    job_id: typeof data.job_id === 'string' ? data.job_id : '',
    file_type: typeof data.file_type === 'string' ? data.file_type : '',
    file_name: typeof data.file_name === 'string' ? data.file_name : '',
    file_hash: typeof data.file_hash === 'string' ? data.file_hash : '',
    size_bytes: typeof data.size_bytes === 'number' ? data.size_bytes : 0,
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
  };
}

function parseBackupJob(data: Record<string, unknown>): BackupJob {
  const status = data.status as string;
  const filesRaw = Array.isArray(data.files) ? data.files : [];
  return {
    id: typeof data.id === 'string' ? data.id : '',
    device_id: typeof data.device_id === 'string' ? data.device_id : '',
    status: (['pending', 'running', 'success', 'failed'].includes(status)
      ? status
      : 'pending') as BackupStatus,
    error_message: typeof data.error_message === 'string' ? data.error_message : '',
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
    files: filesRaw.map((f) => parseBackupFile(f as Record<string, unknown>)),
  };
}

export async function triggerBackup(deviceId: string): Promise<BackupJob> {
  const response = await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/backups`,
    'POST',
  );
  const data = (response as Record<string, unknown>)?.data as Record<string, unknown>;
  return parseBackupJob(data);
}

export async function fetchBackupJobs(deviceId: string): Promise<BackupJob[]> {
  const payload = await requestJSON(`/api/v1/devices/${encodeURIComponent(deviceId)}/backups`);
  const data = (payload as Record<string, unknown>)?.data;
  if (!Array.isArray(data)) return [];
  return data.map((item) => parseBackupJob(item as Record<string, unknown>));
}

export async function fetchBackupJob(jobId: string): Promise<BackupJob> {
  const payload = await requestJSON(`/api/v1/backup-jobs/${encodeURIComponent(jobId)}`);
  const data = (payload as Record<string, unknown>)?.data as Record<string, unknown>;
  return parseBackupJob(data);
}

export async function fetchLatestBackupJob(deviceId: string): Promise<BackupJob | null> {
  try {
    const payload = await requestJSON(
      `/api/v1/devices/${encodeURIComponent(deviceId)}/backups/latest`,
    );
    const data = (payload as Record<string, unknown>)?.data as Record<string, unknown>;
    return parseBackupJob(data);
  } catch {
    return null;
  }
}

export async function deleteBackupJob(jobId: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/backup-jobs/${encodeURIComponent(jobId)}`, 'DELETE');
}

export function backupFileDownloadUrl(fileId: string): string {
  return `/api/v1/backup-files/${encodeURIComponent(fileId)}/download`;
}

function parseBackupFileContent(data: Record<string, unknown>, fileId: string): BackupFileContent {
  const content = typeof data.content === 'string' ? data.content : '';
  const inline = data.inline !== false;
  const downloadURL =
    typeof data.download_url === 'string' && data.download_url
      ? data.download_url
      : backupFileDownloadUrl(fileId);
  const reason = typeof data.reason === 'string' ? data.reason : undefined;

  return {
    content,
    inline,
    download_url: downloadURL,
    ...(reason ? { reason } : {}),
    size_bytes: typeof data.size_bytes === 'number' ? data.size_bytes : 0,
    max_inline_size_bytes:
      typeof data.max_inline_size_bytes === 'number' ? data.max_inline_size_bytes : 0,
  };
}

export async function fetchBackupFileContent(fileId: string): Promise<BackupFileContent> {
  const payload = await requestJSON(`/api/v1/backup-files/${encodeURIComponent(fileId)}/content`);
  const payloadRecord =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const data =
    typeof payloadRecord.data === 'object' && payloadRecord.data !== null
      ? (payloadRecord.data as Record<string, unknown>)
      : {};
  return parseBackupFileContent(data, fileId);
}

// --- Bulk Backup ---

export function bulkDownloadUrl(_deviceIds: string[]): string {
  // We use a form POST for the download, so return the endpoint URL
  return '/api/v1/backups/bulk-download';
}

export async function triggerBulkDownload(deviceIds: string[]): Promise<void> {
  const response = await fetch('/api/v1/backups/bulk-download', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ device_ids: deviceIds }),
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as Record<string, unknown> | null;
    const errorMessage =
      payload && typeof payload.error === 'string' ? payload.error : response.statusText;
    throw new Error(errorMessage);
  }
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const match = disposition.match(/filename="(.+?)"/);
  const filename =
    match?.[1] ??
    `${new Date().toISOString().replace(/[-:T]/g, '').slice(0, 15)}_THEIA_BACKUPS.zip`;

  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

// --- Vendor Configs ---

export async function fetchVendorConfigs(): Promise<VendorConfig[]> {
  const payload = await requestJSON('/api/v1/vendors');
  const data = (payload as Record<string, unknown>)?.data;
  if (!Array.isArray(data)) return [];
  return data.map((item) => {
    const rec = item as Record<string, unknown>;
    return {
      name: typeof rec.name === 'string' ? rec.name : '',
      display_name: typeof rec.display_name === 'string' ? rec.display_name : '',
      config: rec.config as VendorConfig['config'],
    };
  });
}

export async function fetchVendorConfig(name: string): Promise<VendorConfig> {
  const payload = await requestJSON(`/api/v1/vendors/${encodeURIComponent(name)}`);
  const rec = (payload as Record<string, unknown>)?.data as Record<string, unknown>;
  return {
    name: typeof rec.name === 'string' ? rec.name : '',
    display_name: typeof rec.display_name === 'string' ? rec.display_name : '',
    config: rec.config as VendorConfig['config'],
  };
}

export async function updateVendorConfig(
  name: string,
  config: VendorConfig['config'],
): Promise<VendorConfig> {
  const response = await requestJSONWithBody(
    `/api/v1/vendors/${encodeURIComponent(name)}`,
    'PUT',
    config,
  );
  const rec = (response as Record<string, unknown>)?.data as Record<string, unknown>;
  return {
    name: typeof rec.name === 'string' ? rec.name : '',
    display_name: typeof rec.display_name === 'string' ? rec.display_name : '',
    config: rec.config as VendorConfig['config'],
  };
}

// --- Instance Backups ---

function parseInstanceBackup(data: Record<string, unknown>): InstanceBackup {
  const status = typeof data.status === 'string' ? data.status : 'running';
  const trigger = typeof data.trigger === 'string' ? data.trigger : 'manual';
  const progress = parseInstanceBackupProgress(data.progress);
  return {
    id: typeof data.id === 'string' ? data.id : '',
    file_name: typeof data.file_name === 'string' ? data.file_name : '',
    size_bytes: typeof data.size_bytes === 'number' ? data.size_bytes : 0,
    sha256: typeof data.sha256 === 'string' ? data.sha256 : '',
    app_version: typeof data.app_version === 'string' ? data.app_version : '',
    migration_version: typeof data.migration_version === 'number' ? data.migration_version : 0,
    status: (['running', 'success', 'failed', 'cancelled'].includes(status)
      ? status
      : 'running') as InstanceBackupStatus,
    error_message: typeof data.error_message === 'string' ? data.error_message : '',
    ...(progress ? { progress } : {}),
    trigger: (trigger === 'scheduled' ? 'scheduled' : 'manual') as 'manual' | 'scheduled',
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
  };
}

function parseInstanceBackupProgress(value: unknown): InstanceBackupProgress | undefined {
  if (!value || typeof value !== 'object') return undefined;
  const record = value as Record<string, unknown>;
  return {
    phase: typeof record.phase === 'string' ? record.phase : '',
    message: typeof record.message === 'string' ? record.message : '',
    current: typeof record.current === 'number' ? record.current : 0,
    total: typeof record.total === 'number' ? record.total : 0,
  };
}

export async function createInstanceBackup(): Promise<InstanceBackup> {
  const response = await requestJSONWithBody('/api/v1/instance-backups', 'POST');
  const data = (response as Record<string, unknown>)?.data as Record<string, unknown>;
  return parseInstanceBackup(data);
}

export async function fetchInstanceBackups(): Promise<InstanceBackup[]> {
  const payload = await requestJSON('/api/v1/instance-backups');
  const data = (payload as Record<string, unknown>)?.data;
  if (!Array.isArray(data)) return [];
  return data.map((item) => parseInstanceBackup(item as Record<string, unknown>));
}

export async function deleteInstanceBackup(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/instance-backups/${encodeURIComponent(id)}`, 'DELETE');
}

export async function cancelInstanceBackup(id: string): Promise<InstanceBackup> {
  const response = await requestJSONWithBody(
    `/api/v1/instance-backups/${encodeURIComponent(id)}/cancel`,
    'POST',
  );
  const data = (response as Record<string, unknown>)?.data as Record<string, unknown>;
  return parseInstanceBackup(data);
}

export function instanceBackupDownloadUrl(id: string): string {
  return `/api/v1/instance-backups/${encodeURIComponent(id)}/download`;
}

export async function restoreInstanceBackup(file: File, dryRun: boolean): Promise<RestoreReport> {
  const formData = new FormData();
  formData.append('file', file);

  const url = dryRun
    ? '/api/v1/instance-backups/restore?dry_run=true'
    : '/api/v1/instance-backups/restore';

  const response = await fetch(url, {
    method: 'POST',
    body: formData,
    // Do NOT set Content-Type — browser sets multipart boundary automatically
  });

  if (!response.ok) {
    const payload = await response.json().catch(() => null);
    const errorMessage =
      typeof payload === 'object' &&
      payload !== null &&
      'error' in payload &&
      typeof (payload as Record<string, unknown>).error === 'string'
        ? ((payload as Record<string, unknown>).error as string)
        : response.statusText;

    if (response.status === 400) {
      throw new ValidationError(errorMessage);
    }

    if (response.status === 500) {
      const refMatch = /ref:\s*([a-zA-Z0-9-]+)/.exec(errorMessage);
      const correlationId = refMatch ? refMatch[1] : undefined;
      const userMessage = correlationId
        ? `Something went wrong (ref: ${correlationId})`
        : 'Something went wrong';
      throw new ServerError(userMessage, correlationId);
    }

    throw new Error(`${url} failed: ${response.status} ${errorMessage}`);
  }

  const payload = (await response.json()) as Record<string, unknown>;
  const data = payload.data as Record<string, unknown>;
  return parseRestoreReport(data);
}

function parseRestoreReport(data: Record<string, unknown>): RestoreReport {
  return {
    valid: typeof data.valid === 'boolean' ? data.valid : false,
    app_version: typeof data.app_version === 'string' ? data.app_version : '',
    git_commit: typeof data.git_commit === 'string' ? data.git_commit : '',
    migration_version: typeof data.migration_version === 'number' ? data.migration_version : 0,
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
    db_size_bytes: typeof data.db_size_bytes === 'number' ? data.db_size_bytes : 0,
    backup_file_count: typeof data.backup_file_count === 'number' ? data.backup_file_count : 0,
    total_size_bytes: typeof data.total_size_bytes === 'number' ? data.total_size_bytes : 0,
    needs_migration: typeof data.needs_migration === 'boolean' ? data.needs_migration : false,
    current_migration_version:
      typeof data.current_migration_version === 'number' ? data.current_migration_version : 0,
    message: typeof data.message === 'string' ? data.message : '',
  };
}
