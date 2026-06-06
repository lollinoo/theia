import { useCallback, useEffect, useState } from 'react';

export const canvasDiagnosticsStorageKey = 'theia.canvas.diagnostics';

export function initialCanvasDiagnosticsVisible(): boolean {
  const queryEnabled = new URLSearchParams(window.location.search).get('canvasDiagnostics') === '1';
  const storageEnabled = window.localStorage.getItem(canvasDiagnosticsStorageKey) === 'true';
  if (queryEnabled) {
    window.localStorage.setItem(canvasDiagnosticsStorageKey, 'true');
  }
  return queryEnabled || storageEnabled;
}

export function isCanvasDiagnosticsShortcut(event: KeyboardEvent): boolean {
  const isPhysicalD = event.code === 'KeyD' || event.key.toLowerCase() === 'd';
  return event.altKey && (event.ctrlKey || event.metaKey) && isPhysicalD;
}

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
