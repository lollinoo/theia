/**
 * Owns the persisted canvas snapping preference used by canvas interaction and data boundaries.
 */
import { useCallback, useState } from 'react';

/** Storage key for the user's optional canvas snapping preference. */
export const canvasSnapPreferenceStorageKey = 'theia.canvas.snapToGrid';

function readCanvasSnapPreference(): boolean {
  try {
    return window.localStorage.getItem(canvasSnapPreferenceStorageKey) !== 'false';
  } catch {
    return true;
  }
}

/** Provides the default-on snap preference and a stable persisted toggle. */
export function useCanvasSnapPreference(): {
  snapToGrid: boolean;
  toggleSnapToGrid: () => void;
} {
  const [snapToGrid, setSnapToGrid] = useState(readCanvasSnapPreference);
  const toggleSnapToGrid = useCallback(() => {
    setSnapToGrid((current) => {
      const next = !current;
      try {
        window.localStorage.setItem(canvasSnapPreferenceStorageKey, String(next));
      } catch {
        // Storage can be denied while the in-memory interaction remains usable.
      }
      return next;
    });
  }, []);

  return { snapToGrid, toggleSnapToGrid };
}
