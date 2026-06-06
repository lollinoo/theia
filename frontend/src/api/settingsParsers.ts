/**
 * Normalizes backend settings payloads into frontend-safe shapes.
 * Keeps API boundary validation close to the transport helpers that consume it.
 */
import { recordField, stringField } from './parsers';
import type {
  BridgeConnectorConfigResponse,
  BridgeConnectorDownload,
  HealthRuntime,
  SettingsWithMetadata,
} from './settings';

// parseBridgeConnectorDownload accepts complete connector download rows and drops malformed entries.
export function parseBridgeConnectorDownload(value: unknown): BridgeConnectorDownload | null {
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

// parseBridgeConnectorConfig normalizes connector config and filters invalid download options.
export function parseBridgeConnectorConfig(payload: unknown): BridgeConnectorConfigResponse {
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

// parseHealthRuntimePayload normalizes deployment environment metadata from health.
export function parseHealthRuntimePayload(payload: unknown): HealthRuntime {
  const record = recordField(payload) ?? {};
  const environment = stringField(record, 'environment');
  if (environment === 'staging' || environment === 'production') {
    return { environment };
  }
  return {
    environment: 'development',
  };
}

// parseSettingsPayload preserves settings strings and secret metadata from the backend envelope.
export function parseSettingsPayload(payload: unknown): SettingsWithMetadata {
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
