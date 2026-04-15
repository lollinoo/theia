import { useEffect, useState } from 'react';
import { nextFreshnessUpdateDelayMs } from '../utils/freshness';

export function useFreshnessClock(
  lastPolledAt: string | undefined,
  expectedIntervalSeconds: number | null | undefined,
  enabled = true,
): number {
  const [nowMs, setNowMs] = useState(() => Date.now());

  useEffect(() => {
    if (!enabled) {
      return;
    }
    setNowMs(Date.now());
  }, [enabled, lastPolledAt, expectedIntervalSeconds]);

  useEffect(() => {
    if (!enabled) {
      return;
    }
    const delayMs = nextFreshnessUpdateDelayMs(lastPolledAt, expectedIntervalSeconds, nowMs);
    if (delayMs === null || typeof window === 'undefined') {
      return;
    }

    const timerID = window.setTimeout(() => {
      setNowMs(Date.now());
    }, delayMs);

    return () => {
      window.clearTimeout(timerID);
    };
  }, [enabled, expectedIntervalSeconds, lastPolledAt, nowMs]);

  return nowMs;
}
