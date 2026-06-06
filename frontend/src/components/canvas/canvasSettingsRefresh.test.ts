/**
 * Exercises canvas settings refresh topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it, vi } from 'vitest';

import type { GrafanaDashboardConfig } from '../../types/api';
import { refreshCanvasSettings } from './canvasSettingsRefresh';

function ref<T>(current: T): { current: T } {
  return { current };
}

const emptyGrafanaConfig: GrafanaDashboardConfig = {
  profiles: [],
  default_profile_id: '',
  device_overrides: {},
};

async function flushPromises(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

describe('refreshCanvasSettings', () => {
  it('updates grafana URL and dashboard config refs independently', async () => {
    const grafanaUrlRef = ref('');
    const grafanaDashboardConfigRef = ref<GrafanaDashboardConfig | null>(null);
    const fetchSettings = vi.fn().mockResolvedValue({ grafana_url: 'https://grafana.example' });
    const fetchGrafanaDashboardConfig = vi.fn().mockResolvedValue(emptyGrafanaConfig);

    refreshCanvasSettings({
      fetchSettings,
      fetchGrafanaDashboardConfig,
      grafanaUrlRef,
      grafanaDashboardConfigRef,
    });
    await flushPromises();

    expect(grafanaUrlRef.current).toBe('https://grafana.example');
    expect(grafanaDashboardConfigRef.current).toBe(emptyGrafanaConfig);
  });

  it('keeps settings failures non-fatal while still updating dashboard config', async () => {
    const grafanaUrlRef = ref('previous');
    const grafanaDashboardConfigRef = ref<GrafanaDashboardConfig | null>(null);
    const fetchSettings = vi.fn().mockRejectedValue(new Error('offline'));
    const fetchGrafanaDashboardConfig = vi.fn().mockResolvedValue(emptyGrafanaConfig);

    refreshCanvasSettings({
      fetchSettings,
      fetchGrafanaDashboardConfig,
      grafanaUrlRef,
      grafanaDashboardConfigRef,
    });
    await flushPromises();

    expect(grafanaUrlRef.current).toBe('previous');
    expect(grafanaDashboardConfigRef.current).toBe(emptyGrafanaConfig);
  });

  it('clears dashboard config on dashboard config failures', async () => {
    const grafanaUrlRef = ref('');
    const grafanaDashboardConfigRef = ref<GrafanaDashboardConfig | null>(emptyGrafanaConfig);
    const fetchSettings = vi.fn().mockResolvedValue({});
    const fetchGrafanaDashboardConfig = vi.fn().mockRejectedValue(new Error('offline'));

    refreshCanvasSettings({
      fetchSettings,
      fetchGrafanaDashboardConfig,
      grafanaUrlRef,
      grafanaDashboardConfigRef,
    });
    await flushPromises();

    expect(grafanaUrlRef.current).toBe('');
    expect(grafanaDashboardConfigRef.current).toBeNull();
  });
});
