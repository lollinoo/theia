/**
 * Exercises canvas topology diagnostics topology canvas behavior so refactors preserve the documented contract.
 */
import { beforeEach, describe, expect, it } from 'vitest';

import {
  getCanvasDiagnosticEvents,
  getCanvasDiagnosticsSnapshot,
  resetCanvasDiagnostics,
  updateCanvasDiagnosticsState,
} from './canvasDiagnostics';
import {
  recordCanvasTopologyLoadFailed,
  recordCanvasTopologyLoadStarted,
  recordCanvasTopologyLoadSucceeded,
} from './canvasTopologyDiagnostics';

const metadata = {
  reason: 'manual_refresh' as const,
  silent: false,
  mapId: 'map-1',
  mapName: 'Core',
};

describe('canvas topology diagnostics helpers', () => {
  beforeEach(() => {
    resetCanvasDiagnostics();
  });

  it('records topology load start state and event metadata', () => {
    recordCanvasTopologyLoadStarted(metadata);

    expect(getCanvasDiagnosticsSnapshot().topology).toMatchObject({
      lastTopologyLoadReason: 'manual_refresh',
      lastTopologyLoadStatus: 'loading',
      lastTopologyLoadError: undefined,
    });
    expect(getCanvasDiagnosticEvents()).toContainEqual(
      expect.objectContaining({
        level: 'info',
        source: 'topology',
        event: 'topology.load.started',
        message: 'Canvas topology load started',
        metadata,
      }),
    );
  });

  it('records a not-modified topology load success without graph counts', () => {
    recordCanvasTopologyLoadSucceeded({
      metadata,
      durationMs: 12.3456,
      notModified: true,
    });

    const snapshot = getCanvasDiagnosticsSnapshot();
    expect(snapshot.topology).toMatchObject({
      lastTopologyLoadReason: 'manual_refresh',
      lastTopologyLoadDurationMs: 12.346,
      lastTopologyLoadStatus: 'success',
      lastTopologyLoadError: undefined,
    });
    expect(snapshot.topology.lastTopologyLoadAt).toEqual(expect.any(String));
    expect(snapshot.graph.canonicalNodeCount).toBe(0);
    expect(getCanvasDiagnosticEvents()).toContainEqual(
      expect.objectContaining({
        level: 'info',
        source: 'topology',
        event: 'topology.load.succeeded',
        message: 'Canvas topology read model not modified',
        metadata: {
          ...metadata,
          notModified: true,
        },
      }),
    );
  });

  it('records a topology load success with graph counts and clears pending layout', () => {
    updateCanvasDiagnosticsState({ layout: { pendingLayout: true } });

    recordCanvasTopologyLoadSucceeded({
      metadata,
      durationMs: 3.2,
      deviceCount: 4,
      linkCount: 5,
      positionCount: 6,
      placementDeviceCount: 7,
      structureChanged: true,
    });

    const snapshot = getCanvasDiagnosticsSnapshot();
    expect(snapshot.topology).toMatchObject({
      lastTopologyLoadReason: 'manual_refresh',
      lastTopologyLoadDurationMs: 3.2,
      lastTopologyLoadStatus: 'success',
      lastTopologyLoadError: undefined,
    });
    expect(snapshot.graph).toMatchObject({
      canonicalNodeCount: 4,
      canonicalEdgeCount: 5,
    });
    expect(snapshot.layout.pendingLayout).toBe(false);
    expect(getCanvasDiagnosticEvents()).toContainEqual(
      expect.objectContaining({
        level: 'info',
        source: 'topology',
        event: 'topology.load.succeeded',
        message: 'Canvas topology load succeeded',
        metadata: {
          ...metadata,
          deviceCount: 4,
          linkCount: 5,
          positionCount: 6,
          placementDeviceCount: 7,
          structureChanged: true,
        },
      }),
    );
  });

  it('records topology load failure and clears pending layout', () => {
    updateCanvasDiagnosticsState({ layout: { pendingLayout: true } });

    recordCanvasTopologyLoadFailed({
      metadata,
      durationMs: 8.7654,
      error: 'backend unavailable',
    });

    const snapshot = getCanvasDiagnosticsSnapshot();
    expect(snapshot.topology).toMatchObject({
      lastTopologyLoadReason: 'manual_refresh',
      lastTopologyLoadDurationMs: 8.765,
      lastTopologyLoadStatus: 'error',
      lastTopologyLoadError: 'backend unavailable',
    });
    expect(snapshot.layout.pendingLayout).toBe(false);
    expect(getCanvasDiagnosticEvents()).toContainEqual(
      expect.objectContaining({
        level: 'error',
        source: 'topology',
        event: 'topology.load.failed',
        message: 'Canvas topology load failed',
        metadata: {
          ...metadata,
          error: 'backend unavailable',
        },
      }),
    );
  });
});
