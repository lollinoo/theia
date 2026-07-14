/**
 * Exercises use WebSocket hook lifecycle behavior so refactors preserve the documented contract.
 */
import { act, renderHook } from '@testing-library/react';
import { createElement, type ReactNode, StrictMode } from 'react';
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

function makeRuntimeSnapshot(cpuPercent: number, timestamp: string) {
  return {
    devices: {
      'dev-1': makeDeviceRuntime({
        cpu_percent: cpuPercent,
        last_collected_at: timestamp,
        last_polled_at: timestamp,
      }),
    },
    links: {},
  };
}

function spyOnDispatchEvent() {
  return vi.spyOn(window, 'dispatchEvent');
}

function mockJSONResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    statusText: 'OK',
    json: () => Promise.resolve(body),
    headers: new Headers(),
  } as unknown as Response;
}

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

async function flushAsyncWork(): Promise<void> {
  for (let index = 0; index < 5; index += 1) {
    await Promise.resolve();
  }
}

async function advanceTimersByTime(milliseconds: number): Promise<void> {
  await act(async () => {
    vi.advanceTimersByTime(milliseconds);
    await flushAsyncWork();
  });
}

async function resolveDeferredResponse(
  deferred: ReturnType<typeof createDeferred<Response>>,
  response: Response,
): Promise<void> {
  await act(async () => {
    deferred.resolve(response);
    await deferred.promise;
    await flushAsyncWork();
  });
}

function expectDeviceCpuPercent(
  snapshot: ReturnType<typeof useWebSocket>['snapshot'],
  value: number,
) {
  if (!snapshot) {
    throw new Error('expected snapshot to be populated');
  }
  expect(snapshot.devices['dev-1'].cpu_percent).toBe(value);
}

function getWebSocketDiagnostics() {
  return exportCanvasDiagnostics().diagnostics.websocket;
}

function publishRuntimeBootstrapSnapshot(
  snapshot: ReturnType<typeof makeRuntimeSnapshot>,
  runtimeVersion: number,
  runtimeIdentity: string,
  runtimeStreamId?: string,
) {
  act(() => {
    publishCanvasRuntimeBootstrap({
      snapshot,
      runtimeStreamId,
      runtimeVersion,
      runtimeIdentity,
    });
  });
}

const PRIMARY_DETAIL_DEVICE_ID = 'dev-1';
const NEXT_DETAIL_DEVICE_ID = 'dev-2';
const NEWER_RUNTIME_CPU_PERCENT = 70;
const FRESH_RUNTIME_CPU_PERCENT = 99;
const NEWER_RUNTIME_TIMESTAMP = '2026-01-01T00:02:00Z';
const FRESH_RUNTIME_TIMESTAMP = '2026-01-01T00:03:00Z';
const BACKEND_RESYNC_REQUIRED_EVENT = 'backend-resync-required';
const OVERVIEW_RESYNC_SCOPE = 'overview';
const CLIENT_RESYNC_SCHEDULED_REASON = 'client_resync_scheduled';
const BASE_VERSION_MISMATCH_REASON = 'base_version_mismatch';
const RUNTIME_STREAM_ID = 'runtime-stream-1';

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
let mockUrls: string[] = [];

function sendFrames(frames: unknown[], socket = mockInstance): void {
  act(() => {
    for (const frame of frames) {
      socket.simulateMessage(frame);
    }
  });
}

function streamRecoveryMarker(targetVersion: number, streamId = RUNTIME_STREAM_ID) {
  return {
    type: 'resync_required',
    payload: {
      scope: 'overview',
      reason: 'state_changes_dropped',
      strategy: 'stream',
      target_version: targetVersion,
      runtime_stream_id: streamId,
    },
  };
}

function readyFrame(version: number, streamId = RUNTIME_STREAM_ID, syncMode = 'current') {
  return {
    type: 'ready',
    payload: {
      runtime_version: version,
      runtime_stream_id: streamId,
      sync_mode: syncMode,
    },
  };
}

function runtimeReplayFrame({
  fromVersion,
  version,
  streamId = RUNTIME_STREAM_ID,
  delta = { devices: {}, links: {} },
}: {
  fromVersion: number | string;
  version: number;
  streamId?: string;
  delta?: unknown;
}) {
  return {
    type: 'runtime_replay',
    payload: {
      from_version: fromVersion,
      version,
      runtime_stream_id: streamId,
      delta,
    },
  };
}

function runtimeDeltaFrame({
  baseVersion,
  version,
  streamId = RUNTIME_STREAM_ID,
  delta = { devices: {}, links: {} },
}: {
  baseVersion: number;
  version: number;
  streamId?: string | null;
  delta?: unknown;
}) {
  return {
    type: 'runtime_delta',
    payload: {
      base_version: baseVersion,
      version,
      ...(streamId === null ? {} : { runtime_stream_id: streamId }),
      delta,
    },
  };
}

function runtimeSnapshotFrame({
  version,
  streamId = RUNTIME_STREAM_ID,
  cpuPercent,
  runtimeIdentity,
}: {
  version: number;
  streamId?: string | null;
  cpuPercent: number;
  runtimeIdentity?: string;
}) {
  return {
    type: 'snapshot',
    payload: {
      version,
      ...(streamId === null ? {} : { runtime_stream_id: streamId }),
      runtime_identity: runtimeIdentity,
      snapshot: makeRuntimeSnapshot(cpuPercent, '2026-01-01T00:05:00Z'),
    },
  };
}

function sentControls(socket = mockInstance): unknown[] {
  return socket.send.mock.calls.map(([control]) => JSON.parse(control as string));
}

function openWithRuntimeSnapshot({
  socket = mockInstance,
  streamId = RUNTIME_STREAM_ID,
  version = 10,
  cpuPercent = 50,
  runtimeIdentity,
}: {
  socket?: MockWebSocket;
  streamId?: string;
  version?: number;
  cpuPercent?: number;
  runtimeIdentity?: string;
} = {}): void {
  act(() => {
    if (socket.readyState !== MockWebSocket.OPEN) {
      socket.simulateOpen();
    }
    socket.simulateMessage({
      type: 'snapshot',
      payload: {
        version,
        runtime_stream_id: streamId,
        runtime_identity: runtimeIdentity,
        snapshot: makeRuntimeSnapshot(cpuPercent, '2026-01-01T00:00:00Z'),
      },
    });
  });
}

function sendRuntimeGap({
  socket = mockInstance,
  streamId = RUNTIME_STREAM_ID,
  baseVersion = 12,
  version = 13,
}: {
  socket?: MockWebSocket;
  streamId?: string;
  baseVersion?: number;
  version?: number;
} = {}): void {
  sendFrames([runtimeDeltaFrame({ baseVersion, version, streamId })], socket);
}

function runtimeOverviewResponse({
  streamId,
  version,
  cpuPercent,
}: {
  streamId: string;
  version: number;
  cpuPercent: number;
}): Response {
  return mockJSONResponse({
    schema_version: 1,
    runtime_stream_id: streamId,
    runtime_version: version,
    runtime_identity: `rt-sha256:${version}`,
    runtime_snapshot: makeRuntimeSnapshot(cpuPercent, '2026-01-01T00:05:00Z'),
  });
}

beforeEach(() => {
  mockInstances = [];
  mockUrls = [];
  vi.useFakeTimers();
  resetCanvasDiagnostics();
  resetCanvasRuntimeBootstrap();

  // Replace the global WebSocket with our MockWebSocket class.
  // When the hook calls `new WebSocket(url)`, it will construct a MockWebSocket.
  const OriginalMock = class extends MockWebSocket {
    constructor(url: string) {
      super();
      mockUrls.push(url);
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
  vi.unstubAllGlobals();
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
        runtimeStreamId: RUNTIME_STREAM_ID,
        runtimeVersion: 42,
        runtimeIdentity: 'rt-sha256:abc',
      });
    });

    expect(mockInstances).toHaveLength(1);
    expect(mockUrls[0]).toContain('canvas_schema_version=1');
    expect(mockUrls[0]).toContain('runtime_protocol=2');
    expect(mockUrls[0]).toContain(`runtime_stream_id=${RUNTIME_STREAM_ID}`);
    expect(mockUrls[0]).toContain('runtime_version=42');
    expect(mockUrls[0]).toContain('runtime_identity=rt-sha256%3Aabc');

    act(() => {
      mockInstance.simulateOpen();
    });

    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'hello',
        payload: {
          canvas_schema_version: 1,
          runtime_protocol: 2,
          topology_version: undefined,
          runtime_stream_id: RUNTIME_STREAM_ID,
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

  it.each([
    {
      name: 'blank stream',
      runtimeStreamId: '   ',
      runtimeVersion: 42,
    },
    {
      name: 'unsafe version',
      runtimeStreamId: RUNTIME_STREAM_ID,
      runtimeVersion: Number.MAX_SAFE_INTEGER + 1,
    },
  ])('omits the resumable cursor from hello for an invalid $name', (runtimeCursor) => {
    act(() => {
      publishCanvasRuntimeBootstrap({
        snapshot: { devices: {}, links: {} },
        runtimeStreamId: runtimeCursor.runtimeStreamId,
        runtimeVersion: runtimeCursor.runtimeVersion,
        runtimeIdentity: 'rt-sha256:bootstrap',
      });
    });

    renderHook(() =>
      useWebSocket('ws://localhost:8080/ws', null, { requireRuntimeBootstrap: true }),
    );

    expect(mockUrls[0]).not.toContain('runtime_stream_id=');
    expect(mockUrls[0]).not.toContain('runtime_version=');

    act(() => {
      mockInstance.simulateOpen();
    });

    const hello = JSON.parse(mockInstance.send.mock.calls[0]?.[0] as string) as {
      payload: Record<string, unknown>;
    };
    expect(hello.payload).not.toHaveProperty('runtime_stream_id');
    expect(hello.payload).not.toHaveProperty('runtime_version');
  });

  it('sends a fresh hello after HTTP runtime resync on an open socket', () => {
    renderHook(() =>
      useWebSocket('ws://localhost:8080/ws', null, { requireRuntimeBootstrap: true }),
    );

    act(() => {
      publishCanvasRuntimeBootstrap({
        snapshot: { devices: {}, links: {} },
        runtimeStreamId: RUNTIME_STREAM_ID,
        runtimeVersion: 10,
        runtimeIdentity: 'rt-sha256:initial',
      });
    });

    act(() => {
      mockInstance.simulateOpen();
    });

    mockInstance.send.mockClear();

    act(() => {
      mockInstance.simulateMessage({
        type: 'resync_required',
        payload: {
          scope: 'overview',
          reason: 'client_resync_scheduled',
        },
      });
      publishCanvasRuntimeBootstrap({
        snapshot: { devices: {}, links: {} },
        runtimeStreamId: RUNTIME_STREAM_ID,
        runtimeVersion: 12,
        runtimeIdentity: 'rt-sha256:resynced',
      });
    });

    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'hello',
        payload: {
          canvas_schema_version: 1,
          runtime_protocol: 2,
          topology_version: undefined,
          runtime_stream_id: RUNTIME_STREAM_ID,
          runtime_version: 12,
          runtime_identity: 'rt-sha256:resynced',
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
    expect(mockInstance.close).not.toHaveBeenCalled();
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
          runtime_stream_id: RUNTIME_STREAM_ID,
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
          runtime_protocol: 2,
          topology_version: undefined,
          runtime_stream_id: RUNTIME_STREAM_ID,
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

  it('does not advance a legacy cursor from a mismatched ready without state', () => {
    const dispatchSpy = spyOnDispatchEvent();
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    act(() => {
      mockInstance.simulateOpen();
    });
    sendFrames([
      runtimeSnapshotFrame({ version: 10, streamId: null, cpuPercent: 50 }),
      { type: 'ready', payload: { runtime_version: 20 } },
    ]);

    expect(getWebSocketDiagnostics().lastAppliedSnapshotVersion).toBe('10');
    expect(dispatchSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: BACKEND_RESYNC_REQUIRED_EVENT,
        detail: { scope: 'overview', reason: 'client_resync_scheduled' },
      }),
    );
  });

  it('rejects inexact legacy ready barriers without discarding an idle v2 cursor', () => {
    const dispatchSpy = spyOnDispatchEvent();
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();

    sendFrames([{ type: 'ready', payload: {} }]);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeStreamId: RUNTIME_STREAM_ID,
      lastAppliedSnapshotVersion: '10',
    });
    expect(
      dispatchSpy.mock.calls.filter(([event]) => event.type === BACKEND_RESYNC_REQUIRED_EVENT),
    ).toHaveLength(1);

    sendFrames([
      { type: 'ready', payload: { runtime_version: -1 } },
      { type: 'ready', payload: { runtime_version: Number.MAX_SAFE_INTEGER + 1 } },
      { type: 'ready', payload: { runtime_version: 20 } },
    ]);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeStreamId: RUNTIME_STREAM_ID,
      lastAppliedSnapshotVersion: '10',
    });
    expect(
      dispatchSpy.mock.calls.filter(([event]) => event.type === BACKEND_RESYNC_REQUIRED_EVENT),
    ).toHaveLength(1);

    act(() => {
      mockInstance.simulateClose();
      vi.advanceTimersByTime(1_000);
    });
    expect(mockUrls[1]).toContain(`runtime_stream_id=${RUNTIME_STREAM_ID}`);
    expect(mockUrls[1]).toContain('runtime_version=10');
  });

  it('accepts an exact legacy ready as a reconnect downgrade from idle v2 state', () => {
    const dispatchSpy = spyOnDispatchEvent();
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();

    act(() => {
      mockInstance.simulateClose();
      vi.advanceTimersByTime(1_000);
    });
    const legacySocket = mockInstances[1];
    if (!legacySocket) {
      throw new Error('expected legacy reconnect socket instance');
    }
    act(() => {
      legacySocket.simulateOpen();
    });
    sendFrames([{ type: 'ready', payload: { runtime_version: 10 } }], legacySocket);

    expect(getWebSocketDiagnostics().runtimeStreamId).toBeUndefined();
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );

    act(() => {
      legacySocket.simulateClose();
      vi.advanceTimersByTime(1_000);
    });
    expect(mockUrls[2]).not.toContain('runtime_stream_id=');
    expect(mockUrls[2]).not.toContain('runtime_version=');
  });

  it('releases a legacy recovery gate after reconnecting through an exact v2 ready', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    sendFrames([
      {
        type: 'resync_required',
        payload: { scope: 'overview', reason: 'state_changes_dropped' },
      },
    ]);

    act(() => {
      mockInstance.simulateClose();
      vi.advanceTimersByTime(1_000);
    });
    const reconnectSocket = mockInstances[1];
    if (!reconnectSocket) {
      throw new Error('expected reconnect socket instance');
    }
    act(() => {
      reconnectSocket.simulateOpen();
    });
    sendFrames(
      [
        readyFrame(10),
        runtimeDeltaFrame({
          baseVersion: 10,
          version: 11,
          delta: {
            devices: { 'dev-1': { device_id: 'dev-1', cpu_percent: 70 } },
            links: {},
          },
        }),
      ],
      reconnectSocket,
    );

    expectDeviceCpuPercent(result.current.snapshot, 70);
    expect(getWebSocketDiagnostics().lastAppliedDeltaVersion).toBe('11');
  });

  it('applies one runtime replay and acknowledges only after the matching ready barrier', () => {
    const dispatchSpy = spyOnDispatchEvent();
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendFrames([
      streamRecoveryMarker(12),
      runtimeReplayFrame({
        fromVersion: 10,
        version: 12,
        delta: {
          devices: { 'dev-1': { device_id: 'dev-1', cpu_percent: 72 } },
          links: {},
        },
      }),
      readyFrame(12, RUNTIME_STREAM_ID, 'replay'),
    ]);

    expectDeviceCpuPercent(result.current.snapshot, 72);
    expect(sentControls()).toEqual([
      {
        type: 'runtime_ack',
        payload: {
          runtime_stream_id: RUNTIME_STREAM_ID,
          runtime_version: 12,
        },
      },
    ]);
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );
  });

  it('sends one resume request for repeated client-detected gaps', () => {
    const dispatchSpy = spyOnDispatchEvent();
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendRuntimeGap({ version: 13 });
    sendRuntimeGap({ version: 14 });

    expectDeviceCpuPercent(result.current.snapshot, 50);
    expect(sentControls()).toEqual([
      {
        type: 'resume_runtime',
        payload: {
          runtime_stream_id: RUNTIME_STREAM_ID,
          runtime_version: 10,
        },
      },
    ]);
    expect(mockInstance.close).not.toHaveBeenCalled();
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );
  });

  it('requests recovery for a stream mismatch without discarding the last valid snapshot', () => {
    const dispatchSpy = spyOnDispatchEvent();
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendFrames([
      runtimeDeltaFrame({
        baseVersion: 10,
        version: 11,
        streamId: 'runtime-stream-2',
        delta: {
          devices: { 'dev-1': { device_id: 'dev-1', cpu_percent: 99 } },
          links: {},
        },
      }),
    ]);

    expectDeviceCpuPercent(result.current.snapshot, 50);
    expect(sentControls()).toContainEqual({
      type: 'resume_runtime',
      payload: {
        runtime_stream_id: RUNTIME_STREAM_ID,
        runtime_version: 10,
      },
    });
    expect(mockInstance.close).not.toHaveBeenCalled();
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );
  });

  it('keeps recovery active through a rotated snapshot until an exact ready barrier', () => {
    const rotatedStreamId = 'runtime-stream-2';
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendFrames([
      streamRecoveryMarker(5, rotatedStreamId),
      runtimeSnapshotFrame({
        version: 5,
        streamId: rotatedStreamId,
        runtimeIdentity: 'rt-sha256:rotated',
        cpuPercent: 80,
      }),
      {
        type: 'ready',
        payload: {
          runtime_version: 4,
          runtime_stream_id: rotatedStreamId,
          runtime_identity: 'rt-sha256:mismatched',
        },
      },
    ]);

    expectDeviceCpuPercent(result.current.snapshot, 80);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'stream',
      runtimeRecoveryTargetVersion: '5',
      runtimeStreamId: rotatedStreamId,
      lastAppliedSnapshotVersion: '5',
    });
    expect(getWebSocketDiagnostics().lastRuntimeRecoveryMode).toBeUndefined();
    expect(getWebSocketDiagnostics().lastAppliedRuntimeIdentity).toBe('rt-sha256:rotated');
    expect(mockInstance.send).not.toHaveBeenCalled();

    sendFrames([
      {
        type: 'ready',
        payload: {
          runtime_version: 5,
          runtime_stream_id: rotatedStreamId,
          runtime_identity: 'rt-sha256:ready',
          sync_mode: 'snapshot',
        },
      },
    ]);

    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'idle',
      lastRuntimeRecoveryMode: 'snapshot',
      lastAppliedRuntimeIdentity: 'rt-sha256:ready',
      lastRuntimeAckVersion: '5',
    });
    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'runtime_ack',
        payload: {
          runtime_stream_id: rotatedStreamId,
          runtime_version: 5,
        },
      }),
    );
  });

  it.each([
    { name: 'missing stream', streamId: undefined, version: 20 },
    { name: 'blank stream', streamId: '   ', version: 20 },
    { name: 'negative version', streamId: RUNTIME_STREAM_ID, version: -1 },
    {
      name: 'unsafe version',
      streamId: RUNTIME_STREAM_ID,
      version: Number.MAX_SAFE_INTEGER + 1,
    },
  ])('does not fabricate a v2 cursor from a snapshot with an invalid $name', (invalidCursor) => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendFrames([
      streamRecoveryMarker(20),
      {
        type: 'snapshot',
        payload: {
          version: invalidCursor.version,
          ...(invalidCursor.streamId === undefined
            ? {}
            : { runtime_stream_id: invalidCursor.streamId }),
          snapshot: makeRuntimeSnapshot(99, '2026-01-01T00:05:00Z'),
        },
      },
    ]);

    expectDeviceCpuPercent(result.current.snapshot, 50);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'stream',
      lastAppliedSnapshotVersion: '10',
      runtimeStreamId: RUNTIME_STREAM_ID,
    });
    expect(mockInstance.send).not.toHaveBeenCalled();
  });

  it('recovers from a blank snapshot stream without mutating an idle v2 cursor', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendFrames([runtimeSnapshotFrame({ version: 20, streamId: '   ', cpuPercent: 99 })]);

    expectDeviceCpuPercent(result.current.snapshot, 50);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'stream',
      lastAppliedSnapshotVersion: '10',
      runtimeStreamId: RUNTIME_STREAM_ID,
    });
    expect(sentControls()).toEqual([
      {
        type: 'resume_runtime',
        payload: {
          runtime_stream_id: RUNTIME_STREAM_ID,
          runtime_version: 10,
        },
      },
    ]);
  });

  it('keeps omitted snapshot lineage as an idle legacy downgrade', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendFrames([runtimeSnapshotFrame({ version: 5, streamId: null, cpuPercent: 80 })]);

    expectDeviceCpuPercent(result.current.snapshot, 80);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'idle',
      lastAppliedSnapshotVersion: '5',
      runtimeStreamId: undefined,
    });
    expect(sentControls()).toEqual([]);
  });

  it('does not advance an applied cursor from a newer normal ready message', () => {
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot({ runtimeIdentity: 'rt-sha256:10' });
    mockInstance.send.mockClear();

    sendFrames([
      {
        type: 'ready',
        payload: {
          runtime_version: 12,
          runtime_stream_id: RUNTIME_STREAM_ID,
          runtime_identity: 'rt-sha256:12',
        },
      },
    ]);

    expect(getWebSocketDiagnostics().lastAppliedSnapshotVersion).toBe('10');
    expect(getWebSocketDiagnostics().lastAppliedRuntimeIdentity).toBe('rt-sha256:10');
    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'resume_runtime',
        payload: {
          runtime_stream_id: RUNTIME_STREAM_ID,
          runtime_version: 10,
        },
      }),
    );
  });

  it('coalesces normal protocol-v2 delta acknowledgements', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendFrames(
      [
        [10, 11, 60],
        [11, 12, 70],
      ].map(([baseVersion, version, cpuPercent]) =>
        runtimeDeltaFrame({
          baseVersion,
          version,
          delta: {
            devices: {
              'dev-1': { device_id: 'dev-1', cpu_percent: cpuPercent },
            },
            links: {},
          },
        }),
      ),
    );

    expectDeviceCpuPercent(result.current.snapshot, 70);
    expect(mockInstance.send).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(249);
    });
    expect(mockInstance.send).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(1);
    });

    expect(mockInstance.send).toHaveBeenCalledTimes(1);
    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'runtime_ack',
        payload: {
          runtime_stream_id: RUNTIME_STREAM_ID,
          runtime_version: 12,
        },
      }),
    );
    expect(getWebSocketDiagnostics().lastRuntimeAckVersion).toBe('12');
  });

  it('falls back once to the runtime-only snapshot and resumes before applying replay', async () => {
    const dispatchSpy = spyOnDispatchEvent();
    const deferredResponse = createDeferred<Response>();
    const fetchMock = vi.fn(() => deferredResponse.promise);
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();
    sendRuntimeGap();
    await advanceTimersByTime(5_000);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/runtime/overview', {
      cache: 'no-store',
      headers: { Accept: 'application/json' },
    });

    await advanceTimersByTime(10_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await resolveDeferredResponse(
      deferredResponse,
      runtimeOverviewResponse({ streamId: 'runtime-stream-2', version: 5, cpuPercent: 60 }),
    );

    expectDeviceCpuPercent(result.current.snapshot, 60);
    expect(sentControls()).toEqual(
      expect.arrayContaining([
        {
          type: 'resume_runtime',
          payload: {
            runtime_stream_id: RUNTIME_STREAM_ID,
            runtime_version: 10,
          },
        },
        {
          type: 'resume_runtime',
          payload: {
            runtime_stream_id: 'runtime-stream-2',
            runtime_version: 5,
          },
        },
      ]),
    );
    const fallbackControls = sentControls() as Array<{
      type: string;
      payload: { runtime_stream_id?: string; runtime_version?: number };
    }>;
    const fallbackAckIndex = fallbackControls.findIndex(
      (control) =>
        control.type === 'runtime_ack' &&
        control.payload.runtime_stream_id === 'runtime-stream-2' &&
        control.payload.runtime_version === 5,
    );
    const fallbackResumeIndex = fallbackControls.findIndex(
      (control) =>
        control.type === 'resume_runtime' &&
        control.payload.runtime_stream_id === 'runtime-stream-2' &&
        control.payload.runtime_version === 5,
    );
    expect(fallbackAckIndex).toBeGreaterThanOrEqual(0);
    expect(fallbackResumeIndex).toBeGreaterThan(fallbackAckIndex);

    sendFrames([
      runtimeReplayFrame({
        fromVersion: 5,
        version: 6,
        streamId: 'runtime-stream-2',
        delta: {
          devices: {
            'dev-1': { device_id: 'dev-1', cpu_percent: 70 },
          },
          links: {},
        },
      }),
      readyFrame(6, 'runtime-stream-2', 'replay'),
    ]);

    expectDeviceCpuPercent(result.current.snapshot, 70);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'idle',
      lastRuntimeRecoveryMode: 'http-fallback',
      runtimeHttpFallbackCount: 1,
      lastRuntimeAckVersion: '6',
    });
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );
    expect(fetchMock.mock.calls.every(([path]) => path === '/api/v1/runtime/overview')).toBe(true);
  });

  it('acknowledges the current barrier after an HTTP fallback pre-resume ACK', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        runtimeOverviewResponse({
          streamId: 'runtime-stream-2',
          version: 5,
          cpuPercent: 60,
        }),
      ),
    );
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();
    sendRuntimeGap();
    await advanceTimersByTime(5_000);

    sendFrames([readyFrame(5, 'runtime-stream-2', 'current')]);

    const controls = sentControls() as Array<{
      type: string;
      payload: { runtime_stream_id?: string; runtime_version?: number };
    }>;
    const fallbackBarrierControls = controls.filter(
      (control) =>
        control.payload.runtime_stream_id === 'runtime-stream-2' &&
        control.payload.runtime_version === 5,
    );
    expect(fallbackBarrierControls.map((control) => control.type)).toEqual([
      'runtime_ack',
      'resume_runtime',
      'runtime_ack',
    ]);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'idle',
      lastRuntimeRecoveryMode: 'http-fallback',
      lastRuntimeAckVersion: '5',
    });
  });

  it('lets an exact current ready win while the HTTP fallback request is pending', async () => {
    const dispatchSpy = spyOnDispatchEvent();
    const deferredResponse = createDeferred<Response>();
    const fetchMock = vi.fn(() => deferredResponse.promise);
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendRuntimeGap();
    await advanceTimersByTime(5_000);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'http-fallback',
      runtimeRecoveryCount: 1,
      runtimeHttpFallbackCount: 1,
    });
    expect(getWebSocketDiagnostics().lastRuntimeRecoveryMode).toBeUndefined();

    sendFrames([readyFrame(10)]);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'idle',
      lastRuntimeRecoveryMode: 'current',
      lastRuntimeRecoveryDurationMs: 5_000,
      runtimeRecoveryCount: 1,
      runtimeHttpFallbackCount: 1,
      runtimeReplayRecoveryCount: 0,
      runtimeSnapshotRecoveryCount: 0,
    });
    const sendCountAfterReady = mockInstance.send.mock.calls.length;

    await resolveDeferredResponse(
      deferredResponse,
      runtimeOverviewResponse({
        streamId: 'runtime-stream-late',
        version: 99,
        cpuPercent: 99,
      }),
    );

    expectDeviceCpuPercent(result.current.snapshot, 50);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'idle',
      lastRuntimeRecoveryMode: 'current',
      lastAppliedSnapshotVersion: '10',
      runtimeStreamId: RUNTIME_STREAM_ID,
      runtimeHttpFallbackCount: 1,
    });
    expect(mockInstance.send).toHaveBeenCalledTimes(sendCountAfterReady);
    expect(mockInstance.close).not.toHaveBeenCalled();
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );
  });

  it.each([
    {
      name: 'replay',
      frames: [
        runtimeReplayFrame({
          fromVersion: 10,
          version: 12,
          delta: {
            devices: { 'dev-1': { device_id: 'dev-1', cpu_percent: 72 } },
            links: {},
          },
        }),
        readyFrame(12, RUNTIME_STREAM_ID, 'replay'),
      ],
      expectedCpuPercent: 72,
      expectedMode: 'replay',
      expectedDiagnostics: {
        lastAppliedDeltaVersion: '12',
        runtimeStreamId: RUNTIME_STREAM_ID,
        runtimeReplayRecoveryCount: 1,
        runtimeSnapshotRecoveryCount: 0,
      },
    },
    {
      name: 'snapshot',
      frames: [
        runtimeSnapshotFrame({
          version: 5,
          streamId: 'runtime-stream-ws',
          cpuPercent: 80,
        }),
        readyFrame(5, 'runtime-stream-ws', 'snapshot'),
      ],
      expectedCpuPercent: 80,
      expectedMode: 'snapshot',
      expectedDiagnostics: {
        lastAppliedSnapshotVersion: '5',
        runtimeStreamId: 'runtime-stream-ws',
        runtimeReplayRecoveryCount: 0,
        runtimeSnapshotRecoveryCount: 1,
      },
    },
  ])('lets a WebSocket $name win while the HTTP fallback request is pending', async (winner) => {
    const dispatchSpy = spyOnDispatchEvent();
    const deferredResponse = createDeferred<Response>();
    const fetchMock = vi.fn(() => deferredResponse.promise);
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendRuntimeGap();
    await advanceTimersByTime(5_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    sendFrames(winner.frames);
    expectDeviceCpuPercent(result.current.snapshot, winner.expectedCpuPercent);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'idle',
      lastRuntimeRecoveryMode: winner.expectedMode,
      lastRuntimeRecoveryDurationMs: 5_000,
      runtimeRecoveryCount: 1,
      runtimeHttpFallbackCount: 1,
      ...winner.expectedDiagnostics,
    });
    const sendCountAfterReady = mockInstance.send.mock.calls.length;

    await act(async () => {
      deferredResponse.reject(new Error('late runtime fallback failure'));
      await deferredResponse.promise.catch(() => undefined);
      await flushAsyncWork();
    });

    expectDeviceCpuPercent(result.current.snapshot, winner.expectedCpuPercent);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'idle',
      lastRuntimeRecoveryMode: winner.expectedMode,
      runtimeHttpFallbackCount: 1,
      ...winner.expectedDiagnostics,
    });
    expect(mockInstance.send).toHaveBeenCalledTimes(sendCountAfterReady);
    expect(mockInstance.close).not.toHaveBeenCalled();
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );
  });

  it('uses one deadline and no resume for duplicate stream markers with a raised target', async () => {
    const deferredResponse = createDeferred<Response>();
    const fetchMock = vi.fn(() => deferredResponse.promise);
    vi.stubGlobal('fetch', fetchMock);
    const { unmount } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    sendFrames([readyFrame(10)]);
    mockInstance.send.mockClear();

    sendFrames([streamRecoveryMarker(12), streamRecoveryMarker(14)]);
    expect(getWebSocketDiagnostics().runtimeRecoveryTargetVersion).toBe('14');
    expect(sentControls()).toEqual([]);

    await advanceTimersByTime(4_999);
    expect(fetchMock).not.toHaveBeenCalled();

    await advanceTimersByTime(10_001);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(sentControls()).toEqual([]);

    act(() => {
      unmount();
    });
  });

  it('closes the socket when the runtime-only HTTP fallback fails', async () => {
    const dispatchSpy = spyOnDispatchEvent();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('runtime unavailable')));
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    sendFrames([
      streamRecoveryMarker(12),
      runtimeReplayFrame({ fromVersion: 10, version: 12 }),
      readyFrame(12, RUNTIME_STREAM_ID, 'replay'),
    ]);
    expect(getWebSocketDiagnostics().lastRuntimeRecoveryMode).toBe('replay');

    sendRuntimeGap({ baseVersion: 14, version: 15 });
    await advanceTimersByTime(5_000);

    expect(mockInstance.close).toHaveBeenCalledTimes(1);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'failed',
      lastRuntimeRecoveryMode: 'http-fallback',
      lastRuntimeRecoveryDurationMs: 5_000,
      runtimeRecoveryCount: 2,
      runtimeReplayRecoveryCount: 1,
      runtimeRecoveryFailureCount: 1,
    });
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );

    const failedSocket = mockInstance;
    act(() => {
      failedSocket.simulateClose();
      vi.advanceTimersByTime(1_000);
    });
    const reconnectedSocket = mockInstances[1];
    if (!reconnectedSocket) {
      throw new Error('expected reconnect socket instance');
    }
    act(() => {
      reconnectedSocket.simulateOpen();
    });
    sendFrames([readyFrame(12)], reconnectedSocket);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'idle',
      runtimeRecoveryFailureCount: 1,
    });
  });

  it('fails recovery instead of regressing a same-stream HTTP fallback cursor', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        runtimeOverviewResponse({
          streamId: RUNTIME_STREAM_ID,
          version: 5,
          cpuPercent: 5,
        }),
      ),
    );
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    sendRuntimeGap();
    await advanceTimersByTime(5_000);

    expectDeviceCpuPercent(result.current.snapshot, 50);
    expect(mockInstance.close).toHaveBeenCalledTimes(1);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'failed',
      lastAppliedSnapshotVersion: '10',
      runtimeStreamId: RUNTIME_STREAM_ID,
    });
  });

  it('cancels recovery and ACK timers and ignores a late HTTP completion after cleanup', async () => {
    const deferredResponse = createDeferred<Response>();
    const fetchMock = vi.fn(() => deferredResponse.promise);
    vi.stubGlobal('fetch', fetchMock);
    const { unmount } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    sendRuntimeGap();
    await advanceTimersByTime(5_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const sendCountBeforeCleanup = mockInstance.send.mock.calls.length;

    act(() => {
      unmount();
    });
    expect(vi.getTimerCount()).toBe(0);

    await resolveDeferredResponse(
      deferredResponse,
      runtimeOverviewResponse({ streamId: 'runtime-stream-late', version: 99, cpuPercent: 99 }),
    );

    expect(mockInstance.send).toHaveBeenCalledTimes(sendCountBeforeCleanup);
    expect(getWebSocketDiagnostics().lastAppliedSnapshotVersion).not.toBe('99');
  });

  it('starts a new fallback after reconnect without waiting for the stale socket fetch', async () => {
    const firstResponse = createDeferred<Response>();
    const secondResponse = createDeferred<Response>();
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(() => firstResponse.promise)
      .mockImplementationOnce(() => secondResponse.promise);
    vi.stubGlobal('fetch', fetchMock);
    const { result, unmount } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    openWithRuntimeSnapshot();
    sendRuntimeGap();
    await advanceTimersByTime(5_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const firstSocket = mockInstance;
    act(() => {
      firstSocket.simulateClose();
      vi.advanceTimersByTime(1_000);
    });
    const secondSocket = mockInstances[1];
    if (!secondSocket) {
      throw new Error('expected reconnect socket instance');
    }

    act(() => {
      secondSocket.simulateOpen();
    });
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryPhase: 'idle',
      runtimeHttpFallbackCount: 1,
    });
    sendRuntimeGap({ socket: secondSocket });
    await advanceTimersByTime(5_000);

    expect(fetchMock).toHaveBeenCalledTimes(2);

    await resolveDeferredResponse(
      firstResponse,
      runtimeOverviewResponse({
        streamId: 'runtime-stream-stale',
        version: 90,
        cpuPercent: 90,
      }),
    );
    expectDeviceCpuPercent(result.current.snapshot, 50);

    await resolveDeferredResponse(
      secondResponse,
      runtimeOverviewResponse({
        streamId: 'runtime-stream-current',
        version: 20,
        cpuPercent: 80,
      }),
    );
    expectDeviceCpuPercent(result.current.snapshot, 80);

    act(() => {
      unmount();
    });
  });

  it('does not reuse the completed recovery mode in the next generation', () => {
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();

    sendFrames([
      streamRecoveryMarker(12),
      runtimeReplayFrame({ fromVersion: 10, version: 12 }),
      readyFrame(12, RUNTIME_STREAM_ID, 'replay'),
    ]);
    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryCount: 1,
      runtimeReplayRecoveryCount: 1,
      lastRuntimeRecoveryMode: 'replay',
    });

    sendFrames([streamRecoveryMarker(12), readyFrame(12)]);

    expect(getWebSocketDiagnostics()).toMatchObject({
      runtimeRecoveryCount: 2,
      runtimeReplayRecoveryCount: 1,
      lastRuntimeRecoveryMode: 'current',
    });
  });

  it('routes malformed protocol-v2 runtime frames through one stream recovery', () => {
    const dispatchSpy = spyOnDispatchEvent();
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));
    openWithRuntimeSnapshot();
    mockInstance.send.mockClear();

    sendFrames([
      {
        type: 'runtime_delta',
        payload: {
          base_version: 10,
          version: 11,
          runtime_stream_id: RUNTIME_STREAM_ID,
          delta: {
            devices: {
              'dev-1': { primary_health: 'not-a-runtime-health' },
            },
            links: {},
          },
        },
      },
      {
        type: 'runtime_replay',
        payload: {
          from_version: 'invalid',
          version: 11,
          runtime_stream_id: RUNTIME_STREAM_ID,
          delta: { devices: {}, links: {} },
        },
      },
    ]);

    expect(mockInstance.send).toHaveBeenCalledTimes(1);
    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'resume_runtime',
        payload: {
          runtime_stream_id: RUNTIME_STREAM_ID,
          runtime_version: 10,
        },
      }),
    );
    expect(mockInstance.close).not.toHaveBeenCalled();
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );
  });

  it('requests resync when ready arrives before any base snapshot exists', () => {
    const dispatchSpy = spyOnDispatchEvent();
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
          base_version: 9,
          version: 10,
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

  it('records each applied runtime delta once under React Strict Mode', () => {
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(StrictMode, null, children);
    renderHook(() => useWebSocket('ws://localhost:8080/ws'), { wrapper });

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
    });

    const appliedEvents = exportCanvasDiagnostics().events.filter(
      (event) => event.event === 'runtime.delta.applied',
    );
    expect(appliedEvents).toHaveLength(1);
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
    const dispatchSpy = spyOnDispatchEvent();
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
    const dispatchSpy = spyOnDispatchEvent();
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

  it('keeps legacy deltas gated until the HTTP runtime bootstrap is replaced', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() =>
      useWebSocket('ws://localhost:8080/ws', null, { requireRuntimeBootstrap: true }),
    );
    publishRuntimeBootstrapSnapshot(
      makeRuntimeSnapshot(50, '2026-01-01T00:00:00Z'),
      10,
      'rt-sha256:10',
    );
    act(() => {
      mockInstance.simulateOpen();
    });

    sendFrames([
      {
        type: 'resync_required',
        payload: { scope: 'overview', reason: 'state_changes_dropped' },
      },
      runtimeDeltaFrame({
        baseVersion: 10,
        version: 11,
        streamId: null,
        delta: {
          devices: { 'dev-1': { device_id: 'dev-1', cpu_percent: 99 } },
          links: {},
        },
      }),
    ]);
    expectDeviceCpuPercent(result.current.snapshot, 50);

    publishRuntimeBootstrapSnapshot(
      makeRuntimeSnapshot(60, '2026-01-01T00:01:00Z'),
      11,
      'rt-sha256:11',
    );
    sendFrames([
      runtimeDeltaFrame({
        baseVersion: 11,
        version: 12,
        streamId: null,
        delta: {
          devices: { 'dev-1': { device_id: 'dev-1', cpu_percent: 70 } },
          links: {},
        },
      }),
    ]);

    expectDeviceCpuPercent(result.current.snapshot, 70);
    act(() => {
      vi.advanceTimersByTime(5_000);
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('dispatches topology-changed with versioned invalidation detail', () => {
    const dispatchSpy = spyOnDispatchEvent();
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
    const snapshot = result.current.snapshot;
    if (!snapshot) {
      throw new Error('expected snapshot to be populated');
    }
    expect(snapshot.devices).toEqual({});
    expect(snapshot).not.toHaveProperty('alerts');
    expect(snapshot).not.toHaveProperty('device_metrics');
    expect(snapshot).not.toHaveProperty('link_metrics');
    expect(snapshot).not.toHaveProperty('device_statuses');
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

    expectDeviceCpuPercent(result.current.snapshot, 90);
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

  it('defers runtime snapshot publication while runtime updates are paused', () => {
    const { result, rerender } = renderHook(
      ({ paused }: { paused: boolean }) =>
        useWebSocket('ws://localhost:8080/ws', null, { runtimeUpdatesPaused: paused }),
      { initialProps: { paused: false } },
    );

    act(() => {
      mockInstance.simulateOpen();
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
      rerender({ paused: true });
    });

    act(() => {
      mockInstance.simulateMessage({
        type: 'runtime_delta',
        payload: {
          base_version: 1,
          version: 2,
          delta: {
            devices: {
              'dev-1': {
                device_id: 'dev-1',
                cpu_percent: 51,
              },
            },
            links: {},
          },
        },
      });
      mockInstance.simulateMessage({
        type: 'runtime_delta',
        payload: {
          base_version: 2,
          version: 3,
          delta: {
            devices: {
              'dev-1': {
                device_id: 'dev-1',
                cpu_percent: 52,
              },
            },
            links: {},
          },
        },
      });
    });

    expectDeviceCpuPercent(result.current.snapshot, 50);
    expect(exportCanvasDiagnostics().diagnostics.websocket.lastAppliedDeltaVersion).toBe('3');

    act(() => {
      rerender({ paused: false });
    });

    expect(result.current.snapshot!.devices['dev-1'].cpu_percent).toBe(52);
  });

  it('handles runtime_delta partial patches without dropping existing runtime fields', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 1,
          snapshot: {
            devices: {
              'dev-1': makeDeviceRuntime({
                cpu_percent: 50,
                mem_percent: 25,
                primary_health: 'up_fresh',
              }),
            },
            links: {
              'link-1': makeLinkRuntime({
                rx_bps: 200,
                tx_bps: 100,
              }),
            },
          },
        },
      });
      mockInstance.simulateMessage({
        type: 'runtime_delta',
        payload: {
          base_version: 1,
          version: 2,
          delta: {
            devices: {
              'dev-1': {
                device_id: 'dev-1',
                cpu_percent: null,
              },
            },
            links: {
              'link-1': {
                link_id: 'link-1',
                rx_bps: 250,
              },
            },
          },
        },
      });
    });

    expect(result.current.snapshot!.devices['dev-1'].cpu_percent).toBeNull();
    expect(result.current.snapshot!.devices['dev-1'].mem_percent).toBe(25);
    expect(result.current.snapshot!.devices['dev-1'].primary_health).toBe('up_fresh');
    expect(result.current.snapshot!.links['link-1'].rx_bps).toBe(250);
    expect(result.current.snapshot!.links['link-1'].tx_bps).toBe(100);
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

  it('defers polling health publication while runtime updates are paused', () => {
    const { result, rerender } = renderHook(
      ({ paused }: { paused: boolean }) =>
        useWebSocket('ws://localhost:8080/ws', null, { runtimeUpdatesPaused: paused }),
      { initialProps: { paused: true } },
    );

    act(() => {
      mockInstance.simulateOpen();
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

    expect(result.current.pollingHealth).toBeNull();

    act(() => {
      rerender({ paused: false });
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
    renderHook(() => useWebSocket('ws://localhost:8080/ws', PRIMARY_DETAIL_DEVICE_ID));

    act(() => {
      mockInstance.simulateOpen();
    });

    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'subscribe_detail',
        payload: { device_id: PRIMARY_DETAIL_DEVICE_ID },
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
      { initialProps: { detailDeviceId: PRIMARY_DETAIL_DEVICE_ID } },
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
        payload: { device_id: PRIMARY_DETAIL_DEVICE_ID },
      }),
    );
  });

  it('sends unsubscribe then subscribe when switching devices', () => {
    const { rerender } = renderHook(
      ({ detailDeviceId }: { detailDeviceId: string | null }) =>
        useWebSocket('ws://localhost:8080/ws', detailDeviceId),
      { initialProps: { detailDeviceId: PRIMARY_DETAIL_DEVICE_ID } },
    );

    act(() => {
      mockInstance.simulateOpen();
    });

    mockInstance.send.mockClear();

    act(() => {
      rerender({ detailDeviceId: NEXT_DETAIL_DEVICE_ID });
    });

    expect(mockInstance.send).toHaveBeenNthCalledWith(
      1,
      JSON.stringify({
        type: 'unsubscribe_detail',
        payload: { device_id: PRIMARY_DETAIL_DEVICE_ID },
      }),
    );
    expect(mockInstance.send).toHaveBeenNthCalledWith(
      2,
      JSON.stringify({
        type: 'subscribe_detail',
        payload: { device_id: NEXT_DETAIL_DEVICE_ID },
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
    const dispatchSpy = spyOnDispatchEvent();
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

  it('requests a session check whenever the backend connection closes', () => {
    const dispatchSpy = spyOnDispatchEvent();
    renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateClose();
    });

    expect(dispatchSpy).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'backend-session-check-required' }),
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

    expectDeviceCpuPercent(result.current.snapshot, 50);

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
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
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
    expect(consoleError).toHaveBeenCalledWith(
      'Failed to parse WebSocket message',
      expect.objectContaining({
        message: 'unsupported websocket message type: unknown_type',
      }),
    );
  });

  it('requests resync when a runtime_delta payload cannot be parsed', () => {
    const dispatchSpy = spyOnDispatchEvent();
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
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
              'dev-1': {
                primary_health: 'not-a-runtime-health',
              },
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
    expect(exportCanvasDiagnostics().diagnostics.websocket.lastRejectedDeltaReason).toBe(
      'invalid_runtime_delta_payload',
    );
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

    expectDeviceCpuPercent(result.current.snapshot, 90);
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

    expectDeviceCpuPercent(result.current.snapshot, 50);
  });

  it('ignores stale runtime deltas after a newer HTTP bootstrap without reconnecting', () => {
    const dispatchSpy = spyOnDispatchEvent();
    const { result } = renderHook(() =>
      useWebSocket('ws://localhost:8080/ws', null, { requireRuntimeBootstrap: true }),
    );

    publishRuntimeBootstrapSnapshot(
      makeRuntimeSnapshot(NEWER_RUNTIME_CPU_PERCENT, NEWER_RUNTIME_TIMESTAMP),
      10,
      'rt-sha256:newer',
    );

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'runtime_delta',
        payload: {
          base_version: 8,
          version: 9,
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

    expectDeviceCpuPercent(result.current.snapshot, NEWER_RUNTIME_CPU_PERCENT);
    expect(mockInstance.close).not.toHaveBeenCalled();
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );
    expect(getWebSocketDiagnostics().resyncRequiredCount).toBe(0);
  });

  it('ignores stale full snapshots after a newer HTTP bootstrap without reconnecting', () => {
    const dispatchSpy = spyOnDispatchEvent();
    const { result } = renderHook(() =>
      useWebSocket('ws://localhost:8080/ws', null, { requireRuntimeBootstrap: true }),
    );

    publishRuntimeBootstrapSnapshot(
      makeRuntimeSnapshot(NEWER_RUNTIME_CPU_PERCENT, NEWER_RUNTIME_TIMESTAMP),
      10,
      'rt-sha256:newer',
    );

    act(() => {
      mockInstance.simulateOpen();
      mockInstance.simulateMessage({
        type: 'snapshot',
        payload: {
          version: 8,
          runtime_identity: 'rt-sha256:older',
          snapshot: {
            devices: {
              'dev-1': makeDeviceRuntime({
                cpu_percent: 20,
                last_collected_at: '2026-01-01T00:01:00Z',
                last_polled_at: '2026-01-01T00:01:00Z',
              }),
            },
            links: {},
          },
        },
      });
    });

    expectDeviceCpuPercent(result.current.snapshot, NEWER_RUNTIME_CPU_PERCENT);
    expect(mockInstance.close).not.toHaveBeenCalled();
    expect(dispatchSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: BACKEND_RESYNC_REQUIRED_EVENT }),
    );
    expect(getWebSocketDiagnostics().lastAppliedSnapshotVersion).toBe('10');
  });

  it('keeps HTTP-bootstrap socket open while requesting resync for future runtime delta base', () => {
    const dispatchSpy = spyOnDispatchEvent();
    renderHook(() =>
      useWebSocket('ws://localhost:8080/ws', null, { requireRuntimeBootstrap: true }),
    );

    publishRuntimeBootstrapSnapshot(
      makeRuntimeSnapshot(NEWER_RUNTIME_CPU_PERCENT, NEWER_RUNTIME_TIMESTAMP),
      10,
      'rt-sha256:base',
    );

    act(() => {
      mockInstance.simulateOpen();
    });
    mockInstance.send.mockClear();
    mockInstance.close.mockClear();

    act(() => {
      mockInstance.simulateMessage({
        type: 'runtime_delta',
        payload: {
          base_version: 12,
          version: 13,
          delta: {
            devices: {
              'dev-1': makeDeviceRuntime({
                cpu_percent: 99,
                last_collected_at: '2026-01-01T00:03:00Z',
                last_polled_at: '2026-01-01T00:03:00Z',
              }),
            },
            links: {},
          },
        },
      });
    });

    expect(mockInstance.close).not.toHaveBeenCalled();
    expect(dispatchSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: BACKEND_RESYNC_REQUIRED_EVENT,
        detail: {
          scope: OVERVIEW_RESYNC_SCOPE,
          reason: CLIENT_RESYNC_SCHEDULED_REASON,
        },
      }),
    );
    expect(getWebSocketDiagnostics().lastRejectedDeltaReason).toBe(BASE_VERSION_MISMATCH_REASON);

    publishRuntimeBootstrapSnapshot(
      makeRuntimeSnapshot(FRESH_RUNTIME_CPU_PERCENT, FRESH_RUNTIME_TIMESTAMP),
      13,
      'rt-sha256:fresh',
      RUNTIME_STREAM_ID,
    );

    expect(mockInstance.send).toHaveBeenCalledWith(
      JSON.stringify({
        type: 'hello',
        payload: {
          canvas_schema_version: 1,
          runtime_protocol: 2,
          topology_version: undefined,
          runtime_stream_id: RUNTIME_STREAM_ID,
          runtime_version: 13,
          runtime_identity: 'rt-sha256:fresh',
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

  it('requests a fresh websocket snapshot when a versioned delta base is ahead of the local base', () => {
    const dispatchSpy = spyOnDispatchEvent();
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
          base_version: 5,
          version: 6,
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
