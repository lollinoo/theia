/**
 * Defines canvas layout diagnostics behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import { recordCanvasDiagnosticEvent, updateCanvasDiagnosticsState } from './canvasDiagnostics';
import type { CanvasMeasurementTrigger } from './canvasInstrumentation';

interface CanvasLayoutStartedInput {
  reason: CanvasMeasurementTrigger;
  nodeCount: number;
  edgeCount: number;
}

interface CanvasLayoutCompletedInput {
  reason: CanvasMeasurementTrigger;
  nodeCount: number;
  durationMs: number;
}

function roundDiagnosticDurationMs(durationMs: number): number {
  return Number(Math.max(0, durationMs).toFixed(3));
}

/** Records canvas layout started for the topology canvas. */
export function recordCanvasLayoutStarted({
  reason,
  nodeCount,
  edgeCount,
}: CanvasLayoutStartedInput): void {
  updateCanvasDiagnosticsState({
    layout: {
      pendingLayout: true,
      lastLayoutReason: reason,
      lastLayoutNodeCount: nodeCount,
    },
  });
  recordCanvasDiagnosticEvent({
    level: 'debug',
    source: 'layout',
    event: 'layout.started',
    message: 'Canvas incremental layout started',
    metadata: {
      reason,
      nodeCount,
      edgeCount,
    },
  });
}

/** Records canvas layout completed for the topology canvas. */
export function recordCanvasLayoutCompleted({
  reason,
  nodeCount,
  durationMs,
}: CanvasLayoutCompletedInput): void {
  updateCanvasDiagnosticsState({
    layout: {
      lastLayoutAt: new Date().toISOString(),
      lastLayoutDurationMs: roundDiagnosticDurationMs(durationMs),
      lastLayoutNodeCount: nodeCount,
      lastLayoutReason: reason,
      pendingLayout: false,
    },
  });
  recordCanvasDiagnosticEvent({
    level: 'info',
    source: 'layout',
    event: 'layout.completed',
    message: 'Canvas incremental layout completed',
    metadata: {
      reason,
      nodeCount,
    },
  });
}
