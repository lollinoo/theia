import { useEffect, useState } from 'react';

const BRIDGE_HEALTH_URL = 'http://localhost:1337/health';
const POLL_INTERVAL_MS = 30_000;

export function useBridgeHealth(): { bridgeRunning: boolean } {
  const [bridgeRunning, setBridgeRunning] = useState(false);

  useEffect(() => {
    let cancelled = false;

    async function check() {
      try {
        const resp = await fetch(BRIDGE_HEALTH_URL);
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
  }, []);

  return { bridgeRunning };
}
