import { recordField, stringField } from './parsers';
import { requestJSON, requestJSONWithBody } from './transport';

export interface UserSettingsResponse {
  user: {
    id: string;
    username: string;
    email: string;
    display_name: string;
    last_login_at?: string;
    password_changed_at?: string;
  };
  preferences: {
    timezone: string;
    locale: string;
    bridge_port: number;
    global_bridge_port: number;
    bridge_port_override: number | null;
  };
  bridge: BridgeSettingsState;
}

export interface BridgeCredentialMetadata {
  id: string;
  secret_prefix: string;
  status: string;
  created_at: string;
  rotated_at?: string;
  revoked_at?: string;
  last_used_at?: string;
  expires_at?: string;
}

export interface BridgeSettingsState {
  configured: boolean;
  credential?: BridgeCredentialMetadata;
}

export interface BridgeSecretResult {
  credential: BridgeCredentialMetadata;
  secret: string;
  shown_once: boolean;
}

export interface BridgeConnectorDownload {
  label: string;
  os: string;
  arch: string;
  url: string;
  available: boolean;
}

export interface UpdateUserSettingsPayload {
  display_name?: string;
  username?: string;
  email?: string;
  timezone?: string;
  locale?: string;
  bridge_port?: number;
  bridge_port_override?: number | null;
}

export interface BridgeConnectorConfigResponse {
  config: Record<string, unknown>;
  downloads: BridgeConnectorDownload[];
}

export interface HealthVersion {
  version: string;
  git_commit: string;
  build_date: string;
}

export interface SettingSecretState {
  present: boolean;
  redacted: boolean;
}

export interface SettingsWithMetadata {
  data: Record<string, string>;
  secrets: Record<string, SettingSecretState>;
}

function parseBridgeConnectorDownload(value: unknown): BridgeConnectorDownload | null {
  const record = recordField(value);
  if (!record) {
    return null;
  }
  const label = stringField(record, 'label');
  const os = stringField(record, 'os');
  const arch = stringField(record, 'arch');
  const url = stringField(record, 'url');
  if (!label || !os || !arch || !url) {
    return null;
  }
  return {
    label,
    os,
    arch,
    url,
    available: record.available === true,
  };
}

function parseBridgeConnectorConfig(payload: unknown): BridgeConnectorConfigResponse {
  const record = recordField(payload) ?? {};
  const config = recordField(record.config) ?? {};
  const downloads = Array.isArray(record.downloads)
    ? record.downloads.flatMap((item) => {
        const parsed = parseBridgeConnectorDownload(item);
        return parsed ? [parsed] : [];
      })
    : [];
  return { config, downloads };
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

export async function fetchUserSettings(): Promise<UserSettingsResponse> {
  return (await requestJSON('/api/v1/settings/me')) as UserSettingsResponse;
}

export async function updateUserSettings(
  payload: UpdateUserSettingsPayload,
): Promise<UserSettingsResponse> {
  return (await requestJSONWithBody(
    '/api/v1/settings/me',
    'PATCH',
    payload,
  )) as UserSettingsResponse;
}

export async function generateBridgeSecret(): Promise<BridgeSecretResult> {
  return (await requestJSONWithBody(
    '/api/v1/settings/bridge/secret',
    'POST',
  )) as BridgeSecretResult;
}

export async function rotateBridgeSecret(reason = 'rotated by user'): Promise<BridgeSecretResult> {
  return (await requestJSONWithBody('/api/v1/settings/bridge/secret/rotate', 'POST', {
    reason,
  })) as BridgeSecretResult;
}

export async function revokeBridgeSecret(
  reason = 'revoked by user',
): Promise<BridgeCredentialMetadata> {
  return (await requestJSONWithBody('/api/v1/settings/bridge/secret/revoke', 'POST', {
    reason,
  })) as BridgeCredentialMetadata;
}

export async function fetchBridgeConnectorConfig(): Promise<BridgeConnectorConfigResponse> {
  return parseBridgeConnectorConfig(await requestJSON('/api/v1/settings/bridge/connector/config'));
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
