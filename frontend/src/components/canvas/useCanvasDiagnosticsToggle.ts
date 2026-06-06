/**
 * Coordinates canvas diagnostics toggle state for the topology canvas.
 * Keeps canvas lifecycle, projected graph state, and cleanup behavior explicit for callers.
 */
import { useCallback, useEffect, useState } from 'react';

/** Defines canvas diagnostics storage key constants and helper contracts for the topology canvas. */
export const canvasDiagnosticsStorageKey = 'theia.canvas.diagnostics';

/** Initial canvas diagnostics visible for the topology canvas. */
export function initialCanvasDiagnosticsVisible(): boolean {
  const queryEnabled = new URLSearchParams(window.location.search).get('canvasDiagnostics') === '1';
  const storageEnabled = window.localStorage.getItem(canvasDiagnosticsStorageKey) === 'true';
  if (queryEnabled) {
    window.localStorage.setItem(canvasDiagnosticsStorageKey, 'true');
  }
  return queryEnabled || storageEnabled;
}

/** Identifies canvas diagnostics shortcut for the topology canvas. */
export function isCanvasDiagnosticsShortcut(event: KeyboardEvent): boolean {
  const isPhysicalD = event.code === 'KeyD' || event.key.toLowerCase() === 'd';
  return event.altKey && (event.ctrlKey || event.metaKey) && isPhysicalD;
}

/** Coordinates canvas diagnostics toggle behavior for the topology canvas. */
export function useCanvasDiagnosticsToggle(): {
  diagnosticsVisible: boolean;
  closeDiagnostics: () => void;
} {
  const [diagnosticsVisible, setDiagnosticsVisible] = useState(initialCanvasDiagnosticsVisible);

  useEffect(() => {
    const handleDiagnosticsShortcut = (event: KeyboardEvent) => {
      if (!isCanvasDiagnosticsShortcut(event)) {
        return;
      }

      event.preventDefault();
      setDiagnosticsVisible((current) => {
        const next = !current;
        window.localStorage.setItem(canvasDiagnosticsStorageKey, String(next));
        return next;
      });
    };

    window.addEventListener('keydown', handleDiagnosticsShortcut, true);
    return () => {
      window.removeEventListener('keydown', handleDiagnosticsShortcut, true);
    };
  }, []);

  const closeDiagnostics = useCallback(() => {
    window.localStorage.setItem(canvasDiagnosticsStorageKey, 'false');
    setDiagnosticsVisible(false);
  }, []);

  return { diagnosticsVisible, closeDiagnostics };
}
