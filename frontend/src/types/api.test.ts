/**
 * Exercises API type-contract behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import {
  parseCanvasMapResponse,
  parseCanvasMapsResponse,
  parseCanvasTopologyResponse,
  parseDevicesResponse,
  parseRuntimeOverviewResponse,
} from './api';

function runtimeOverviewPayload(overrides: Record<string, unknown> = {}) {
  return {
    schema_version: 1,
    runtime_stream_id: 'runtime-stream-42',
    runtime_version: 42,
    runtime_identity: 'rt-sha256:exact',
    runtime_snapshot: {
      devices: {},
      links: {},
    },
    ...overrides,
  };
}

function deviceResource(id: string, deviceType: string) {
  return {
    id,
    attributes: {
      hostname: `${id}.example.test`,
      ip: `10.0.0.${id === 'ap-1' ? '1' : '2'}`,
      notes: id === 'ap-1' ? 'Managed by NOC' : null,
      device_type: deviceType,
      status: 'up',
      sys_name: `${id}.example.test`,
      sys_descr: 'Test device',
      hardware_model: 'Model X',
      vendor: 'mikrotik',
      managed: true,
      backup_supported: true,
      poll_class: 'standard',
      poll_interval_override: null,
      polling_enabled: true,
      metrics_source: 'prometheus',
      prometheus_label_name: 'instance',
      prometheus_label_value: `${id}.example.test:9100`,
      topology_discovery_mode: 'inherit',
      effective_topology_discovery_mode: 'off',
      topology_bootstrap_state: 'idle',
      last_topology_discovery_at: null,
      last_topology_discovery_result: '',
    },
    relationships: {
      interfaces: {
        data: [],
      },
    },
  };
}

function canvasTopologyPayload(overrides: Record<string, unknown> = {}) {
  return {
    schema_version: 1,
    topology_version: 'topo-test',
    generated_at: '2026-05-07T00:00:00Z',
    devices: [],
    links: [],
    positions: {},
    areas: [],
    capabilities: {
      supports_topology_delta: false,
      supports_position_revision: false,
      supports_area_filtering: true,
    },
    settings: {
      layout: {
        version: 1,
      },
    },
    ...overrides,
  };
}

describe('parseDevicesResponse', () => {
  it('maps backend access-point values to ap and preserves firewall devices', () => {
    const devices = parseDevicesResponse({
      data: [deviceResource('ap-1', 'access_point'), deviceResource('fw-1', 'firewall')],
    });

    expect(devices[0].device_type).toBe('ap');
    expect(devices[1].device_type).toBe('firewall');
    expect(devices[0].notes).toBe('Managed by NOC');
    expect(devices[1].notes).toBeNull();
  });

  it('parses topology discovery fields from the device payload', () => {
    const devices = parseDevicesResponse({
      data: [
        {
          ...deviceResource('router-1', 'router'),
          attributes: {
            ...deviceResource('router-1', 'router').attributes,
            topology_discovery_mode: 'bootstrap_once',
            effective_topology_discovery_mode: 'bootstrap_once',
            topology_bootstrap_state: 'followup_scheduled',
            last_topology_discovery_at: '2026-04-18T12:34:56Z',
            last_topology_discovery_result: 'ports_pending',
          },
        },
      ],
    });

    expect(devices[0].topology_discovery_mode).toBe('bootstrap_once');
    expect(devices[0].effective_topology_discovery_mode).toBe('bootstrap_once');
    expect(devices[0].topology_bootstrap_state).toBe('followup_scheduled');
    expect(devices[0].last_topology_discovery_at).toBe('2026-04-18T12:34:56Z');
    expect(devices[0].last_topology_discovery_result).toBe('ports_pending');
  });

  it('defaults polling_enabled to true when omitted', () => {
    const resource = deviceResource('router-2', 'router');
    (resource.attributes as Record<string, unknown>).polling_enabled = undefined;

    const devices = parseDevicesResponse({ data: [resource] });

    expect(devices[0].polling_enabled).toBe(true);
  });

  it('preserves map-local visual color from canvas topology device attributes', () => {
    const resource = deviceResource('virtual-1', 'virtual');
    (resource.attributes as Record<string, unknown>).map_visual_color = '#123ABC';

    const devices = parseDevicesResponse({ data: [resource] });

    expect(devices[0].map_visual_color).toBe('#123ABC');
  });

  it('preserves explicit polling_enabled false', () => {
    const devices = parseDevicesResponse({
      data: [
        {
          ...deviceResource('router-3', 'router'),
          attributes: {
            ...deviceResource('router-3', 'router').attributes,
            polling_enabled: false,
          },
        },
      ],
    });

    expect(devices[0].polling_enabled).toBe(false);
  });

  it('synthesizes a primary address from legacy ip when addresses are omitted', () => {
    const devices = parseDevicesResponse({
      data: [deviceResource('router-4', 'router')],
    });

    expect(devices[0].addresses).toEqual([
      {
        id: '',
        device_id: 'router-4',
        address: '10.0.0.2',
        label: '',
        role: 'primary',
        is_primary: true,
        priority: 0,
        probe_ports: null,
      },
    ]);
  });

  it('parses populated device address collections and skips malformed entries', () => {
    const resource = deviceResource('router-5', 'router');
    resource.attributes = {
      ...resource.attributes,
      probe_ports: [22, 8291],
      addresses: [
        {
          id: 'addr-primary',
          device_id: 'router-5',
          address: '10.0.0.5',
          label: 'LAN',
          role: 'primary',
          is_primary: true,
          priority: 0,
          probe_ports: [22],
        },
        {
          id: 'addr-backup',
          device_id: 'router-5',
          address: '192.0.2.5',
          label: 'OOB',
          role: 'backup',
          is_primary: false,
          priority: 10,
          probe_ports: null,
        },
        { address: 123 },
      ],
    };

    const devices = parseDevicesResponse({ data: [resource] });

    expect(devices[0].addresses).toEqual([
      {
        id: 'addr-primary',
        device_id: 'router-5',
        address: '10.0.0.5',
        label: 'LAN',
        role: 'primary',
        is_primary: true,
        priority: 0,
        probe_ports: [22],
      },
      {
        id: 'addr-backup',
        device_id: 'router-5',
        address: '192.0.2.5',
        label: 'OOB',
        role: 'backup',
        is_primary: false,
        priority: 10,
        probe_ports: null,
      },
    ]);
    expect(devices[0].probe_ports).toEqual([22, 8291]);
  });
});

describe('parseCanvasMapResponse', () => {
  it('parses wrapped canvas map payloads and defaults counts', () => {
    const map = parseCanvasMapResponse({
      data: {
        id: 'map-1',
        name: 'Backbone',
        description: 'Backbone map',
        source_area_id: null,
        filter: {},
        is_default: false,
        created_at: '2026-05-07T00:00:00Z',
        updated_at: '2026-05-07T00:00:00Z',
      },
    });

    expect(map).toMatchObject({
      id: 'map-1',
      name: 'Backbone',
      source_area_id: null,
      filter: {},
      device_count: 0,
      link_count: 0,
      position_count: 0,
    });
  });

  it('rejects invalid map filter payloads', () => {
    expect(() => parseCanvasMapResponse({ id: 'map-1', name: 'Broken', filter: 'area-a' })).toThrow(
      'invalid canvas map filter',
    );
  });

  it.each([
    ['area_id', { area_id: 123 }],
    ['device_ids', { device_ids: ['device-1', 123] }],
    ['include_cross_area_links', { include_cross_area_links: 'yes' }],
    ['include_ghost_devices', { include_ghost_devices: 'no' }],
    ['tags', { tags: { role: 'core', invalid: 123 } }],
  ])('rejects invalid %s filter fields', (_field, filter) => {
    expect(() => parseCanvasMapResponse({ id: 'map-1', name: 'Broken', filter })).toThrow(
      'invalid canvas map filter',
    );
  });
});

describe('parseCanvasMapsResponse', () => {
  it('parses map list payloads', () => {
    expect(
      parseCanvasMapsResponse({
        data: [{ id: 'default', name: 'Default', is_default: true, filter: {} }],
      }),
    ).toHaveLength(1);
  });
});

describe('parseCanvasTopologyResponse', () => {
  it('parses valid saved link routes and omits invalid entries', () => {
    const topology = parseCanvasTopologyResponse(
      canvasTopologyPayload({
        link_routes: {
          'link-1': { version: 1, waypoints: [{ x: 12.5, y: -8 }] },
          'wrong-version': { version: 2, waypoints: [{ x: 0, y: 0 }] },
          empty: { version: 1, waypoints: [] },
          'too-many': {
            version: 1,
            waypoints: Array.from({ length: 17 }, (_, index) => ({ x: index, y: index })),
          },
          'non-finite': { version: 1, waypoints: [{ x: Number.POSITIVE_INFINITY, y: 0 }] },
          'non-number': { version: 1, waypoints: [{ x: '12.5', y: 0 }] },
        },
      }),
    );

    expect(topology.link_routes).toEqual({
      'link-1': { version: 1, waypoints: [{ x: 12.5, y: -8 }] },
    });
  });

  it.each([
    undefined,
    null,
    [],
    'invalid',
  ])('defaults absent or malformed link routes to {}', (linkRoutes) => {
    expect(
      parseCanvasTopologyResponse(canvasTopologyPayload({ link_routes: linkRoutes })).link_routes,
    ).toEqual({});
  });

  it('parses the versioned canvas read model into frontend topology types', () => {
    const payload = {
      schema_version: 1,
      topology_version: 'topo-abc123',
      runtime_version: 456,
      runtime_stream_id: 'runtime-stream-456',
      runtime_identity: 'rt-sha256:abc',
      runtime_snapshot: {
        devices: {
          'router-1': {
            device_id: 'router-1',
            operational_status: 'down',
            primary_health: 'unreachable',
            runtime_flags: [],
            field_states: {
              cpu: 'missing',
              memory: 'missing',
              uptime: 'error',
            },
            network_reachable: 'false',
            snmp_reachable: 'false',
            reachability: 'hard_down',
            health: 'unknown',
            freshness: 'fresh',
            primary_reason: 'device_unreachable',
            metrics_status: 'unavailable',
            metrics_reason: 'device_unreachable',
            alert_status: 'normal',
            firing_alert_count: 0,
            last_collected_at: null,
            last_polled_at: null,
            expected_poll_interval_seconds: null,
            cpu_percent: null,
            mem_percent: null,
            temp_celsius: null,
            uptime_secs: null,
          },
        },
        links: {},
      },
      generated_at: '2026-04-30T12:00:00Z',
      map: { id: 'map-1', name: 'Backbone', is_default: false, filter: {} },
      devices: [deviceResource('router-1', 'router')],
      links: [
        {
          id: 'link-1',
          source_device_id: 'router-1',
          source_if_name: 'ether1',
          target_device_id: 'router-2',
          target_if_name: 'ether2',
          discovery_protocol: 'lldp',
          source_if_speed: 1000000000,
          source_if_oper_status: 'up',
          target_if_speed: 100000000,
          target_if_oper_status: 'down',
        },
      ],
      positions: {
        'router-1': {
          x: 120,
          y: 240,
          pinned: true,
          updated_at: '2026-04-30T12:01:00Z',
        },
      },
      areas: [
        {
          id: 'area-1',
          name: 'Backbone',
          description: 'Core links',
          color: '#2979FF',
          device_count: 1,
          created_at: '2026-04-30T12:00:00Z',
          updated_at: '2026-04-30T12:00:00Z',
        },
      ],
      capabilities: {
        supports_topology_delta: false,
        supports_position_revision: false,
        supports_area_filtering: true,
      },
      settings: {
        layout: {
          version: 1,
        },
      },
    };

    const topology = parseCanvasTopologyResponse(payload);

    expect(topology.schema_version).toBe(1);
    expect(topology.topology_version).toBe('topo-abc123');
    expect(topology.runtime_version).toBe(456);
    expect(topology.runtime_stream_id).toBe('runtime-stream-456');
    expect(topology.runtime_identity).toBe('rt-sha256:abc');
    expect(topology.runtime_snapshot?.devices['router-1'].operational_status).toBe('down');
    expect(topology.map?.id).toBe('map-1');
    expect(topology.devices[0].hostname).toBe('router-1.example.test');
    expect(topology.devices[0].map_visual_color).toBeNull();
    expect(topology.links[0]).toMatchObject({
      id: 'link-1',
      source_if_speed: 1000000000,
      target_if_oper_status: 'down',
    });
    expect(topology.positions['router-1']).toEqual({
      device_id: 'router-1',
      x: 120,
      y: 240,
      pinned: true,
      updated_at: '2026-04-30T12:01:00Z',
    });
    expect(topology.areas[0].name).toBe('Backbone');
    expect(topology.capabilities.supports_area_filtering).toBe(true);
    expect(topology.settings.layout.version).toBe(1);
  });

  it('leaves missing map metadata undefined', () => {
    expect(parseCanvasTopologyResponse(canvasTopologyPayload()).map).toBeUndefined();
  });

  it('rejects invalid present map metadata', () => {
    expect(() => parseCanvasTopologyResponse(canvasTopologyPayload({ map: 'map-1' }))).toThrow(
      'invalid canvas map payload',
    );
  });

  it('rejects invalid fields in present map metadata', () => {
    expect(() =>
      parseCanvasTopologyResponse(
        canvasTopologyPayload({
          map: {
            id: 'map-1',
            name: 'Broken',
            is_default: false,
            filter: { device_ids: ['device-1', 123] },
          },
        }),
      ),
    ).toThrow('invalid canvas map filter');
  });
});

describe('parseRuntimeOverviewResponse', () => {
  it('parses an exact runtime cursor and returns an unaliased snapshot', () => {
    const payload = runtimeOverviewPayload();

    const result = parseRuntimeOverviewResponse(payload);

    expect(result).toEqual({
      schema_version: 1,
      runtime_stream_id: 'runtime-stream-42',
      runtime_version: 42,
      runtime_identity: 'rt-sha256:exact',
      runtime_snapshot: { devices: {}, links: {} },
    });
    expect(result.runtime_snapshot).not.toBe(payload.runtime_snapshot);
    (result.runtime_snapshot.devices as Record<string, unknown>)['device-new'] = {};
    expect(payload.runtime_snapshot.devices).toEqual({});
  });

  it.each([
    ['missing schema', { schema_version: undefined }],
    ['unsupported schema', { schema_version: 2 }],
    ['string schema', { schema_version: '1' }],
    ['missing stream', { runtime_stream_id: undefined }],
    ['empty stream', { runtime_stream_id: '' }],
    ['blank stream', { runtime_stream_id: '   ' }],
    ['missing version', { runtime_version: undefined }],
    ['negative version', { runtime_version: -1 }],
    ['fractional version', { runtime_version: 1.5 }],
    ['unsafe version', { runtime_version: Number.MAX_SAFE_INTEGER + 1 }],
    ['non-finite version', { runtime_version: Number.POSITIVE_INFINITY }],
    ['missing identity', { runtime_identity: undefined }],
    ['empty identity', { runtime_identity: '' }],
    ['blank identity', { runtime_identity: '  ' }],
    ['non-string identity', { runtime_identity: 7 }],
    ['missing snapshot', { runtime_snapshot: undefined }],
    ['malformed snapshot', { runtime_snapshot: { devices: {} } }],
  ])('rejects %s', (_name, overrides) => {
    const payload = runtimeOverviewPayload(overrides);
    for (const [key, value] of Object.entries(overrides)) {
      if (value === undefined) {
        delete (payload as Record<string, unknown>)[key];
      }
    }

    expect(() => parseRuntimeOverviewResponse(payload)).toThrow(
      'invalid runtime overview response',
    );
  });

  it('rejects legacy alias fields in place of the exact runtime contract', () => {
    expect(() =>
      parseRuntimeOverviewResponse({
        schema_version: 1,
        stream_id: 'runtime-stream-42',
        version: 42,
        identity: 'rt-sha256:exact',
        snapshot: { devices: {}, links: {} },
      }),
    ).toThrow('invalid runtime overview response');
  });
});
