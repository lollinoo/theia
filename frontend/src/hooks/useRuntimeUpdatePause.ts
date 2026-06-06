import { useEffect, useState } from 'react';

const runtimeUpdatePauseIdleDelayMs = 1500;

/**
 * Keeps runtime WebSocket updates paused briefly after canvas interaction ends.
 * This prevents background deltas from moving nodes while drag/selection gestures are settling.
 */
export function useRuntimeUpdatePause(canvasInteractionActive: boolean): boolean {
  const [runtimeUpdatesPaused, setRuntimeUpdatesPaused] = useState(false);

  useEffect(() => {
    if (canvasInteractionActive) {
      setRuntimeUpdatesPaused(true);
      return;
    }

    if (!runtimeUpdatesPaused) {
      return;
    }

    const timer = window.setTimeout(() => {
      setRuntimeUpdatesPaused(false);
    }, runtimeUpdatePauseIdleDelayMs);

    return () => {
      window.clearTimeout(timer);
    };
  }, [canvasInteractionActive, runtimeUpdatesPaused]);

  return runtimeUpdatesPaused;
}
