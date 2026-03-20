import {
  type Device,
  type InterfaceInfo,
  type Link,
  type SNMPProfile,
  type SSHProfile,
  type BackupJob,
  type BackupFile,
  type BackupStatus,
  type VendorConfig,
  parseDevicesResponse,
  parseInterfacesResponse,
  parseLinksResponse,
  parseSNMPProfilesResponse,
  parseSNMPProfileResponse,
  parseSSHProfilesResponse,
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
  vendor?: string;
  metrics_source?: string;
  prometheus_label_name?: string;
  prometheus_label_value?: string;
  ssh_profile_id?: string;
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
    snmp: SNMPPayload;
    tags: Record<string, string>;
    vendor: string;
    metrics_source: string;
    prometheus_label_name: string;
    prometheus_label_value: string;
    ssh_profile_id: string;
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

export interface SNMPProfilePayload {
  name: string;
  description?: string;
  snmp: SNMPPayload;
}

export interface PrometheusHealthResult {
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

export async function updateSNMPProfile(id: string, payload: SNMPProfilePayload): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}`, 'PUT', payload),
  );
}

export async function deleteSNMPProfile(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}`, 'DELETE');
}

// --- SNMP Test ---

export async function testSNMPConnection(deviceId: string): Promise<{ success: boolean; sys_name?: string; sys_descr?: string; error?: string; target_ip?: string; snmp_version?: string }> {
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

// --- SSH Profiles ---

export async function fetchSSHProfiles(): Promise<SSHProfile[]> {
  return parseSSHProfilesResponse(await requestJSON('/api/v1/ssh-profiles'));
}

export interface SSHProfilePayload {
  name: string;
  description?: string;
  username: string;
  port: number;
  auth_method: string;
  secret: string;
}

export async function createSSHProfile(payload: SSHProfilePayload): Promise<SSHProfile> {
  const response = await requestJSONWithBody('/api/v1/ssh-profiles', 'POST', payload);
  const data = (response as Record<string, unknown>)?.data as Record<string, unknown>;
  return {
    id: typeof data.id === 'string' ? data.id : '',
    name: typeof data.name === 'string' ? data.name : '',
    description: typeof data.description === 'string' ? data.description : '',
    username: typeof data.username === 'string' ? data.username : '',
    port: typeof data.port === 'number' ? data.port : 22,
    auth_method: data.auth_method === 'key' ? 'key' : 'password',
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
    updated_at: typeof data.updated_at === 'string' ? data.updated_at : '',
  };
}

export async function updateSSHProfile(id: string, payload: SSHProfilePayload): Promise<SSHProfile> {
  const response = await requestJSONWithBody(
    `/api/v1/ssh-profiles/${encodeURIComponent(id)}`,
    'PUT',
    payload,
  );
  const data = (response as Record<string, unknown>)?.data as Record<string, unknown>;
  return {
    id: typeof data.id === 'string' ? data.id : '',
    name: typeof data.name === 'string' ? data.name : '',
    description: typeof data.description === 'string' ? data.description : '',
    username: typeof data.username === 'string' ? data.username : '',
    port: typeof data.port === 'number' ? data.port : 22,
    auth_method: data.auth_method === 'key' ? 'key' : 'password',
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
    updated_at: typeof data.updated_at === 'string' ? data.updated_at : '',
  };
}

export async function deleteSSHProfile(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/ssh-profiles/${encodeURIComponent(id)}`, 'DELETE');
}

export async function testSSHProfile(id: string, targetIP: string): Promise<{ success: boolean; error?: string }> {
  const response = await requestJSONWithBody(
    `/api/v1/ssh-profiles/${encodeURIComponent(id)}/test`,
    'POST',
    { target_ip: targetIP },
  );
  const data = response as Record<string, unknown>;
  return {
    success: data.success === true,
    error: typeof data.error === 'string' ? data.error : undefined,
  };
}

export async function testSSHConnection(deviceId: string): Promise<{ success: boolean; error?: string }> {
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
    status: (['pending', 'running', 'success', 'failed'].includes(status) ? status : 'pending') as BackupStatus,
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

export async function fetchBackupFileContent(fileId: string): Promise<string> {
  const payload = await requestJSON(`/api/v1/backup-files/${encodeURIComponent(fileId)}/content`);
  const data = (payload as Record<string, unknown>)?.data as Record<string, unknown>;
  return typeof data?.content === 'string' ? data.content : '';
}

// --- Bulk Backup ---

export function bulkDownloadUrl(deviceIds: string[]): string {
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
    const payload = await response.json().catch(() => null) as Record<string, unknown> | null;
    const errorMessage = payload && typeof payload.error === 'string' ? payload.error : response.statusText;
    throw new Error(errorMessage);
  }
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const match = disposition.match(/filename="(.+?)"/);
  const filename = match?.[1] ?? `${new Date().toISOString().replace(/[-:T]/g, '').slice(0, 15)}_THEIA_BACKUPS.zip`;

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

export async function updateVendorConfig(name: string, config: VendorConfig['config']): Promise<VendorConfig> {
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
