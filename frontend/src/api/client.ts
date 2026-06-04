import {
  type Area,
  type BackupFile,
  type BackupFileContent,
  type BackupJob,
  type BackupStatus,
  type BulkBackupRun,
  type BulkBackupRunItem,
  type BulkBackupRunItemStatus,
  type BulkBackupRunStatus,
  type BulkOperationStatus,
  type CredentialProfile,
  type Device,
  type DeviceCredentialProfile,
  type GrafanaDashboardConfig,
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
  parseCredentialProfileResponse,
  parseCredentialProfilesResponse,
  parseDeviceCredentialProfilesResponse,
  parseDevicesResponse,
  parseGrafanaDashboardConfigResponse,
  parseInterfacesResponse,
  parseLinksResponse,
  parseSNMPProfileResponse,
  parseSNMPProfilesResponse,
  parseWinBoxCredentialsResponse,
} from '../types/api';
import { ServerError, ValidationError } from './errors';
import { recordField, stringField } from './parsers';
import { type ErrorPayload, headersWithCsrf, requestJSON, requestJSONWithBody } from './transport';

export { ValidationError, ServerError };
export * from './admin';
export * from './auth';
export * from './canvas';
export * from './settings';
export { headersWithCsrf } from './transport';

export interface BridgeLaunchRequestResponse {
  launch_token: string;
  expires_at?: string;
}

export async function fetchDevices(): Promise<Device[]> {
  try {
    return parseDevicesResponse(await requestJSON('/api/v1/devices'));
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch devices: ${message}`);
  }
}

export async function fetchOrphanDevices(): Promise<Device[]> {
  try {
    return parseDevicesResponse(await requestJSON('/api/v1/devices/orphans'));
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch orphan devices: ${message}`);
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
  skip_primary_map_membership?: boolean;
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

export interface GrafanaDashboardProfilePayload {
  name: string;
  url_template: string;
  variable_source: string;
  is_default?: boolean;
}

export interface GrafanaDeviceOverridePayload {
  profile_id: string | null;
  custom_url: string;
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

export async function fetchGrafanaDashboardConfig(): Promise<GrafanaDashboardConfig> {
  return parseGrafanaDashboardConfigResponse(
    await requestJSON('/api/v1/grafana/dashboard-profiles'),
  );
}

export async function createGrafanaDashboardProfile(
  payload: GrafanaDashboardProfilePayload,
): Promise<GrafanaDashboardConfig> {
  return parseGrafanaDashboardConfigResponse(
    await requestJSONWithBody('/api/v1/grafana/dashboard-profiles', 'POST', payload),
  );
}

export async function updateGrafanaDashboardProfile(
  id: string,
  payload: GrafanaDashboardProfilePayload,
): Promise<GrafanaDashboardConfig> {
  return parseGrafanaDashboardConfigResponse(
    await requestJSONWithBody(
      `/api/v1/grafana/dashboard-profiles/${encodeURIComponent(id)}`,
      'PUT',
      payload,
    ),
  );
}

export async function deleteGrafanaDashboardProfile(id: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/grafana/dashboard-profiles/${encodeURIComponent(id)}`,
    'DELETE',
  );
}

export async function saveDeviceGrafanaDashboardOverride(
  deviceId: string,
  payload: GrafanaDeviceOverridePayload,
): Promise<GrafanaDashboardConfig> {
  return parseGrafanaDashboardConfigResponse(
    await requestJSONWithBody(
      `/api/v1/grafana/device-overrides/${encodeURIComponent(deviceId)}`,
      'PUT',
      payload,
    ),
  );
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

export type BulkBackupResult = {
  device_id: string;
  device_name: string;
  status: 'queued' | 'skipped';
  reason?: string;
  job_id?: string;
};

function parseBulkBackupResult(data: Record<string, unknown>): BulkBackupResult {
  const status = data.status === 'queued' ? 'queued' : 'skipped';
  return {
    device_id: typeof data.device_id === 'string' ? data.device_id : '',
    device_name: typeof data.device_name === 'string' ? data.device_name : '',
    status,
    reason: typeof data.reason === 'string' ? data.reason : undefined,
    job_id: typeof data.job_id === 'string' ? data.job_id : undefined,
  };
}

const bulkBackupRunStatuses: BulkBackupRunStatus[] = [
  'running',
  'pausing',
  'paused',
  'cancelling',
  'success',
  'partial',
  'failed',
  'cancelled',
];

const bulkBackupRunItemStatuses: BulkBackupRunItemStatus[] = [
  'checking',
  'skipped',
  'active',
  'queued',
  'running',
  'success',
  'failed',
  'cancelled',
];

function parseBulkBackupRunItem(data: Record<string, unknown>): BulkBackupRunItem {
  const status = typeof data.status === 'string' ? data.status : '';
  return {
    id: typeof data.id === 'string' ? data.id : '',
    run_id: typeof data.run_id === 'string' ? data.run_id : '',
    device_id: typeof data.device_id === 'string' ? data.device_id : '',
    device_name: typeof data.device_name === 'string' ? data.device_name : '',
    status: bulkBackupRunItemStatuses.includes(status as BulkBackupRunItemStatus)
      ? (status as BulkBackupRunItemStatus)
      : 'checking',
    reason: typeof data.reason === 'string' ? data.reason : undefined,
    backup_job_id: typeof data.backup_job_id === 'string' ? data.backup_job_id : undefined,
    file_count: typeof data.file_count === 'number' ? data.file_count : 0,
    byte_count: typeof data.byte_count === 'number' ? data.byte_count : 0,
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
    updated_at: typeof data.updated_at === 'string' ? data.updated_at : '',
    completed_at: typeof data.completed_at === 'string' ? data.completed_at : undefined,
  };
}

function parseBulkBackupRun(data: Record<string, unknown>): BulkBackupRun {
  const status = typeof data.status === 'string' ? data.status : '';
  const items = Array.isArray(data.items) ? data.items : [];
  return {
    id: typeof data.id === 'string' ? data.id : '',
    status: bulkBackupRunStatuses.includes(status as BulkBackupRunStatus)
      ? (status as BulkBackupRunStatus)
      : 'running',
    batch_size: typeof data.batch_size === 'number' ? data.batch_size : 0,
    total_count: typeof data.total_count === 'number' ? data.total_count : 0,
    queued_count: typeof data.queued_count === 'number' ? data.queued_count : 0,
    running_count: typeof data.running_count === 'number' ? data.running_count : 0,
    completed_count: typeof data.completed_count === 'number' ? data.completed_count : 0,
    success_count: typeof data.success_count === 'number' ? data.success_count : 0,
    failed_count: typeof data.failed_count === 'number' ? data.failed_count : 0,
    skipped_count: typeof data.skipped_count === 'number' ? data.skipped_count : 0,
    cancelled_count: typeof data.cancelled_count === 'number' ? data.cancelled_count : 0,
    file_count: typeof data.file_count === 'number' ? data.file_count : 0,
    byte_count: typeof data.byte_count === 'number' ? data.byte_count : 0,
    current_device_id:
      typeof data.current_device_id === 'string' ? data.current_device_id : undefined,
    current_device_name:
      typeof data.current_device_name === 'string' ? data.current_device_name : undefined,
    current_job_id: typeof data.current_job_id === 'string' ? data.current_job_id : undefined,
    error_message: typeof data.error_message === 'string' ? data.error_message : '',
    cancel_requested: data.cancel_requested === true,
    created_by: typeof data.created_by === 'string' ? data.created_by : '',
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
    started_at: typeof data.started_at === 'string' ? data.started_at : undefined,
    completed_at: typeof data.completed_at === 'string' ? data.completed_at : undefined,
    items: items.map((item) => parseBulkBackupRunItem(item as Record<string, unknown>)),
  };
}

function parseBulkBackupRunResponse(payload: unknown): BulkBackupRun | null {
  const data = (payload as Record<string, unknown>)?.data;
  if (data === null || typeof data !== 'object') return null;
  return parseBulkBackupRun(data as Record<string, unknown>);
}

function numericField(record: Record<string, unknown> | undefined, key: string): number {
  return record && typeof record[key] === 'number' ? record[key] : 0;
}

function booleanField(record: Record<string, unknown> | undefined, key: string): boolean {
  return record?.[key] === true;
}

function parseBulkOperationStatus(payload: unknown): BulkOperationStatus {
  const payloadRecord = recordField(payload) ?? {};
  const data = recordField(payloadRecord.data) ?? {};
  const bulkBackup = recordField(data.bulk_backup) ?? {};
  const bulkBackupConcurrency = recordField(bulkBackup.concurrency) ?? {};
  const bulkBackupLegacyEndpoint = recordField(bulkBackup.legacy_endpoint) ?? {};
  const bulkBackupRun = recordField(data.bulk_backup_run) ?? {};
  const bulkDownload = recordField(data.bulk_download) ?? {};

  return {
    bulk_backup: {
      max_devices: numericField(bulkBackup, 'max_devices'),
      max_queued_jobs: numericField(bulkBackup, 'max_queued_jobs'),
      concurrency: {
        max_concurrent: numericField(bulkBackupConcurrency, 'max_concurrent'),
        configurable: booleanField(bulkBackupConcurrency, 'configurable'),
        distributed: booleanField(bulkBackupConcurrency, 'distributed'),
        distributed_max_concurrent: numericField(
          bulkBackupConcurrency,
          'distributed_max_concurrent',
        ),
      },
      legacy_endpoint: {
        path: stringField(bulkBackupLegacyEndpoint, 'path'),
        deprecated: booleanField(bulkBackupLegacyEndpoint, 'deprecated'),
      },
    },
    bulk_backup_run: {
      max_devices: numericField(bulkBackupRun, 'max_devices'),
      max_queued_jobs: numericField(bulkBackupRun, 'max_queued_jobs'),
      batch_size: numericField(bulkBackupRun, 'batch_size'),
      max_active_runs: numericField(bulkBackupRun, 'max_active_runs'),
      configurable_concurrency: booleanField(bulkBackupRun, 'configurable_concurrency'),
      distributed: booleanField(bulkBackupRun, 'distributed'),
      distributed_max_active_runs: numericField(bulkBackupRun, 'distributed_max_active_runs'),
      can_pause: booleanField(bulkBackupRun, 'can_pause'),
      can_resume: booleanField(bulkBackupRun, 'can_resume'),
      can_cancel: booleanField(bulkBackupRun, 'can_cancel'),
    },
    bulk_download: {
      max_devices: numericField(bulkDownload, 'max_devices'),
      max_files: numericField(bulkDownload, 'max_files'),
      max_bytes: numericField(bulkDownload, 'max_bytes'),
      max_concurrent_per_actor: numericField(bulkDownload, 'max_concurrent_per_actor'),
      max_concurrent_global: numericField(bulkDownload, 'max_concurrent_global'),
      distributed: booleanField(bulkDownload, 'distributed'),
      distributed_max_concurrent_per_actor: numericField(
        bulkDownload,
        'distributed_max_concurrent_per_actor',
      ),
      distributed_max_concurrent_global: numericField(
        bulkDownload,
        'distributed_max_concurrent_global',
      ),
    },
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

export async function triggerBulkBackup(deviceIds: string[]): Promise<BulkBackupResult[]> {
  const payload = await requestBulkJSON(
    '/api/v1/backups/bulk',
    { device_ids: deviceIds },
    'bulk backup',
  );
  const data = (payload as Record<string, unknown>)?.data;
  if (!Array.isArray(data)) return [];
  return data.map((item) => parseBulkBackupResult(item as Record<string, unknown>));
}

export async function startBulkBackupRun(deviceIds: string[]): Promise<BulkBackupRun> {
  const payload = await requestBulkJSON(
    '/api/v1/backups/bulk-runs',
    { device_ids: deviceIds },
    'bulk backup',
    { returnConflictPayload: true },
  );
  const run = parseBulkBackupRunResponse(payload);
  if (!run) throw new Error('bulk backup run response is missing');
  return run;
}

export async function fetchBulkOperationStatus(): Promise<BulkOperationStatus> {
  const payload = await requestJSON('/api/v1/backups/bulk/status');
  return parseBulkOperationStatus(payload);
}

export async function fetchLatestBulkBackupRun(): Promise<BulkBackupRun | null> {
  const payload = await requestJSON('/api/v1/backups/bulk-runs/latest');
  return parseBulkBackupRunResponse(payload);
}

export async function fetchBulkBackupRun(runId: string): Promise<BulkBackupRun> {
  const payload = await requestJSON(`/api/v1/backups/bulk-runs/${encodeURIComponent(runId)}`);
  const run = parseBulkBackupRunResponse(payload);
  if (!run) throw new Error('bulk backup run not found');
  return run;
}

export async function cancelBulkBackupRun(runId: string): Promise<BulkBackupRun> {
  const payload = await requestBulkJSON(
    `/api/v1/backups/bulk-runs/${encodeURIComponent(runId)}/cancel`,
    {},
    'bulk backup cancel',
  );
  const run = parseBulkBackupRunResponse(payload);
  if (!run) throw new Error('bulk backup run response is missing');
  return run;
}

export async function pauseBulkBackupRun(runId: string): Promise<BulkBackupRun> {
  const payload = await requestBulkJSON(
    `/api/v1/backups/bulk-runs/${encodeURIComponent(runId)}/pause`,
    {},
    'bulk backup pause',
  );
  const run = parseBulkBackupRunResponse(payload);
  if (!run) throw new Error('bulk backup run response is missing');
  return run;
}

export async function resumeBulkBackupRun(runId: string): Promise<BulkBackupRun> {
  const payload = await requestBulkJSON(
    `/api/v1/backups/bulk-runs/${encodeURIComponent(runId)}/resume`,
    {},
    'bulk backup resume',
  );
  const run = parseBulkBackupRunResponse(payload);
  if (!run) throw new Error('bulk backup run response is missing');
  return run;
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

export type BulkDownloadResult = 'saved' | 'cancelled';

export type BulkDownloadOptions = {
  filename?: string;
};

export async function triggerBulkDownload(
  deviceIds: string[],
  options: BulkDownloadOptions = {},
): Promise<BulkDownloadResult> {
  const suggestedFilename = options.filename ?? defaultBulkDownloadFilename();
  const response = await fetch('/api/v1/backups/bulk-download', {
    method: 'POST',
    headers: headersWithCsrf({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ device_ids: deviceIds }),
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as Record<string, unknown> | null;
    const errorMessage =
      payload && typeof payload.error === 'string' ? payload.error : response.statusText;
    if (response.status === 413) {
      throw new ValidationError(formatBulkLimitMessage(errorMessage));
    }
    throw new Error(errorMessage);
  }
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const match = disposition.match(/filename="(.+?)"/);
  const filename = options.filename ?? match?.[1] ?? suggestedFilename;
  const saveTarget = prepareStreamingSaveTarget(filename);

  return saveDownloadResponse(response, filename, saveTarget);
}

async function requestBulkJSON(
  path: string,
  body: unknown,
  operation: string,
  options: { returnConflictPayload?: boolean } = {},
): Promise<unknown> {
  const response = await fetch(path, {
    method: 'POST',
    headers: headersWithCsrf({
      Accept: 'application/json',
      'Content-Type': 'application/json',
    }),
    body: JSON.stringify(body),
  });
  const payload = (await response.json().catch(() => null)) as ErrorPayload | unknown;
  if (!response.ok) {
    if (response.status === 409 && options.returnConflictPayload) {
      return payload;
    }
    const errorMessage =
      typeof payload === 'object' &&
      payload !== null &&
      'error' in payload &&
      typeof payload.error === 'string'
        ? payload.error
        : response.statusText;
    if (response.status === 413) {
      throw new ValidationError(formatBulkLimitMessage(errorMessage));
    }
    if (response.status === 400 || response.status === 409) {
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
    throw new Error(`${operation} failed: ${response.status} ${errorMessage}`);
  }
  return payload;
}

function formatBulkLimitMessage(message: string): string {
  const match =
    /^bulk (backup(?: run)?|download) exceeds (devices|queued jobs|files|bytes) limit: requested (\d+), maximum (\d+)$/i.exec(
      message,
    );
  if (!match) {
    return message;
  }
  const [, operation, limit, requested, maximum] = match;
  const normalizedOperation = operation.toLowerCase().replace(/ run$/, '');
  const normalizedLimit = limit.toLowerCase();
  if (normalizedOperation === 'backup' && normalizedLimit === 'devices') {
    return `Too many devices selected for bulk backup. Maximum ${maximum}, requested ${requested}.`;
  }
  if (normalizedOperation === 'backup' && normalizedLimit === 'queued jobs') {
    return `Too many backup jobs would be queued. Maximum ${maximum}, requested ${requested}.`;
  }
  if (normalizedOperation === 'download' && normalizedLimit === 'devices') {
    return `Too many devices selected for bulk download. Maximum ${maximum}, requested ${requested}.`;
  }
  if (normalizedOperation === 'download' && normalizedLimit === 'files') {
    return `Too many backup files selected for bulk download. Maximum ${maximum}, requested ${requested}.`;
  }
  if (normalizedOperation === 'download' && normalizedLimit === 'bytes') {
    return `Bulk download is too large. Maximum ${maximum} bytes, requested ${requested} bytes.`;
  }
  return message;
}

type SaveFilePicker = (options: {
  suggestedName?: string;
  types?: Array<{
    description: string;
    accept: Record<string, string[]>;
  }>;
}) => Promise<{
  createWritable: () => Promise<WritableStream<Uint8Array>>;
}>;

type StreamingSaveTarget = Promise<{
  createWritable: () => Promise<WritableStream<Uint8Array>>;
} | null> | null;

function browserSaveFilePicker(): SaveFilePicker | undefined {
  const candidate = (globalThis as { showSaveFilePicker?: unknown }).showSaveFilePicker;
  return typeof candidate === 'function' ? (candidate as SaveFilePicker) : undefined;
}

function defaultBulkDownloadFilename(): string {
  return `${new Date().toISOString().replace(/[-:T]/g, '').slice(0, 15)}_THEIA_BACKUPS.zip`;
}

function prepareStreamingSaveTarget(filename: string): StreamingSaveTarget {
  const saveFilePicker = browserSaveFilePicker();
  if (!saveFilePicker) {
    return null;
  }
  return saveFilePicker({
    suggestedName: filename,
    types: [
      {
        description: 'ZIP archive',
        accept: { 'application/zip': ['.zip'] },
      },
    ],
  }).catch((error) => {
    if (error instanceof DOMException && error.name === 'AbortError') {
      return null;
    }
    throw error;
  });
}

async function saveDownloadResponse(
  response: Response,
  filename: string,
  saveTarget: StreamingSaveTarget,
): Promise<BulkDownloadResult> {
  if (response.body && saveTarget) {
    try {
      const handle = await saveTarget;
      if (!handle) {
        await response.body.cancel();
        return 'cancelled';
      }
      const writable = await handle.createWritable().catch(async (error) => {
        await response.body?.cancel();
        throw error;
      });
      await response.body.pipeTo(writable);
      return 'saved';
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        return 'cancelled';
      }
      throw error;
    }
  }

  const blob = await response.blob();
  saveBlob(blob, filename);
  return 'saved';
}

function saveBlob(blob: Blob, filename: string): void {
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
    headers: headersWithCsrf({}),
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
