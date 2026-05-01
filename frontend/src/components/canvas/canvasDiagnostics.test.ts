import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  clearCanvasDiagnosticEvents,
  exportCanvasDiagnostics,
  getCanvasDiagnosticsSnapshot,
  recordCanvasDiagnosticEvent,
  resetCanvasDiagnostics,
  subscribeCanvasDiagnostics,
  updateCanvasDiagnosticsState,
} from './canvasDiagnostics';
import {
  clearCanvasMetrics,
  finishCanvasRenderMetric,
  recordCanvasMetric,
  setCanvasRenderMetricsEnabled,
  startCanvasRenderMetric,
} from './canvasInstrumentation';

describe('canvasDiagnostics', () => {
  beforeEach(() => {
    clearCanvasMetrics();
    resetCanvasDiagnostics();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns a safe initial snapshot', () => {
    const snapshot = getCanvasDiagnosticsSnapshot();

    expect(snapshot).toMatchObject({
      topology: { lastTopologyLoadStatus: 'idle' },
      websocket: {
        connected: false,
        reconnectCount: 0,
        resyncRequiredCount: 0,
        topologyChangedCount: 0,
      },
      graph: {
        canonicalNodeCount: 0,
        canonicalEdgeCount: 0,
        displayedNodeCount: 0,
        displayedEdgeCount: 0,
        ghostNodeCount: 0,
        selectedNodeCount: 0,
        selectedEdgeCount: 0,
      },
      layout: { pendingLayout: false },
      positions: {
        pendingSaveCount: 0,
        lastSaveStatus: 'idle',
      },
    });
    expect(() => JSON.stringify(snapshot)).not.toThrow();
  });

  it('merges partial state updates without dropping previous fields', () => {
    updateCanvasDiagnosticsState({
      topology: {
        topologyVersion: 'topo-1',
        lastTopologyLoadStatus: 'loading',
      },
    });
    updateCanvasDiagnosticsState({
      topology: {
        lastTopologyLoadStatus: 'success',
        lastTopologyLoadReason: 'topology_changed',
      },
    });

    expect(getCanvasDiagnosticsSnapshot().topology).toMatchObject({
      topologyVersion: 'topo-1',
      lastTopologyLoadStatus: 'success',
      lastTopologyLoadReason: 'topology_changed',
    });
  });

  it('keeps only the newest 200 diagnostic events', () => {
    for (let index = 0; index < 205; index += 1) {
      recordCanvasDiagnosticEvent({
        level: 'info',
        source: 'topology',
        event: 'topology.load.succeeded',
        message: `load ${index}`,
      });
    }

    const exported = exportCanvasDiagnostics();

    expect(exported.events).toHaveLength(200);
    expect(exported.events[0]).toMatchObject({ message: 'load 5' });
  });

  it('notifies subscribers asynchronously and coalesces rapid diagnostics updates', () => {
    vi.useFakeTimers();
    const listener = vi.fn();
    const unsubscribe = subscribeCanvasDiagnostics(listener);

    updateCanvasDiagnosticsState({
      websocket: {
        connected: true,
      },
    });
    recordCanvasDiagnosticEvent({
      level: 'debug',
      source: 'runtime',
      event: 'runtime.delta.applied',
      message: 'Runtime delta applied',
    });

    expect(listener).not.toHaveBeenCalled();

    vi.runOnlyPendingTimers();

    expect(listener).toHaveBeenCalledTimes(1);
    unsubscribe();
  });

  it('assigns unique ids to diagnostic events recorded in the same millisecond', () => {
    const timestamp = '2026-05-01T22:20:59.399Z';

    recordCanvasDiagnosticEvent({
      timestamp,
      level: 'debug',
      source: 'runtime',
      event: 'runtime.delta.applied',
      message: 'first delta',
    });
    recordCanvasDiagnosticEvent({
      timestamp,
      level: 'debug',
      source: 'runtime',
      event: 'runtime.delta.applied',
      message: 'second delta',
    });

    const ids = exportCanvasDiagnostics().events.map((event) => event.id);

    expect(new Set(ids).size).toBe(ids.length);
  });

  it('clears events independently of diagnostics state', () => {
    updateCanvasDiagnosticsState({
      graph: {
        canonicalNodeCount: 3,
      },
    });
    recordCanvasDiagnosticEvent({
      level: 'warn',
      source: 'runtime',
      event: 'runtime.delta.rejected',
      message: 'delta rejected',
    });

    clearCanvasDiagnosticEvents();

    const exported = exportCanvasDiagnostics();
    expect(exported.events).toEqual([]);
    expect(exported.diagnostics.graph.canonicalNodeCount).toBe(3);
  });

  it('includes aggregated canvas metrics and window helpers in the export', () => {
    recordCanvasMetric({
      name: 'composeCanvasTopology',
      scenario: 'runtime',
      durationMs: 12,
      timestamp: 1,
    });

    const exported = window.__THEIA_CANVAS_DIAGNOSTICS_EXPORT__?.();

    expect(exported).toMatchObject({
      version: 1,
      metrics: {
        'runtime:composeCanvasTopology': {
          count: 1,
          minMs: 12,
          maxMs: 12,
          avgMs: 12,
          p95Ms: 12,
        },
      },
    });
    expect(window.__THEIA_CANVAS_DIAGNOSTICS__?.().performance.metrics).toMatchObject({
      'runtime:composeCanvasTopology': expect.objectContaining({ count: 1 }),
    });
    expect(() => JSON.stringify(exported)).not.toThrow();
  });

  it('includes aggregate component render metrics in diagnostics exports', () => {
    setCanvasRenderMetricsEnabled(true);
    finishCanvasRenderMetric(startCanvasRenderMetric('DeviceCard'), { deviceId: 'dev-1' });

    expect(exportCanvasDiagnostics().metrics).toMatchObject({
      'runtime:deviceCardRender': expect.objectContaining({ count: 1 }),
    });
    expect(getCanvasDiagnosticsSnapshot().performance.metrics).toMatchObject({
      'runtime:deviceCardRender': expect.objectContaining({ count: 1 }),
    });
  });
});
