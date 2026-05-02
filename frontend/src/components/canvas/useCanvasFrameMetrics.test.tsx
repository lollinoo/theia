import { act, render } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { resetCanvasDiagnostics } from './canvasDiagnostics';
import { clearCanvasMetrics, exportCanvasMetrics } from './canvasInstrumentation';
import { useCanvasFrameMetrics } from './useCanvasFrameMetrics';

function Harness({ children }: { children?: ReactNode }) {
  useCanvasFrameMetrics();
  return <>{children}</>;
}

describe('useCanvasFrameMetrics', () => {
  const originalRequestAnimationFrame = window.requestAnimationFrame;
  const originalCancelAnimationFrame = window.cancelAnimationFrame;
  const originalPerformanceObserver = globalThis.PerformanceObserver;
  let frameCallbacks: FrameRequestCallback[];
  let longTaskCallback: PerformanceObserverCallback | undefined;
  let disconnectObserver: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    clearCanvasMetrics();
    resetCanvasDiagnostics();
    frameCallbacks = [];
    longTaskCallback = undefined;
    disconnectObserver = vi.fn();

    window.requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });
    window.cancelAnimationFrame = vi.fn();
  });

  afterEach(() => {
    clearCanvasMetrics();
    resetCanvasDiagnostics();
    window.requestAnimationFrame = originalRequestAnimationFrame;
    window.cancelAnimationFrame = originalCancelAnimationFrame;
    vi.stubGlobal('PerformanceObserver', originalPerformanceObserver);
    vi.restoreAllMocks();
  });

  it('records frame deltas from requestAnimationFrame and cancels on unmount', () => {
    const { unmount } = render(<Harness />);

    expect(window.requestAnimationFrame).toHaveBeenCalledTimes(1);

    act(() => {
      frameCallbacks.shift()?.(100);
    });
    act(() => {
      frameCallbacks.shift()?.(118.25);
    });

    expect(exportCanvasMetrics().aggregates['runtime:frameTime']).toEqual({
      count: 1,
      minMs: 18.25,
      maxMs: 18.25,
      avgMs: 18.25,
      p95Ms: 18.25,
    });

    unmount();

    expect(window.cancelAnimationFrame).toHaveBeenCalled();
  });

  it('records browser long task observer entries when supported', () => {
    class MockPerformanceObserver {
      static supportedEntryTypes = ['longtask'];

      constructor(callback: PerformanceObserverCallback) {
        longTaskCallback = callback;
      }

      observe = vi.fn();
      disconnect = disconnectObserver;
      takeRecords = vi.fn(() => []);
    }

    vi.stubGlobal('PerformanceObserver', MockPerformanceObserver);

    const { unmount } = render(<Harness />);

    act(() => {
      longTaskCallback?.(
        {
          getEntries: () => [
            {
              duration: 76.5,
              name: 'self',
              startTime: 12,
              entryType: 'longtask',
            } as PerformanceEntry,
          ],
        } as PerformanceObserverEntryList,
        {} as PerformanceObserver,
      );
    });

    expect(exportCanvasMetrics().aggregates['runtime:longTask']).toEqual({
      count: 1,
      minMs: 76.5,
      maxMs: 76.5,
      avgMs: 76.5,
      p95Ms: 76.5,
    });

    unmount();

    expect(disconnectObserver).toHaveBeenCalledTimes(1);
  });
});
