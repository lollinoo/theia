/**
 * Provides frontend API helpers for grafana endpoints.
 * Keeps request construction and backend response handling out of UI components.
 */
import { type GrafanaDashboardConfig, parseGrafanaDashboardConfigResponse } from '../types/api';
import { parsePrometheusHealthPayload } from './grafanaParsers';
import { requestJSON, requestJSONWithBody } from './transport';

/** Describes the grafana dashboard profile payload contract used by the frontend API boundary. */
export interface GrafanaDashboardProfilePayload {
  name: string;
  url_template: string;
  variable_source: string;
  is_default?: boolean;
}

/** Describes the grafana device override payload contract used by the frontend API boundary. */
export interface GrafanaDeviceOverridePayload {
  profile_id: string | null;
  custom_url: string;
}

/** Describes the prometheus health result contract used by the frontend API boundary. */
export interface PrometheusHealthResult {
  enabled?: boolean;
  available: boolean;
  url: string;
  error?: string;
}

// checkPrometheusHealth returns a normalized availability result instead of throwing UI errors.
export async function checkPrometheusHealth(): Promise<PrometheusHealthResult> {
  try {
    const payload = await requestJSON('/api/v1/prometheus/health');
    return parsePrometheusHealthPayload(payload);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    return { available: false, url: '', error: message };
  }
}

// fetchGrafanaDashboardConfig loads dashboard profiles and device overrides.
export async function fetchGrafanaDashboardConfig(): Promise<GrafanaDashboardConfig> {
  return parseGrafanaDashboardConfigResponse(
    await requestJSON('/api/v1/grafana/dashboard-profiles'),
  );
}

// createGrafanaDashboardProfile creates one dashboard profile and returns the refreshed config.
export async function createGrafanaDashboardProfile(
  payload: GrafanaDashboardProfilePayload,
): Promise<GrafanaDashboardConfig> {
  return parseGrafanaDashboardConfigResponse(
    await requestJSONWithBody('/api/v1/grafana/dashboard-profiles', 'POST', payload),
  );
}

// updateGrafanaDashboardProfile updates one dashboard profile and returns the refreshed config.
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

// deleteGrafanaDashboardProfile removes one dashboard profile.
export async function deleteGrafanaDashboardProfile(id: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/grafana/dashboard-profiles/${encodeURIComponent(id)}`,
    'DELETE',
  );
}

// saveDeviceGrafanaDashboardOverride saves one device override and returns the refreshed config.
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
