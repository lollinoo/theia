import {
  type Area,
  type CredentialProfile,
  type GrafanaDashboardConfig,
  type InstanceBackup,
  type InstanceBackupProgress,
  type InstanceBackupStatus,
  type RestoreReport,
  type SNMPProfile,
  type VendorConfig,
  parseAreaResponse,
  parseAreasResponse,
  parseCredentialProfileResponse,
  parseCredentialProfilesResponse,
  parseGrafanaDashboardConfigResponse,
  parseSNMPProfileResponse,
  parseSNMPProfilesResponse,
} from '../types/api';
import { type SNMPPayload } from './device';
import { ServerError, ValidationError } from './errors';
import { headersWithCsrf, requestJSON, requestJSONWithBody } from './transport';

export { ValidationError, ServerError };
export * from './admin';
export * from './auth';
export * from './backup';
export * from './canvas';
export * from './device';
export * from './settings';
export { headersWithCsrf } from './transport';

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

export async function revealSNMPProfile(id: string, reason: string): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}/reveal`, 'POST', {
      reason,
    }),
  );
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
