/**
 * Coordinates bridge health state and side effects for consuming components.
 * Owns cleanup-sensitive lifecycle work so callers receive stable state and actions.
 */
import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchBridgeWithTimeout, getBridgeHealthErrorMessage } from '../utils/bridgeRequests';

/** Coordinates bridge health behavior for the React hook lifecycle. */
export function useBridgeHealth(bridgePort: string): {
  bridgeRunning: boolean;
  bridgeChecked: boolean;
  bridgeError: string | null;
  checkBridgeHealth: (bridgePortOverride?: string) => void;
} {
  const [bridgeRunning, setBridgeRunning] = useState(false);
  const [bridgeChecked, setBridgeChecked] = useState(false);
  const [bridgeError, setBridgeError] = useState<string | null>(null);
  const unmountedRef = useRef(false);
  const latestCheckRef = useRef(0);

  useEffect(
    () => () => {
      unmountedRef.current = true;
    },
    [],
  );

  const checkBridgeHealth = useCallback(
    (bridgePortOverride?: string) => {
      const checkId = latestCheckRef.current + 1;
      latestCheckRef.current = checkId;
      setBridgeChecked(false);
      setBridgeError(null);
      const url = `http://localhost:${bridgePortOverride ?? bridgePort}/health`;

      void (async () => {
        try {
          const resp = await fetchBridgeWithTimeout(url);
          if (!unmountedRef.current && latestCheckRef.current === checkId) {
            setBridgeRunning(resp.ok);
            setBridgeChecked(true);
            setBridgeError(resp.ok ? null : `WinBox bridge health check failed (${resp.status}).`);
          }
        } catch (error) {
          if (!unmountedRef.current && latestCheckRef.current === checkId) {
            setBridgeRunning(false);
            setBridgeChecked(true);
            setBridgeError(getBridgeHealthErrorMessage(error));
          }
        }
      })();
    },
    [bridgePort],
  );

  return { bridgeRunning, bridgeChecked, bridgeError, checkBridgeHealth };
}
