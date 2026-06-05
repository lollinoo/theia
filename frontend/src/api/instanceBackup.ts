import { type InstanceBackup, type RestoreReport, type RestoreStatus } from '../types/api';
import { ServerError, ValidationError } from './errors';
import { parseInstanceBackup, parseRestoreReport, parseRestoreStatus } from './instanceBackupParsers';
import { headersWithCsrf, requestJSON, requestJSONWithBody } from './transport';

// createInstanceBackup starts an instance backup and parses the created backup envelope.
export async function createInstanceBackup(): Promise<InstanceBackup> {
  const response = await requestJSONWithBody('/api/v1/instance-backups', 'POST');
  const data = (response as Record<string, unknown>)?.data as Record<string, unknown>;
  return parseInstanceBackup(data);
}

// fetchInstanceBackups loads backup history and preserves the existing empty-list fallback.
export async function fetchInstanceBackups(): Promise<InstanceBackup[]> {
  const payload = await requestJSON('/api/v1/instance-backups');
  const data = (payload as Record<string, unknown>)?.data;
  if (!Array.isArray(data)) return [];
  return data.map((item) => parseInstanceBackup(item as Record<string, unknown>));
}

// deleteInstanceBackup removes a stored instance backup by encoded identifier.
export async function deleteInstanceBackup(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/instance-backups/${encodeURIComponent(id)}`, 'DELETE');
}

// cancelInstanceBackup cancels a running backup and parses the updated backup state.
export async function cancelInstanceBackup(id: string): Promise<InstanceBackup> {
  const response = await requestJSONWithBody(
    `/api/v1/instance-backups/${encodeURIComponent(id)}/cancel`,
    'POST',
  );
  const data = (response as Record<string, unknown>)?.data as Record<string, unknown>;
  return parseInstanceBackup(data);
}

// instanceBackupDownloadUrl builds the download endpoint while preserving encoded IDs.
export function instanceBackupDownloadUrl(id: string): string {
  return `/api/v1/instance-backups/${encodeURIComponent(id)}/download`;
}

export async function fetchRestoreStatus(): Promise<RestoreStatus | null> {
  const payload = await requestJSON('/api/v1/instance-backups/restore-status');
  const data = (payload as Record<string, unknown>)?.data;
  return parseRestoreStatus(data);
}

// restoreInstanceBackup uploads an archive through multipart form data and parses the restore report.
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
    // Do NOT set Content-Type - browser sets multipart boundary automatically
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
