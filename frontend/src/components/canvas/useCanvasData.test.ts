import { act, renderHook } from '@testing-library/react';
import type { ReactFlowInstance } from '@xyflow/react';
import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  createLink,
  fetchDevices,
  fetchLinks,
  fetchSettings,
} from '../../api/client';
import { computeForceLayout } from '../../hooks/useAutoLayout';
import type { Device } from '../../types/api';
import type { PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import type { CanvasMeasurementRecord } from './canvasInstrumentation';
import { manualEdgeStorageKey, staleThresholdMs } from './canvasHelpers';
import { useCanvasData } from './useCanvasData';

vi.mock('../../api/client', () => ({
  fetchDevices: vi.fn(),
  fetchLinks: vi.fn(),
  fetchSettings: vi.fn(),
  createLink: vi.fn(),
}));

vi.mock('../../hooks/usePositions', () => ({
  usePositions: () => ({
    fetchPositions: vi.fn().mockResolvedValue(new Map()),
    savePositions: vi.fn(),
  }),
}));

vi.mock('../../hooks/useAutoLayout', () => ({
  computeForceLayout: vi.fn(() => new Map([['dev-1', { x: 120, y: 180 }]])),
}));

vi.mock('./edgeBuilder', () => ({
  alertStatusForLink: vi.fn(() => 'normal'),
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

function mockSnapshot(overrides: Partial<SnapshotPayload> = {}): SnapshotPayload {
  return {
    device_metrics: {
      'dev-1': {
        device_id: 'dev-1',
        cpu_percent: 42,
        mem_percent: 68,
        temp_celsius: 55,
        uptime_secs: 86400,
        collected_at: '2026-04-13T11:59:45Z',
        health: 'warning',
        stale: false,
        last_polled_at: '2026-04-13T11:59:30Z',
        expected_poll_interval_seconds: 30,
      },
    },
    link_metrics: {},
    alerts: [],
    device_statuses: {
      'dev-1': 'up',
    },
    device_hostnames: {},
    device_models: {},
    ...overrides,
  };
}

function renderUseCanvasData(snapshot: SnapshotPayload | null, prometheusStatus: PrometheusStatusPayload | null = null) {
  const reactFlow = {
    fitView: vi.fn(),
  } as unknown as ReactFlowInstance<DeviceNode, LinkEdgeType>;

  const rendered = renderHook(({ currentSnapshot }) => {
    const [nodes, setNodes] = useState<DeviceNode[]>([]);
    const [edges, setEdges] = useState<LinkEdgeType[]>([]);

    const hook = useCanvasData({
      snapshot: currentSnapshot,
      reconnecting: false,
      prometheusStatus,
      editMode: false,
      openDeviceMenu: vi.fn(),
      openEdgeMenu: vi.fn(),
      reactFlow,
      nodes,
      setNodes,
      setEdges,
    });

    return {
      ...hook,
      nodes,
      edges,
    };
  }, {
    initialProps: {
      currentSnapshot: snapshot,
    },
  });

  return {
    ...rendered,
    reactFlow,
  };
}

function canvasMetrics(): CanvasMeasurementRecord[] {
  return window.__THEIA_CANVAS_METRICS__ ?? [];
}

describe('useCanvasData', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-04-13T12:00:00Z'));
    vi.mocked(fetchDevices).mockResolvedValue([mockDevice()]);
    vi.mocked(fetchLinks).mockResolvedValue([]);
    vi.mocked(fetchSettings).mockResolvedValue({});
    vi.mocked(createLink).mockResolvedValue(undefined as never);
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
    vi.stubGlobal('cancelAnimationFrame', vi.fn());
    window.__THEIA_CANVAS_METRICS__ = [];
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('snapshot application keeps overview metadata attached when device status is down', async () => {
    const { result } = renderUseCanvasData(
      mockSnapshot({
        device_statuses: {
          'dev-1': 'down',
        },
      }),
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].data.device.status).toBe('down');
    expect(result.current.nodes[0].data.metrics).toMatchObject({
      health: 'warning',
      last_polled_at: '2026-04-13T11:59:30Z',
      expected_poll_interval_seconds: 30,
    });
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
        device_statuses: {
          'dev-1': 'down',
        },
      }),
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].data.monitoringState).toBe('unmonitored');
    expect(result.current.nodes[0].data.metrics).toBeNull();
  });

  it('stale fallback blanks numeric metrics but preserves health freshness and cadence metadata', async () => {
    const { result } = renderUseCanvasData(mockSnapshot());

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(staleThresholdMs + 10_000);
    });

    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].data.metrics).toMatchObject({
      cpu_percent: null,
      mem_percent: null,
      temp_celsius: null,
      uptime_secs: null,
      health: 'warning',
      stale: false,
      last_polled_at: '2026-04-13T11:59:30Z',
      expected_poll_interval_seconds: 30,
    });
  });

  it('does not force Prometheus devices down when Prometheus is disabled', async () => {
    const { result } = renderUseCanvasData(
      mockSnapshot(),
      { enabled: false, available: false },
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.nodes).toHaveLength(1);
    expect(result.current.nodes[0].data.device.status).toBe('up');
  });

  it('retains only failed manual edge migrations in localStorage', async () => {
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
    expect(window.localStorage.getItem(manualEdgeStorageKey)).toBe(
      JSON.stringify([storedEdges[1]]),
    );
  });

  it('clears migrated manual edge storage after all links succeed', async () => {
    const storedEdges = [
      { id: 'edge-1', source: 'dev-1', target: 'dev-2' },
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

    rerender({
      currentSnapshot: mockSnapshot({
        device_statuses: {
          'dev-1': 'down',
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

  it('shows resync-only recovery copy when resync is the sole structural cause', async () => {
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
      message: 'Live topology resynced',
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

    expect(canvasMetrics()).toEqual(expect.arrayContaining([
      expect.objectContaining({ name: 'theia:canvas:topology-load', trigger: 'initial_load' }),
      expect.objectContaining({ name: 'theia:canvas:layout', trigger: 'initial_load' }),
      expect.objectContaining({ name: 'theia:canvas:snapshot-apply', trigger: 'snapshot' }),
    ]));

    await act(async () => {
      window.dispatchEvent(new Event('backend-reconnected'));
      await vi.advanceTimersByTimeAsync(250);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(canvasMetrics()).toEqual(expect.arrayContaining([
      expect.objectContaining({ name: 'theia:canvas:topology-load', trigger: 'backend_reconnected' }),
    ]));
    expect(canvasMetrics()).not.toEqual(expect.arrayContaining([
      expect.objectContaining({ name: 'theia:canvas:layout', trigger: 'backend_reconnected' }),
    ]));
    expect(reactFlow.fitView).toHaveBeenCalledTimes(1);

    rerender({
      currentSnapshot: mockSnapshot({
        device_statuses: {
          'dev-1': 'down',
        },
      }),
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(canvasMetrics()).toEqual(expect.arrayContaining([
      expect.objectContaining({ name: 'theia:canvas:snapshot-apply', trigger: 'snapshot' }),
    ]));
    expect(canvasMetrics().every((measurement) => typeof measurement.durationMs === 'number')).toBe(true);
  });
});
