import { useEffect, useState } from 'react';

const POLL_INTERVAL_MS = 15_000;

export function useBridgeHealth(bridgePort: string, options?: { enabled?: boolean }): { bridgeRunning: boolean } {
  const enabled = options?.enabled ?? false;
  const [bridgeRunning, setBridgeRunning] = useState(false);

  useEffect(() => {
    if (!enabled) {
      setBridgeRunning(false); // reset to false when disabled — prevents stale true after polling stops
      return;
    }

    let cancelled = false;
    const url = `http://localhost:${bridgePort}/health`;

    async function check() {
      try {
        const resp = await fetch(url);
        if (!cancelled) setBridgeRunning(resp.ok);
      } catch {
        if (!cancelled) setBridgeRunning(false);
      }
    }

    void check();
    const id = window.setInterval(() => { void check(); }, POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [bridgePort, enabled]);

  return { bridgeRunning };
}
