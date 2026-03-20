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
});
