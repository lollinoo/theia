/**
 * Exercises Grafana dashboard utility behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';
import type { Device } from '../types/api';
import { type GrafanaDashboardConfig, resolveGrafanaDashboardUrl } from './grafanaDashboard';

function device(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'edge 01',
    ip: '10.0.0.1',
    notes: null,
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    polling_enabled: true,
    status: 'up',
    sys_name: '',
    sys_descr: '',
    hardware_model: '',
    os_version: '',
    vendor: 'default',
    managed: true,
    tags: {},
    interfaces: [],
    area_ids: [],
    backup_supported: true,
    metrics_source: 'snmp',
    prometheus_label_name: '',
    prometheus_label_value: '',
    topology_discovery_mode: 'inherit',
    map_visual_color: null,
    ...overrides,
  };
}

describe('resolveGrafanaDashboardUrl', () => {
  it('prefers a device custom URL even when no global URL is configured', () => {
    const config: GrafanaDashboardConfig = {
      profiles: [],
      default_profile_id: '',
      device_overrides: {
        'dev-1': {
          profile_id: null,
          custom_url: 'https://grafana.example/d/router?var-device=edge-01',
        },
      },
    };

    expect(
      resolveGrafanaDashboardUrl(config, device(), { mapId: 'map-1', mapName: 'Core' }, ''),
    ).toBe('https://grafana.example/d/router?var-device=edge-01');
  });

  it('uses the assigned profile before the default profile and encodes placeholders', () => {
    const config: GrafanaDashboardConfig = {
      default_profile_id: 'default-profile',
      profiles: [
        {
          id: 'default-profile',
          name: 'Default',
          url_template: 'https://grafana.example/d/default?var-device={{ip}}',
          variable_source: 'ip',
        },
        {
          id: 'assigned-profile',
          name: 'Assigned',
          url_template:
            'https://grafana.example/d/router?var-device={{hostname}}&var-map={{map_name}}',
          variable_source: 'hostname',
        },
      ],
      device_overrides: {
        'dev-1': {
          profile_id: 'assigned-profile',
          custom_url: '',
        },
      },
    };

    expect(
      resolveGrafanaDashboardUrl(config, device(), { mapId: 'map-1', mapName: 'Core Map' }, ''),
    ).toBe('https://grafana.example/d/router?var-device=edge%2001&var-map=Core%20Map');
  });

  it('uses discovered sysName for hostname templates when an existing device was added by IP', () => {
    const config: GrafanaDashboardConfig = {
      default_profile_id: 'router-profile',
      profiles: [
        {
          id: 'router-profile',
          name: 'Router',
          url_template: 'https://grafana.example/d/router?var-routerboard={{hostname}}',
          variable_source: 'hostname',
        },
      ],
      device_overrides: {},
    };

    expect(
      resolveGrafanaDashboardUrl(
        config,
        device({
          hostname: '10.0.0.1',
          ip: '10.0.0.1',
          sys_name: 'edge-router-01',
        }),
        { mapId: 'map-1', mapName: 'Core Map' },
        '',
      ),
    ).toBe('https://grafana.example/d/router?var-routerboard=edge-router-01');
  });

  it('keeps a manually configured hostname before the discovered sysName', () => {
    const config: GrafanaDashboardConfig = {
      default_profile_id: 'router-profile',
      profiles: [
        {
          id: 'router-profile',
          name: 'Router',
          url_template: 'https://grafana.example/d/router?var-routerboard={{hostname}}',
          variable_source: 'hostname',
        },
      ],
      device_overrides: {},
    };

    expect(
      resolveGrafanaDashboardUrl(
        config,
        device({
          hostname: 'manual-router-name',
          ip: '10.0.0.1',
          sys_name: 'edge-router-01',
        }),
        { mapId: 'map-1', mapName: 'Core Map' },
        '',
      ),
    ).toBe('https://grafana.example/d/router?var-routerboard=manual-router-name');
  });
});
