import {
  parseBridgeConnectorConfig,
  parseHealthVersionPayload,
  parseSettingsPayload,
} from './settingsParsers';
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

// fetchUserSettings loads account-scoped settings for the current user.
export async function fetchUserSettings(): Promise<UserSettingsResponse> {
  return (await requestJSON('/api/v1/settings/me')) as UserSettingsResponse;
}

// updateUserSettings patches account preferences and bridge overrides.
export async function updateUserSettings(
  payload: UpdateUserSettingsPayload,
): Promise<UserSettingsResponse> {
  return (await requestJSONWithBody(
    '/api/v1/settings/me',
    'PATCH',
    payload,
  )) as UserSettingsResponse;
}

// generateBridgeSecret creates a bridge credential and returns the one-time secret.
export async function generateBridgeSecret(): Promise<BridgeSecretResult> {
  return (await requestJSONWithBody(
    '/api/v1/settings/bridge/secret',
    'POST',
  )) as BridgeSecretResult;
}

// rotateBridgeSecret rotates the bridge credential with a persisted audit reason.
export async function rotateBridgeSecret(reason = 'rotated by user'): Promise<BridgeSecretResult> {
  return (await requestJSONWithBody('/api/v1/settings/bridge/secret/rotate', 'POST', {
    reason,
  })) as BridgeSecretResult;
}

// revokeBridgeSecret revokes the bridge credential with a persisted audit reason.
export async function revokeBridgeSecret(
  reason = 'revoked by user',
): Promise<BridgeCredentialMetadata> {
  return (await requestJSONWithBody('/api/v1/settings/bridge/secret/revoke', 'POST', {
    reason,
  })) as BridgeCredentialMetadata;
}

// fetchBridgeConnectorConfig loads connector configuration and available binary downloads.
export async function fetchBridgeConnectorConfig(): Promise<BridgeConnectorConfigResponse> {
  return parseBridgeConnectorConfig(await requestJSON('/api/v1/settings/bridge/connector/config'));
}

// fetchHealthVersion returns build metadata and falls back to unknown values on request failure.
export async function fetchHealthVersion(): Promise<HealthVersion> {
  try {
    const payload = await requestJSON('/api/v1/health');
    return parseHealthVersionPayload(payload);
  } catch {
    return { version: 'unknown', git_commit: 'unknown', build_date: 'unknown' };
  }
}

// fetchSettingsWithMetadata loads global settings and secret redaction metadata.
export async function fetchSettingsWithMetadata(): Promise<SettingsWithMetadata> {
  try {
    const payload = await requestJSON('/api/v1/settings');
    return parseSettingsPayload(payload);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to fetch settings: ${message}`);
  }
}

// fetchSettings preserves the legacy settings-only API by dropping metadata.
export async function fetchSettings(): Promise<Record<string, string>> {
  const settings = await fetchSettingsWithMetadata();
  return settings.data;
}

// updateSetting stores one global setting value by encoded key.
export async function updateSetting(key: string, value: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/settings/${encodeURIComponent(key)}`, 'PUT', { value });
}
