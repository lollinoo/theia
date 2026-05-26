import type { Device, GrafanaDashboardConfig, GrafanaVariableSource } from '../types/api';

export type { GrafanaDashboardConfig, GrafanaDashboardProfile } from '../types/api';

export interface GrafanaMapContext {
  mapId: string | null;
  mapName: string;
}

const placeholderPattern = /\{\{\s*(hostname|ip|map_name|map_id)\s*\}\}/g;

export function resolveGrafanaDashboardUrl(
  config: GrafanaDashboardConfig | null,
  device: Device | undefined,
  mapContext: GrafanaMapContext,
  globalGrafanaUrl: string,
): string {
  if (!device) return globalGrafanaUrl;
  const override = config?.device_overrides[device.id];
  if (override?.custom_url.trim()) {
    return override.custom_url.trim();
  }

  const profileId = override?.profile_id || config?.default_profile_id || '';
  const profile = config?.profiles.find((candidate) => candidate.id === profileId);
  if (profile) {
    return renderGrafanaDashboardTemplate(profile.url_template, device, mapContext);
  }

  return globalGrafanaUrl;
}

export function renderGrafanaDashboardTemplate(
  template: string,
  device: Device,
  mapContext: GrafanaMapContext,
): string {
  return template.replace(placeholderPattern, (_match, token: GrafanaVariableSource) =>
    encodeURIComponent(grafanaPlaceholderValue(token, device, mapContext)),
  );
}

function grafanaPlaceholderValue(
  token: GrafanaVariableSource,
  device: Device,
  mapContext: GrafanaMapContext,
): string {
  switch (token) {
    case 'hostname':
      return device.hostname || device.sys_name || device.ip || device.id;
    case 'ip':
      return device.ip || device.hostname || device.id;
    case 'map_name':
      return mapContext.mapName || 'Default';
    case 'map_id':
      return mapContext.mapId ?? 'default';
  }
}

export const EMPTY_GRAFANA_DASHBOARD_CONFIG: GrafanaDashboardConfig = {
  profiles: [],
  default_profile_id: '',
  device_overrides: {},
};
