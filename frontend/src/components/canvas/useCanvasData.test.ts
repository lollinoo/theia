/**
 * Exercises use canvas data topology canvas behavior so refactors preserve the documented contract.
 */
import { act, renderHook } from '@testing-library/react';
import type { ReactFlowInstance } from '@xyflow/react';
import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  type CanvasTopologyFetchResult,
  createLink,
  fetchCanvasBootstrap,
  fetchCanvasMapBootstrap,
  fetchCanvasMapTopology,
  fetchCanvasTopology,
  fetchDevices,
  fetchGrafanaDashboardConfig,
  fetchLinks,
  fetchSettings,
} from '../../api/client';
import {
  getCanvasRuntimeBootstrap,
  resetCanvasRuntimeBootstrap,
} from '../../hooks/canvasRuntimeBootstrap';
import { computeForceLayout } from '../../hooks/useAutoLayout';
import type { Area, Device } from '../../types/api';
import type { PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { exportCanvasDiagnostics, resetCanvasDiagnostics } from './canvasDiagnostics';
import {
  manualEdgeMigrationStorageKey,
  manualEdgeStorageKey,
  staleThresholdMs,
} from './canvasHelpers';
import type { CanvasMeasurementRecord } from './canvasInstrumentation';
import { buildTopologyEdges } from './edgeBuilder';
import { manualEdgeMigrationMaxAttempts } from './manualEdgeMigration';
import { useCanvasData } from './useCanvasData';

vi.mock('../../api/client', () => ({
  fetchCanvasBootstrap: vi.fn(),
  fetchCanvasMapBootstrap: vi.fn(),
  fetchCanvasMapTopology: vi.fn(),
  fetchCanvasTopology: vi.fn(),
  fetchDevices: vi.fn(),
  fetchGrafanaDashboardConfig: vi.fn(),
  fetchLinks: vi.fn(),
  fetchSettings: vi.fn(),
  createLink: vi.fn(),
}));

const positionMocks = vi.hoisted(() => {
  const fetchPositions = vi.fn();
  const savePositions = vi.fn();
  return {
    fetchPositions,
    savePositions,
    usePositions: vi.fn((_mapId: string | null) => ({
      fetchPositions,
      savePositions,
    })),
  };
});

vi.mock('../../hooks/usePositions', () => ({
  usePositions: positionMocks.usePositions,
}));

vi.mock('../../hooks/useAutoLayout', () => ({
  computeForceLayout: vi.fn(() => new Map([['dev-1', { x: 120, y: 180 }]])),
}));

vi.mock('./edgeBuilder', () => ({
  alertStatusForLink: vi.fn(() => 'normal'),
  buildEdgeData: vi.fn((link, _devicesById, existingData, onContextMenu) => ({
    link,
    ...existingData,
    onContextMenu,
  })),
  buildTopologyEdges: vi.fn(() => []),
  preferVisibleLinks: vi.fn((links) => links),
}));

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    area_ids: [],
    backup_supported: true,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    ...overrides,
  };
}

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: 'Core',
    color: '#00E676',
    device_count: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function mockSnapshot(overrides: Partial<SnapshotPayload> = {}): SnapshotPayload {
  return {
    devices: {
      'dev-1': {
        device_id: 'dev-1',
        operational_status: 'up',
        reachability: 'up',
        health: 'warning',
        freshness: 'fresh',
        primary_reason: 'ok',
        metrics_status: 'available',
        metrics_reason: 'ok',
        alert_status: 'normal',
        firing_alert_count: 0,
        last_collected_at: '2026-04-13T11:59:45Z',
        last_polled_at: '2026-04-13T11:59:45Z',
        expected_poll_interval_seconds: 60,
        cpu_percent: 42,
        mem_percent: 68,
        temp_celsius: null,
        uptime_secs: null,
      },
    },
    links: {},
    ...overrides,
  };
}

function renderUseCanvasData(
  snapshot: SnapshotPayload | null,
  prometheusStatus: PrometheusStatusPayload | null = null,
  options: {
    mapId?: string | null;
    mapName?: string;
    onDevicesChange?: (devices: Device[]) => void;
    onTopologyAreasChange?: (areas: Area[]) => void;
  } = {},
) {
  const reactFlow = {
    fitView: vi.fn(),
  } as unknown as ReactFlowInstance<DeviceNode, LinkEdgeType>;
  const openDeviceMenu = vi.fn();
  const openEdgeMenu = vi.fn();

  const rendered = renderHook(
    ({ currentSnapshot, currentMapId = null, currentMapName = 'Default' }) => {
      const [nodes, setNodes] = useState<DeviceNode[]>([]);
      const [edges, setEdges] = useState<LinkEdgeType[]>([]);

      const hook = useCanvasData({
        mapId: currentMapId,
        mapName: currentMapName,
        snapshot: currentSnapshot,
        reconnecting: false,
        prometheusStatus,
        editMode: false,
        openDeviceMenu,
        openEdgeMenu,
        reactFlow,
        nodes,
        setNodes,
        setEdges,
        onDevicesChange: options.onDevicesChange,
        onTopologyAreasChange: options.onTopologyAreasChange,
      });

      return {
        ...hook,
        nodes,
        edges,
        setNodesForTest: setNodes,
      };
    },
    {
      initialProps: {
        currentSnapshot: snapshot,
        currentMapId: options.mapId ?? null,
        currentMapName: options.mapName ?? 'Default',
      },
    },
  );

  return {
    ...rendered,
    reactFlow,
  };
}

function canvasMetrics(): CanvasMeasurementRecord[] {
  return window.__THEIA_CANVAS_METRICS__ ?? [];
}

function storedManualEdgeMigrationState(overrides: Record<string, unknown> = {}) {
  return {
    schema_version: 1,
    status: 'idle',
    attempt_count: 0,
    pending_count: 0,
    applied_count: 0,
    failed_count: 0,
    skipped_count: 0,
    applied_keys: [],
    failed_keys: [],
    ...overrides,
  };
}

function canvasTopologyOkResponse(
  overrides: Partial<Extract<CanvasTopologyFetchResult, { status: 'ok' }>['topology']> = {},
): Extract<CanvasTopologyFetchResult, { status: 'ok' }> {
  return {
    status: 'ok',
    etag: '"canvas-topology-1"',
    topology: {
      schema_version: 1,
      topology_version: 'topo-abc123',
      generated_at: '2026-04-30T12:00:00Z',
      devices: [
        mockDevice(),
        mockDevice({
          id: 'dev-2',
          hostname: 'router-02',
          ip: '10.0.0.2',
          sys_name: 'router-02',
        }),
      ],
      links: [],
      positions: {},
      areas: [],
      capabilities: {
        supports_topology_delta: false,
        supports_position_revision: false,
        supports_area_filtering: true,
      },
      settings: { layout: { version: 1 } },
      ...overrides,
    },
  };
}

function canvasBootstrapResponse(
  overrides: Partial<Extract<CanvasTopologyFetchResult, { status: 'ok' }>['topology']> = {},
): { topology: Extract<CanvasTopologyFetchResult, { status: 'ok' }>['topology'] } {
  return { topology: canvasTopologyOkResponse(overrides).topology };
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

describe('useCanvasData', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-04-13T12:00:00Z'));
    positionMocks.usePositions.mockImplementation((_mapId: string | null) => ({
      fetchPositions: positionMocks.fetchPositions,
      savePositions: positionMocks.savePositions,
    }));
    positionMocks.fetchPositions.mockResolvedValue(new Map());
    vi.mocked(fetchCanvasBootstrap).mockRejectedValue({ status: 404 });
    vi.mocked(fetchCanvasMapBootstrap).mockRejectedValue({ status: 404 });
    vi.mocked(fetchCanvasMapTopology).mockRejectedValue({ status: 404 });
    vi.mocked(fetchCanvasTopology).mockRejectedValue({ status: 404 });
    vi.mocked(fetchDevices).mockResolvedValue([mockDevice()]);
    vi.mocked(fetchLinks).mockResolvedValue([]);
    vi.mocked(fetchSettings).mockResolvedValue({});
    vi.mocked(fetchGrafanaDashboardConfig).mockResolvedValue({
      profiles: [],
      default_profile_id: '',
      device_overrides: {},
    });
    vi.mocked(createLink).mockResolvedValue(undefined as never);
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
    vi.stubGlobal('cancelAnimationFrame', vi.fn());
    window.__THEIA_CANVAS_METRICS__ = [];
    resetCanvasDiagnostics();
    resetCanvasRuntimeBootstrap();
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('uses default bootstrap when mapId is null', async () => {
    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce(canvasBootstrapResponse());

    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(positionMocks.usePositions).toHaveBeenCalledWith(null);
    expect(fetchCanvasBootstrap).toHaveBeenCalledWith({ force: false });
    expect(fetchCanvasMapBootstrap).not.toHaveBeenCalled();
  });

  it('uses saved map bootstrap on initial load when mapId is set', async () => {
    vi.mocked(fetchCanvasMapBootstrap).mockResolvedValueOnce(canvasBootstrapResponse());

    renderUseCanvasData(null, null, { mapId: 'map-1', mapName: 'Core Map' });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(positionMocks.usePositions).toHaveBeenCalledWith('map-1');
    expect(fetchCanvasMapBootstrap).toHaveBeenCalledWith('map-1', { force: false });
    expect(fetchCanvasBootstrap).not.toHaveBeenCalled();
  });

  it.each([
    { label: 'default canvas', mapId: null },
    { label: 'saved map', mapId: 'map-1' },
  ])('ignores protocol-v2 stream recovery events on $label without structural fetches or notices', async ({
    mapId,
  }) => {
    const onTopologyAreasChange = vi.fn();
    const bootstrap = canvasBootstrapResponse({
      runtime_stream_id: 'runtime-stream-1',
      runtime_version: 7,
      runtime_identity: 'rt-sha256:initial',
      runtime_snapshot: mockSnapshot(),
    });
    if (mapId === null) {
      vi.mocked(fetchCanvasBootstrap).mockResolvedValue(bootstrap);
    } else {
      vi.mocked(fetchCanvasMapBootstrap).mockResolvedValue(bootstrap);
    }
    const { result } = renderUseCanvasData(null, null, {
      mapId,
      mapName: mapId === null ? 'Default' : 'Saved Map',
      onTopologyAreasChange,
    });

    await act(async () => {
      for (let turn = 0; turn < 6; turn += 1) {
        await Promise.resolve();
      }
    });

    vi.mocked(fetchCanvasBootstrap).mockClear();
    vi.mocked(fetchCanvasMapBootstrap).mockClear();
    vi.mocked(fetchCanvasTopology).mockClear();
    vi.mocked(fetchCanvasMapTopology).mockClear();
    vi.mocked(fetchDevices).mockClear();
    vi.mocked(fetchLinks).mockClear();
    vi.mocked(fetchSettings).mockClear();
    vi.mocked(fetchGrafanaDashboardConfig).mockClear();
    positionMocks.fetchPositions.mockClear();
    positionMocks.savePositions.mockClear();
    onTopologyAreasChange.mockClear();

    await act(async () => {
      window.dispatchEvent(
        new CustomEvent('backend-resync-required', { detail: { strategy: 'stream' } }),
      );
      await vi.advanceTimersByTimeAsync(250);
      for (let turn = 0; turn < 6; turn += 1) {
        await Promise.resolve();
      }
    });

    expect(fetchCanvasBootstrap).not.toHaveBeenCalled();
    expect(fetchCanvasMapBootstrap).not.toHaveBeenCalled();
    expect(fetchCanvasTopology).not.toHaveBeenCalled();
    expect(fetchCanvasMapTopology).not.toHaveBeenCalled();
    expect(fetchDevices).not.toHaveBeenCalled();
    expect(fetchLinks).not.toHaveBeenCalled();
    expect(positionMocks.fetchPositions).not.toHaveBeenCalled();
    expect(positionMocks.savePositions).not.toHaveBeenCalled();
    expect(fetchSettings).not.toHaveBeenCalled();
    expect(fetchGrafanaDashboardConfig).not.toHaveBeenCalled();
    expect(onTopologyAreasChange).not.toHaveBeenCalled();
    expect(result.current.topologyRecoveryNotice).toBeNull();
  });

  it('continues to revalidate structure for topology changes and reconnects', async () => {
    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce(canvasBootstrapResponse());
    vi.mocked(fetchCanvasTopology)
      .mockResolvedValueOnce({ status: 'not-modified', etag: '"topo-abc123"' })
      .mockResolvedValueOnce({ status: 'not-modified', etag: '"topo-abc123"' });
    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    vi.mocked(fetchCanvasTopology).mockClear();

    await act(async () => {
      window.dispatchEvent(new Event('topology-changed'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
    });

    expect(fetchCanvasTopology).toHaveBeenCalledTimes(1);

    await act(async () => {
      window.dispatchEvent(new Event('backend-reconnected'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
    });

    expect(fetchCanvasTopology).toHaveBeenCalledTimes(2);
  });

  it('runs one merged structural follow-up after the active refresh settles', async () => {
    const activeRefresh = deferred<CanvasTopologyFetchResult>();
    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce(canvasBootstrapResponse());
    vi.mocked(fetchCanvasTopology)
      .mockReturnValueOnce(activeRefresh.promise)
      .mockResolvedValueOnce({ status: 'not-modified', etag: '"topo-abc123"' });
    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      window.dispatchEvent(new Event('backend-reconnected'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
    });

    expect(fetchCanvasTopology).toHaveBeenCalledTimes(1);

    await act(async () => {
      window.dispatchEvent(new Event('topology-changed'));
      window.dispatchEvent(new Event('backend-reconnected'));
      await vi.advanceTimersByTimeAsync(250);
    });

    expect(fetchCanvasTopology).toHaveBeenCalledTimes(1);

    await act(async () => {
      activeRefresh.resolve({ status: 'not-modified', etag: '"topo-abc123"' });
      await activeRefresh.promise;
      for (let turn = 0; turn < 4; turn += 1) {
        await Promise.resolve();
      }
    });

    expect(fetchCanvasTopology).toHaveBeenCalledTimes(2);
  });

  it('publishes topology areas from the active map load', async () => {
    const onTopologyAreasChange = vi.fn();
    const mapArea = mockArea({ id: 'map-area-1', name: 'Map Local Area' });
    vi.mocked(fetchCanvasMapBootstrap).mockResolvedValueOnce(
      canvasBootstrapResponse({ areas: [mapArea] }),
    );

    const { result } = renderUseCanvasData(null, null, {
      mapId: 'map-1',
      mapName: 'Core Map',
      onTopologyAreasChange,
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.topologyAreas).toEqual([mapArea]);
    expect(onTopologyAreasChange).toHaveBeenCalledWith([mapArea]);
  });

  it('uses saved map topology on silent refresh when mapId is set', async () => {
    vi.mocked(fetchCanvasMapBootstrap).mockResolvedValueOnce(canvasBootstrapResponse());
    vi.mocked(fetchCanvasMapTopology).mockResolvedValueOnce({
      status: 'not-modified',
      etag: '"topo-abc123"',
    });
    const { result } = renderUseCanvasData(null, null, {
      mapId: 'map-1',
      mapName: 'Core Map',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const edgeBuildCallsAfterBootstrap = vi.mocked(buildTopologyEdges).mock.calls.length;

    await act(async () => {
      await result.current.loadTopology(true);
    });

    expect(fetchCanvasMapTopology).toHaveBeenCalledWith('map-1', '"topo-abc123"');
    expect(fetchCanvasTopology).not.toHaveBeenCalled();
    expect(buildTopologyEdges).toHaveBeenCalledTimes(edgeBuildCallsAfterBootstrap);
  });

  it('reuses composed topology when repeated forced bootstraps return the same canvas input', async () => {
    vi.mocked(fetchCanvasBootstrap).mockImplementation(() =>
      Promise.resolve(
        canvasBootstrapResponse({
          runtime_version: 3,
          runtime_identity: 'rt-sha256:stable',
          runtime_snapshot: mockSnapshot(),
        }),
      ),
    );
    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const edgeBuildCallsAfterInitialLoad = vi.mocked(buildTopologyEdges).mock.calls.length;

    await act(async () => {
      window.dispatchEvent(new Event('backend-resync-required'));
      vi.advanceTimersByTime(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasBootstrap).toHaveBeenCalledTimes(2);

    const edgeBuildCallsAfterFirstResync = vi.mocked(buildTopologyEdges).mock.calls.length;
    expect(edgeBuildCallsAfterFirstResync).toBeGreaterThanOrEqual(edgeBuildCallsAfterInitialLoad);

    await act(async () => {
      window.dispatchEvent(new Event('backend-resync-required'));
      vi.advanceTimersByTime(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasBootstrap).toHaveBeenCalledTimes(3);
    expect(buildTopologyEdges).toHaveBeenCalledTimes(edgeBuildCallsAfterFirstResync);
  });

  it('treats a saved map id matching the old internal default key as distinct from the default canvas', async () => {
    const defaultDevice = mockDevice({
      id: 'default-dev',
      hostname: 'default-router',
      ip: '10.0.0.10',
      sys_name: 'default-router',
    });
    const savedMapDevice = mockDevice({
      id: 'saved-default-dev',
      hostname: 'saved-default-router',
      ip: '10.0.0.20',
      sys_name: 'saved-default-router',
    });
    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce(
      canvasBootstrapResponse({
        devices: [defaultDevice],
        topology_version: 'topo-default-bootstrap',
      }),
    );
    vi.mocked(fetchCanvasMapTopology).mockImplementation((requestedMapId: string) => {
      if (requestedMapId === '__default__') {
        return Promise.resolve(
          canvasTopologyOkResponse({
            devices: [savedMapDevice],
            topology_version: 'topo-saved-default-id',
          }),
        );
      }
      return Promise.reject(new Error(`Unexpected map id ${requestedMapId}`));
    });

    const { result, rerender } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes.map((node) => node.id)).toEqual(['default-dev']);

    rerender({
      currentSnapshot: null,
      currentMapId: '__default__',
      currentMapName: 'Saved Default',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasMapTopology).toHaveBeenCalledWith('__default__', undefined);
    expect(result.current.nodes.map((node) => node.id)).toEqual(['saved-default-dev']);
  });

  it('skips legacy manual edge migration for saved maps and retains localStorage', async () => {
    const storedEdges = [{ id: 'edge-1', source: 'dev-1', target: 'dev-2' }];
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));
    vi.mocked(fetchCanvasMapBootstrap).mockResolvedValueOnce(
      canvasBootstrapResponse({
        devices: [
          mockDevice(),
          mockDevice({
            id: 'dev-2',
            hostname: 'router-02',
            ip: '10.0.0.2',
            sys_name: 'router-02',
          }),
        ],
      }),
    );

    renderUseCanvasData(null, null, { mapId: 'map-1', mapName: 'Core Map' });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(createLink).not.toHaveBeenCalled();
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBe(JSON.stringify(storedEdges));
    expect(exportCanvasDiagnostics().events).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          event: 'manual_edges.migration.skipped_saved_map',
          metadata: expect.objectContaining({ mapId: 'map-1' }),
        }),
      ]),
    );
  });

  it('keeps cached topology ETags scoped by map key', async () => {
    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce(canvasBootstrapResponse());
    vi.mocked(fetchCanvasTopology).mockResolvedValueOnce({
      ...canvasTopologyOkResponse({ topology_version: 'topo-default' }),
      etag: '"default-etag"',
    });
    vi.mocked(fetchCanvasMapTopology)
      .mockResolvedValueOnce({
        ...canvasTopologyOkResponse({
          topology_version: 'topo-map-1',
        }),
        etag: '"map-etag"',
      })
      .mockResolvedValueOnce({
        status: 'not-modified',
        etag: '"map-etag"',
      });
    const { result, rerender } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      await result.current.loadTopology(true);
    });

    rerender({
      currentSnapshot: null,
      currentMapId: 'map-1',
      currentMapName: 'Core Map',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      await result.current.loadTopology(true);
    });

    expect(fetchCanvasTopology).toHaveBeenCalledWith('"topo-abc123"');
    expect(fetchCanvasMapTopology).toHaveBeenNthCalledWith(1, 'map-1', undefined);
    expect(fetchCanvasMapTopology).toHaveBeenNthCalledWith(2, 'map-1', '"map-etag"');
  });

  it('does not reuse a cached ETag when switching back to a map whose nodes are not rendered', async () => {
    const defaultDevice = mockDevice({
      id: 'default-dev',
      hostname: 'default-router',
      ip: '10.0.0.10',
      sys_name: 'default-router',
    });
    const mapDevice = mockDevice({
      id: 'map-dev',
      hostname: 'map-router',
      ip: '10.0.0.20',
      sys_name: 'map-router',
    });

    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce(
      canvasBootstrapResponse({
        devices: [defaultDevice],
        topology_version: 'topo-default-bootstrap',
      }),
    );
    vi.mocked(fetchCanvasTopology).mockImplementation((etag?: string) => {
      if (etag === '"default-etag"') {
        return Promise.resolve({
          status: 'not-modified',
          etag: '"default-etag"',
        });
      }

      return Promise.resolve({
        ...canvasTopologyOkResponse({
          devices: [defaultDevice],
          topology_version: 'topo-default',
        }),
        etag: '"default-etag"',
      });
    });
    vi.mocked(fetchCanvasMapTopology).mockResolvedValue({
      ...canvasTopologyOkResponse({
        devices: [mapDevice],
        topology_version: 'topo-map-1',
      }),
      etag: '"map-etag"',
    });

    const { result, rerender } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      await result.current.loadTopology(true);
    });

    rerender({
      currentSnapshot: null,
      currentMapId: 'map-1',
      currentMapName: 'Core Map',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes.map((node) => node.id)).toEqual(['map-dev']);

    rerender({
      currentSnapshot: null,
      currentMapId: null,
      currentMapName: 'Default',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasTopology).toHaveBeenLastCalledWith(undefined);
    expect(result.current.nodes.map((node) => node.id)).toEqual(['default-dev']);
  });

  it('ignores stale topology loads that resolve after the active map changes', async () => {
    const defaultLoad = deferred<ReturnType<typeof canvasBootstrapResponse>>();
    const defaultDevice = mockDevice({
      id: 'default-dev',
      hostname: 'default-router',
      ip: '10.0.0.10',
      sys_name: 'default-router',
    });
    const mapDevice = mockDevice({
      id: 'map-dev',
      hostname: 'map-router',
      ip: '10.0.0.20',
      sys_name: 'map-router',
    });
    vi.mocked(fetchCanvasBootstrap).mockReturnValueOnce(defaultLoad.promise);
    vi.mocked(fetchCanvasMapTopology).mockResolvedValueOnce({
      ...canvasTopologyOkResponse({
        devices: [mapDevice],
        topology_version: 'topo-map-1',
      }),
      etag: '"map-etag"',
    });

    const { result, rerender } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
    });

    rerender({
      currentSnapshot: null,
      currentMapId: 'map-1',
      currentMapName: 'Core Map',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes.map((node) => node.id)).toEqual(['map-dev']);

    await act(async () => {
      defaultLoad.resolve(
        canvasBootstrapResponse({
          devices: [defaultDevice],
          topology_version: 'topo-default',
        }),
      );
      await defaultLoad.promise;
      await Promise.resolve();
    });

    expect(result.current.devices.map((device) => device.id)).toEqual(['map-dev']);
    expect(result.current.nodes.map((node) => node.id)).toEqual(['map-dev']);
    expect(exportCanvasDiagnostics().diagnostics.topology).toMatchObject({
      topologyVersion: 'topo-map-1',
      lastTopologyLoadStatus: 'success',
    });
  });

  it('loads saved-map topology when the map id changes', async () => {
    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce(canvasBootstrapResponse());
    vi.mocked(fetchCanvasMapTopology).mockResolvedValueOnce(
      canvasTopologyOkResponse({
        topology_version: 'topo-map-1',
      }),
    );
    const { rerender } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    rerender({
      currentSnapshot: null,
      currentMapId: 'map-1',
      currentMapName: 'Core Map',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasMapTopology).toHaveBeenCalledWith('map-1', undefined);
  });

  it('does not reload topology when only the map name changes', async () => {
    vi.mocked(fetchCanvasMapBootstrap).mockResolvedValueOnce(canvasBootstrapResponse());
    const { rerender } = renderUseCanvasData(null, null, {
      mapId: 'map-1',
      mapName: 'Core Map',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    vi.mocked(fetchCanvasMapBootstrap).mockClear();
    vi.mocked(fetchCanvasMapTopology).mockClear();

    rerender({
      currentSnapshot: null,
      currentMapId: 'map-1',
      currentMapName: 'Renamed Core Map',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasMapBootstrap).not.toHaveBeenCalled();
    expect(fetchCanvasMapTopology).not.toHaveBeenCalled();
  });

  it('emits saved-map manual edge migration skip diagnostics once per pending payload', async () => {
    const storedEdges = [{ id: 'edge-1', source: 'dev-1', target: 'dev-2' }];
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));
    vi.mocked(fetchCanvasMapBootstrap).mockResolvedValueOnce(canvasBootstrapResponse());
    vi.mocked(fetchCanvasMapTopology).mockResolvedValue(canvasTopologyOkResponse());

    const { result } = renderUseCanvasData(null, null, {
      mapId: 'map-1',
      mapName: 'Core Map',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      await result.current.loadTopology(true);
      await result.current.loadTopology(true);
    });

    const skippedEvents = exportCanvasDiagnostics().events.filter(
      (event) => event.event === 'manual_edges.migration.skipped_saved_map',
    );
    expect(skippedEvents).toHaveLength(1);
    expect(skippedEvents[0]).toMatchObject({
      metadata: expect.objectContaining({
        reason: 'initial_load',
        silent: false,
        mapId: 'map-1',
        mapName: 'Core Map',
      }),
    });
  });

  it('keeps current node positions owned by their rendered map during map switches', async () => {
    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce(
      canvasBootstrapResponse({
        devices: [mockDevice()],
        positions: {
          'dev-1': {
            device_id: 'dev-1',
            x: 10,
            y: 20,
            pinned: true,
          },
        },
      }),
    );

    const { result, rerender } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].position).toEqual({ x: 10, y: 20 });

    positionMocks.savePositions.mockClear();
    vi.mocked(fetchCanvasMapTopology).mockResolvedValueOnce({
      ...canvasTopologyOkResponse({
        devices: [mockDevice()],
        topology_version: 'topo-map-positions',
        positions: {
          'dev-1': {
            device_id: 'dev-1',
            x: 300,
            y: 400,
            pinned: true,
          },
        },
      }),
      etag: '"map-position-etag"',
    });

    rerender({
      currentSnapshot: null,
      currentMapId: 'map-1',
      currentMapName: 'Core Map',
    });

    expect(result.current.nodes[0].position).toEqual({ x: 10, y: 20 });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasMapTopology).toHaveBeenCalledWith('map-1', undefined);
    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].position).toEqual({ x: 300, y: 400 });
    expect(positionMocks.savePositions).not.toHaveBeenCalled();
  });

  it('fits the view after switching maps even when the target map already has saved positions', async () => {
    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce(
      canvasBootstrapResponse({
        devices: [mockDevice()],
        positions: {
          'dev-1': {
            device_id: 'dev-1',
            x: 10,
            y: 20,
            pinned: true,
          },
        },
      }),
    );

    vi.mocked(fetchCanvasMapTopology).mockResolvedValueOnce({
      ...canvasTopologyOkResponse({
        devices: [mockDevice()],
        topology_version: 'topo-map-fit',
        positions: {
          'dev-1': {
            device_id: 'dev-1',
            x: 300,
            y: 400,
            pinned: true,
          },
        },
      }),
      etag: '"map-fit-etag"',
    });

    const { rerender, reactFlow } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(reactFlow.fitView).toHaveBeenCalledTimes(1);

    rerender({
      currentSnapshot: null,
      currentMapId: 'map-1',
      currentMapName: 'Core Map',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(reactFlow.fitView).toHaveBeenCalledTimes(2);
    expect(reactFlow.fitView).toHaveBeenLastCalledWith({
      padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
      duration: 320,
    });
  });

  it('records topology diagnostics with map metadata', async () => {
    vi.mocked(fetchCanvasMapBootstrap).mockResolvedValueOnce(canvasBootstrapResponse());

    renderUseCanvasData(null, null, { mapId: 'map-1', mapName: 'Core Map' });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(exportCanvasDiagnostics().events).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          event: 'topology.load.started',
          metadata: expect.objectContaining({
            reason: 'initial_load',
            silent: false,
            mapId: 'map-1',
            mapName: 'Core Map',
          }),
        }),
        expect.objectContaining({
          event: 'topology.load.succeeded',
          metadata: expect.objectContaining({
            reason: 'initial_load',
            silent: false,
            mapId: 'map-1',
            mapName: 'Core Map',
          }),
        }),
      ]),
    );
  });

  it('snapshot application keeps overview metadata attached when device status is down', async () => {
    const { result } = renderUseCanvasData(
      mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'down',
          },
        },
      }),
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].data.device.status).toBe('up');
    expect(result.current.nodes[0].data.runtime.status).toBe('down');
    expect(result.current.runtimeSummary).toEqual({
      alertCount: 0,
      prometheusDiagnosticsVisible: false,
    });
    expect(result.current.nodes[0].data.runtime.metrics).toMatchObject({
      health: 'warning',
      last_polled_at: '2026-04-13T11:59:45Z',
      expected_poll_interval_seconds: 60,
    });
  });

  it('records topology load diagnostics from the canvas read model', async () => {
    vi.mocked(fetchCanvasTopology).mockResolvedValueOnce({
      status: 'ok',
      etag: '"topo-1"',
      topology: {
        schema_version: 1,
        topology_version: 'topo-1',
        runtime_version: 1,
        generated_at: '2026-04-13T12:00:00Z',
        devices: [mockDevice()],
        links: [],
        positions: {},
        areas: [],
        capabilities: {
          supports_topology_delta: false,
          supports_position_revision: false,
          supports_area_filtering: true,
        },
        settings: { layout: { version: 1 } },
      },
    });

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.loading).toBe(false);
    expect(exportCanvasDiagnostics().diagnostics.topology).toMatchObject({
      topologyVersion: 'topo-1',
      runtimeVersion: '1',
      schemaVersion: 1,
      lastTopologyLoadReason: 'initial_load',
      lastTopologyLoadStatus: 'success',
    });
    expect(exportCanvasDiagnostics().events.map((event) => event.event)).toEqual(
      expect.arrayContaining(['topology.load.started', 'topology.load.succeeded']),
    );
  });

  it('records failed topology load diagnostics', async () => {
    vi.mocked(fetchDevices).mockRejectedValueOnce(new Error('backend offline'));

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.error).toContain('backend offline');
    expect(exportCanvasDiagnostics().diagnostics.topology).toMatchObject({
      lastTopologyLoadReason: 'initial_load',
      lastTopologyLoadStatus: 'error',
      lastTopologyLoadError: 'backend offline',
    });
    expect(exportCanvasDiagnostics().events.map((event) => event.event)).toContain(
      'topology.load.failed',
    );
  });

  it('keeps emitted devices static when runtime snapshot overrides persisted status', async () => {
    const onDevicesChange = vi.fn();

    const { result } = renderUseCanvasData(
      mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'down',
          },
        },
      }),
      null,
      { onDevicesChange },
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.devices[0]?.status).toBe('up');
    expect(result.current.nodes[0].data.runtime.status).toBe('down');
    expect(onDevicesChange).toHaveBeenLastCalledWith([
      expect.objectContaining({ id: 'dev-1', status: 'up' }),
    ]);
  });

  it('snapshot application keeps no-ip virtual placeholders unmonitored and metric-free', async () => {
    vi.mocked(fetchDevices).mockResolvedValue([
      mockDevice({
        device_type: 'virtual',
        ip: '',
        status: 'down',
        tags: { display_name: 'Internet', virtual_subtype: 'internet' },
      }),
    ]);

    const { result } = renderUseCanvasData(
      mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'down',
          },
        },
      }),
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].data.runtime.monitoringState).toBe('unmonitored');
    expect(result.current.nodes[0].data.runtime.metrics).toBeNull();
  });

  it('does not blank runtime telemetry client-side after a local stale timer expires', async () => {
    const { result } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(staleThresholdMs + 10_000);
    });

    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].data.runtime.metrics).toMatchObject({
      cpu_percent: 42,
      mem_percent: 68,
      temp_celsius: null,
      uptime_secs: null,
      health: 'warning',
      freshness: 'fresh',
      metrics_status: 'available',
      last_polled_at: '2026-04-13T11:59:45Z',
      expected_poll_interval_seconds: 60,
    });
  });

  it('does not let Prometheus status override normalized runtime status', async () => {
    const { result } = renderUseCanvasData(mockSnapshot(), { enabled: true, available: false });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].data.device.status).toBe('up');
    expect(result.current.nodes[0].data.runtime.status).toBe('up');
  });

  it('prefers normalized runtime firing alert counts over the raw alert feed', async () => {
    const { result } = renderUseCanvasData(
      mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            firing_alert_count: 3,
          },
        },
      }),
      null,
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.runtimeSummary).toEqual({
      alertCount: 3,
      prometheusDiagnosticsVisible: false,
    });
  });

  it('falls back to the raw alert feed when runtime snapshot devices are unavailable', async () => {
    const reactFlow = {
      fitView: vi.fn(),
    } as unknown as ReactFlowInstance<DeviceNode, LinkEdgeType>;
    const openDeviceMenu = vi.fn();
    const openEdgeMenu = vi.fn();

    const alerts = [
      {
        device_id: 'dev-1',
        alert_name: 'DeviceDown',
        severity: 'critical',
        state: 'firing',
        summary: 'legacy alert feed still firing',
      },
    ] as const;

    const { result } = renderHook(() => {
      const [nodes, setNodes] = useState<DeviceNode[]>([]);
      const [, setEdges] = useState<LinkEdgeType[]>([]);

      return useCanvasData({
        mapId: null,
        mapName: 'Default',
        snapshot: null,
        alerts,
        reconnecting: false,
        prometheusStatus: { enabled: true, available: false },
        editMode: false,
        openDeviceMenu,
        openEdgeMenu,
        reactFlow,
        nodes,
        setNodes,
        setEdges,
      });
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.runtimeSummary).toEqual({
      alertCount: 1,
      prometheusDiagnosticsVisible: true,
    });
  });

  it('surfaces Prometheus degradation as diagnostics without changing alert totals', async () => {
    const { result } = renderUseCanvasData(mockSnapshot(), { enabled: true, available: false });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.runtimeSummary).toEqual({
      alertCount: 0,
      prometheusDiagnosticsVisible: true,
    });
  });

  it('counts runtime-synthesized alerts in the toolbar summary', async () => {
    const { result } = renderUseCanvasData(
      mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'down',
            primary_health: 'unreachable',
            reachability: 'hard_down',
            health: 'critical',
            primary_reason: 'device_unreachable',
            metrics_status: 'unavailable',
            metrics_reason: 'device_unreachable',
            alert_status: 'normal',
            firing_alert_count: 0,
          },
        },
      }),
      null,
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.runtimeSummary).toEqual({
      alertCount: 1,
      prometheusDiagnosticsVisible: false,
    });
  });

  it('counts normalized and raw firing alerts together during mixed runtime coverage', async () => {
    vi.mocked(fetchDevices).mockResolvedValue([
      mockDevice(),
      mockDevice({
        id: 'dev-2',
        hostname: 'switch-01',
        ip: '10.0.0.2',
        sys_name: 'switch-01',
      }),
    ]);

    const reactFlow = {
      fitView: vi.fn(),
    } as unknown as ReactFlowInstance<DeviceNode, LinkEdgeType>;
    const openDeviceMenu = vi.fn();
    const openEdgeMenu = vi.fn();

    const snapshot = mockSnapshot({
      devices: {
        'dev-1': {
          ...mockSnapshot().devices['dev-1'],
          firing_alert_count: 2,
        },
      },
      links: {},
    });
    const alerts = [
      {
        device_id: 'dev-2',
        alert_name: 'DeviceDown',
        severity: 'critical',
        state: 'firing',
        summary: 'legacy alert feed still firing',
      },
    ] as const;

    const { result } = renderHook(() => {
      const [nodes, setNodes] = useState<DeviceNode[]>([]);
      const [, setEdges] = useState<LinkEdgeType[]>([]);

      return useCanvasData({
        mapId: null,
        mapName: 'Default',
        snapshot,
        alerts,
        reconnecting: false,
        prometheusStatus: null,
        editMode: false,
        openDeviceMenu,
        openEdgeMenu,
        reactFlow,
        nodes,
        setNodes,
        setEdges,
      });
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.runtimeSummary).toEqual({
      alertCount: 3,
      prometheusDiagnosticsVisible: false,
    });
  });

  it('ignores runtime alert counts for snapshot devices outside the current topology inventory', async () => {
    const snapshot = mockSnapshot({
      devices: {
        'dev-1': {
          ...mockSnapshot().devices['dev-1'],
          firing_alert_count: 1,
        },
        'dev-2': {
          ...mockSnapshot().devices['dev-1'],
          device_id: 'dev-2',
          firing_alert_count: 5,
        },
      },
    });

    const { result } = renderUseCanvasData(snapshot);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.runtimeSummary).toEqual({
      alertCount: 1,
      prometheusDiagnosticsVisible: false,
    });
  });

  it('does not create manual edge migration state when no pending edge storage exists', async () => {
    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(createLink).not.toHaveBeenCalled();
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBeNull();
    expect(window.localStorage.getItem(manualEdgeMigrationStorageKey)).toBeNull();
    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'idle',
      attemptCount: 0,
    });
  });

  it('loads persisted terminal manual edge migration failures into diagnostics', async () => {
    const terminalState = storedManualEdgeMigrationState({
      status: 'failed',
      attempt_count: manualEdgeMigrationMaxAttempts,
      pending_count: 0,
      failed_count: 1,
      failed_keys: ['dev-1::dev-2'],
      last_attempt_at: '2026-05-05T00:00:00.000Z',
      last_completed_at: '2026-05-05T00:00:01.000Z',
      last_error: 'still unavailable',
    });
    const terminalStateRaw = JSON.stringify(terminalState);
    window.localStorage.setItem(manualEdgeMigrationStorageKey, terminalStateRaw);

    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(createLink).not.toHaveBeenCalled();
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBeNull();
    expect(window.localStorage.getItem(manualEdgeMigrationStorageKey)).toBe(terminalStateRaw);
    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'failed',
      pendingCount: 0,
      failedCount: 1,
      attemptCount: manualEdgeMigrationMaxAttempts,
      lastError: 'still unavailable',
    });
  });

  it('retains only failed manual edge migrations in localStorage', async () => {
    const storedEdges = [
      { id: 'edge-1', source: 'dev-1', target: 'dev-2' },
      { id: 'edge-2', source: 'dev-2', target: 'dev-3' },
    ];
    vi.mocked(fetchDevices).mockResolvedValue([
      mockDevice(),
      mockDevice({
        id: 'dev-2',
        hostname: 'router-02',
        ip: '10.0.0.2',
        sys_name: 'router-02',
      }),
      mockDevice({
        id: 'dev-3',
        hostname: 'router-03',
        ip: '10.0.0.3',
        sys_name: 'router-03',
      }),
    ]);
    vi.mocked(createLink)
      .mockResolvedValueOnce(undefined as never)
      .mockRejectedValueOnce(new Error('backend unavailable') as never);
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));

    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(createLink).toHaveBeenCalledTimes(2);
    expect(createLink).toHaveBeenNthCalledWith(1, {
      source_device_id: 'dev-1',
      source_if_name: '',
      target_device_id: 'dev-2',
      target_if_name: '',
      migration_source: 'browser_localstorage',
    });
    expect(createLink).toHaveBeenNthCalledWith(2, {
      source_device_id: 'dev-2',
      source_if_name: '',
      target_device_id: 'dev-3',
      target_if_name: '',
      migration_source: 'browser_localstorage',
    });
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBe(
      JSON.stringify([storedEdges[1]]),
    );
    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'failed',
      pendingCount: 1,
      failedCount: 1,
      lastError: 'backend unavailable',
    });
    expect(exportCanvasDiagnostics().events.map((event) => event.event)).toContain(
      'manual_edges.migration.failed',
    );
  });

  it('clears migrated manual edge storage after all links succeed', async () => {
    const storedEdges = [{ id: 'edge-1', source: 'dev-1', target: 'dev-2' }];
    vi.mocked(fetchDevices).mockResolvedValue([
      mockDevice(),
      mockDevice({
        id: 'dev-2',
        hostname: 'router-02',
        ip: '10.0.0.2',
        sys_name: 'router-02',
      }),
    ]);
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));

    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(createLink).toHaveBeenCalledTimes(1);
    expect(createLink).toHaveBeenCalledWith({
      source_device_id: 'dev-1',
      source_if_name: '',
      target_device_id: 'dev-2',
      target_if_name: '',
      migration_source: 'browser_localstorage',
    });
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBeNull();
    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'applied',
      pendingCount: 0,
      appliedCount: 1,
    });
    expect(exportCanvasDiagnostics().events.map((event) => event.event)).toContain(
      'manual_edges.migration.applied',
    );
  });

  it('dedupes duplicate reversed manual edge migrations before creating links', async () => {
    const storedEdges = [
      { id: 'edge-1', source: 'dev-1', target: 'dev-2' },
      { id: 'edge-2', source: 'dev-2', target: 'dev-1' },
    ];
    vi.mocked(fetchDevices).mockResolvedValue([
      mockDevice(),
      mockDevice({
        id: 'dev-2',
        hostname: 'router-02',
        ip: '10.0.0.2',
        sys_name: 'router-02',
      }),
    ]);
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));

    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(createLink).toHaveBeenCalledTimes(1);
    expect(createLink).toHaveBeenCalledWith({
      source_device_id: 'dev-1',
      source_if_name: '',
      target_device_id: 'dev-2',
      target_if_name: '',
      migration_source: 'browser_localstorage',
    });
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBeNull();
  });

  it('skips stale pending manual edges that were already applied', async () => {
    const storedEdges = [{ id: 'edge-1', source: 'dev-1', target: 'dev-2' }];
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));
    window.localStorage.setItem(
      manualEdgeMigrationStorageKey,
      JSON.stringify(
        storedManualEdgeMigrationState({
          status: 'applied',
          attempt_count: 1,
          applied_count: 1,
          applied_keys: ['dev-1::dev-2'],
          last_attempt_at: '2026-05-05T00:00:00.000Z',
          last_completed_at: '2026-05-05T00:00:01.000Z',
        }),
      ),
    );

    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(createLink).not.toHaveBeenCalled();
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBeNull();
    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'applied',
      pendingCount: 0,
      appliedCount: 1,
      skippedCount: 1,
    });
    expect(exportCanvasDiagnostics().events.map((event) => event.event)).toContain(
      'manual_edges.migration.skipped',
    );
  });

  it('retries failed manual edge migrations on repeated topology load and then clears storage', async () => {
    const storedEdges = [{ id: 'edge-1', source: 'dev-1', target: 'dev-2' }];
    vi.mocked(createLink)
      .mockRejectedValueOnce(new Error('backend unavailable') as never)
      .mockResolvedValueOnce(undefined as never);
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'failed',
      pendingCount: 1,
      failedCount: 1,
      attemptCount: 1,
      lastError: 'backend unavailable',
    });

    await act(async () => {
      await result.current.loadTopology(true);
    });

    expect(createLink).toHaveBeenCalledTimes(2);
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBeNull();
    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'applied',
      pendingCount: 0,
      failedCount: 0,
      attemptCount: 2,
    });
  });

  it('bypasses cached read model ETags to retry pending manual edge migrations', async () => {
    const storedEdges = [{ id: 'edge-1', source: 'dev-1', target: 'dev-2' }];
    const topologyOkResponse = canvasTopologyOkResponse();
    vi.mocked(fetchCanvasTopology).mockImplementation((etag?: string) => {
      if (etag === '"canvas-topology-1"') {
        return Promise.resolve({
          status: 'not-modified',
          etag: '"canvas-topology-1"',
        });
      }

      return Promise.resolve(topologyOkResponse);
    });
    vi.mocked(createLink)
      .mockRejectedValueOnce(new Error('backend unavailable') as never)
      .mockResolvedValueOnce(undefined as never);
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasTopology).toHaveBeenLastCalledWith(undefined);
    expect(createLink).toHaveBeenCalledTimes(1);
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBe(JSON.stringify(storedEdges));

    await act(async () => {
      await result.current.loadTopology(true);
    });

    expect(fetchCanvasTopology).toHaveBeenLastCalledWith(undefined);
    expect(createLink).toHaveBeenCalledTimes(2);
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBeNull();
    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'applied',
      pendingCount: 0,
      attemptCount: 2,
    });
  });

  it('stops bypassing cached read model ETags after migration retry limit', async () => {
    const storedEdges = [{ id: 'edge-1', source: 'dev-1', target: 'dev-2' }];
    const topologyOkResponse = canvasTopologyOkResponse();
    vi.mocked(fetchCanvasTopology).mockImplementation((etag?: string) => {
      if (etag === '"canvas-topology-1"') {
        return Promise.resolve({
          status: 'not-modified',
          etag: '"canvas-topology-1"',
        });
      }

      return Promise.resolve(topologyOkResponse);
    });
    vi.mocked(createLink).mockRejectedValueOnce(new Error('still unavailable') as never);
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));
    window.localStorage.setItem(
      manualEdgeMigrationStorageKey,
      JSON.stringify(
        storedManualEdgeMigrationState({
          status: 'failed',
          attempt_count: manualEdgeMigrationMaxAttempts - 1,
          pending_count: 1,
          failed_count: 1,
          failed_keys: ['dev-1::dev-2'],
          last_error: 'backend unavailable',
        }),
      ),
    );

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasTopology).toHaveBeenLastCalledWith(undefined);
    expect(createLink).toHaveBeenCalledTimes(1);
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBeNull();
    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'failed',
      pendingCount: 0,
      failedCount: 1,
      attemptCount: manualEdgeMigrationMaxAttempts,
      lastError: 'still unavailable',
    });

    await act(async () => {
      await result.current.loadTopology(true);
    });

    expect(fetchCanvasTopology).toHaveBeenLastCalledWith('"canvas-topology-1"');
    expect(createLink).toHaveBeenCalledTimes(1);
  });

  it('does not recreate applied manual edges during backend resync refresh', async () => {
    const storedEdges = [{ id: 'edge-1', source: 'dev-1', target: 'dev-2' }];
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));

    renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(createLink).toHaveBeenCalledTimes(1);

    await act(async () => {
      window.dispatchEvent(new Event('backend-resync-required'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(createLink).toHaveBeenCalledTimes(1);
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBeNull();
  });

  it('preserves an unsaved in-memory node position across silent refreshes', async () => {
    vi.mocked(fetchDevices)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([mockDevice()])
      .mockResolvedValueOnce([mockDevice()]);

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      await result.current.loadTopology(true, { x: 400, y: 500 });
    });

    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].position).toEqual({ x: 400, y: 500 });

    await act(async () => {
      await result.current.loadTopology(true);
    });

    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].position).toEqual({ x: 400, y: 500 });
  });

  it('does not fetch topology or rerun layout for runtime-only snapshot updates', async () => {
    const { rerender } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    vi.mocked(fetchDevices).mockClear();
    vi.mocked(fetchLinks).mockClear();
    vi.mocked(computeForceLayout).mockClear();
    vi.mocked(buildTopologyEdges).mockClear();

    rerender({
      currentSnapshot: mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'down',
          },
        },
      }),
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchDevices).not.toHaveBeenCalled();
    expect(fetchLinks).not.toHaveBeenCalled();
    expect(computeForceLayout).not.toHaveBeenCalled();
    expect(buildTopologyEdges).not.toHaveBeenCalled();
  });

  it('uses the canvas topology read model when available and skips legacy roundtrips', async () => {
    vi.mocked(fetchCanvasTopology).mockResolvedValue({
      status: 'ok',
      etag: '"canvas-topology-1"',
      topology: {
        schema_version: 1,
        topology_version: 'topo-abc123',
        generated_at: '2026-04-30T12:00:00Z',
        devices: [mockDevice()],
        links: [],
        positions: {
          'dev-1': {
            device_id: 'dev-1',
            x: 222,
            y: 333,
            pinned: true,
          },
        },
        areas: [],
        capabilities: {
          supports_topology_delta: false,
          supports_position_revision: false,
          supports_area_filtering: true,
        },
        settings: { layout: { version: 1 } },
      },
    });

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasTopology).toHaveBeenCalledWith(undefined);
    expect(fetchDevices).not.toHaveBeenCalled();
    expect(fetchLinks).not.toHaveBeenCalled();
    expect(positionMocks.fetchPositions).not.toHaveBeenCalled();
    expect(result.current.nodes[0].position).toEqual({ x: 222, y: 333 });
  });

  it('uses the full canvas bootstrap on initial load and publishes runtime base for websocket', async () => {
    vi.mocked(fetchCanvasBootstrap).mockResolvedValue({
      topology: {
        schema_version: 1,
        topology_version: 'topo-abc123',
        runtime_version: 42,
        runtime_identity: 'rt-sha256:abc',
        runtime_snapshot: mockSnapshot({
          devices: {
            'dev-1': {
              ...mockSnapshot().devices['dev-1'],
              operational_status: 'down',
              primary_health: 'unreachable',
            },
          },
        }),
        generated_at: '2026-04-30T12:00:00Z',
        devices: [mockDevice()],
        links: [],
        positions: {},
        areas: [],
        capabilities: {
          supports_topology_delta: false,
          supports_position_revision: false,
          supports_area_filtering: true,
        },
        settings: { layout: { version: 1 } },
      },
    });

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasBootstrap).toHaveBeenCalledTimes(1);
    expect(fetchCanvasTopology).not.toHaveBeenCalled();
    expect(result.current.nodes[0].data.device.status).toBe('up');
    expect(result.current.nodes[0].data.runtime.status).toBe('down');
    expect(getCanvasRuntimeBootstrap()).toMatchObject({
      runtimeVersion: 42,
      runtimeIdentity: 'rt-sha256:abc',
    });
    expect(getCanvasRuntimeBootstrap()?.snapshot.devices['dev-1'].operational_status).toBe('down');
  });

  it('does not let a stale topology load replace the active runtime cursor or snapshot', async () => {
    const initialBootstrap = deferred<ReturnType<typeof canvasBootstrapResponse>>();
    const activeSnapshot = mockSnapshot({
      devices: {
        'dev-1': {
          ...mockSnapshot().devices['dev-1'],
          cpu_percent: 84,
        },
      },
    });
    vi.mocked(fetchCanvasBootstrap).mockReturnValueOnce(initialBootstrap.promise);
    vi.mocked(fetchCanvasMapTopology)
      .mockResolvedValueOnce(
        canvasTopologyOkResponse({
          topology_version: 'topo-primary-map',
          runtime_stream_id: 'runtime-stream-primary',
          runtime_version: 7,
          runtime_identity: 'rt-sha256:primary',
          runtime_snapshot: activeSnapshot,
        }),
      )
      .mockResolvedValueOnce(
        canvasTopologyOkResponse({
          topology_version: 'topo-primary-map',
        }),
      );

    const { result, rerender } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
    });

    await act(async () => {
      rerender({
        currentSnapshot: null,
        currentMapId: 'primary-map',
        currentMapName: 'Primary Map',
      });
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes[0]?.data.runtime.metrics).toEqual(
      expect.objectContaining({ cpu_percent: 84 }),
    );

    await act(async () => {
      initialBootstrap.resolve(
        canvasBootstrapResponse({
          topology_version: 'topo-stale-default',
          runtime_stream_id: 'runtime-stream-stale',
          runtime_version: 42,
          runtime_identity: 'rt-sha256:stale',
          runtime_snapshot: mockSnapshot({
            devices: {
              'dev-1': {
                ...mockSnapshot().devices['dev-1'],
                cpu_percent: 13,
              },
            },
          }),
        }),
      );
      await initialBootstrap.promise;
      await Promise.resolve();
    });

    expect(fetchCanvasMapTopology).toHaveBeenCalledWith('primary-map', undefined);

    await act(async () => {
      await result.current.loadTopology(true);
    });

    expect(fetchCanvasMapTopology).toHaveBeenCalledTimes(2);
    expect(result.current.nodes[0]?.data.runtime.metrics).toEqual(
      expect.objectContaining({ cpu_percent: 84 }),
    );
    expect(getCanvasRuntimeBootstrap()).toMatchObject({
      runtimeStreamId: 'runtime-stream-primary',
      runtimeVersion: 7,
      runtimeIdentity: 'rt-sha256:primary',
    });
  });

  it('forces runtime bootstrap on backend resync after manual edge migration', async () => {
    const storedEdges = [{ id: 'edge-1', source: 'dev-1', target: 'dev-2' }];
    window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(storedEdges));
    const dev2 = mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    });
    const bootstrapResponse = (runtimeVersion: number, runtimeIdentity: string, cpu: number) => ({
      topology: {
        schema_version: 1,
        topology_version: `topo-${runtimeVersion}`,
        runtime_version: runtimeVersion,
        runtime_identity: runtimeIdentity,
        runtime_snapshot: mockSnapshot({
          devices: {
            'dev-1': {
              ...mockSnapshot().devices['dev-1'],
              cpu_percent: cpu,
            },
          },
        }),
        generated_at: '2026-04-30T12:00:00Z',
        devices: [mockDevice(), dev2],
        links: [],
        positions: {},
        areas: [],
        capabilities: {
          supports_topology_delta: false,
          supports_position_revision: false,
          supports_area_filtering: true,
        },
        settings: { layout: { version: 1 } },
      },
    });
    vi.mocked(fetchCanvasBootstrap)
      .mockResolvedValueOnce(bootstrapResponse(1, 'rt-sha256:initial', 42))
      .mockResolvedValueOnce(bootstrapResponse(2, 'rt-sha256:resynced', 84));

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasBootstrap).toHaveBeenNthCalledWith(1, { force: false });
    expect(createLink).toHaveBeenCalledTimes(1);
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBeNull();
    expect(getCanvasRuntimeBootstrap()).toMatchObject({
      runtimeVersion: 1,
      runtimeIdentity: 'rt-sha256:initial',
    });

    await act(async () => {
      window.dispatchEvent(
        new CustomEvent('backend-resync-required', { detail: { reason: 'legacy-backend' } }),
      );
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasBootstrap).toHaveBeenNthCalledWith(2, { force: true });
    expect(createLink).toHaveBeenCalledTimes(1);
    expect(getCanvasRuntimeBootstrap()).toMatchObject({
      runtimeVersion: 2,
      runtimeIdentity: 'rt-sha256:resynced',
    });
    expect(result.current.nodes.find((node) => node.id === 'dev-1')?.data.runtime.metrics).toEqual(
      expect.objectContaining({ cpu_percent: 84 }),
    );
  });

  it('refreshes composed runtime data when backend resync changes only runtime identity', async () => {
    const bootstrapResponse = (runtimeIdentity: string, cpu: number) => ({
      topology: {
        schema_version: 1,
        topology_version: 'topo-stable',
        runtime_version: 1,
        runtime_identity: runtimeIdentity,
        runtime_snapshot: mockSnapshot({
          devices: {
            'dev-1': {
              ...mockSnapshot().devices['dev-1'],
              cpu_percent: cpu,
            },
          },
        }),
        generated_at: '2026-04-30T12:00:00Z',
        devices: [mockDevice()],
        links: [],
        positions: {},
        areas: [],
        capabilities: {
          supports_topology_delta: false,
          supports_position_revision: false,
          supports_area_filtering: true,
        },
        settings: { layout: { version: 1 } },
      },
    });
    vi.mocked(fetchCanvasBootstrap)
      .mockResolvedValueOnce(bootstrapResponse('rt-sha256:before-restart', 42))
      .mockResolvedValueOnce(bootstrapResponse('rt-sha256:after-restart', 84));

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasBootstrap).toHaveBeenNthCalledWith(1, { force: false });
    expect(getCanvasRuntimeBootstrap()).toMatchObject({
      runtimeVersion: 1,
      runtimeIdentity: 'rt-sha256:before-restart',
    });
    expect(result.current.nodes.find((node) => node.id === 'dev-1')?.data.runtime.metrics).toEqual(
      expect.objectContaining({ cpu_percent: 42 }),
    );

    await act(async () => {
      window.dispatchEvent(new Event('backend-resync-required'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchCanvasBootstrap).toHaveBeenNthCalledWith(2, { force: true });
    expect(getCanvasRuntimeBootstrap()).toMatchObject({
      runtimeVersion: 1,
      runtimeIdentity: 'rt-sha256:after-restart',
    });
    expect(result.current.nodes.find((node) => node.id === 'dev-1')?.data.runtime.metrics).toEqual(
      expect.objectContaining({ cpu_percent: 84 }),
    );
  });

  it('skips structural recomposition when the canvas read model is not modified', async () => {
    vi.mocked(fetchCanvasTopology)
      .mockResolvedValueOnce({
        status: 'ok',
        etag: '"canvas-topology-1"',
        topology: {
          schema_version: 1,
          topology_version: 'topo-abc123',
          generated_at: '2026-04-30T12:00:00Z',
          devices: [mockDevice()],
          links: [],
          positions: {},
          areas: [],
          capabilities: {
            supports_topology_delta: false,
            supports_position_revision: false,
            supports_area_filtering: true,
          },
          settings: { layout: { version: 1 } },
        },
      })
      .mockResolvedValueOnce({
        status: 'not-modified',
        etag: '"canvas-topology-1"',
      });

    const { result } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    vi.mocked(buildTopologyEdges).mockClear();

    await act(async () => {
      await result.current.loadTopology(true);
    });

    expect(fetchCanvasTopology).toHaveBeenLastCalledWith('"canvas-topology-1"');
    expect(buildTopologyEdges).not.toHaveBeenCalled();
    expect(result.current.nodes).toHaveLength(1);
  });

  it('preserves measured node dimensions across runtime-only snapshot updates', async () => {
    const { result, rerender } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      result.current.setNodesForTest((currentNodes) =>
        currentNodes.map((node) => ({
          ...node,
          width: 268,
          height: 142,
          measured: { width: 268, height: 142 },
        })),
      );
    });

    rerender({
      currentSnapshot: mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            cpu_percent: 83,
            mem_percent: 72,
          },
        },
      }),
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes[0]).toMatchObject({
      width: 268,
      height: 142,
      measured: { width: 268, height: 142 },
    });
  });

  it('owns drag position persistence by patching canonical nodes in useCanvasData', async () => {
    const { result } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    positionMocks.savePositions.mockClear();

    await act(async () => {
      result.current.updateNodePosition('dev-1', { x: 321, y: 654 });
    });

    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0]).toMatchObject({
      id: 'dev-1',
      position: { x: 321, y: 654 },
      data: { pinned: true },
    });
    expect(positionMocks.savePositions).toHaveBeenCalledWith([
      { device_id: 'dev-1', x: 321, y: 654, pinned: true },
    ]);
  });

  it('ignores retained drag callbacks after the active map changes before the next map finishes loading', async () => {
    const mapLoad = deferred<CanvasTopologyFetchResult>();
    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce(
      canvasBootstrapResponse({
        devices: [mockDevice()],
        positions: {
          'dev-1': {
            device_id: 'dev-1',
            x: 10,
            y: 20,
            pinned: true,
          },
        },
      }),
    );
    vi.mocked(fetchCanvasMapTopology).mockReturnValueOnce(mapLoad.promise);

    const { result, rerender } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const staleUpdateNodePosition = result.current.updateNodePosition;
    expect(result.current.nodes[0].position).toEqual({ x: 10, y: 20 });
    positionMocks.savePositions.mockClear();

    rerender({
      currentSnapshot: null,
      currentMapId: 'map-1',
      currentMapName: 'Core Map',
    });

    await act(async () => {
      await Promise.resolve();
    });

    await act(async () => {
      staleUpdateNodePosition('dev-1', { x: 321, y: 654 });
    });

    expect(positionMocks.savePositions).not.toHaveBeenCalled();
    expect(result.current.nodes[0].position).toEqual({ x: 10, y: 20 });

    await act(async () => {
      mapLoad.resolve(
        canvasTopologyOkResponse({
          devices: [
            mockDevice({
              id: 'map-dev',
              hostname: 'map-router',
              ip: '10.0.0.20',
              sys_name: 'map-router',
            }),
          ],
          topology_version: 'topo-map-1',
        }),
      );
      await mapLoad.promise;
      await Promise.resolve();
    });

    expect(result.current.nodes.map((node) => node.id)).toEqual(['map-dev']);
  });

  it('ignores ghost node move requests before they can mutate canonical state', async () => {
    const { result } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      result.current.setNodesForTest([
        {
          ...result.current.nodes[0],
          id: 'ghost-dev-1',
          position: { x: 10, y: 20 },
          data: {
            ...result.current.nodes[0].data,
            kind: 'ghost-device',
            isGhost: true,
            pinned: false,
          },
        },
      ]);
    });
    positionMocks.savePositions.mockClear();

    await act(async () => {
      result.current.updateNodePosition('ghost-dev-1', { x: 321, y: 654 });
    });

    expect(result.current.nodes[0]).toMatchObject({
      id: 'ghost-dev-1',
      position: { x: 10, y: 20 },
      data: { kind: 'ghost-device', isGhost: true, pinned: false },
    });
    expect(positionMocks.savePositions).not.toHaveBeenCalled();
  });

  it('shows reconnect-only recovery copy when reconnect is the sole structural cause', async () => {
    const { result } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      window.dispatchEvent(new Event('backend-reconnected'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.topologyRecoveryNotice).toMatchObject({
      tone: 'success',
      message: 'Topology refreshed after reconnect',
    });
  });

  it('does not apply a recovery notice from a stale structural refresh after the active map changes', async () => {
    const map1Refresh = deferred<CanvasTopologyFetchResult>();
    const map1Device = mockDevice({
      id: 'map-1-dev',
      hostname: 'map-1-router',
      ip: '10.0.0.11',
      sys_name: 'map-1-router',
    });
    const map2Device = mockDevice({
      id: 'map-2-dev',
      hostname: 'map-2-router',
      ip: '10.0.0.12',
      sys_name: 'map-2-router',
    });

    vi.mocked(fetchCanvasMapBootstrap).mockResolvedValueOnce(
      canvasBootstrapResponse({
        devices: [map1Device],
        topology_version: 'topo-map-1-bootstrap',
      }),
    );
    vi.mocked(fetchCanvasMapTopology).mockImplementation((requestedMapId: string) => {
      if (requestedMapId === 'map-1') {
        return map1Refresh.promise;
      }
      if (requestedMapId === 'map-2') {
        return Promise.resolve(
          canvasTopologyOkResponse({
            devices: [map2Device],
            topology_version: 'topo-map-2',
          }),
        );
      }
      return Promise.reject(new Error(`Unexpected map id ${requestedMapId}`));
    });

    const { result, rerender } = renderUseCanvasData(null, null, {
      mapId: 'map-1',
      mapName: 'Map 1',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes.map((node) => node.id)).toEqual(['map-1-dev']);

    await act(async () => {
      window.dispatchEvent(new Event('backend-reconnected'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
    });

    rerender({
      currentSnapshot: null,
      currentMapId: 'map-2',
      currentMapName: 'Map 2',
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes.map((node) => node.id)).toEqual(['map-2-dev']);

    await act(async () => {
      map1Refresh.resolve(
        canvasTopologyOkResponse({
          devices: [
            mockDevice({
              id: 'stale-map-1-dev',
              hostname: 'stale-map-1-router',
              ip: '10.0.0.21',
              sys_name: 'stale-map-1-router',
            }),
          ],
          topology_version: 'topo-map-1-stale',
        }),
      );
      await map1Refresh.promise;
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes.map((node) => node.id)).toEqual(['map-2-dev']);
    expect(result.current.topologyRecoveryNotice).toBeNull();
  });

  it('applies restored backend positions after reconnect when topology is unchanged', async () => {
    positionMocks.fetchPositions
      .mockResolvedValueOnce(new Map([['dev-1', { x: 10, y: 20, pinned: true }]]))
      .mockResolvedValueOnce(new Map([['dev-1', { x: 110.5, y: 220.25, pinned: true }]]));

    const { result } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes[0].position).toEqual({ x: 10, y: 20 });

    await act(async () => {
      window.dispatchEvent(new Event('backend-reconnected'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(positionMocks.fetchPositions).toHaveBeenCalledTimes(2);
    expect(result.current.nodes[0].position).toEqual({ x: 110.5, y: 220.25 });
    expect(result.current.nodes[0].data.pinned).toBe(true);
  });

  it('uses structural recovery copy for a legacy backend resync', async () => {
    const { result } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      window.dispatchEvent(new Event('backend-resync-required'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.topologyRecoveryNotice).toMatchObject({
      tone: 'success',
      message: 'Topology refreshed after backend resync',
    });
  });

  it('only places newly added devices and preserves existing positioned nodes', async () => {
    vi.mocked(fetchDevices)
      .mockResolvedValueOnce([mockDevice()])
      .mockResolvedValueOnce([
        mockDevice(),
        mockDevice({
          id: 'dev-2',
          hostname: 'router-02',
          ip: '10.0.0.2',
          sys_name: 'router-02',
        }),
      ]);
    vi.mocked(computeForceLayout)
      .mockReturnValueOnce(new Map([['dev-1', { x: 120, y: 180 }]]))
      .mockReturnValueOnce(new Map([['dev-2', { x: 320, y: 420 }]]));

    const { result, reactFlow } = renderUseCanvasData(null);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    vi.mocked(computeForceLayout).mockClear();
    expect(result.current.nodes.find((node) => node.id === 'dev-1')?.position).toEqual({
      x: 120,
      y: 180,
    });

    await act(async () => {
      await result.current.loadTopology(true);
    });

    expect(computeForceLayout).toHaveBeenCalledTimes(1);
    expect(vi.mocked(computeForceLayout).mock.calls[0]?.[0]).toEqual([
      expect.objectContaining({ id: 'dev-2' }),
    ]);
    expect(result.current.nodes.find((node) => node.id === 'dev-1')?.position).toEqual({
      x: 120,
      y: 180,
    });
    expect(result.current.nodes.find((node) => node.id === 'dev-2')?.position).toEqual({
      x: 320,
      y: 420,
    });
    expect(reactFlow.fitView).toHaveBeenCalledTimes(1);
    expect(reactFlow.fitView).toHaveBeenCalledWith({
      padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
      duration: 320,
    });
  });

  it('coalesces reconnect, resync, and topology-changed events into one structural refresh pass', async () => {
    const { result } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    vi.mocked(fetchDevices).mockClear();
    vi.mocked(fetchLinks).mockClear();

    await act(async () => {
      window.dispatchEvent(new Event('backend-reconnected'));
      window.dispatchEvent(new Event('backend-resync-required'));
      window.dispatchEvent(new Event('topology-changed'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchDevices).toHaveBeenCalledTimes(1);
    expect(fetchLinks).toHaveBeenCalledTimes(1);
    expect(result.current.topologyRecoveryNotice).toMatchObject({
      tone: 'success',
      message: 'Topology refreshed',
    });
  });

  it('keeps the current graph visible and shows a retry notice when structural refresh fails', async () => {
    const { result } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes).toHaveLength(1);
    vi.mocked(fetchDevices).mockRejectedValueOnce(new Error('backend unavailable') as never);

    await act(async () => {
      window.dispatchEvent(new Event('backend-reconnected'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.error).toBeNull();
    expect(result.current.topologyRecoveryNotice).toMatchObject({
      tone: 'warning',
      message: 'Live topology refresh delayed',
      actionLabel: 'Retry topology refresh',
    });
  });

  it('records stable topology and snapshot measurements without relayout on unchanged reconnects', async () => {
    const { rerender, reactFlow } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(canvasMetrics()).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ name: 'theia:canvas:topology-load', trigger: 'initial_load' }),
        expect.objectContaining({ name: 'theia:canvas:layout', trigger: 'initial_load' }),
        expect.objectContaining({ name: 'theia:canvas:snapshot-apply', trigger: 'snapshot' }),
      ]),
    );

    await act(async () => {
      window.dispatchEvent(new Event('backend-reconnected'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(canvasMetrics()).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          name: 'theia:canvas:topology-load',
          trigger: 'backend_reconnected',
        }),
      ]),
    );
    expect(canvasMetrics()).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({ name: 'theia:canvas:layout', trigger: 'backend_reconnected' }),
      ]),
    );
    expect(reactFlow.fitView).toHaveBeenCalledTimes(1);

    rerender({
      currentSnapshot: mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'down',
          },
        },
      }),
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(canvasMetrics()).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ name: 'theia:canvas:snapshot-apply', trigger: 'snapshot' }),
      ]),
    );
    expect(canvasMetrics().every((measurement) => typeof measurement.durationMs === 'number')).toBe(
      true,
    );
  });
});
