import { useEffect, useState } from 'react';

const POLL_INTERVAL_MS = 15_000;

export function useBridgeHealth(bridgePort: string): { bridgeRunning: boolean } {
  const [bridgeRunning, setBridgeRunning] = useState(false);

  useEffect(() => {
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
  }, [bridgePort]);

  return { bridgeRunning };
}
