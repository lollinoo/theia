/**
 * Normalizes backend grafana payloads into frontend-safe shapes.
 * Keeps API boundary validation close to the transport helpers that consume it.
 */
import type { PrometheusHealthResult } from './grafana';

// parsePrometheusHealthPayload normalizes health payloads and preserves invalid-response fallback behavior.
export function parsePrometheusHealthPayload(payload: unknown): PrometheusHealthResult {
  if (typeof payload === 'object' && payload !== null) {
    const record = payload as Record<string, unknown>;
    return {
      enabled: typeof record.enabled === 'boolean' ? record.enabled : undefined,
      available: record.available === true,
      url: typeof record.url === 'string' ? record.url : '',
      error: typeof record.error === 'string' ? record.error : undefined,
    };
  }
  return { available: false, url: '', error: 'invalid response' };
}
