/**
 * Defines canvas settings refresh behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { MutableRefObject } from 'react';

import type { GrafanaDashboardConfig } from '../../types/api';

/** Describes the canvas settings refresh dependencies contract used by the topology canvas. */
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
