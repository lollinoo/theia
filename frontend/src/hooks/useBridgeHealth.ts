import { useCallback, useEffect, useRef, useState } from 'react';

export function useBridgeHealth(bridgePort: string): {
  bridgeRunning: boolean;
  bridgeChecked: boolean;
  checkBridgeHealth: () => void;
} {
  const [bridgeRunning, setBridgeRunning] = useState(false);
  const [bridgeChecked, setBridgeChecked] = useState(false);
  const unmountedRef = useRef(false);
  const latestCheckRef = useRef(0);

  useEffect(() => () => {
    unmountedRef.current = true;
  }, []);

  const checkBridgeHealth = useCallback(() => {
    const checkId = latestCheckRef.current + 1;
    latestCheckRef.current = checkId;
    setBridgeChecked(false);
    const url = `http://localhost:${bridgePort}/health`;

    void (async () => {
      try {
        const resp = await fetch(url);
        if (!unmountedRef.current && latestCheckRef.current === checkId) {
          setBridgeRunning(resp.ok);
          setBridgeChecked(true);
        }
      } catch {
        if (!unmountedRef.current && latestCheckRef.current === checkId) {
          setBridgeRunning(false);
          setBridgeChecked(true);
        }
      }
    })();
  }, [bridgePort]);

  return { bridgeRunning, bridgeChecked, checkBridgeHealth };
}
