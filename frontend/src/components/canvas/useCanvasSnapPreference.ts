/**
 * Owns the persisted canvas snapping preference used by canvas interaction and data boundaries.
 */
import { useCallback, useState } from 'react';

/** Storage key for the user's optional canvas snapping preference. */
export const canvasSnapPreferenceStorageKey = 'theia.canvas.snapToGrid';

/** Provides the default-on snap preference and a stable persisted toggle. */
export function useCanvasSnapPreference(): {
  snapToGrid: boolean;
  toggleSnapToGrid: () => void;
} {
  const [snapToGrid, setSnapToGrid] = useState(
    () => window.localStorage.getItem(canvasSnapPreferenceStorageKey) !== 'false',
  );
  const toggleSnapToGrid = useCallback(() => {
    setSnapToGrid((current) => {
      const next = !current;
      window.localStorage.setItem(canvasSnapPreferenceStorageKey, String(next));
      return next;
    });
  }, []);

  return { snapToGrid, toggleSnapToGrid };
}
