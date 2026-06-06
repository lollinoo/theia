/**
 * Exercises canvas layout diagnostics topology canvas behavior so refactors preserve the documented contract.
 */
import { beforeEach, describe, expect, it } from 'vitest';

import {
  getCanvasDiagnosticEvents,
  getCanvasDiagnosticsSnapshot,
  resetCanvasDiagnostics,
} from './canvasDiagnostics';
import { recordCanvasLayoutCompleted, recordCanvasLayoutStarted } from './canvasLayoutDiagnostics';

describe('canvas layout diagnostics helpers', () => {
  beforeEach(() => {
    resetCanvasDiagnostics();
  });

  it('records layout start state and event metadata', () => {
    recordCanvasLayoutStarted({
      reason: 'manual_refresh',
      nodeCount: 3,
      edgeCount: 2,
    });

    expect(getCanvasDiagnosticsSnapshot().layout).toMatchObject({
      pendingLayout: true,
      lastLayoutReason: 'manual_refresh',
      lastLayoutNodeCount: 3,
    });
    expect(getCanvasDiagnosticEvents()).toContainEqual(
      expect.objectContaining({
        level: 'debug',
        source: 'layout',
        event: 'layout.started',
        message: 'Canvas incremental layout started',
        metadata: {
          reason: 'manual_refresh',
          nodeCount: 3,
          edgeCount: 2,
        },
      }),
    );
  });

  it('records layout completion state and rounded duration', () => {
    recordCanvasLayoutCompleted({
      reason: 'topology_changed',
      nodeCount: 4,
      durationMs: 12.3456,
    });

    expect(getCanvasDiagnosticsSnapshot().layout).toMatchObject({
      pendingLayout: false,
      lastLayoutReason: 'topology_changed',
      lastLayoutNodeCount: 4,
      lastLayoutDurationMs: 12.346,
    });
    expect(getCanvasDiagnosticsSnapshot().layout.lastLayoutAt).toEqual(expect.any(String));
    expect(getCanvasDiagnosticEvents()).toContainEqual(
      expect.objectContaining({
        level: 'info',
        source: 'layout',
        event: 'layout.completed',
        message: 'Canvas incremental layout completed',
        metadata: {
          reason: 'topology_changed',
          nodeCount: 4,
        },
      }),
    );
  });
});
