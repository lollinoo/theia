/**
 * Exercises topology composition cache topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it, vi } from 'vitest';

import type { Device, Link, LinkRouteMap } from '../../types/api';
import type { AlertDTO, PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import { buildRuntimeState } from './runtimeAdapters';
import { composeCanvasTopology } from './topologyComposer';
import {
  type BuildCanvasTopologyCompositionCacheKeyInput,
  buildCanvasTopologyCompositionCacheKey,
  createCanvasTopologyCompositionCache,
} from './topologyCompositionCache';

const noopDeviceMenu = vi.fn();
const noopEdgeMenu = vi.fn();

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    polling_enabled: true,
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    tags: {},
    interfaces: [],
    area_ids: [],
    backup_supported: true,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    ...overrides,
  };
}

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'dev-1',
    source_if_name: 'ether1',
    target_device_id: 'dev-2',
    target_if_name: 'ether2',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

function baseInput(
  overrides: Partial<BuildCanvasTopologyCompositionCacheKeyInput> = {},
): BuildCanvasTopologyCompositionCacheKeyInput {
  return {
    mapKey: 'map:default',
    topologySignature: '{"deviceKeys":["dev-1"],"linkKeys":["link-1"]}',
    topologyVersion: 'topo-1',
    topologyEtag: '"canvas-topology-1"',
    schemaVersion: 1,
    devices: [mockDevice()],
    links: [mockLink()],
    savedPositions: new Map([['dev-1', { x: 100, y: 120, pinned: true }]]),
    computedPositions: new Map(),
    currentPositions: new Map(),
    explicitPositions: new Map(),
    editMode: false,
    snapGrid: null,
    placementDeviceIds: new Set(),
    runtimeIdentity: 'rt-sha256:abc',
    runtimeVersion: 7,
    runtimeSnapshot: null,
    alerts: [],
    prometheusStatus: { enabled: true, available: true },
    openDeviceMenu: noopDeviceMenu,
    openEdgeMenu: noopEdgeMenu,
    ...overrides,
  };
}

function buildKey(overrides: Partial<BuildCanvasTopologyCompositionCacheKeyInput> = {}) {
  return buildCanvasTopologyCompositionCacheKey(baseInput(overrides));
}

function expectCacheInvalidates(
  first: Partial<BuildCanvasTopologyCompositionCacheKeyInput>,
  second: Partial<BuildCanvasTopologyCompositionCacheKeyInput>,
) {
  const firstResult = { nodes: [], edges: [] };
  const secondResult = { nodes: [], edges: [] };
  const composer = vi.fn().mockReturnValueOnce(firstResult).mockReturnValueOnce(secondResult);
  const cache = createCanvasTopologyCompositionCache(composer);
  const compositionInput = {} as Parameters<
    ReturnType<typeof createCanvasTopologyCompositionCache>['compose']
  >[0];

  const firstComposed = cache.compose(compositionInput, buildKey(first));
  const secondComposed = cache.compose(compositionInput, buildKey(second));

  expect(firstComposed).toBe(firstResult);
  expect(secondComposed).toBe(secondResult);
  expect(composer).toHaveBeenCalledTimes(2);
}

describe('buildCanvasTopologyCompositionCacheKey', () => {
  it('uses a deterministic route signature when server topology identifiers are absent', () => {
    const firstRoutes: LinkRouteMap = {
      'link-2': { version: 1, waypoints: [{ x: 30, y: 40 }] },
      'link-1': { version: 1, waypoints: [{ x: 10, y: 20 }] },
    };
    const secondRoutes: LinkRouteMap = {
      'link-1': { version: 1, waypoints: [{ x: 10, y: 20 }] },
      'link-2': { version: 1, waypoints: [{ x: 30, y: 40 }] },
    };

    const first = buildKey({
      topologyVersion: undefined,
      topologyEtag: null,
      linkRoutes: firstRoutes,
    });
    const second = buildKey({
      topologyVersion: undefined,
      topologyEtag: null,
      linkRoutes: secondRoutes,
    });

    expect(first.signature).toBe(second.signature);
  });

  it('invalidates when route coordinates change without server topology identifiers', () => {
    expectCacheInvalidates(
      {
        topologyVersion: undefined,
        topologyEtag: null,
        linkRoutes: { 'link-1': { version: 1, waypoints: [{ x: 10, y: 20 }] } },
      },
      {
        topologyVersion: undefined,
        topologyEtag: null,
        linkRoutes: { 'link-1': { version: 1, waypoints: [{ x: 11, y: 20 }] } },
      },
    );
  });

  it('invalidates when the route commit callback identity changes', () => {
    expectCacheInvalidates({ onLinkRouteCommit: vi.fn() }, { onLinkRouteCommit: vi.fn() });
  });

  it('uses different signatures for disabled and enabled grid modes', () => {
    const disabled = buildKey({ snapGrid: null });
    const enabled = buildKey({ snapGrid: [30, 30] });

    expect(enabled.signature).not.toBe(disabled.signature);
  });

  it('uses server topology identity without traversing device or link presentation fields', () => {
    const explodingDevice = Object.defineProperties(
      {},
      {
        hostname: {
          get() {
            throw new Error('device presentation should not be read');
          },
        },
        interfaces: {
          get() {
            throw new Error('interfaces should not be read');
          },
        },
      },
    ) as Device;
    const explodingLink = Object.defineProperties(
      {},
      {
        source_device_id: {
          get() {
            throw new Error('link presentation should not be read');
          },
        },
      },
    ) as Link;

    expect(() =>
      buildKey({
        topologyVersion: 'topo-2',
        topologyEtag: '"canvas-topology-2"',
        devices: [explodingDevice],
        links: [explodingLink],
      }),
    ).not.toThrow();
  });

  it('invalidates when saved positions change under the same topology and runtime ids', () => {
    expectCacheInvalidates(
      { savedPositions: new Map([['dev-1', { x: 100, y: 120, pinned: true }]]) },
      { savedPositions: new Map([['dev-1', { x: 101, y: 120, pinned: true }]]) },
    );
  });

  it('includes keyed explicit positions in a stable signature', () => {
    const empty = buildKey({ explicitPositions: new Map() });
    const first = buildKey({
      explicitPositions: new Map([
        ['dev-old', { x: 10, y: 20 }],
        ['dev-new', { x: 50, y: 60 }],
      ]),
    });
    const changedX = buildKey({
      explicitPositions: new Map([
        ['dev-old', { x: 10, y: 20 }],
        ['dev-new', { x: 51, y: 60 }],
      ]),
    });
    const reverseInsertionOrder = buildKey({
      explicitPositions: new Map([
        ['dev-new', { x: 50, y: 60 }],
        ['dev-old', { x: 10, y: 20 }],
      ]),
    });

    expect(first.signature).not.toBe(empty.signature);
    expect(changedX.signature).not.toBe(first.signature);
    expect(reverseInsertionOrder.signature).toBe(first.signature);
  });

  it('keeps mutable input signatures stable when equal values arrive in different orders', () => {
    const highCpuAlert: AlertDTO = {
      device_id: 'dev-1',
      alert_name: 'HighCPU',
      state: 'firing',
      severity: 'critical',
      summary: 'CPU high',
    };
    const linkDownAlert: AlertDTO = {
      device_id: 'dev-2',
      alert_name: 'LinkDown',
      state: 'firing',
      severity: 'warning',
      summary: 'Link down',
    };
    const first = buildKey({
      savedPositions: new Map([
        ['dev-1', { x: 10, y: 20, pinned: true }],
        ['dev-2', { x: 30, y: 40, pinned: false }],
      ]),
      computedPositions: new Map([
        ['dev-1', { x: 50, y: 60 }],
        ['dev-2', { x: 70, y: 80 }],
      ]),
      currentPositions: new Map([
        ['dev-1', { x: 90, y: 100, pinned: true }],
        ['dev-2', { x: 110, y: 120, pinned: false }],
      ]),
      placementDeviceIds: new Set(['dev-1', 'dev-2']),
      alerts: [highCpuAlert, linkDownAlert],
    });
    const second = buildKey({
      savedPositions: new Map([
        ['dev-2', { x: 30, y: 40, pinned: false }],
        ['dev-1', { x: 10, y: 20, pinned: true }],
      ]),
      computedPositions: new Map([
        ['dev-2', { x: 70, y: 80 }],
        ['dev-1', { x: 50, y: 60 }],
      ]),
      currentPositions: new Map([
        ['dev-2', { x: 110, y: 120, pinned: false }],
        ['dev-1', { x: 90, y: 100, pinned: true }],
      ]),
      placementDeviceIds: new Set(['dev-2', 'dev-1']),
      alerts: [linkDownAlert, highCpuAlert],
    });

    expect(first.signature).toBe(second.signature);
  });

  it('invalidates when a cached position map reference changes values', () => {
    const savedPositions = new Map([['dev-1', { x: 100, y: 120, pinned: true }]]);

    const first = buildKey({ savedPositions });
    savedPositions.set('dev-1', { x: 101, y: 120, pinned: true });
    const second = buildKey({ savedPositions });

    expect(first.signature).not.toBe(second.signature);
  });

  it('invalidates when a cached position map reference changes keys at the same size', () => {
    const savedPositions = new Map([['dev-1', { x: 100, y: 120, pinned: true }]]);

    const first = buildKey({ savedPositions });
    savedPositions.delete('dev-1');
    savedPositions.set('dev-2', { x: 100, y: 120, pinned: true });
    const second = buildKey({ savedPositions });

    expect(first.signature).not.toBe(second.signature);
  });

  it('invalidates when a cached placement set reference changes values at the same size', () => {
    const placementDeviceIds = new Set(['dev-1']);

    const first = buildKey({ placementDeviceIds });
    placementDeviceIds.delete('dev-1');
    placementDeviceIds.add('dev-2');
    const second = buildKey({ placementDeviceIds });

    expect(first.signature).not.toBe(second.signature);
  });

  it('invalidates when a cached alerts array reference changes content', () => {
    const alerts: AlertDTO[] = [
      {
        device_id: 'dev-1',
        alert_name: 'HighCPU',
        state: 'firing',
        severity: 'critical',
        summary: 'CPU high',
      },
    ];

    const first = buildKey({ alerts });
    alerts[0] = { ...alerts[0]!, severity: 'warning' };
    const second = buildKey({ alerts });

    expect(first.signature).not.toBe(second.signature);
  });

  it('reuses sorted mutable input signatures for unchanged references', () => {
    const nativeSort = Array.prototype.sort;
    const savedPositions = new Map([
      ['dev-2', { x: 30, y: 40, pinned: false }],
      ['dev-1', { x: 10, y: 20, pinned: true }],
    ]);
    const computedPositions = new Map([
      ['dev-2', { x: 70, y: 80 }],
      ['dev-1', { x: 50, y: 60 }],
    ]);
    const currentPositions = new Map([
      ['dev-2', { x: 110, y: 120, pinned: false }],
      ['dev-1', { x: 90, y: 100, pinned: true }],
    ]);
    const explicitPositions = new Map<string, { x: number; y: number }>();
    const placementDeviceIds = new Set(['dev-2', 'dev-1']);
    const alerts: AlertDTO[] = [
      {
        device_id: 'dev-2',
        alert_name: 'LinkDown',
        state: 'firing',
        severity: 'warning',
        summary: 'Link down',
      },
      {
        device_id: 'dev-1',
        alert_name: 'HighCPU',
        state: 'firing',
        severity: 'critical',
        summary: 'CPU high',
      },
    ];

    const sortSpy = vi
      .spyOn(Array.prototype, 'sort')
      .mockImplementation(function sortWithNative(compareFn) {
        return nativeSort.call(this, compareFn);
      });

    try {
      buildKey({
        savedPositions,
        computedPositions,
        currentPositions,
        explicitPositions,
        placementDeviceIds,
        alerts,
      });
      sortSpy.mockClear();
      buildKey({
        savedPositions,
        computedPositions,
        currentPositions,
        explicitPositions,
        placementDeviceIds,
        alerts,
      });

      expect(sortSpy).not.toHaveBeenCalled();
    } finally {
      sortSpy.mockRestore();
    }
  });

  it('invalidates when alerts change under the same topology and runtime ids', () => {
    const alert: AlertDTO = {
      device_id: 'dev-1',
      alert_name: 'HighCPU',
      state: 'firing',
      severity: 'critical',
      summary: 'CPU high',
    };

    expectCacheInvalidates({ alerts: [] }, { alerts: [alert] });
  });

  it('invalidates when Prometheus status changes under the same topology and runtime ids', () => {
    const unavailable: PrometheusStatusPayload = {
      enabled: true,
      available: false,
      error: 'connection refused',
    };

    expectCacheInvalidates({ prometheusStatus: null }, { prometheusStatus: unavailable });
  });

  it('invalidates when runtime version changes even if runtime identity is stable', () => {
    expectCacheInvalidates(
      { runtimeIdentity: 'rt-sha256:abc', runtimeVersion: 7 },
      { runtimeIdentity: 'rt-sha256:abc', runtimeVersion: 8 },
    );
  });

  it('invalidates when runtime identity changes even if runtime version is reused', () => {
    expectCacheInvalidates(
      { runtimeIdentity: 'rt-sha256:before-restart', runtimeVersion: 1 },
      { runtimeIdentity: 'rt-sha256:after-restart', runtimeVersion: 1 },
    );
  });

  it('falls back to device and link presentation signatures when server topology ids are absent', () => {
    const first = buildKey({
      topologyVersion: undefined,
      topologyEtag: null,
      devices: [mockDevice({ hostname: 'router-01' })],
    });
    const second = buildKey({
      topologyVersion: undefined,
      topologyEtag: null,
      devices: [mockDevice({ hostname: 'router-02' })],
    });

    expect(first.signature).not.toBe(second.signature);
  });

  it('uses runtime snapshot reference only when runtime identity and version are absent', () => {
    const firstSnapshot = { devices: {}, links: {} } as SnapshotPayload;
    const secondSnapshot = { devices: {}, links: {} } as SnapshotPayload;

    expectCacheInvalidates(
      { runtimeIdentity: undefined, runtimeVersion: undefined, runtimeSnapshot: firstSnapshot },
      { runtimeIdentity: undefined, runtimeVersion: undefined, runtimeSnapshot: secondSnapshot },
    );
  });
});

describe('composeCanvasTopology saved routes', () => {
  it.each([
    { editMode: false, expectedEditable: false },
    { editMode: true, expectedEditable: true },
  ])('attaches an isolated matching saved route with editability $expectedEditable', ({
    editMode,
    expectedEditable,
  }) => {
    const devices = [
      mockDevice(),
      mockDevice({
        id: 'dev-2',
        hostname: 'switch-01',
        ip: '10.0.0.2',
        sys_name: 'switch-01',
      }),
    ];
    const links = [mockLink()];
    const route = { version: 1 as const, waypoints: [{ x: 12.5, y: -8 }] };
    const onLinkRouteCommit = vi.fn();
    const runtimeState = buildRuntimeState({
      devices,
      links,
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });

    const { edges } = composeCanvasTopology({
      devices,
      links,
      linkRoutes: {
        'link-1': route,
        orphan: { version: 1, waypoints: [{ x: 99, y: 100 }] },
      },
      onLinkRouteCommit,
      runtimeState,
      savedPositions: new Map(),
      computedPositions: new Map([
        ['dev-1', { x: 100, y: 120 }],
        ['dev-2', { x: 320, y: 120 }],
      ]),
      currentPositions: new Map(),
      explicitPositions: new Map(),
      editMode,
      openDeviceMenu: vi.fn(),
      openEdgeMenu: vi.fn(),
      placementDeviceIds: new Set(['dev-1', 'dev-2']),
      alerts: [],
      snapGrid: null,
    });

    expect(edges).toHaveLength(1);
    expect(edges[0]?.data.route).toEqual(route);
    expect(edges[0]?.data.route).not.toBe(route);
    expect(edges[0]?.data.routeEditable).toBe(expectedEditable);
    expect(edges[0]?.data.onRouteCommit).toBe(onLinkRouteCommit);

    edges[0]!.data.route!.waypoints[0]!.x = 999;
    expect(route.waypoints[0]?.x).toBe(12.5);
  });
});
