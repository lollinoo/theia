import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useWebSocket } from './useWebSocket';

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
});

describe('useWebSocket', () => {
  it('sets connected=true after open', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    expect(result.current.connected).toBe(false);

    act(() => {
      mockInstance.simulateOpen();
    });

    expect(result.current.connected).toBe(true);
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

  it('handles snapshot message', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          device_metrics: {},
          link_metrics: {},
          device_statuses: {},
        },
      });
    });

    expect(result.current.snapshot).not.toBeNull();
    expect(result.current.snapshot!.device_metrics).toEqual({});
    expect((result.current.snapshot! as Record<string, unknown>).alerts).toBeUndefined();
    expect((result.current.snapshot! as Record<string, unknown>).device_hostnames).toBeUndefined();
    expect((result.current.snapshot! as Record<string, unknown>).device_models).toBeUndefined();
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
          device_metrics: {
            'dev-1': {
              device_id: 'dev-1',
              cpu_percent: 50,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              collected_at: '2026-01-01T00:00:00Z',
            },
            'dev-2': {
              device_id: 'dev-2',
              cpu_percent: 75,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              collected_at: '2026-01-01T00:00:00Z',
            },
          },
          link_metrics: {},
          device_statuses: {},
        },
      });
    });

    // Send delta with only dev-1 updated (cpu=90)
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          device_metrics: {
            'dev-1': {
              device_id: 'dev-1',
              cpu_percent: 90,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              collected_at: '2026-01-01T00:01:00Z',
            },
          },
          link_metrics: {},
          device_statuses: {},
        },
      });
    });

    expect(result.current.snapshot!.device_metrics['dev-1'].cpu_percent).toBe(90);
    expect(result.current.snapshot!.device_metrics['dev-2'].cpu_percent).toBe(75);
  });

  it('handles snapshot_delta message with targeted link_metrics by merging into existing snapshot', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          device_metrics: {},
          link_metrics: {
            'dev-1': [
              {
                device_id: 'dev-1',
                if_name: 'ether1',
                tx_bps: 100,
                rx_bps: 200,
                utilization: 0.1,
                collected_at: '2026-01-01T00:00:00Z',
              },
            ],
            'dev-2': [
              {
                device_id: 'dev-2',
                if_name: 'ether2',
                tx_bps: 300,
                rx_bps: 400,
                utilization: 0.2,
                collected_at: '2026-01-01T00:00:00Z',
              },
            ],
          },
          device_statuses: {},
        },
      });
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          device_metrics: {},
          link_metrics: {
            'dev-1': [
              {
                device_id: 'dev-1',
                if_name: 'ether1',
                tx_bps: 150,
                rx_bps: 250,
                utilization: 0.15,
                collected_at: '2026-01-01T00:01:00Z',
              },
            ],
          },
          device_statuses: {},
        },
      });
    });

    expect(result.current.snapshot!.link_metrics['dev-1']).toEqual([
      {
        device_id: 'dev-1',
        if_name: 'ether1',
        tx_bps: 150,
        rx_bps: 250,
        utilization: 0.15,
        collected_at: '2026-01-01T00:01:00Z',
      },
    ]);
    expect(result.current.snapshot!.link_metrics['dev-2']).toEqual([
      {
        device_id: 'dev-2',
        if_name: 'ether2',
        tx_bps: 300,
        rx_bps: 400,
        utilization: 0.2,
        collected_at: '2026-01-01T00:00:00Z',
      },
    ]);
  });

  it('sends subscribe_detail on open when detailDeviceId is preset', () => {
    renderHook(() => useWebSocket('ws://localhost:8080/ws', 'dev-1'));

    act(() => {
      mockInstance.simulateOpen();
    });

    expect(mockInstance.send).toHaveBeenCalledWith(JSON.stringify({
      type: 'subscribe_detail',
      payload: { device_id: 'dev-1' },
    }));
  });

  it('stores separate alert messages in dedicated alert state', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'alert',
        payload: {
          device_id: 'dev-1',
          severity: 'critical',
          alert_name: 'DeviceDown',
          state: 'firing',
          summary: 'router unreachable',
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

  it('preserves detail-only device metric fields from detail subscription deltas', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          device_metrics: {
            'dev-1': {
              device_id: 'dev-1',
              cpu_percent: 50,
              mem_percent: null,
              collected_at: '2026-01-01T00:00:00Z',
            },
          },
          link_metrics: {},
          device_statuses: {},
        },
      });
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          device_metrics: {
            'dev-1': {
              device_id: 'dev-1',
              cpu_percent: 51,
              mem_percent: 52,
              temp_celsius: 53,
              uptime_secs: 54,
              last_polled_at: '2026-01-01T00:00:30Z',
              expected_poll_interval_seconds: 30,
              collected_at: '2026-01-01T00:00:30Z',
            },
          },
          link_metrics: {},
          device_statuses: {},
        },
      });
    });

    expect(result.current.snapshot?.device_metrics['dev-1']).toMatchObject({
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
      ({ detailDeviceId }: { detailDeviceId: string | null }) => useWebSocket('ws://localhost:8080/ws', detailDeviceId),
      { initialProps: { detailDeviceId: 'dev-1' } },
    );

    act(() => {
      mockInstance.simulateOpen();
    });

    mockInstance.send.mockClear();

    act(() => {
      rerender({ detailDeviceId: null });
    });

    expect(mockInstance.send).toHaveBeenCalledWith(JSON.stringify({
      type: 'unsubscribe_detail',
      payload: { device_id: 'dev-1' },
    }));
  });

  it('sends unsubscribe then subscribe when switching devices', () => {
    const { rerender } = renderHook(
      ({ detailDeviceId }: { detailDeviceId: string | null }) => useWebSocket('ws://localhost:8080/ws', detailDeviceId),
      { initialProps: { detailDeviceId: 'dev-1' } },
    );

    act(() => {
      mockInstance.simulateOpen();
    });

    mockInstance.send.mockClear();

    act(() => {
      rerender({ detailDeviceId: 'dev-2' });
    });

    expect(mockInstance.send).toHaveBeenNthCalledWith(1, JSON.stringify({
      type: 'unsubscribe_detail',
      payload: { device_id: 'dev-1' },
    }));
    expect(mockInstance.send).toHaveBeenNthCalledWith(2, JSON.stringify({
      type: 'subscribe_detail',
      payload: { device_id: 'dev-2' },
    }));
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

    expect(secondSocket.send).toHaveBeenCalledWith(JSON.stringify({
      type: 'subscribe_detail',
      payload: { device_id: 'dev-1' },
    }));
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

    expect(dispatchSpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'backend-reconnected' }));
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
          device_metrics: {
            'dev-1': {
              device_id: 'dev-1',
              cpu_percent: 50,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              collected_at: '2026-01-01T00:00:00Z',
            },
          },
          link_metrics: {},
          device_statuses: {},
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

    expect(result.current.snapshot!.device_metrics['dev-1'].cpu_percent).toBe(50);

    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          device_metrics: {
            'dev-2': {
              device_id: 'dev-2',
              cpu_percent: 10,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              collected_at: '2026-01-01T00:02:00Z',
            },
          },
          link_metrics: {},
          device_statuses: {},
        },
      });
    });

    expect(result.current.snapshot!.device_metrics['dev-2'].cpu_percent).toBe(10);
    expect(result.current.snapshot!.device_metrics['dev-1']).toBeUndefined();
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
          device_metrics: {
            'dev-1': {
              device_id: 'dev-1',
              cpu_percent: 90,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              collected_at: '2026-01-01T00:00:00Z',
            },
          },
          link_metrics: {},
          device_statuses: {},
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
          device_metrics: {
            'dev-1': {
              device_id: 'dev-1',
              cpu_percent: 50,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              collected_at: '2026-01-01T00:00:00Z',
            },
            'dev-2': {
              device_id: 'dev-2',
              cpu_percent: 75,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              collected_at: '2026-01-01T00:00:00Z',
            },
          },
          link_metrics: {},
          device_statuses: {},
        },
      });
    });

    // Send delta updating dev-1
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          device_metrics: {
            'dev-1': {
              device_id: 'dev-1',
              cpu_percent: 90,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              collected_at: '2026-01-01T00:01:00Z',
            },
          },
          link_metrics: {},
          alerts: [],
          device_statuses: {},
          device_hostnames: {},
        },
      });
    });

    // Send a new full snapshot with only dev-3 — should replace entirely
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          device_metrics: {
            'dev-3': {
              device_id: 'dev-3',
              cpu_percent: 10,
              mem_percent: null,
              temp_celsius: null,
              uptime_secs: null,
              collected_at: '2026-01-01T00:02:00Z',
            },
          },
          link_metrics: {},
          alerts: [],
          device_statuses: {},
          device_hostnames: {},
        },
      });
    });

    expect(result.current.snapshot!.device_metrics['dev-3'].cpu_percent).toBe(10);
    expect(result.current.snapshot!.device_metrics['dev-1']).toBeUndefined();
    expect(result.current.snapshot!.device_metrics['dev-2']).toBeUndefined();
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
          device_metrics: {},
          link_metrics: {},
          device_statuses: {},
        },
      });
    });

    // Alert-only deltas should not affect the slim overview snapshot.
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          device_metrics: {},
          link_metrics: {},
          device_statuses: {},
        },
      });
    });

    expect(result.current.snapshot).toEqual({
      device_metrics: {},
      link_metrics: {},
      device_statuses: {},
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
          device_metrics: {},
          link_metrics: {},
          device_statuses: {},
        },
      });
    });

    // An empty slim delta should keep the current snapshot as-is.
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          device_metrics: {},
          link_metrics: {},
          device_statuses: {},
        },
      });
    });

    expect(result.current.snapshot).toEqual({
      device_metrics: {},
      link_metrics: {},
      device_statuses: {},
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
            device_metrics: {
              'dev-1': {
                device_id: 'dev-1',
                cpu_percent: 50,
                mem_percent: null,
                temp_celsius: null,
                uptime_secs: null,
                collected_at: '2026-01-01T00:00:00Z',
              },
            },
            link_metrics: {},
            device_statuses: {},
          },
        },
      });
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          base_version: 10,
          version: 11,
          delta: {
            device_metrics: {
              'dev-1': {
                device_id: 'dev-1',
                cpu_percent: 90,
                mem_percent: null,
                temp_celsius: null,
                uptime_secs: null,
                collected_at: '2026-01-01T00:01:00Z',
              },
            },
            link_metrics: {},
            device_statuses: {},
          },
        },
      });
    });

    expect(result.current.snapshot!.device_metrics['dev-1'].cpu_percent).toBe(90);
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
            device_metrics: {
              'dev-1': {
                device_id: 'dev-1',
                cpu_percent: 50,
                mem_percent: null,
                temp_celsius: null,
                uptime_secs: null,
                collected_at: '2026-01-01T00:00:00Z',
              },
            },
            link_metrics: {},
            device_statuses: {},
          },
        },
      });
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          base_version: 2,
          version: 4,
          delta: {
            device_metrics: {
              'dev-1': {
                device_id: 'dev-1',
                cpu_percent: 99,
                mem_percent: null,
                temp_celsius: null,
                uptime_secs: null,
                collected_at: '2026-01-01T00:01:00Z',
              },
            },
            link_metrics: {},
            device_statuses: {},
          },
        },
      });
    });

    expect(result.current.snapshot!.device_metrics['dev-1'].cpu_percent).toBe(50);
  });
});
