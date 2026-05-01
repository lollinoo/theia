import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  exportCanvasDiagnostics,
  resetCanvasDiagnostics,
} from '../components/canvas/canvasDiagnostics';
import {
  publishCanvasRuntimeBootstrap,
  resetCanvasRuntimeBootstrap,
} from './canvasRuntimeBootstrap';
import { useWebSocket } from './useWebSocket';

function makeDeviceRuntime(overrides: Record<string, unknown> = {}) {
  return {
    device_id: 'dev-1',
    operational_status: 'up',
    primary_health: 'up_fresh',
    runtime_flags: [],
    field_states: { uptime: 'ok', cpu: 'ok', memory: 'ok' },
    network_reachable: 'true',
    snmp_reachable: 'true',
    reachability: 'up',
    health: 'healthy',
    freshness: 'fresh',
    primary_reason: 'ok',
    metrics_status: 'available',
    metrics_reason: 'ok',
    alert_status: 'normal',
    firing_alert_count: 0,
    last_collected_at: '2026-01-01T00:00:00Z',
    last_polled_at: '2026-01-01T00:00:00Z',
    expected_poll_interval_seconds: 30,
    cpu_percent: 50,
    mem_percent: 25,
    temp_celsius: 55,
    uptime_secs: 86400,
    ...overrides,
  };
}

function makeLinkRuntime(overrides: Record<string, unknown> = {}) {
  return {
    link_id: 'link-1',
    source_device_id: 'dev-1',
    target_device_id: 'dev-2',
    source_if_name: 'ether1',
    target_if_name: 'ether2',
    metrics_status: 'available',
    metrics_reason: 'ok',
    last_collected_at: '2026-01-01T00:00:00Z',
    tx_bps: 100,
    rx_bps: 200,
    utilization: 0.1,
    ...overrides,
  };
}

class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;
  send = vi.fn();
  close = vi.fn();
  readyState = MockWebSocket.CONNECTING;

  simulateOpen() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  simulateMessage(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent);
  }

  simulateClose() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.();
  }
}

let mockInstance: MockWebSocket;
let mockInstances: MockWebSocket[] = [];

beforeEach(() => {
  mockInstances = [];
  vi.useFakeTimers();
  resetCanvasDiagnostics();
  resetCanvasRuntimeBootstrap();

  // Replace the global WebSocket with our MockWebSocket class.
  // When the hook calls `new WebSocket(url)`, it will construct a MockWebSocket.
  const OriginalMock = class extends MockWebSocket {
    constructor(_url: string) {
      super();
      mockInstance = this;
      mockInstances.push(this);
    }
  };
  vi.stubGlobal('WebSocket', OriginalMock);
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
  resetCanvasRuntimeBootstrap();
});

describe('useWebSocket', () => {
  it('waits for HTTP runtime bootstrap before opening the socket when required', () => {
    renderHook(() =>
      useWebSocket('ws://localhost:8080/ws', null, { requireRuntimeBootstrap: true }),
    );

    expect(mockInstances).toHaveLength(0);

    act(() => {
      publishCanvasRuntimeBootstrap({
        snapshot: { devices: {}, links: {} },
        runtimeVersion: 42,
        runtimeIdentity: 'rt-sha256:abc',
      });
    });

    expect(mockInstances).toHaveLength(1);

    act(() => {
      mockInstance.simulateOpen();
    });

    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'hello',
        payload: {
          canvas_schema_version: 1,
          topology_version: undefined,
          runtime_version: 42,
          runtime_identity: 'rt-sha256:abc',
          alert_version: undefined,
          subscriptions: {
            runtime: true,
            topology: true,
            alerts: true,
            details_device_id: null,
          },
        },
      }),
    );
  });

  it('sets connected=true after open', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    expect(result.current.connected).toBe(false);

    act(() => {
      mockInstance.simulateOpen();
    });

    expect(result.current.connected).toBe(true);
  });

  it('sends hello with known runtime version after reconnect', () => {
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 42,
          runtime_identity: 'rt-sha256:abc',
          snapshot: {
            devices: {},
            links: {},
          },
        },
      });
    });

    const firstSocket = mockInstance;

    act(() => {
      firstSocket.simulateClose();
      vi.advanceTimersByTime(1000);
    });

    const secondSocket = mockInstances[1];
    expect(secondSocket).toBeDefined();
    if (!secondSocket) {
      throw new Error('expected reconnect socket instance');
    }

    act(() => {
      secondSocket.simulateOpen();
    });

    expect(secondSocket.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'hello',
        payload: {
          canvas_schema_version: 1,
          topology_version: undefined,
          runtime_version: 42,
          runtime_identity: 'rt-sha256:abc',
          alert_version: undefined,
          subscriptions: {
            runtime: true,
            topology: true,
            alerts: true,
            details_device_id: null,
          },
        },
      }),
    );
  });

  it('does not reuse diagnostics runtime identity after the hook remounts without a base snapshot', () => {
    const { unmount } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 42,
          runtime_identity: 'rt-sha256:abc',
          snapshot: {
            devices: {},
            links: {},
          },
        },
      });
    });

    unmount();
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    const secondSocket = mockInstances[1];
    expect(secondSocket).toBeDefined();
    if (!secondSocket) {
      throw new Error('expected remounted socket instance');
    }

    act(() => {
      secondSocket.simulateOpen();
    });

    const hello = JSON.parse(secondSocket.send.mock.calls[0]?.[0] as string) as {
      type: string;
      payload: Record<string, unknown>;
    };
    expect(hello.type).toBe('hello');
    expect(hello.payload).not.toHaveProperty('runtime_identity');
    expect(hello.payload).not.toHaveProperty('runtime_version');
  });

  it('does not reuse stored runtime identity after a full page refresh clears the base snapshot', () => {
    const { unmount } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 42,
          runtime_identity: 'rt-sha256:abc',
          snapshot: {
            devices: {},
            links: {},
          },
        },
      });
    });

    unmount();
    resetCanvasDiagnostics();
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    const secondSocket = mockInstances[1];
    expect(secondSocket).toBeDefined();
    if (!secondSocket) {
      throw new Error('expected refreshed socket instance');
    }

    act(() => {
      secondSocket.simulateOpen();
    });

    const hello = JSON.parse(secondSocket.send.mock.calls[0]?.[0] as string) as {
      type: string;
      payload: Record<string, unknown>;
    };
    expect(hello.type).toBe('hello');
    expect(hello.payload).not.toHaveProperty('runtime_identity');
    expect(hello.payload).not.toHaveProperty('runtime_version');
  });

  it('handles ready handshake without replacing the existing snapshot', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 42,
          runtime_identity: 'rt-sha256:abc',
          snapshot: {
            devices: {
              'dev-1': makeDeviceRuntime({ cpu_percent: 50 }),
            },
            links: {},
          },
        },
      });
      mockInstance.simulateMessage({
        type: 'ready',
        payload: {
          runtime_version: 42,
          runtime_identity: 'rt-sha256:abc',
          alert_version: 7,
        },
      });
    });

    expect(result.current.snapshot?.devices['dev-1'].cpu_percent).toBe(50);
  });

  it('requests resync when ready arrives before any base snapshot exists', () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent');
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'ready',
        payload: {
          runtime_version: 42,
          runtime_identity: 'rt-sha256:abc',
        },
      });
    });

    expect(mockInstance.close).toHaveBeenCalled();
    expect(dispatchSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'backend-resync-required',
        detail: {
          scope: 'overview',
          reason: 'client_missing_runtime_snapshot',
        },
      }),
    );
  });

  it('records websocket snapshot, delta and rejected delta diagnostics', () => {
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 7,
          snapshot: {
            devices: { 'dev-1': makeDeviceRuntime() },
            links: {},
          },
        },
      });
      mockInstance.simulateMessage({
        type: 'runtime_delta',
        payload: {
          base_version: 7,
          version: 8,
          delta: {
            devices: {
              'dev-1': makeDeviceRuntime({
                cpu_percent: 51,
                last_collected_at: '2026-01-01T00:01:00Z',
                last_polled_at: '2026-01-01T00:01:00Z',
              }),
            },
            links: {},
          },
        },
      });
      mockInstance.simulateMessage({
        type: 'runtime_delta',
        payload: {
          base_version: 6,
          version: 9,
          delta: {
            devices: {},
            links: {},
          },
        },
      });
    });

    const exported = exportCanvasDiagnostics();

    expect(exported.diagnostics.websocket).toMatchObject({
      connected: true,
      lastMessageType: 'runtime_delta',
      lastAppliedSnapshotVersion: '7',
      lastAppliedDeltaVersion: '8',
      lastRejectedDeltaReason: 'base_version_mismatch',
      resyncRequiredCount: 1,
    });
    expect(exported.events.map((event) => event.event)).toEqual(
      expect.arrayContaining([
        'websocket.connected',
        'runtime.snapshot.applied',
        'runtime.delta.applied',
        'runtime.delta.rejected',
      ]),
    );
  });

  it('handles prometheus_status message', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'prometheus_status',
        payload: { enabled: true, available: false, error: 'connection refused' },
      });
    });

    expect(result.current.prometheusStatus).toEqual({
      enabled: true,
      available: false,
      error: 'connection refused',
    });
  });

  it('dispatches backend-resync-required when resync_required arrives', () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent');
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'resync_required',
        payload: {
          scope: 'overview',
          reason: 'state_changes_dropped',
        },
      });
    });

    expect(dispatchSpy).toHaveBeenCalledTimes(1);
    const event = dispatchSpy.mock.calls[0]?.[0];
    expect(event?.type).toBe('backend-resync-required');
    expect((event as CustomEvent).detail).toEqual({
      scope: 'overview',
      reason: 'state_changes_dropped',
    });
  });

  it('forwards hub_buffer_full resync detail unchanged', () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent');
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'resync_required',
        payload: {
          scope: 'overview',
          reason: 'hub_buffer_full',
        },
      });
    });

    const event = dispatchSpy.mock.calls[0]?.[0] as CustomEvent | undefined;
    expect(event?.type).toBe('backend-resync-required');
    expect(event?.detail).toEqual({
      scope: 'overview',
      reason: 'hub_buffer_full',
    });
  });

  it('dispatches topology-changed with versioned invalidation detail', () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent');
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'topology_changed',
        payload: {
          topology_version: 12,
          reason: 'topology_dirty',
          recommended_endpoint: '/api/v1/topology/canvas',
        },
      });
    });

    expect(dispatchSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'topology-changed',
        detail: {
          topology_version: 12,
          reason: 'topology_dirty',
          recommended_endpoint: '/api/v1/topology/canvas',
        },
      }),
    );
  });

  it('handles snapshot message', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          devices: {},
          links: {},
        },
      });
    });

    expect(result.current.snapshot).not.toBeNull();
    expect(result.current.snapshot!.devices).toEqual({});
    expect((result.current.snapshot! as Record<string, unknown>).alerts).toBeUndefined();
    expect((result.current.snapshot! as Record<string, unknown>).device_metrics).toBeUndefined();
    expect((result.current.snapshot! as Record<string, unknown>).link_metrics).toBeUndefined();
    expect((result.current.snapshot! as Record<string, unknown>).device_statuses).toBeUndefined();
  });

  it('closes WebSocket on unmount', () => {
    const { result, unmount } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    expect(result.current.connected).toBe(true);

    const wsInstance = mockInstance;
    unmount();

    expect(wsInstance.close).toHaveBeenCalled();
  });

  it('handles snapshot_delta message by merging into existing snapshot', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    // Send full snapshot with dev-1 (cpu=50) and dev-2 (cpu=75)
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          devices: {
            'dev-1': makeDeviceRuntime({
              cpu_percent: 50,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
            }),
            'dev-2': makeDeviceRuntime({
              device_id: 'dev-2',
              cpu_percent: 75,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
            }),
          },
          links: {},
        },
      });
    });

    // Send delta with only dev-1 updated (cpu=90)
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          devices: {
            'dev-1': makeDeviceRuntime({
              cpu_percent: 90,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              last_collected_at: '2026-01-01T00:01:00Z',
              last_polled_at: '2026-01-01T00:01:00Z',
            }),
          },
          links: {},
        },
      });
    });

    expect(result.current.snapshot!.devices['dev-1'].cpu_percent).toBe(90);
    expect(result.current.snapshot!.devices['dev-2'].cpu_percent).toBe(75);
  });

  it('handles runtime_delta message by merging into existing snapshot', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 1,
          snapshot: {
            devices: {
              'dev-1': makeDeviceRuntime({ cpu_percent: 50 }),
            },
            links: {},
          },
        },
      });
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'runtime_delta',
        payload: {
          base_version: 1,
          version: 2,
          delta: {
            devices: {
              'dev-1': makeDeviceRuntime({
                cpu_percent: null,
                mem_percent: 42,
                primary_health: 'snmp_degraded',
                runtime_flags: ['partial_telemetry'],
              }),
            },
            links: {},
          },
        },
      });
    });

    expect(result.current.snapshot!.devices['dev-1'].cpu_percent).toBeNull();
    expect(result.current.snapshot!.devices['dev-1'].mem_percent).toBe(42);
    expect(result.current.snapshot!.devices['dev-1'].runtime_flags).toEqual(['partial_telemetry']);
  });

  it('exposes polling health changes', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'polling_health_changed',
        payload: {
          essential_overloaded: true,
          degraded_risk: false,
          essential_queue_lag_seconds: 1.25,
          deadline_miss_total: 3,
          active_workers: 64,
          configured_workers: 64,
        },
      });
    });

    expect(result.current.pollingHealth).toEqual({
      essential_overloaded: true,
      degraded_risk: false,
      essential_queue_lag_seconds: 1.25,
      deadline_miss_total: 3,
      active_workers: 64,
      configured_workers: 64,
    });
  });

  it('handles snapshot_delta message with targeted links by merging into existing snapshot', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          devices: {},
          links: {
            'link-1': makeLinkRuntime(),
            'link-2': makeLinkRuntime({
              link_id: 'link-2',
              source_device_id: 'dev-2',
              target_device_id: 'dev-3',
              source_if_name: 'ether2',
              target_if_name: 'ether3',
              tx_bps: 300,
              rx_bps: 400,
              utilization: 0.2,
            }),
          },
        },
      });
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          devices: {},
          links: {
            'link-1': makeLinkRuntime({
              tx_bps: 150,
              rx_bps: 250,
              utilization: 0.15,
              last_collected_at: '2026-01-01T00:01:00Z',
            }),
          },
        },
      });
    });

    expect(result.current.snapshot!.links['link-1']).toEqual(
      makeLinkRuntime({
        tx_bps: 150,
        rx_bps: 250,
        utilization: 0.15,
        last_collected_at: '2026-01-01T00:01:00Z',
      }),
    );
    expect(result.current.snapshot!.links['link-2']).toEqual(
      makeLinkRuntime({
        link_id: 'link-2',
        source_device_id: 'dev-2',
        target_device_id: 'dev-3',
        source_if_name: 'ether2',
        target_if_name: 'ether3',
        tx_bps: 300,
        rx_bps: 400,
        utilization: 0.2,
      }),
    );
  });

  it('sends subscribe_detail on open when detailDeviceId is preset', () => {
    renderHook(() => useWebSocket('ws://localhost:8080/ws', 'dev-1'));

    act(() => {
      mockInstance.simulateOpen();
    });

    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'subscribe_detail',
        payload: { device_id: 'dev-1' },
      }),
    );
  });

  it('stores separate alert messages in dedicated alert state', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'alert',
        payload: {
          alerts: [
            {
              device_id: 'dev-1',
              severity: 'critical',
              alert_name: 'DeviceDown',
              state: 'firing',
              summary: 'router unreachable',
            },
          ],
        },
      });
    });

    expect(result.current.alerts).toEqual([
      {
        device_id: 'dev-1',
        severity: 'critical',
        alert_name: 'DeviceDown',
        state: 'firing',
        summary: 'router unreachable',
      },
    ]);
  });

  it('ignores stale versioned alert payloads after a newer alert update', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'alert',
        payload: {
          version: 11,
          alerts: [
            {
              device_id: 'dev-1',
              severity: 'warning',
              alert_name: 'CpuHigh',
              state: 'firing',
              summary: 'cpu high',
            },
          ],
        },
      });
      mockInstance.simulateMessage({
        type: 'alert',
        payload: {
          version: 10,
          alerts: [
            {
              device_id: 'dev-1',
              severity: 'critical',
              alert_name: 'DeviceDown',
              state: 'firing',
              summary: 'device down',
            },
          ],
        },
      });
    });

    expect(result.current.alerts).toEqual([
      {
        device_id: 'dev-1',
        severity: 'warning',
        alert_name: 'CpuHigh',
        state: 'firing',
        summary: 'cpu high',
      },
    ]);
  });

  it('clears alerts and resets alert version when resync is required', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'alert',
        payload: {
          version: 11,
          alerts: [
            {
              device_id: 'dev-1',
              severity: 'warning',
              alert_name: 'CpuHigh',
              state: 'firing',
              summary: 'cpu high',
            },
          ],
        },
      });
    });

    expect(result.current.alerts).toHaveLength(1);

    act(() => {
      mockInstance.simulateMessage({
        type: 'resync_required',
        payload: {
          scope: 'overview',
          reason: 'state_changes_dropped',
        },
      });
    });

    expect(result.current.alerts).toEqual([]);

    act(() => {
      mockInstance.simulateMessage({
        type: 'alert',
        payload: {
          version: 1,
          alerts: [
            {
              device_id: 'dev-2',
              severity: 'critical',
              alert_name: 'DeviceDown',
              state: 'firing',
              summary: 'device down',
            },
          ],
        },
      });
    });

    expect(result.current.alerts).toEqual([
      {
        device_id: 'dev-2',
        severity: 'critical',
        alert_name: 'DeviceDown',
        state: 'firing',
        summary: 'device down',
      },
    ]);
  });

  it('clears alerts while reconnecting and accepts a fresh alert stream after reconnect', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'alert',
        payload: {
          version: 11,
          alerts: [
            {
              device_id: 'dev-1',
              severity: 'warning',
              alert_name: 'CpuHigh',
              state: 'firing',
              summary: 'cpu high',
            },
          ],
        },
      });
    });

    const firstSocket = mockInstance;

    act(() => {
      firstSocket.simulateClose();
    });

    expect(result.current.reconnecting).toBe(true);
    expect(result.current.alerts).toEqual([]);

    act(() => {
      vi.advanceTimersByTime(1000);
    });

    const secondSocket = mockInstances[1];
    if (!secondSocket) {
      throw new Error('expected reconnect socket instance');
    }

    act(() => {
      secondSocket.simulateOpen();
      secondSocket.simulateMessage({
        type: 'alert',
        payload: {
          version: 1,
          alerts: [
            {
              device_id: 'dev-2',
              severity: 'critical',
              alert_name: 'DeviceDown',
              state: 'firing',
              summary: 'device down',
            },
          ],
        },
      });
    });

    expect(result.current.alerts).toEqual([
      {
        device_id: 'dev-2',
        severity: 'critical',
        alert_name: 'DeviceDown',
        state: 'firing',
        summary: 'device down',
      },
    ]);
  });

  it('preserves detail-only device metric fields from detail subscription deltas', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          devices: {
            'dev-1': makeDeviceRuntime({
              cpu_percent: 50,
              mem_percent: null,
            }),
          },
          links: {},
        },
      });
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          devices: {
            'dev-1': makeDeviceRuntime({
              cpu_percent: 51,
              mem_percent: 52,
              temp_celsius: 53,
              uptime_secs: 54,
              last_polled_at: '2026-01-01T00:00:30Z',
              expected_poll_interval_seconds: 30,
              last_collected_at: '2026-01-01T00:00:30Z',
            }),
          },
          links: {},
        },
      });
    });

    expect(result.current.snapshot?.devices['dev-1']).toMatchObject({
      cpu_percent: 51,
      mem_percent: 52,
      temp_celsius: 53,
      uptime_secs: 54,
      last_polled_at: '2026-01-01T00:00:30Z',
      expected_poll_interval_seconds: 30,
    });
  });

  it('sends unsubscribe_detail when detailDeviceId becomes null', () => {
    const { rerender } = renderHook(
      ({ detailDeviceId }: { detailDeviceId: string | null }) =>
        useWebSocket('ws://localhost:8080/ws', detailDeviceId),
      { initialProps: { detailDeviceId: 'dev-1' } },
    );

    act(() => {
      mockInstance.simulateOpen();
    });

    mockInstance.send.mockClear();

    act(() => {
      rerender({ detailDeviceId: null });
    });

    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'unsubscribe_detail',
        payload: { device_id: 'dev-1' },
      }),
    );
  });

  it('sends unsubscribe then subscribe when switching devices', () => {
    const { rerender } = renderHook(
      ({ detailDeviceId }: { detailDeviceId: string | null }) =>
        useWebSocket('ws://localhost:8080/ws', detailDeviceId),
      { initialProps: { detailDeviceId: 'dev-1' } },
    );

    act(() => {
      mockInstance.simulateOpen();
    });

    mockInstance.send.mockClear();

    act(() => {
      rerender({ detailDeviceId: 'dev-2' });
    });

    expect(mockInstance.send).toHaveBeenNthCalledWith(
      1,
      JSON.stringify({
        type: 'unsubscribe_detail',
        payload: { device_id: 'dev-1' },
      }),
    );
    expect(mockInstance.send).toHaveBeenNthCalledWith(
      2,
      JSON.stringify({
        type: 'subscribe_detail',
        payload: { device_id: 'dev-2' },
      }),
    );
  });

  it('re-sends the current device subscription after reconnect', () => {
    renderHook(() => useWebSocket('ws://localhost:8080/ws', 'dev-1'));

    act(() => {
      mockInstance.simulateOpen();
    });

    const firstSocket = mockInstance;

    act(() => {
      firstSocket.simulateClose();
      vi.advanceTimersByTime(1000);
    });

    const secondSocket = mockInstances[1];
    expect(secondSocket).toBeDefined();
    if (!secondSocket) {
      throw new Error('expected reconnect socket instance');
    }

    act(() => {
      secondSocket.simulateOpen();
    });

    expect(secondSocket.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'subscribe_detail',
        payload: { device_id: 'dev-1' },
      }),
    );
  });

  it('dispatches backend-reconnected after reconnect', () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent');
    renderHook(() => useWebSocket('ws://localhost:8080/ws', 'dev-1'));

    act(() => {
      mockInstance.simulateOpen();
    });

    const firstSocket = mockInstance;

    act(() => {
      firstSocket.simulateClose();
      vi.advanceTimersByTime(1000);
    });

    const secondSocket = mockInstances[1];
    if (!secondSocket) {
      throw new Error('expected reconnect socket instance');
    }

    act(() => {
      secondSocket.simulateOpen();
    });

    expect(dispatchSpy).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'backend-reconnected' }),
    );
  });

  it('full snapshot replaces state after resync_required', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          devices: {
            'dev-1': makeDeviceRuntime({
              cpu_percent: 50,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
            }),
          },
          links: {},
        },
      });
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'resync_required',
        payload: {
          scope: 'overview',
          reason: 'hub_buffer_full',
        },
      });
    });

    expect(result.current.snapshot!.devices['dev-1'].cpu_percent).toBe(50);

    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          devices: {
            'dev-2': makeDeviceRuntime({
              device_id: 'dev-2',
              cpu_percent: 10,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              last_collected_at: '2026-01-01T00:02:00Z',
              last_polled_at: '2026-01-01T00:02:00Z',
            }),
          },
          links: {},
        },
      });
    });

    expect(result.current.snapshot!.devices['dev-2'].cpu_percent).toBe(10);
    expect(result.current.snapshot!.devices['dev-1']).toBeUndefined();
  });

  it('ignores snapshot_delta when no base snapshot exists', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    // Send delta without a prior full snapshot
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          devices: {
            'dev-1': makeDeviceRuntime({
              cpu_percent: 90,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
            }),
          },
          links: {},
        },
      });
    });

    expect(result.current.snapshot).toBeNull();
  });

  it('full snapshot replaces state after delta', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    // Send full snapshot with dev-1 and dev-2
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          devices: {
            'dev-1': makeDeviceRuntime({
              cpu_percent: 50,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
            }),
            'dev-2': makeDeviceRuntime({
              device_id: 'dev-2',
              cpu_percent: 75,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
            }),
          },
          links: {},
        },
      });
    });

    // Send delta updating dev-1
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          devices: {
            'dev-1': makeDeviceRuntime({
              cpu_percent: 90,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              last_collected_at: '2026-01-01T00:01:00Z',
              last_polled_at: '2026-01-01T00:01:00Z',
            }),
          },
          links: {},
        },
      });
    });

    // Send a new full snapshot with only dev-3 — should replace entirely
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          devices: {
            'dev-3': makeDeviceRuntime({
              device_id: 'dev-3',
              cpu_percent: 10,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              last_collected_at: '2026-01-01T00:02:00Z',
              last_polled_at: '2026-01-01T00:02:00Z',
            }),
          },
          links: {},
        },
      });
    });

    expect(result.current.snapshot!.devices['dev-3'].cpu_percent).toBe(10);
    expect(result.current.snapshot!.devices['dev-1']).toBeUndefined();
    expect(result.current.snapshot!.devices['dev-2']).toBeUndefined();
  });

  it('ignores alert-only snapshot_delta payloads', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    // Seed a valid snapshot first.
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          devices: {},
          links: {},
        },
      });
    });

    // Alert-only deltas should not affect the slim overview snapshot.
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          devices: {},
          links: {},
        },
      });
    });

    expect(result.current.snapshot).toEqual({
      devices: {},
      links: {},
    });
  });

  it('does not crash when receiving an unknown message type', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    // Send a message with an unrecognised type — parseWSMessage throws, caught internally.
    expect(() => {
      act(() => {
        mockInstance.simulateMessage({
          type: 'unknown_type',
          payload: { some: 'data' },
        });
      });
    }).not.toThrow();

    // snapshot must remain null — no state mutation from the unknown message.
    expect(result.current.snapshot).toBeNull();
  });

  it('snapshot_delta without core changes leaves snapshot unchanged', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    // Seed a valid snapshot first.
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          devices: {},
          links: {},
        },
      });
    });

    // An empty slim delta should keep the current snapshot as-is.
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          devices: {},
          links: {},
        },
      });
    });

    expect(result.current.snapshot).toEqual({
      devices: {},
      links: {},
    });
  });

  it('applies versioned snapshot_delta only when base_version matches local version', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 10,
          snapshot: {
            devices: {
              'dev-1': makeDeviceRuntime({
                cpu_percent: 50,
                mem_percent: null,
                temp_celsius: null,
                uptime_secs: null,
              }),
            },
            links: {},
          },
        },
      });
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          base_version: 10,
          version: 11,
          delta: {
            devices: {
              'dev-1': makeDeviceRuntime({
                cpu_percent: 90,
                mem_percent: null,
                temp_celsius: null,
                uptime_secs: null,
                last_collected_at: '2026-01-01T00:01:00Z',
                last_polled_at: '2026-01-01T00:01:00Z',
              }),
            },
            links: {},
          },
        },
      });
    });

    expect(result.current.snapshot!.devices['dev-1'].cpu_percent).toBe(90);
  });

  it('ignores versioned snapshot_delta when base_version does not match local version', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 3,
          snapshot: {
            devices: {
              'dev-1': makeDeviceRuntime({
                cpu_percent: 50,
                mem_percent: null,
                temp_celsius: null,
                uptime_secs: null,
              }),
            },
            links: {},
          },
        },
      });
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          base_version: 2,
          version: 4,
          delta: {
            devices: {
              'dev-1': makeDeviceRuntime({
                cpu_percent: 99,
                mem_percent: null,
                temp_celsius: null,
                uptime_secs: null,
                last_collected_at: '2026-01-01T00:01:00Z',
                last_polled_at: '2026-01-01T00:01:00Z',
              }),
            },
            links: {},
          },
        },
      });
    });

    expect(result.current.snapshot!.devices['dev-1'].cpu_percent).toBe(50);
  });

  it('requests a fresh websocket snapshot when a versioned delta base does not match', () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent');
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 3,
          snapshot: {
            devices: {
              'dev-1': makeDeviceRuntime(),
            },
            links: {},
          },
        },
      });
      mockInstance.simulateMessage({
        type: 'runtime_delta',
        payload: {
          base_version: 2,
          version: 4,
          delta: {
            devices: {
              'dev-1': makeDeviceRuntime({
                cpu_percent: 99,
                last_collected_at: '2026-01-01T00:01:00Z',
                last_polled_at: '2026-01-01T00:01:00Z',
              }),
            },
            links: {},
          },
        },
      });
    });

    expect(mockInstance.close).toHaveBeenCalled();
    expect(dispatchSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'backend-resync-required',
        detail: {
          scope: 'overview',
          reason: 'client_resync_scheduled',
        },
      }),
    );
  });
});
