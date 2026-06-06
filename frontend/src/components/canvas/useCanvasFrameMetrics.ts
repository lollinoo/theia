/**
 * Coordinates canvas frame metrics state for the topology canvas.
 * Keeps canvas lifecycle, projected graph state, and cleanup behavior explicit for callers.
 */
import { useEffect } from 'react';

import { recordCanvasDiagnosticEvent } from './canvasDiagnostics';
import { recordCanvasFrameTime, recordCanvasLongTask } from './canvasInstrumentation';

function isDocumentHidden(): boolean {
  return typeof document !== 'undefined' && document.visibilityState === 'hidden';
}

function supportsLongTaskObserver(): boolean {
  if (typeof PerformanceObserver === 'undefined') {
    return false;
  }

  const supportedEntryTypes = PerformanceObserver.supportedEntryTypes;
  return Array.isArray(supportedEntryTypes) && supportedEntryTypes.includes('longtask');
}

/** Coordinates canvas frame metrics behavior for the topology canvas. */
export function useCanvasFrameMetrics(): void {
  useEffect(() => {
    if (
      typeof window === 'undefined' ||
      typeof window.requestAnimationFrame !== 'function' ||
      typeof window.cancelAnimationFrame !== 'function'
    ) {
      return;
    }

    let disposed = false;
    let frameId: number | undefined;
    let previousFrameAt: number | undefined;

    const scheduleNextFrame = () => {
      frameId = window.requestAnimationFrame((timestamp) => {
        if (disposed) {
          return;
        }

        if (isDocumentHidden()) {
          previousFrameAt = undefined;
        } else if (previousFrameAt === undefined) {
          previousFrameAt = timestamp;
        } else {
          const durationMs = timestamp - previousFrameAt;
          previousFrameAt = timestamp;
          recordCanvasFrameTime(durationMs);
        }

        scheduleNextFrame();
      });
    };

    scheduleNextFrame();

    let longTaskObserver: PerformanceObserver | undefined;
    if (supportsLongTaskObserver()) {
      longTaskObserver = new PerformanceObserver((entryList) => {
        for (const entry of entryList.getEntries()) {
          const attribution = (entry as { attribution?: unknown[] }).attribution;
          const metadata = {
            name: entry.name,
            startTimeMs: Number(entry.startTime.toFixed(3)),
            attributionCount: Array.isArray(attribution) ? attribution.length : 0,
          };

          recordCanvasLongTask(entry.duration, metadata);
          recordCanvasDiagnosticEvent({
            level: 'warn',
            source: 'performance',
            event: 'browser.long_task',
            message: 'Browser main thread long task detected',
            metadata: {
              ...metadata,
              durationMs: Number(entry.duration.toFixed(3)),
            },
          });
        }
      });

      try {
        longTaskObserver.observe({ type: 'longtask', buffered: true });
      } catch {
        longTaskObserver.disconnect();
        longTaskObserver = undefined;
      }
    }

    return () => {
      disposed = true;
      if (frameId !== undefined) {
        window.cancelAnimationFrame(frameId);
      }
      longTaskObserver?.disconnect();
    };
  }, []);
}
