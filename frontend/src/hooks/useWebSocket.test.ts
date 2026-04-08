import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useWebSocket } from './useWebSocket';

class MockWebSocket {
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;
  close = vi.fn();

  simulateOpen() {
    this.onopen?.();
  }

  simulateMessage(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent);
  }

  simulateClose() {
    this.onclose?.();
  }
}

let mockInstance: MockWebSocket;

beforeEach(() => {
  // Replace the global WebSocket with our MockWebSocket class.
  // When the hook calls `new WebSocket(url)`, it will construct a MockWebSocket.
  const OriginalMock = class extends MockWebSocket {
    constructor(_url: string) {
      super();
      mockInstance = this;
    }
  };
  vi.stubGlobal('WebSocket', OriginalMock);
});

afterEach(() => {
  vi.restoreAllMocks();
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
        payload: { available: false, error: 'connection refused' },
      });
    });

    expect(result.current.prometheusStatus).toEqual({
      available: false,
      error: 'connection refused',
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
          alerts: [],
          device_statuses: {},
          device_hostnames: {},
        },
      });
    });

    expect(result.current.snapshot).not.toBeNull();
    expect(result.current.snapshot!.device_metrics).toEqual({});
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
          alerts: [],
          device_statuses: {},
          device_hostnames: {},
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
          alerts: [],
          device_statuses: {},
          device_hostnames: {},
        },
      });
    });

    expect(result.current.snapshot!.device_metrics['dev-1'].cpu_percent).toBe(90);
    expect(result.current.snapshot!.device_metrics['dev-2'].cpu_percent).toBe(75);
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
          alerts: [],
          device_statuses: {},
          device_hostnames: {},
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
          alerts: [],
          device_statuses: {},
          device_hostnames: {},
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

  it('snapshot_delta with alerts replaces existing alerts', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    // Send full snapshot with one existing alert
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          device_metrics: {},
          link_metrics: {},
          alerts: [
            {
              device_id: 'd1',
              severity: 'warning',
              alert_name: 'HighCPU',
              state: 'firing',
              summary: 'CPU high',
            },
          ],
          device_statuses: {},
          device_hostnames: {},
        },
      });
    });

    // Send delta with a different alert
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          device_metrics: {},
          link_metrics: {},
          alerts: [
            {
              device_id: 'd1',
              severity: 'critical',
              alert_name: 'Down',
              state: 'firing',
              summary: 'Device down',
            },
          ],
          device_statuses: {},
          device_hostnames: {},
        },
      });
    });

    expect(result.current.snapshot!.alerts).toHaveLength(1);
    expect(result.current.snapshot!.alerts[0].alert_name).toBe('Down');
  });

  it('snapshot_delta with empty alerts preserves existing alerts', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    // Send full snapshot with one existing alert
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          device_metrics: {},
          link_metrics: {},
          alerts: [
            {
              device_id: 'd1',
              severity: 'warning',
              alert_name: 'HighCPU',
              state: 'firing',
              summary: 'CPU high',
            },
          ],
          device_statuses: {},
          device_hostnames: {},
        },
      });
    });

    // Send delta with empty alerts — should preserve existing
    act(() => {
      mockInstance.simulateMessage({
        type: 'snapshot_delta',
        payload: {
          device_metrics: {},
          link_metrics: {},
          alerts: [],
          device_statuses: {},
          device_hostnames: {},
        },
      });
    });

    expect(result.current.snapshot!.alerts).toHaveLength(1);
    expect(result.current.snapshot!.alerts[0].alert_name).toBe('HighCPU');
  });
});
