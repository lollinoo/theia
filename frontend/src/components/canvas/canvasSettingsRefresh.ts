import type { MutableRefObject } from 'react';

import type { GrafanaDashboardConfig } from '../../types/api';

export interface CanvasSettingsRefreshDependencies {
  fetchSettings: () => Promise<Record<string, string>>;
  fetchGrafanaDashboardConfig: () => Promise<GrafanaDashboardConfig>;
  grafanaUrlRef: Pick<MutableRefObject<string>, 'current'>;
  grafanaDashboardConfigRef: Pick<MutableRefObject<GrafanaDashboardConfig | null>, 'current'>;
}

// refreshCanvasSettings refreshes non-critical settings refs without failing topology loads.
export function refreshCanvasSettings({
  fetchSettings,
  fetchGrafanaDashboardConfig,
  grafanaUrlRef,
  grafanaDashboardConfigRef,
}: CanvasSettingsRefreshDependencies): void {
  fetchSettings()
    .then((settings) => {
      grafanaUrlRef.current = settings['grafana_url'] ?? '';
    })
    .catch(() => {
      // Settings fetch failure is non-fatal; Grafana links will be disabled.
    });
  fetchGrafanaDashboardConfig()
    .then((config) => {
      grafanaDashboardConfigRef.current = config;
    })
    .catch(() => {
      grafanaDashboardConfigRef.current = null;
    });
}
